/*
 * @Author: Marlon.M
 * @Email: maiguangyang@163.com
 * @Date: 2025-12-24
 *
 * Network Probe - 网络质量自动探测
 * 基于 WebRTC Stats API 自动采集网络质量指标
 * 用于动态选举评分
 */
package sfu

import (
	"sync"
	"time"

	"github.com/pion/webrtc/v4"
)

// NetworkMetrics 网络质量指标
type NetworkMetrics struct {
	// 基础指标
	RTT        time.Duration `json:"rtt_ms"`      // 往返时间
	Jitter     time.Duration `json:"jitter_ms"`   // 抖动
	PacketLoss float64       `json:"packet_loss"` // 丢包率 (0-1)

	// 带宽指标
	AvailableBandwidth int64 `json:"available_bw"`    // 可用带宽 (bps)
	CurrentBitrate     int64 `json:"current_bitrate"` // 当前码率 (bps)

	// 统计指标
	PacketsSent     uint64 `json:"packets_sent"`
	PacketsReceived uint64 `json:"packets_received"`
	BytesSent       uint64 `json:"bytes_sent"`
	BytesReceived   uint64 `json:"bytes_received"`

	// 质量评分 (0-100)
	QualityScore float64 `json:"quality_score"`

	// 时间戳
	Timestamp time.Time `json:"timestamp"`
}

// NetworkProbe 网络质量探测器
type NetworkProbe struct {
	mu sync.RWMutex

	pc *webrtc.PeerConnection

	// 历史指标
	history     []NetworkMetrics
	historySize int

	// 最新指标
	latest NetworkMetrics

	// 回调
	onMetricsUpdated func(metrics NetworkMetrics)

	// 控制
	interval time.Duration
	stopCh   chan struct{}
	running  bool
}

// NewNetworkProbe 创建网络探测器
func NewNetworkProbe(pc *webrtc.PeerConnection) *NetworkProbe {
	return &NetworkProbe{
		pc:          pc,
		history:     make([]NetworkMetrics, 0, 60),
		historySize: 60, // 保留最近 60 个采样点
		interval:    time.Second,
		stopCh:      make(chan struct{}),
	}
}

// SetOnMetricsUpdated 设置指标更新回调
func (p *NetworkProbe) SetOnMetricsUpdated(fn func(metrics NetworkMetrics)) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.onMetricsUpdated = fn
}

// SetInterval 设置采样间隔
func (p *NetworkProbe) SetInterval(interval time.Duration) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.interval = interval
}

// Start 开始探测
func (p *NetworkProbe) Start() {
	p.mu.Lock()
	if p.running {
		p.mu.Unlock()
		return
	}
	p.running = true
	p.mu.Unlock()

	go p.probeLoop()
}

// probeLoop 探测循环
func (p *NetworkProbe) probeLoop() {
	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	for {
		select {
		case <-p.stopCh:
			return
		case <-ticker.C:
			p.probe()
		}
	}
}

// probe 执行一次探测
func (p *NetworkProbe) probe() {
	if p.pc == nil {
		return
	}

	stats := p.pc.GetStats()
	metrics := p.extractMetrics(stats)

	p.mu.Lock()
	p.latest = metrics

	// 添加到历史
	p.history = append(p.history, metrics)
	if len(p.history) > p.historySize {
		p.history = p.history[1:]
	}

	callback := p.onMetricsUpdated
	p.mu.Unlock()

	if callback != nil {
		callback(metrics)
	}
}

// extractMetrics 从 WebRTC Stats 提取指标
func (p *NetworkProbe) extractMetrics(stats webrtc.StatsReport) NetworkMetrics {
	metrics := NetworkMetrics{
		Timestamp: time.Now(),
	}

	for _, stat := range stats {
		switch s := stat.(type) {
		case webrtc.ICECandidatePairStats:
			// ICE 候选对统计
			metrics.RTT = time.Duration(s.CurrentRoundTripTime * float64(time.Second))
			metrics.AvailableBandwidth = int64(s.AvailableOutgoingBitrate)
			metrics.BytesSent = s.BytesSent
			metrics.BytesReceived = s.BytesReceived

		case webrtc.InboundRTPStreamStats:
			// 入站 RTP 流统计
			metrics.PacketsReceived += uint64(s.PacketsReceived)
			metrics.Jitter = time.Duration(s.Jitter * float64(time.Second))
			if s.PacketsReceived > 0 && s.PacketsLost > 0 {
				total := float64(s.PacketsReceived) + float64(s.PacketsLost)
				metrics.PacketLoss = float64(s.PacketsLost) / total
			}

		case webrtc.OutboundRTPStreamStats:
			// 出站 RTP 流统计
			metrics.PacketsSent += uint64(s.PacketsSent)
			metrics.CurrentBitrate = int64(s.TargetBitrate)
		}
	}

	// 计算质量评分
	metrics.QualityScore = p.calculateQualityScore(metrics)

	return metrics
}

// calculateQualityScore 计算质量评分 (0-100)
func (p *NetworkProbe) calculateQualityScore(m NetworkMetrics) float64 {
	score := 100.0

	// RTT 评分（< 50ms = 满分，> 300ms = 0分）
	rttMs := m.RTT.Milliseconds()
	if rttMs > 300 {
		score -= 30
	} else if rttMs > 100 {
		score -= float64(rttMs-100) / 200 * 20
	} else if rttMs > 50 {
		score -= float64(rttMs-50) / 50 * 10
	}

	// 丢包率评分（0% = 满分，> 5% = 扣 30 分）
	if m.PacketLoss > 0.05 {
		score -= 30
	} else if m.PacketLoss > 0.02 {
		score -= (m.PacketLoss - 0.02) / 0.03 * 20
	} else if m.PacketLoss > 0 {
		score -= m.PacketLoss / 0.02 * 10
	}

	// 抖动评分（< 20ms = 满分，> 100ms = 扣 20 分）
	jitterMs := m.Jitter.Milliseconds()
	if jitterMs > 100 {
		score -= 20
	} else if jitterMs > 50 {
		score -= float64(jitterMs-50) / 50 * 15
	} else if jitterMs > 20 {
		score -= float64(jitterMs-20) / 30 * 10
	}

	if score < 0 {
		score = 0
	}

	return score
}

// GetLatest 获取最新指标
func (p *NetworkProbe) GetLatest() NetworkMetrics {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.latest
}

// GetHistory 获取历史指标
func (p *NetworkProbe) GetHistory() []NetworkMetrics {
	p.mu.RLock()
	defer p.mu.RUnlock()

	result := make([]NetworkMetrics, len(p.history))
	copy(result, p.history)
	return result
}

// GetAverage 获取平均指标
func (p *NetworkProbe) GetAverage() NetworkMetrics {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if len(p.history) == 0 {
		return NetworkMetrics{}
	}

	var totalRTT, totalJitter time.Duration
	var totalPacketLoss, totalScore float64
	var totalBandwidth int64

	for _, m := range p.history {
		totalRTT += m.RTT
		totalJitter += m.Jitter
		totalPacketLoss += m.PacketLoss
		totalScore += m.QualityScore
		totalBandwidth += m.AvailableBandwidth
	}

	n := len(p.history)
	return NetworkMetrics{
		RTT:                totalRTT / time.Duration(n),
		Jitter:             totalJitter / time.Duration(n),
		PacketLoss:         totalPacketLoss / float64(n),
		QualityScore:       totalScore / float64(n),
		AvailableBandwidth: totalBandwidth / int64(n),
		Timestamp:          time.Now(),
	}
}

// Stop 停止探测
func (p *NetworkProbe) Stop() {
	p.mu.Lock()
	if !p.running {
		p.mu.Unlock()
		return
	}
	p.running = false
	p.mu.Unlock()

	close(p.stopCh)
}

// IsRunning 是否正在运行
func (p *NetworkProbe) IsRunning() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.running
}

// NetworkProbeManager 网络探测管理器（管理多个 Peer 的探测）
type NetworkProbeManager struct {
	mu     sync.RWMutex
	probes map[string]*NetworkProbe
}

// NewNetworkProbeManager 创建网络探测管理器
func NewNetworkProbeManager() *NetworkProbeManager {
	return &NetworkProbeManager{
		probes: make(map[string]*NetworkProbe),
	}
}

// AddProbe 添加探测器
func (m *NetworkProbeManager) AddProbe(peerID string, pc *webrtc.PeerConnection) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.probes[peerID]; exists {
		return
	}

	probe := NewNetworkProbe(pc)
	m.probes[peerID] = probe
	probe.Start()
}

// RemoveProbe 移除探测器
func (m *NetworkProbeManager) RemoveProbe(peerID string) {
	m.mu.Lock()
	probe, exists := m.probes[peerID]
	if exists {
		delete(m.probes, peerID)
	}
	m.mu.Unlock()

	if probe != nil {
		probe.Stop()
	}
}

// GetMetrics 获取指定 Peer 的指标
func (m *NetworkProbeManager) GetMetrics(peerID string) *NetworkMetrics {
	m.mu.RLock()
	probe, exists := m.probes[peerID]
	m.mu.RUnlock()

	if !exists {
		return nil
	}

	metrics := probe.GetLatest()
	return &metrics
}

// GetAllMetrics 获取所有 Peer 的指标
func (m *NetworkProbeManager) GetAllMetrics() map[string]NetworkMetrics {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string]NetworkMetrics, len(m.probes))
	for peerID, probe := range m.probes {
		result[peerID] = probe.GetLatest()
	}
	return result
}

// StopAll 停止所有探测
func (m *NetworkProbeManager) StopAll() {
	m.mu.Lock()
	probes := make([]*NetworkProbe, 0, len(m.probes))
	for _, probe := range m.probes {
		probes = append(probes, probe)
	}
	m.probes = make(map[string]*NetworkProbe)
	m.mu.Unlock()

	for _, probe := range probes {
		probe.Stop()
	}
}
