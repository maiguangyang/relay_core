/*
 * @Author: Marlon.M
 * @Email: maiguangyang@163.com
 * @Date: 2025-12-24
 *
 * 增强版动态选举系统
 * 基于设备类型、连接类型和网络质量的综合评分
 */
package election

import (
	"sort"
	"sync"
	"time"
)

// DeviceType 设备类型
type DeviceType int

const (
	DeviceTypeUnknown DeviceType = iota
	DeviceTypePC                 // PC/Mac 台式机/笔记本
	DeviceTypePad                // iPad/Android Tablet
	DeviceTypeMobile             // iPhone/Android Phone
	DeviceTypeTV                 // 智能电视
)

func (d DeviceType) String() string {
	switch d {
	case DeviceTypePC:
		return "PC"
	case DeviceTypePad:
		return "Pad"
	case DeviceTypeMobile:
		return "Mobile"
	case DeviceTypeTV:
		return "TV"
	default:
		return "Unknown"
	}
}

// 设备类型基础分数 (根据技术规范)
func (d DeviceType) BaseScore() float64 {
	switch d {
	case DeviceTypePC:
		return 100.0 // Tier 1: PC/Mac 最优
	case DeviceTypeTV:
		return 90.0 // 智能电视次之
	case DeviceTypePad:
		return 60.0 // Tier 3: 平板
	case DeviceTypeMobile:
		return 40.0 // Tier 4: 手机
	default:
		return 20.0
	}
}

// ConnectionType 连接类型
type ConnectionType int

const (
	ConnectionTypeUnknown  ConnectionType = iota
	ConnectionTypeEthernet                // 有线网络
	ConnectionTypeWiFi                    // WiFi
	ConnectionTypeCellular                // 蜂窝网络
)

func (c ConnectionType) String() string {
	switch c {
	case ConnectionTypeEthernet:
		return "Ethernet"
	case ConnectionTypeWiFi:
		return "WiFi"
	case ConnectionTypeCellular:
		return "Cellular"
	default:
		return "Unknown"
	}
}

// 连接类型权重乘数
func (c ConnectionType) Multiplier() float64 {
	switch c {
	case ConnectionTypeEthernet:
		return 1.0 // 有线最稳定
	case ConnectionTypeWiFi:
		return 0.8 // WiFi 稍逊
	case ConnectionTypeCellular:
		return 0.3 // 蜂窝网络不适合做 Relay
	default:
		return 0.5
	}
}

// PowerState 电源状态
type PowerState int

const (
	PowerStateUnknown    PowerState = iota
	PowerStatePluggedIn             // 充电中
	PowerStateBattery               // 电池供电
	PowerStateLowBattery            // 低电量
)

func (p PowerState) String() string {
	switch p {
	case PowerStatePluggedIn:
		return "PluggedIn"
	case PowerStateBattery:
		return "Battery"
	case PowerStateLowBattery:
		return "LowBattery"
	default:
		return "Unknown"
	}
}

// 电源状态权重乘数
func (p PowerState) Multiplier() float64 {
	switch p {
	case PowerStatePluggedIn:
		return 1.0 // 充电中最佳
	case PowerStateBattery:
		return 0.7 // 电池供电降权
	case PowerStateLowBattery:
		return 0.3 // 低电量大幅降权
	default:
		return 0.8
	}
}

// Candidate 表示可以成为代理的节点
type Candidate struct {
	PeerID string

	// 设备信息
	DeviceType     DeviceType
	ConnectionType ConnectionType
	PowerState     PowerState

	// 网络质量指标
	Bandwidth  int64   // 可用带宽 bytes/sec
	Latency    int64   // 平均延迟 ms
	PacketLoss float64 // 丢包率 (0-1)

	// 计算得分
	Score float64

	// 状态
	IsProxy    bool // 当前是否为代理
	LastUpdate time.Time

	// 附加信息
	DeviceName string // 设备名称（用于展示）
	OSVersion  string // 操作系统版本
}

// ElectionResult 选举结果
type ElectionResult struct {
	ProxyID        string
	Score          float64
	DeviceType     string
	ConnectionType string
	Reason         string
	Timestamp      time.Time
}

// ElectionCallback 选举完成回调
type ElectionCallback func(result ElectionResult)

// Elector 管理房间内的代理选举
type Elector struct {
	mu           sync.RWMutex
	roomID       string
	candidates   map[string]*Candidate
	currentProxy string

	// 配置
	minCandidates    int
	scoreThreshold   float64
	electionInterval time.Duration

	// 权重配置
	deviceWeight     float64 // 设备类型权重
	networkWeight    float64 // 网络质量权重
	connectionWeight float64 // 连接类型权重
	powerWeight      float64 // 电源状态权重

	// 回调
	onElection ElectionCallback

	// 状态
	closed bool
	stopCh chan struct{}
}

// ElectorConfig 选举器配置
type ElectorConfig struct {
	MinCandidates    int
	ScoreThreshold   float64
	ElectionInterval time.Duration

	// 权重配置 (总和应为 1.0)
	DeviceWeight     float64
	NetworkWeight    float64
	ConnectionWeight float64
	PowerWeight      float64
}

// DefaultElectorConfig 返回默认配置
func DefaultElectorConfig() ElectorConfig {
	return ElectorConfig{
		MinCandidates:    1, // 至少 1 个候选
		ScoreThreshold:   10.0,
		ElectionInterval: 10 * time.Second,

		// 权重分配 (根据场景需求调整)
		DeviceWeight:     0.4, // 设备类型最重要
		NetworkWeight:    0.3, // 网络质量次之
		ConnectionWeight: 0.2, // 连接类型
		PowerWeight:      0.1, // 电源状态
	}
}

// NewElector 创建选举器
func NewElector(roomID string, config ElectorConfig) *Elector {
	return &Elector{
		roomID:           roomID,
		candidates:       make(map[string]*Candidate),
		minCandidates:    config.MinCandidates,
		scoreThreshold:   config.ScoreThreshold,
		electionInterval: config.ElectionInterval,
		deviceWeight:     config.DeviceWeight,
		networkWeight:    config.NetworkWeight,
		connectionWeight: config.ConnectionWeight,
		powerWeight:      config.PowerWeight,
		stopCh:           make(chan struct{}),
	}
}

// SetOnElection 设置选举回调
func (e *Elector) SetOnElection(fn ElectionCallback) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.onElection = fn
}

// UpdateCandidate 更新或添加候选者
func (e *Elector) UpdateCandidate(candidate Candidate) {
	e.mu.Lock()
	defer e.mu.Unlock()

	candidate.LastUpdate = time.Now()
	candidate.Score = e.calculateScore(&candidate)
	e.candidates[candidate.PeerID] = &candidate
}

// UpdateDeviceInfo 更新设备信息
func (e *Elector) UpdateDeviceInfo(peerID string, deviceType DeviceType, connType ConnectionType, powerState PowerState) {
	e.mu.Lock()
	defer e.mu.Unlock()

	c, exists := e.candidates[peerID]
	if !exists {
		c = &Candidate{PeerID: peerID}
		e.candidates[peerID] = c
	}

	c.DeviceType = deviceType
	c.ConnectionType = connType
	c.PowerState = powerState
	c.LastUpdate = time.Now()
	c.Score = e.calculateScore(c)
}

// UpdateNetworkMetrics 更新网络指标
func (e *Elector) UpdateNetworkMetrics(peerID string, bandwidth, latency int64, packetLoss float64) {
	e.mu.Lock()
	defer e.mu.Unlock()

	c, exists := e.candidates[peerID]
	if !exists {
		c = &Candidate{PeerID: peerID}
		e.candidates[peerID] = c
	}

	c.Bandwidth = bandwidth
	c.Latency = latency
	c.PacketLoss = packetLoss
	c.LastUpdate = time.Now()
	c.Score = e.calculateScore(c)
}

// RemoveCandidate 移除候选者
func (e *Elector) RemoveCandidate(peerID string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	delete(e.candidates, peerID)

	// 如果移除的是当前代理，触发新选举
	if e.currentProxy == peerID {
		e.currentProxy = ""
		e.triggerElection("proxy_left")
	}
}

// calculateScore 计算候选者综合得分
// 公式: (设备分 × 设备权重 + 网络分 × 网络权重) × 连接类型乘数 × 电源状态乘数
func (e *Elector) calculateScore(c *Candidate) float64 {
	// 1. 设备类型基础分
	deviceScore := c.DeviceType.BaseScore()

	// 2. 网络质量分 (0-100)
	var networkScore float64 = 50.0 // 默认中等
	if c.Bandwidth > 0 || c.Latency > 0 {
		// 带宽分 (假设 10Mbps = 满分)
		bandwidthScore := float64(c.Bandwidth) / 10000000.0 * 100.0
		if bandwidthScore > 100 {
			bandwidthScore = 100
		}

		// 延迟分 (假设 < 20ms = 满分，> 200ms = 0分)
		latencyScore := 100.0 - float64(c.Latency)/2.0
		if latencyScore < 0 {
			latencyScore = 0
		}
		if latencyScore > 100 {
			latencyScore = 100
		}

		// 丢包分
		packetLossScore := (1.0 - c.PacketLoss) * 100.0

		// 综合网络分
		networkScore = 0.4*bandwidthScore + 0.4*latencyScore + 0.2*packetLossScore
	}

	// 3. 加权求和
	baseScore := e.deviceWeight*deviceScore + e.networkWeight*networkScore

	// 4. 应用连接类型乘数
	score := baseScore * c.ConnectionType.Multiplier()

	// 5. 应用电源状态乘数
	score *= c.PowerState.Multiplier()

	return score
}

// triggerElection 执行选举
func (e *Elector) triggerElection(reason string) {
	if len(e.candidates) < e.minCandidates {
		return
	}

	// 找出最佳候选者
	var bestCandidate *Candidate
	for _, c := range e.candidates {
		if c.Score >= e.scoreThreshold {
			if bestCandidate == nil || c.Score > bestCandidate.Score {
				bestCandidate = c
			}
		}
	}

	if bestCandidate == nil {
		return
	}

	// 只有代理变更时才触发回调
	if bestCandidate.PeerID != e.currentProxy {
		e.currentProxy = bestCandidate.PeerID
		bestCandidate.IsProxy = true

		// 取消旧代理标记
		for _, c := range e.candidates {
			if c.PeerID != bestCandidate.PeerID {
				c.IsProxy = false
			}
		}

		result := ElectionResult{
			ProxyID:        bestCandidate.PeerID,
			Score:          bestCandidate.Score,
			DeviceType:     bestCandidate.DeviceType.String(),
			ConnectionType: bestCandidate.ConnectionType.String(),
			Reason:         reason,
			Timestamp:      time.Now(),
		}

		if e.onElection != nil {
			go e.onElection(result)
		}
	}
}

// Elect 手动触发选举
func (e *Elector) Elect() *ElectionResult {
	e.mu.Lock()
	defer e.mu.Unlock()

	if len(e.candidates) < e.minCandidates {
		return nil
	}

	// 按分数排序
	sorted := make([]*Candidate, 0, len(e.candidates))
	for _, c := range e.candidates {
		sorted = append(sorted, c)
	}
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Score > sorted[j].Score
	})

	if len(sorted) == 0 || sorted[0].Score < e.scoreThreshold {
		return nil
	}

	bestCandidate := sorted[0]
	e.currentProxy = bestCandidate.PeerID

	return &ElectionResult{
		ProxyID:        bestCandidate.PeerID,
		Score:          bestCandidate.Score,
		DeviceType:     bestCandidate.DeviceType.String(),
		ConnectionType: bestCandidate.ConnectionType.String(),
		Reason:         "manual_election",
		Timestamp:      time.Now(),
	}
}

// GetCurrentProxy 返回当前代理 ID
func (e *Elector) GetCurrentProxy() string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.currentProxy
}

// GetCandidates 返回所有候选者（按分数排序）
func (e *Elector) GetCandidates() []Candidate {
	e.mu.RLock()
	defer e.mu.RUnlock()

	candidates := make([]Candidate, 0, len(e.candidates))
	for _, c := range e.candidates {
		candidates = append(candidates, *c)
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].Score > candidates[j].Score
	})

	return candidates
}

// GetCandidateCount 返回候选者数量
func (e *Elector) GetCandidateCount() int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return len(e.candidates)
}

// Start 开始定期选举评估
func (e *Elector) Start() {
	go func() {
		ticker := time.NewTicker(e.electionInterval)
		defer ticker.Stop()

		for {
			select {
			case <-e.stopCh:
				return
			case <-ticker.C:
				e.mu.Lock()
				e.triggerElection("periodic")
				e.mu.Unlock()
			}
		}
	}()
}

// Close 停止选举器
func (e *Elector) Close() {
	e.mu.Lock()
	if e.closed {
		e.mu.Unlock()
		return
	}
	e.closed = true
	e.mu.Unlock()

	close(e.stopCh)
}
