/*
 * @Author: Marlon.M
 * @Email: maiguangyang@163.com
 * @Date: 2025-12-24
 *
 * ProxyModeCoordinator - 代理模式协调器
 *
 * 将所有组件串联起来，实现完全自动的无感故障切换：
 * - Keepalive 检测心跳
 * - Election 计算分数和选举
 * - FailoverManager 协调切换
 * - RelayRoom 管理 P2P 连接
 * - SourceSwitcher 切换数据源
 *
 * 用户只需调用一个 Enable 方法，其他全自动。
 */
package sfu

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/maiguangyang/relay_core/pkg/election"
)

// CoordinatorConfig 协调器配置
type CoordinatorConfig struct {
	// Keepalive 配置
	KeepaliveInterval time.Duration
	KeepaliveTimeout  time.Duration

	// Failover 配置
	FailoverBackoffPerPoint  time.Duration
	FailoverOfflineThreshold int

	// Election 配置
	ElectionInterval time.Duration
}

// DefaultCoordinatorConfig 默认配置
func DefaultCoordinatorConfig() CoordinatorConfig {
	return CoordinatorConfig{
		KeepaliveInterval:        3 * time.Second,
		KeepaliveTimeout:         10 * time.Second,
		FailoverBackoffPerPoint:  10 * time.Millisecond,
		FailoverOfflineThreshold: 2,
		ElectionInterval:         5 * time.Second,
	}
}

// CoordinatorEvent 协调器事件类型
type CoordinatorEventType int

const (
	CoordinatorEventRelayChanged CoordinatorEventType = iota // Relay 节点变更
	CoordinatorEventBecomeRelay                              // 本机成为 Relay
	CoordinatorEventRelayFailed                              // Relay 失效
	CoordinatorEventPeerJoined                               // 新 Peer 加入
	CoordinatorEventPeerLeft                                 // Peer 离开
)

// CoordinatorEvent 协调器事件
type CoordinatorEvent struct {
	Type      CoordinatorEventType
	RoomID    string
	PeerID    string
	Data      map[string]interface{}
	Timestamp time.Time
}

// ProxyModeCoordinator 代理模式协调器
// 一键启用，全自动管理 Relay 选举和故障切换
type ProxyModeCoordinator struct {
	mu sync.RWMutex

	roomID      string
	localPeerID string
	config      CoordinatorConfig

	// 核心组件
	elector   *election.Elector
	keepalive *KeepaliveManager
	failover  *FailoverManager
	relayRoom *RelayRoom
	switcher  *SourceSwitcher

	// 状态
	isRelay        bool
	currentRelayID string
	epoch          uint64

	// 所有已知的 Peer
	peers map[string]bool

	// 事件回调
	onEvent func(event CoordinatorEvent)

	// 控制
	stopCh chan struct{}
	closed bool
}

// NewProxyModeCoordinator 创建协调器
func NewProxyModeCoordinator(roomID, localPeerID string, config CoordinatorConfig) (*ProxyModeCoordinator, error) {
	// 创建选举器
	electionConfig := election.DefaultElectorConfig()
	electionConfig.ElectionInterval = config.ElectionInterval
	elector := election.NewElector(roomID, electionConfig)

	// 创建心跳管理器
	keepaliveConfig := KeepaliveConfig{
		Interval:      config.KeepaliveInterval,
		Timeout:       config.KeepaliveTimeout,
		SlowThreshold: config.KeepaliveTimeout / 3,
		MaxRetries:    config.FailoverOfflineThreshold,
	}
	keepalive := NewKeepaliveManager(keepaliveConfig)

	// 创建源切换器
	switcher, err := NewSourceSwitcher(roomID)
	if err != nil {
		elector.Close()
		return nil, err
	}

	// 创建故障切换器
	failoverConfig := FailoverConfig{
		BackoffPerPoint:  config.FailoverBackoffPerPoint,
		MaxBackoff:       2 * time.Second,
		ClaimTimeout:     500 * time.Millisecond,
		OfflineThreshold: config.FailoverOfflineThreshold,
	}
	failover := NewFailoverManager(roomID, localPeerID, elector, keepalive, failoverConfig)

	pmc := &ProxyModeCoordinator{
		roomID:      roomID,
		localPeerID: localPeerID,
		config:      config,
		elector:     elector,
		keepalive:   keepalive,
		failover:    failover,
		switcher:    switcher,
		peers:       make(map[string]bool),
		stopCh:      make(chan struct{}),
	}

	// 设置回调，串联所有组件
	pmc.setupCallbacks()

	return pmc, nil
}

// setupCallbacks 设置所有组件的回调
func (pmc *ProxyModeCoordinator) setupCallbacks() {
	// Keepalive: Ping 请求 -> 由外部信令发送
	pmc.keepalive.SetOnPing(func(peerID string) {
		pmc.emitEvent(CoordinatorEvent{
			Type:   CoordinatorEventPeerLeft, // 用特殊 type 触发 ping
			RoomID: pmc.roomID,
			PeerID: peerID,
			Data:   map[string]interface{}{"action": "ping_request"},
		})
	})

	// Keepalive: Peer 离线
	pmc.keepalive.SetOnPeerOffline(func(peerID string) {
		pmc.handlePeerOffline(peerID)
	})

	// Failover: Relay 失效
	pmc.failover.SetCallbacks(
		// onRelayFailed
		func(roomID, relayID string) {
			pmc.emitEvent(CoordinatorEvent{
				Type:   CoordinatorEventRelayFailed,
				RoomID: roomID,
				PeerID: relayID,
				Data:   map[string]interface{}{"reason": "offline"},
			})
		},
		// onNewRelayElected
		func(roomID, newRelayID string, epoch uint64) {
			pmc.mu.Lock()
			pmc.currentRelayID = newRelayID
			pmc.epoch = epoch
			pmc.mu.Unlock()

			pmc.emitEvent(CoordinatorEvent{
				Type:   CoordinatorEventRelayChanged,
				RoomID: roomID,
				PeerID: newRelayID,
				Data:   map[string]interface{}{"epoch": epoch},
			})
		},
		// onBecomeRelay
		func(roomID string) {
			pmc.handleBecomeRelay()
		},
	)

	// 源切换器回调
	pmc.switcher.SetOnSourceChanged(func(roomID string, sourceType SourceType, sharerID string) {
		pmc.emitEvent(CoordinatorEvent{
			Type:   CoordinatorEventRelayChanged,
			RoomID: roomID,
			PeerID: sharerID,
			Data: map[string]interface{}{
				"source_type": sourceType.String(),
				"sharer_id":   sharerID,
			},
		})
	})
}

// handlePeerOffline 处理 Peer 离线
func (pmc *ProxyModeCoordinator) handlePeerOffline(peerID string) {
	pmc.mu.Lock()
	isCurrentRelay := peerID == pmc.currentRelayID
	pmc.mu.Unlock()

	if isCurrentRelay {
		// Relay 离线，Failover 会自动处理
		// FailoverManager 的 handlePeerOffline 已经通过 keepalive 触发了
	} else {
		// 普通 Peer 离线
		pmc.removePeer(peerID)
		pmc.emitEvent(CoordinatorEvent{
			Type:   CoordinatorEventPeerLeft,
			RoomID: pmc.roomID,
			PeerID: peerID,
		})
	}
}

// handleBecomeRelay 本机成为 Relay
func (pmc *ProxyModeCoordinator) handleBecomeRelay() {
	pmc.mu.Lock()
	pmc.isRelay = true
	pmc.currentRelayID = pmc.localPeerID
	pmc.mu.Unlock()

	// 自动创建 RelayRoom（如果还没有）
	if pmc.relayRoom == nil {
		room, err := NewRelayRoom(pmc.roomID, nil) // TODO: 从外部传入 ICE servers
		if err == nil {
			pmc.relayRoom = room
			room.BecomeRelay(pmc.localPeerID)

			// 设置 RelayRoom 回调
			room.SetCallbacks(
				func(roomID, peerID string) {
					pmc.emitEvent(CoordinatorEvent{
						Type:   CoordinatorEventPeerJoined,
						RoomID: roomID,
						PeerID: peerID,
					})
				},
				func(roomID, peerID string) {
					pmc.emitEvent(CoordinatorEvent{
						Type:   CoordinatorEventPeerLeft,
						RoomID: roomID,
						PeerID: peerID,
					})
				},
				nil, nil, nil,
			)
		}
	}

	pmc.emitEvent(CoordinatorEvent{
		Type:   CoordinatorEventBecomeRelay,
		RoomID: pmc.roomID,
		PeerID: pmc.localPeerID,
		Data: map[string]interface{}{
			"epoch": pmc.epoch,
		},
	})
}

// Start 启动协调器
func (pmc *ProxyModeCoordinator) Start() {
	pmc.mu.Lock()
	if pmc.closed {
		pmc.mu.Unlock()
		return
	}
	pmc.mu.Unlock()

	// 添加本机到选举候选
	pmc.elector.UpdateCandidate(election.Candidate{
		PeerID: pmc.localPeerID,
	})

	// 启动心跳
	pmc.keepalive.Start()

	// 启动选举（定期重新评估）
	pmc.elector.Start()
}

// AddPeer 添加 Peer
func (pmc *ProxyModeCoordinator) AddPeer(peerID string, deviceType, connectionType, powerState int) {
	pmc.mu.Lock()
	pmc.peers[peerID] = true
	pmc.mu.Unlock()

	// 添加到心跳监控
	pmc.keepalive.AddPeer(peerID)

	// 添加到选举候选
	pmc.elector.UpdateCandidate(election.Candidate{
		PeerID:         peerID,
		DeviceType:     election.DeviceType(deviceType),
		ConnectionType: election.ConnectionType(connectionType),
		PowerState:     election.PowerState(powerState),
	})
}

// removePeer 移除 Peer（内部方法）
func (pmc *ProxyModeCoordinator) removePeer(peerID string) {
	pmc.mu.Lock()
	delete(pmc.peers, peerID)
	pmc.mu.Unlock()

	pmc.keepalive.RemovePeer(peerID)
	pmc.elector.RemoveCandidate(peerID)
}

// RemovePeer 移除 Peer
func (pmc *ProxyModeCoordinator) RemovePeer(peerID string) {
	pmc.removePeer(peerID)
}

// HandlePong 处理 Pong 响应
func (pmc *ProxyModeCoordinator) HandlePong(peerID string) {
	pmc.keepalive.HandlePong(peerID)
	pmc.failover.ResetOfflineCount(peerID)
}

// SetCurrentRelay 设置当前 Relay（由外部信令通知）
func (pmc *ProxyModeCoordinator) SetCurrentRelay(relayID string, epoch uint64) {
	pmc.mu.Lock()
	pmc.currentRelayID = relayID
	pmc.epoch = epoch
	pmc.isRelay = (relayID == pmc.localPeerID)
	pmc.mu.Unlock()

	pmc.failover.SetCurrentRelay(relayID, epoch)
}

// ReceiveRelayClaim 接收 Relay 声明（来自其他节点）
func (pmc *ProxyModeCoordinator) ReceiveRelayClaim(peerID string, epoch uint64, score float64) {
	pmc.failover.ReceiveRelayClaim(peerID, epoch, score)

	pmc.mu.Lock()
	if epoch > pmc.epoch {
		pmc.currentRelayID = peerID
		pmc.epoch = epoch
		pmc.isRelay = false
	}
	pmc.mu.Unlock()
}

// UpdateLocalDeviceInfo 更新本机设备信息
func (pmc *ProxyModeCoordinator) UpdateLocalDeviceInfo(deviceType, connectionType, powerState int) {
	pmc.elector.UpdateDeviceInfo(
		pmc.localPeerID,
		election.DeviceType(deviceType),
		election.ConnectionType(connectionType),
		election.PowerState(powerState),
	)

	// 更新本机在 Failover 中的分数
	candidates := pmc.elector.GetCandidates()
	for _, c := range candidates {
		if c.PeerID == pmc.localPeerID {
			pmc.failover.UpdateLocalScore(c.Score)
			break
		}
	}
}

// InjectSFUPacket 注入 SFU RTP 包
func (pmc *ProxyModeCoordinator) InjectSFUPacket(isVideo bool, data []byte) error {
	return pmc.switcher.InjectSFUPacket(isVideo, data)
}

// InjectLocalPacket 注入本地分享 RTP 包
func (pmc *ProxyModeCoordinator) InjectLocalPacket(isVideo bool, data []byte) error {
	return pmc.switcher.InjectLocalPacket(isVideo, data)
}

// StartLocalShare 开始本地分享
func (pmc *ProxyModeCoordinator) StartLocalShare(sharerID string) {
	pmc.switcher.StartLocalShare(sharerID)
}

// StopLocalShare 停止本地分享
func (pmc *ProxyModeCoordinator) StopLocalShare() {
	pmc.switcher.StopLocalShare()
}

// SetOnEvent 设置事件回调
func (pmc *ProxyModeCoordinator) SetOnEvent(fn func(event CoordinatorEvent)) {
	pmc.mu.Lock()
	defer pmc.mu.Unlock()
	pmc.onEvent = fn
}

func (pmc *ProxyModeCoordinator) emitEvent(event CoordinatorEvent) {
	event.Timestamp = time.Now()

	pmc.mu.RLock()
	fn := pmc.onEvent
	pmc.mu.RUnlock()

	if fn != nil {
		go fn(event)
	}
}

// GetStatus 获取状态
func (pmc *ProxyModeCoordinator) GetStatus() map[string]interface{} {
	pmc.mu.RLock()
	defer pmc.mu.RUnlock()

	status := map[string]interface{}{
		"room_id":        pmc.roomID,
		"local_peer_id":  pmc.localPeerID,
		"is_relay":       pmc.isRelay,
		"current_relay":  pmc.currentRelayID,
		"epoch":          pmc.epoch,
		"peer_count":     len(pmc.peers),
		"failover_state": pmc.failover.GetState().String(),
	}

	if pmc.switcher != nil {
		// 获取 SourceSwitcher 状态并放到顶层
		ssStatus := pmc.switcher.GetStatus()
		status["source_switcher"] = ssStatus
		// 同时也放到顶层，方便 Dart 直接访问
		status["sfu_packets"] = ssStatus.SFUPackets
		status["local_packets"] = ssStatus.LocalPackets
		status["active_source"] = ssStatus.ActiveSource
		status["sfu_active"] = ssStatus.SFUActive
		status["local_active"] = ssStatus.LocalActive
	} else {
		// 如果没有 SourceSwitcher，返回默认值
		status["sfu_packets"] = uint64(0)
		status["local_packets"] = uint64(0)
	}

	return status
}

// GetStatusJSON 获取 JSON 状态
func (pmc *ProxyModeCoordinator) GetStatusJSON() string {
	data, _ := json.Marshal(pmc.GetStatus())
	return string(data)
}

// IsRelay 是否是 Relay
func (pmc *ProxyModeCoordinator) IsRelay() bool {
	pmc.mu.RLock()
	defer pmc.mu.RUnlock()
	return pmc.isRelay
}

// GetRelayRoom 获取 RelayRoom（仅当本机是 Relay 时有效）
func (pmc *ProxyModeCoordinator) GetRelayRoom() *RelayRoom {
	return pmc.relayRoom
}

// GetSourceSwitcher 获取 SourceSwitcher
func (pmc *ProxyModeCoordinator) GetSourceSwitcher() *SourceSwitcher {
	return pmc.switcher
}

// Close 关闭协调器
func (pmc *ProxyModeCoordinator) Close() {
	pmc.mu.Lock()
	if pmc.closed {
		pmc.mu.Unlock()
		return
	}
	pmc.closed = true
	pmc.mu.Unlock()

	close(pmc.stopCh)

	if pmc.keepalive != nil {
		pmc.keepalive.Stop()
	}
	if pmc.elector != nil {
		pmc.elector.Close()
	}
	if pmc.failover != nil {
		pmc.failover.Close()
	}
	if pmc.relayRoom != nil {
		pmc.relayRoom.Close()
	}
	if pmc.switcher != nil {
		pmc.switcher.Close()
	}
}
