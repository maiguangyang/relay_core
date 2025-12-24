/*
 * @Author: Marlon.M
 * @Email: maiguangyang@163.com
 * @Date: 2025-12-24
 *
 * Stats - 流量统计与监控
 * 提供实时流量统计、码率计算、丢包率统计等功能
 */
package sfu

import (
	"encoding/json"
	"sync"
	"sync/atomic"
	"time"
)

// TrafficStats 流量统计
type TrafficStats struct {
	mu sync.RWMutex

	// 累计统计
	TotalBytesIn    uint64 `json:"total_bytes_in"`
	TotalBytesOut   uint64 `json:"total_bytes_out"`
	TotalPacketsIn  uint64 `json:"total_packets_in"`
	TotalPacketsOut uint64 `json:"total_packets_out"`

	// 丢包统计
	PacketsLost    uint64 `json:"packets_lost"`
	PacketsRetrans uint64 `json:"packets_retrans"`

	// 码率计算（每秒更新）
	lastCalcTime time.Time
	lastBytesIn  uint64
	lastBytesOut uint64
	BitrateIn    float64 `json:"bitrate_in_bps"`
	BitrateOut   float64 `json:"bitrate_out_bps"`

	// 时间窗口统计
	windowStart      time.Time
	windowBytesIn    uint64
	windowBytesOut   uint64
	windowPacketsIn  uint64
	windowPacketsOut uint64
}

// NewTrafficStats 创建流量统计
func NewTrafficStats() *TrafficStats {
	now := time.Now()
	return &TrafficStats{
		lastCalcTime: now,
		windowStart:  now,
	}
}

// AddBytesIn 添加接收字节数
func (s *TrafficStats) AddBytesIn(bytes uint64) {
	atomic.AddUint64(&s.TotalBytesIn, bytes)
	atomic.AddUint64(&s.windowBytesIn, bytes)
}

// AddBytesOut 添加发送字节数
func (s *TrafficStats) AddBytesOut(bytes uint64) {
	atomic.AddUint64(&s.TotalBytesOut, bytes)
	atomic.AddUint64(&s.windowBytesOut, bytes)
}

// AddPacketIn 添加接收包数
func (s *TrafficStats) AddPacketIn() {
	atomic.AddUint64(&s.TotalPacketsIn, 1)
	atomic.AddUint64(&s.windowPacketsIn, 1)
}

// AddPacketOut 添加发送包数
func (s *TrafficStats) AddPacketOut() {
	atomic.AddUint64(&s.TotalPacketsOut, 1)
	atomic.AddUint64(&s.windowPacketsOut, 1)
}

// AddPacketLost 添加丢包数
func (s *TrafficStats) AddPacketLost() {
	atomic.AddUint64(&s.PacketsLost, 1)
}

// AddPacketRetrans 添加重传包数
func (s *TrafficStats) AddPacketRetrans() {
	atomic.AddUint64(&s.PacketsRetrans, 1)
}

// CalculateBitrate 计算码率（每秒调用一次）
func (s *TrafficStats) CalculateBitrate() {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(s.lastCalcTime).Seconds()
	if elapsed < 0.1 {
		return // 避免过于频繁计算
	}

	currentBytesIn := atomic.LoadUint64(&s.TotalBytesIn)
	currentBytesOut := atomic.LoadUint64(&s.TotalBytesOut)

	// 计算码率 (bits per second)
	s.BitrateIn = float64(currentBytesIn-s.lastBytesIn) * 8 / elapsed
	s.BitrateOut = float64(currentBytesOut-s.lastBytesOut) * 8 / elapsed

	s.lastBytesIn = currentBytesIn
	s.lastBytesOut = currentBytesOut
	s.lastCalcTime = now
}

// GetLossRate 获取丢包率
func (s *TrafficStats) GetLossRate() float64 {
	totalIn := atomic.LoadUint64(&s.TotalPacketsIn)
	lost := atomic.LoadUint64(&s.PacketsLost)

	if totalIn == 0 {
		return 0
	}
	return float64(lost) / float64(totalIn+lost)
}

// ResetWindow 重置时间窗口统计
func (s *TrafficStats) ResetWindow() {
	s.mu.Lock()
	defer s.mu.Unlock()

	atomic.StoreUint64(&s.windowBytesIn, 0)
	atomic.StoreUint64(&s.windowBytesOut, 0)
	atomic.StoreUint64(&s.windowPacketsIn, 0)
	atomic.StoreUint64(&s.windowPacketsOut, 0)
	s.windowStart = time.Now()
}

// GetWindowStats 获取时间窗口统计
func (s *TrafficStats) GetWindowStats() (duration time.Duration, bytesIn, bytesOut, packetsIn, packetsOut uint64) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return time.Since(s.windowStart),
		atomic.LoadUint64(&s.windowBytesIn),
		atomic.LoadUint64(&s.windowBytesOut),
		atomic.LoadUint64(&s.windowPacketsIn),
		atomic.LoadUint64(&s.windowPacketsOut)
}

// Snapshot 获取当前快照
func (s *TrafficStats) Snapshot() TrafficStatsSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return TrafficStatsSnapshot{
		TotalBytesIn:    atomic.LoadUint64(&s.TotalBytesIn),
		TotalBytesOut:   atomic.LoadUint64(&s.TotalBytesOut),
		TotalPacketsIn:  atomic.LoadUint64(&s.TotalPacketsIn),
		TotalPacketsOut: atomic.LoadUint64(&s.TotalPacketsOut),
		PacketsLost:     atomic.LoadUint64(&s.PacketsLost),
		PacketsRetrans:  atomic.LoadUint64(&s.PacketsRetrans),
		BitrateIn:       s.BitrateIn,
		BitrateOut:      s.BitrateOut,
		LossRate:        s.GetLossRate(),
		Timestamp:       time.Now().Unix(),
	}
}

// TrafficStatsSnapshot 统计快照
type TrafficStatsSnapshot struct {
	TotalBytesIn    uint64  `json:"total_bytes_in"`
	TotalBytesOut   uint64  `json:"total_bytes_out"`
	TotalPacketsIn  uint64  `json:"total_packets_in"`
	TotalPacketsOut uint64  `json:"total_packets_out"`
	PacketsLost     uint64  `json:"packets_lost"`
	PacketsRetrans  uint64  `json:"packets_retrans"`
	BitrateIn       float64 `json:"bitrate_in_bps"`
	BitrateOut      float64 `json:"bitrate_out_bps"`
	LossRate        float64 `json:"loss_rate"`
	Timestamp       int64   `json:"timestamp"`
}

// ToJSON 序列化为 JSON
func (s TrafficStatsSnapshot) ToJSON() string {
	data, _ := json.Marshal(s)
	return string(data)
}

// RoomStats 房间统计
type RoomStats struct {
	mu sync.RWMutex

	roomID string

	// 总体流量
	traffic *TrafficStats

	// 每个 Peer 的统计
	peerStats map[string]*TrafficStats

	// 状态
	StartTime time.Time `json:"start_time"`
	PeerCount int       `json:"peer_count"`
}

// NewRoomStats 创建房间统计
func NewRoomStats(roomID string) *RoomStats {
	return &RoomStats{
		roomID:    roomID,
		traffic:   NewTrafficStats(),
		peerStats: make(map[string]*TrafficStats),
		StartTime: time.Now(),
	}
}

// GetOrCreatePeerStats 获取或创建 Peer 统计
func (r *RoomStats) GetOrCreatePeerStats(peerID string) *TrafficStats {
	r.mu.Lock()
	defer r.mu.Unlock()

	if stats, exists := r.peerStats[peerID]; exists {
		return stats
	}

	stats := NewTrafficStats()
	r.peerStats[peerID] = stats
	r.PeerCount = len(r.peerStats)
	return stats
}

// RemovePeerStats 移除 Peer 统计
func (r *RoomStats) RemovePeerStats(peerID string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.peerStats, peerID)
	r.PeerCount = len(r.peerStats)
}

// GetTraffic 获取总体流量统计
func (r *RoomStats) GetTraffic() *TrafficStats {
	return r.traffic
}

// CalculateAllBitrates 计算所有码率
func (r *RoomStats) CalculateAllBitrates() {
	r.traffic.CalculateBitrate()

	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, stats := range r.peerStats {
		stats.CalculateBitrate()
	}
}

// Snapshot 获取房间统计快照
func (r *RoomStats) Snapshot() RoomStatsSnapshot {
	r.mu.RLock()
	defer r.mu.RUnlock()

	peerSnapshots := make(map[string]TrafficStatsSnapshot, len(r.peerStats))
	for peerID, stats := range r.peerStats {
		peerSnapshots[peerID] = stats.Snapshot()
	}

	return RoomStatsSnapshot{
		RoomID:    r.roomID,
		Traffic:   r.traffic.Snapshot(),
		PeerStats: peerSnapshots,
		PeerCount: len(r.peerStats),
		Uptime:    time.Since(r.StartTime).Seconds(),
	}
}

// RoomStatsSnapshot 房间统计快照
type RoomStatsSnapshot struct {
	RoomID    string                          `json:"room_id"`
	Traffic   TrafficStatsSnapshot            `json:"traffic"`
	PeerStats map[string]TrafficStatsSnapshot `json:"peer_stats"`
	PeerCount int                             `json:"peer_count"`
	Uptime    float64                         `json:"uptime_seconds"`
}

// ToJSON 序列化为 JSON
func (s RoomStatsSnapshot) ToJSON() string {
	data, _ := json.Marshal(s)
	return string(data)
}
