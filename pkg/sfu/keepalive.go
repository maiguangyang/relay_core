/*
 * @Author: Marlon.M
 * @Email: maiguangyang@163.com
 * @Date: 2025-12-24
 *
 * Keepalive - 心跳保活与断线检测
 * 用于检测 Peer 是否离线，触发 Relay 重选举
 */
package sfu

import (
	"sync"
	"sync/atomic"
	"time"
)

// PeerStatus 节点状态
type PeerStatus int32

const (
	PeerStatusUnknown PeerStatus = iota
	PeerStatusOnline
	PeerStatusSlow // 响应缓慢
	PeerStatusOffline
)

func (s PeerStatus) String() string {
	switch s {
	case PeerStatusOnline:
		return "online"
	case PeerStatusSlow:
		return "slow"
	case PeerStatusOffline:
		return "offline"
	default:
		return "unknown"
	}
}

// KeepaliveConfig 心跳配置
type KeepaliveConfig struct {
	// 心跳间隔
	Interval time.Duration
	// 超时时间（超过此时间无响应则认为离线）
	Timeout time.Duration
	// 慢响应阈值（超过此时间认为响应缓慢）
	SlowThreshold time.Duration
	// 最大重试次数
	MaxRetries int
}

// DefaultKeepaliveConfig 返回默认配置
func DefaultKeepaliveConfig() KeepaliveConfig {
	return KeepaliveConfig{
		Interval:      5 * time.Second,
		Timeout:       15 * time.Second,
		SlowThreshold: 3 * time.Second,
		MaxRetries:    3,
	}
}

// PeerHeartbeat 单个 Peer 的心跳状态
type PeerHeartbeat struct {
	mu sync.RWMutex

	peerID      string
	status      atomic.Int32
	lastPing    time.Time     // 上次发送 ping 的时间
	lastPong    time.Time     // 上次收到 pong 的时间
	rtt         time.Duration // 往返时间
	missedPongs int           // 连续丢失的 pong 次数
	totalPings  uint64
	totalPongs  uint64
}

// NewPeerHeartbeat 创建 Peer 心跳
func NewPeerHeartbeat(peerID string) *PeerHeartbeat {
	h := &PeerHeartbeat{
		peerID:   peerID,
		lastPong: time.Now(),
	}
	h.status.Store(int32(PeerStatusOnline))
	return h
}

// MarkPingSent 标记已发送 ping
func (h *PeerHeartbeat) MarkPingSent() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.lastPing = time.Now()
	h.totalPings++
}

// MarkPongReceived 标记收到 pong
func (h *PeerHeartbeat) MarkPongReceived() {
	h.mu.Lock()
	defer h.mu.Unlock()
	now := time.Now()
	h.rtt = now.Sub(h.lastPing)
	h.lastPong = now
	h.missedPongs = 0
	h.totalPongs++
	h.status.Store(int32(PeerStatusOnline))
}

// MarkPongMissed 标记丢失 pong
func (h *PeerHeartbeat) MarkPongMissed() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.missedPongs++
}

// GetStatus 获取状态
func (h *PeerHeartbeat) GetStatus() PeerStatus {
	return PeerStatus(h.status.Load())
}

// GetRTT 获取往返时间
func (h *PeerHeartbeat) GetRTT() time.Duration {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.rtt
}

// GetMissedPongs 获取连续丢失的 pong 次数
func (h *PeerHeartbeat) GetMissedPongs() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.missedPongs
}

// GetLastPong 获取上次 pong 时间
func (h *PeerHeartbeat) GetLastPong() time.Time {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.lastPong
}

// KeepaliveManager 心跳管理器
type KeepaliveManager struct {
	mu     sync.RWMutex
	config KeepaliveConfig

	// 所有 Peer 的心跳状态
	peers map[string]*PeerHeartbeat

	// 回调
	onPeerOnline  func(peerID string)
	onPeerSlow    func(peerID string, rtt time.Duration)
	onPeerOffline func(peerID string)
	onPing        func(peerID string) // 需要发送 ping 时触发

	// 控制
	stopCh chan struct{}
	closed bool
}

// NewKeepaliveManager 创建心跳管理器
func NewKeepaliveManager(config KeepaliveConfig) *KeepaliveManager {
	return &KeepaliveManager{
		config: config,
		peers:  make(map[string]*PeerHeartbeat),
		stopCh: make(chan struct{}),
	}
}

// SetOnPeerOnline 设置 Peer 上线回调
func (m *KeepaliveManager) SetOnPeerOnline(fn func(peerID string)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onPeerOnline = fn
}

// SetOnPeerSlow 设置 Peer 响应缓慢回调
func (m *KeepaliveManager) SetOnPeerSlow(fn func(peerID string, rtt time.Duration)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onPeerSlow = fn
}

// SetOnPeerOffline 设置 Peer 离线回调
func (m *KeepaliveManager) SetOnPeerOffline(fn func(peerID string)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onPeerOffline = fn
}

// SetOnPing 设置发送 ping 的回调
// 调用方需要实现实际的 ping 发送逻辑（如通过 DataChannel 发送）
func (m *KeepaliveManager) SetOnPing(fn func(peerID string)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onPing = fn
}

// AddPeer 添加需要监控的 Peer
func (m *KeepaliveManager) AddPeer(peerID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.peers[peerID]; !exists {
		m.peers[peerID] = NewPeerHeartbeat(peerID)
	}
}

// RemovePeer 移除 Peer
func (m *KeepaliveManager) RemovePeer(peerID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.peers, peerID)
}

// HandlePong 处理收到的 pong
func (m *KeepaliveManager) HandlePong(peerID string) {
	m.mu.RLock()
	peer, exists := m.peers[peerID]
	m.mu.RUnlock()

	if !exists {
		return
	}

	oldStatus := peer.GetStatus()
	peer.MarkPongReceived()
	newStatus := peer.GetStatus()

	// 状态变化回调
	if oldStatus != PeerStatusOnline && newStatus == PeerStatusOnline {
		m.mu.RLock()
		fn := m.onPeerOnline
		m.mu.RUnlock()
		if fn != nil {
			fn(peerID)
		}
	}

	// 检查是否响应缓慢
	rtt := peer.GetRTT()
	if rtt > m.config.SlowThreshold {
		peer.status.Store(int32(PeerStatusSlow))
		m.mu.RLock()
		fn := m.onPeerSlow
		m.mu.RUnlock()
		if fn != nil {
			fn(peerID, rtt)
		}
	}
}

// GetPeerStatus 获取 Peer 状态
func (m *KeepaliveManager) GetPeerStatus(peerID string) PeerStatus {
	m.mu.RLock()
	peer, exists := m.peers[peerID]
	m.mu.RUnlock()

	if !exists {
		return PeerStatusUnknown
	}
	return peer.GetStatus()
}

// GetPeerRTT 获取 Peer 的 RTT
func (m *KeepaliveManager) GetPeerRTT(peerID string) time.Duration {
	m.mu.RLock()
	peer, exists := m.peers[peerID]
	m.mu.RUnlock()

	if !exists {
		return 0
	}
	return peer.GetRTT()
}

// GetAllPeerStatus 获取所有 Peer 的状态
func (m *KeepaliveManager) GetAllPeerStatus() map[string]PeerStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string]PeerStatus, len(m.peers))
	for id, peer := range m.peers {
		result[id] = peer.GetStatus()
	}
	return result
}

// Start 启动心跳检测
func (m *KeepaliveManager) Start() {
	go m.runLoop()
}

// runLoop 心跳检测循环
func (m *KeepaliveManager) runLoop() {
	ticker := time.NewTicker(m.config.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-m.stopCh:
			return
		case <-ticker.C:
			m.checkAll()
		}
	}
}

// checkAll 检查所有 Peer
func (m *KeepaliveManager) checkAll() {
	m.mu.RLock()
	peers := make([]*PeerHeartbeat, 0, len(m.peers))
	for _, peer := range m.peers {
		peers = append(peers, peer)
	}
	onPing := m.onPing
	onOffline := m.onPeerOffline
	m.mu.RUnlock()

	now := time.Now()

	for _, peer := range peers {
		// 检查是否超时
		lastPong := peer.GetLastPong()
		if now.Sub(lastPong) > m.config.Timeout {
			// 超时，标记离线
			oldStatus := peer.GetStatus()
			peer.status.Store(int32(PeerStatusOffline))
			peer.MarkPongMissed()

			if oldStatus != PeerStatusOffline && onOffline != nil {
				onOffline(peer.peerID)
			}
		} else if peer.GetMissedPongs() >= m.config.MaxRetries {
			// 超过最大重试次数
			oldStatus := peer.GetStatus()
			peer.status.Store(int32(PeerStatusOffline))

			if oldStatus != PeerStatusOffline && onOffline != nil {
				onOffline(peer.peerID)
			}
		}

		// 发送 ping
		if onPing != nil {
			peer.MarkPingSent()
			onPing(peer.peerID)
		}
	}
}

// Stop 停止心跳检测
func (m *KeepaliveManager) Stop() {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return
	}
	m.closed = true
	m.mu.Unlock()

	close(m.stopCh)
}

// PeerHeartbeatInfo 心跳信息
type PeerHeartbeatInfo struct {
	PeerID      string `json:"peer_id"`
	Status      string `json:"status"`
	RTT         int64  `json:"rtt_ms"`
	MissedPongs int    `json:"missed_pongs"`
	LastPong    int64  `json:"last_pong_unix"`
}

// GetPeerInfo 获取 Peer 心跳信息
func (m *KeepaliveManager) GetPeerInfo(peerID string) *PeerHeartbeatInfo {
	m.mu.RLock()
	peer, exists := m.peers[peerID]
	m.mu.RUnlock()

	if !exists {
		return nil
	}

	peer.mu.RLock()
	defer peer.mu.RUnlock()

	return &PeerHeartbeatInfo{
		PeerID:      peerID,
		Status:      PeerStatus(peer.status.Load()).String(),
		RTT:         peer.rtt.Milliseconds(),
		MissedPongs: peer.missedPongs,
		LastPong:    peer.lastPong.Unix(),
	}
}

// GetAllPeerInfo 获取所有 Peer 心跳信息
func (m *KeepaliveManager) GetAllPeerInfo() []PeerHeartbeatInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]PeerHeartbeatInfo, 0, len(m.peers))
	for _, peer := range m.peers {
		peer.mu.RLock()
		result = append(result, PeerHeartbeatInfo{
			PeerID:      peer.peerID,
			Status:      PeerStatus(peer.status.Load()).String(),
			RTT:         peer.rtt.Milliseconds(),
			MissedPongs: peer.missedPongs,
			LastPong:    peer.lastPong.Unix(),
		})
		peer.mu.RUnlock()
	}

	return result
}
