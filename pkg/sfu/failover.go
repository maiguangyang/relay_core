/*
 * @Author: Marlon.M
 * @Email: maiguangyang@163.com
 * @Date: 2025-12-24
 *
 * Relay Failover Manager - 自动故障切换管理器
 *
 * 解决的核心问题：
 * 当 Relay 节点故障时，N 个订阅者可能同时检测到，
 * 需要协调避免多个节点同时尝试成为新 Relay。
 *
 * 策略：分数优先 + 延迟选举
 * 1. 检测到 Relay 离线后，每个节点计算自己的分数
 * 2. 等待时间 = (100 - 分数) * BackoffPerPoint，分数越高等待越短
 * 3. 在等待期间，如果收到其他节点的选举声明，则放弃
 * 4. 使用 epoch（纪元号）防止过期选举
 */
package sfu

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/maiguangyang/relay_core/pkg/election"
)

// FailoverConfig 故障切换配置
type FailoverConfig struct {
	// 每分差值对应的退避时间（默认 10ms）
	// 分数 100 的节点等待 0ms，分数 50 的节点等待 500ms
	BackoffPerPoint time.Duration

	// 最大退避时间（上限）
	MaxBackoff time.Duration

	// 选举声明超时时间（等待其他节点响应）
	ClaimTimeout time.Duration

	// 连续离线检测次数后才触发故障切换
	OfflineThreshold int
}

// DefaultFailoverConfig 默认配置
func DefaultFailoverConfig() FailoverConfig {
	return FailoverConfig{
		BackoffPerPoint:  10 * time.Millisecond,
		MaxBackoff:       2 * time.Second,
		ClaimTimeout:     500 * time.Millisecond,
		OfflineThreshold: 2,
	}
}

// FailoverState 故障切换状态
type FailoverState int32

const (
	FailoverStateIdle          FailoverState = iota // 空闲
	FailoverStateDetecting                          // 检测中（收集离线信息）
	FailoverStateWaiting                            // 等待退避
	FailoverStateElecting                           // 选举中
	FailoverStateTransitioning                      // 切换中
)

// FailoverManager 故障切换管理器
type FailoverManager struct {
	mu sync.RWMutex

	roomID string
	config FailoverConfig

	// 关联的组件
	elector   *election.Elector
	keepalive *KeepaliveManager

	// 本机信息
	localPeerID string
	localScore  float64

	// 当前 Relay 信息
	currentRelayID string
	relayEpoch     uint64  // 选举纪元号，每次选举递增
	relayScore     float64 // 当前 Relay 的分数（用于同 epoch 冲突解决）

	// 状态
	state FailoverState

	// 离线计数（需要连续 N 次才触发）
	offlineCount map[string]int

	// 已收到的选举声明（防止重复选举）
	receivedClaims map[string]ClaimInfo // peerID -> ClaimInfo

	// 冲突检测
	isConflicted bool // 检测到冲突

	// 回调
	onRelayFailed     func(roomID, relayID string)
	onNewRelayElected func(roomID, newRelayID string, epoch uint64)
	onBecomeRelay     func(roomID string)                // 本机成为 Relay 时触发
	onConflict        func(roomID string, winner string) // 冲突解决回调

	// 控制
	stopCh chan struct{}
	closed bool
}

// ClaimInfo 声明信息
type ClaimInfo struct {
	Epoch uint64
	Score float64
}

// NewFailoverManager 创建故障切换管理器
func NewFailoverManager(
	roomID string,
	localPeerID string,
	elector *election.Elector,
	keepalive *KeepaliveManager,
	config FailoverConfig,
) *FailoverManager {
	fm := &FailoverManager{
		roomID:         roomID,
		localPeerID:    localPeerID,
		config:         config,
		elector:        elector,
		keepalive:      keepalive,
		offlineCount:   make(map[string]int),
		receivedClaims: make(map[string]ClaimInfo),
		stopCh:         make(chan struct{}),
	}

	// 注册 Keepalive 离线回调
	if keepalive != nil {
		keepalive.SetOnPeerOffline(fm.handlePeerOffline)
	}

	return fm
}

// SetCallbacks 设置回调
func (fm *FailoverManager) SetCallbacks(
	onFailed func(roomID, relayID string),
	onElected func(roomID, newRelayID string, epoch uint64),
	onBecomeRelay func(roomID string),
) {
	fm.mu.Lock()
	defer fm.mu.Unlock()
	fm.onRelayFailed = onFailed
	fm.onNewRelayElected = onElected
	fm.onBecomeRelay = onBecomeRelay
}

// SetCurrentRelay 设置当前 Relay
func (fm *FailoverManager) SetCurrentRelay(relayID string, epoch uint64) {
	fm.mu.Lock()
	defer fm.mu.Unlock()
	fm.currentRelayID = relayID
	if epoch > fm.relayEpoch {
		fm.relayEpoch = epoch
	}
	fm.setState(FailoverStateIdle)
	// 清空之前的选举声明
	fm.receivedClaims = make(map[string]ClaimInfo)
	fm.isConflicted = false
}

// UpdateLocalScore 更新本机分数
func (fm *FailoverManager) UpdateLocalScore(score float64) {
	fm.mu.Lock()
	defer fm.mu.Unlock()
	fm.localScore = score
}

// handlePeerOffline 处理 Peer 离线事件
func (fm *FailoverManager) handlePeerOffline(peerID string) {
	fm.mu.Lock()
	defer fm.mu.Unlock()

	if fm.closed {
		return
	}

	// 累计离线次数
	fm.offlineCount[peerID]++

	// 检查是否是当前 Relay
	if peerID != fm.currentRelayID {
		return
	}

	// 检查是否达到阈值
	if fm.offlineCount[peerID] < fm.config.OfflineThreshold {
		return
	}

	// 已经在处理中
	if fm.state != FailoverStateIdle {
		return
	}

	// 触发故障切换流程
	fm.setState(FailoverStateDetecting)

	// 通知 Relay 失效
	if fm.onRelayFailed != nil {
		go fm.onRelayFailed(fm.roomID, peerID)
	}

	// 异步启动选举流程
	go fm.startFailoverProcess()
}

// startFailoverProcess 启动故障切换流程
func (fm *FailoverManager) startFailoverProcess() {
	fm.mu.Lock()
	localScore := fm.localScore
	currentEpoch := fm.relayEpoch
	fm.mu.Unlock()

	// 计算退避时间：分数越高，等待越短
	backoff := time.Duration(100-localScore) * fm.config.BackoffPerPoint
	if backoff > fm.config.MaxBackoff {
		backoff = fm.config.MaxBackoff
	}
	if backoff < 0 {
		backoff = 0
	}

	fm.mu.Lock()
	fm.setState(FailoverStateWaiting)
	fm.mu.Unlock()

	// 等待退避时间
	select {
	case <-time.After(backoff):
		// 继续选举
	case <-fm.stopCh:
		return
	}

	// 检查是否已有其他节点声明
	fm.mu.Lock()
	if fm.state != FailoverStateWaiting {
		fm.mu.Unlock()
		return // 状态已改变，放弃
	}

	// 检查是否有更高 epoch 的声明，或同 epoch 更高分数的声明
	for peerID, claim := range fm.receivedClaims {
		if claim.Epoch > currentEpoch {
			fm.setState(FailoverStateIdle)
			fm.mu.Unlock()
			return // 有人已经声明更高 epoch，放弃
		}
		if claim.Epoch == currentEpoch+1 && claim.Score > localScore {
			// 同一 epoch，对方分数更高
			fm.currentRelayID = peerID
			fm.relayEpoch = claim.Epoch
			fm.relayScore = claim.Score
			fm.setState(FailoverStateIdle)
			fm.mu.Unlock()
			return // 对方分数更高，放弃
		}
	}

	// 开始选举
	fm.setState(FailoverStateElecting)
	newEpoch := currentEpoch + 1
	fm.mu.Unlock()

	// 执行选举
	if fm.elector != nil {
		result := fm.elector.Elect()
		if result != nil && result.ProxyID == fm.localPeerID {
			// 本机赢得选举
			fm.mu.Lock()
			fm.currentRelayID = fm.localPeerID
			fm.relayEpoch = newEpoch
			fm.setState(FailoverStateTransitioning)
			callback := fm.onBecomeRelay
			fm.mu.Unlock()

			// 通知本机成为 Relay
			if callback != nil {
				callback(fm.roomID)
			}

			// 通知新 Relay 选举完成
			fm.mu.RLock()
			electedCallback := fm.onNewRelayElected
			fm.mu.RUnlock()
			if electedCallback != nil {
				electedCallback(fm.roomID, fm.localPeerID, newEpoch)
			}

			fm.mu.Lock()
			fm.setState(FailoverStateIdle)
			fm.mu.Unlock()
		}
	}
}

// ReceiveRelayClaim 接收其他节点的 Relay 声明
// 当收到更高 epoch 的声明时，放弃本机选举
// 当收到同 epoch 但更高分数的声明时，也放弃
func (fm *FailoverManager) ReceiveRelayClaim(peerID string, epoch uint64, score float64) {
	fm.mu.Lock()
	defer fm.mu.Unlock()

	fm.receivedClaims[peerID] = ClaimInfo{Epoch: epoch, Score: score}

	// 检查是否需要让出 Relay 位置
	shouldYield := false
	isConflict := false

	if epoch > fm.relayEpoch {
		// 更高 epoch，直接让出
		shouldYield = true
	} else if epoch == fm.relayEpoch && fm.currentRelayID == fm.localPeerID {
		// 同 epoch 且本机是 Relay，检查是否冲突
		if score > fm.localScore {
			// 对方分数更高，让出
			shouldYield = true
			isConflict = true
		} else if score == fm.localScore && peerID > fm.localPeerID {
			// 分数相同，比较 PeerID（字典序）
			shouldYield = true
			isConflict = true
		}
	}

	if shouldYield {
		fm.currentRelayID = peerID
		fm.relayEpoch = epoch
		fm.relayScore = score
		fm.setState(FailoverStateIdle)
		fm.offlineCount = make(map[string]int)
		fm.isConflicted = isConflict

		// 通知冲突解决
		if isConflict && fm.onConflict != nil {
			go fm.onConflict(fm.roomID, peerID)
		}
	}
}

// ResetOfflineCount 重置某 Peer 的离线计数（收到 Pong 时调用）
func (fm *FailoverManager) ResetOfflineCount(peerID string) {
	fm.mu.Lock()
	defer fm.mu.Unlock()
	delete(fm.offlineCount, peerID)
}

// GetState 获取当前状态
func (fm *FailoverManager) GetState() FailoverState {
	return FailoverState(atomic.LoadInt32((*int32)(&fm.state)))
}

func (fm *FailoverManager) setState(state FailoverState) {
	atomic.StoreInt32((*int32)(&fm.state), int32(state))
}

// GetCurrentRelay 获取当前 Relay
func (fm *FailoverManager) GetCurrentRelay() (string, uint64) {
	fm.mu.RLock()
	defer fm.mu.RUnlock()
	return fm.currentRelayID, fm.relayEpoch
}

// Close 关闭
func (fm *FailoverManager) Close() {
	fm.mu.Lock()
	if fm.closed {
		fm.mu.Unlock()
		return
	}
	fm.closed = true
	fm.mu.Unlock()

	close(fm.stopCh)
}

// FailoverState String
func (s FailoverState) String() string {
	switch s {
	case FailoverStateIdle:
		return "idle"
	case FailoverStateDetecting:
		return "detecting"
	case FailoverStateWaiting:
		return "waiting"
	case FailoverStateElecting:
		return "electing"
	case FailoverStateTransitioning:
		return "transitioning"
	default:
		return "unknown"
	}
}
