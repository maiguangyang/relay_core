/*
 * @Author: Marlon.M
 * @Email: maiguangyang@163.com
 * @Date: 2025-12-24
 *
 * Jitter Buffer - 抖动缓冲
 * 用于平滑网络抖动，提升弱网环境下的播放质量
 * 可选功能，默认禁用以保持最低延迟
 */
package sfu

import (
	"container/heap"
	"sync"
	"time"

	"github.com/pion/rtp"
)

// JitterBufferConfig 抖动缓冲配置
type JitterBufferConfig struct {
	// 是否启用
	Enabled bool
	// 最小延迟（毫秒）
	MinDelay time.Duration
	// 最大延迟（毫秒）
	MaxDelay time.Duration
	// 目标延迟（毫秒）
	TargetDelay time.Duration
	// 最大缓冲包数
	MaxPackets int
}

// DefaultJitterBufferConfig 默认配置（禁用）
func DefaultJitterBufferConfig() JitterBufferConfig {
	return JitterBufferConfig{
		Enabled:     false,
		MinDelay:    20 * time.Millisecond,
		MaxDelay:    200 * time.Millisecond,
		TargetDelay: 50 * time.Millisecond,
		MaxPackets:  100,
	}
}

// BufferedPacket 缓冲的 RTP 包
type BufferedPacket struct {
	Packet       *rtp.Packet
	ReceivedTime time.Time
	index        int // heap index
}

// PacketHeap RTP 包堆（按序号排序）
type PacketHeap []*BufferedPacket

func (h PacketHeap) Len() int { return len(h) }

func (h PacketHeap) Less(i, j int) bool {
	// 处理序号回绕
	seqI := h[i].Packet.SequenceNumber
	seqJ := h[j].Packet.SequenceNumber
	diff := int16(seqI - seqJ)
	return diff < 0
}

func (h PacketHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].index = i
	h[j].index = j
}

func (h *PacketHeap) Push(x interface{}) {
	n := len(*h)
	packet := x.(*BufferedPacket)
	packet.index = n
	*h = append(*h, packet)
}

func (h *PacketHeap) Pop() interface{} {
	old := *h
	n := len(old)
	packet := old[n-1]
	old[n-1] = nil
	packet.index = -1
	*h = old[0 : n-1]
	return packet
}

// JitterBuffer 抖动缓冲
type JitterBuffer struct {
	mu     sync.Mutex
	config JitterBufferConfig

	// 包缓冲区（堆）
	packets PacketHeap

	// 序号跟踪
	lastSeqNum      uint16
	initialized     bool
	packetsReceived uint64
	packetsDropped  uint64
	packetsReorder  uint64

	// 延迟估计
	currentDelay    time.Duration
	jitter          time.Duration // 当前抖动估计值
	lastArrivalTime time.Time     // 上一个包到达时间
	lastTimestamp   uint32        // 上一个包的 RTP 时间戳

	// 输出通道
	outputCh chan *rtp.Packet

	// 控制
	stopCh chan struct{}
	closed bool
}

// NewJitterBuffer 创建抖动缓冲
func NewJitterBuffer(config JitterBufferConfig) *JitterBuffer {
	jb := &JitterBuffer{
		config:       config,
		packets:      make(PacketHeap, 0, config.MaxPackets),
		currentDelay: config.TargetDelay,
		outputCh:     make(chan *rtp.Packet, config.MaxPackets),
		stopCh:       make(chan struct{}),
	}
	heap.Init(&jb.packets)
	return jb
}

// Push 添加 RTP 包
func (jb *JitterBuffer) Push(packet *rtp.Packet) {
	if !jb.config.Enabled {
		// 禁用时直接输出
		select {
		case jb.outputCh <- packet:
		default:
			// 通道满，丢弃
		}
		return
	}

	jb.mu.Lock()
	defer jb.mu.Unlock()

	if jb.closed {
		return
	}

	now := time.Now()
	jb.packetsReceived++

	// 计算抖动 (RFC 3550 算法)
	if jb.initialized && !jb.lastArrivalTime.IsZero() {
		// 计算到达时间差
		arrivalDiff := now.Sub(jb.lastArrivalTime)
		// 计算 RTP 时间戳差（假设 90kHz 时钟）
		timestampDiff := time.Duration(packet.Timestamp-jb.lastTimestamp) * time.Second / 90000

		// 计算偏差
		d := arrivalDiff - timestampDiff
		if d < 0 {
			d = -d
		}

		// 运行平均计算: jitter = jitter + (|D(i-1,i)| - jitter) / 16
		jb.jitter = jb.jitter + (d-jb.jitter)/16

		// 自适应调整延迟：保持在 jitter * 2 和 jitter * 4 之间
		targetDelay := jb.jitter * 3
		if targetDelay < jb.config.MinDelay {
			targetDelay = jb.config.MinDelay
		}
		if targetDelay > jb.config.MaxDelay {
			targetDelay = jb.config.MaxDelay
		}
		// 平滑调整
		jb.currentDelay = jb.currentDelay + (targetDelay-jb.currentDelay)/8
	}

	jb.lastArrivalTime = now
	jb.lastTimestamp = packet.Timestamp

	// 检查是否是旧包（已经播放过的）
	if jb.initialized {
		diff := int16(packet.SequenceNumber - jb.lastSeqNum)
		if diff < -100 { // 过于陈旧的包，丢弃
			jb.packetsDropped++
			return
		}
		if diff < 0 {
			jb.packetsReorder++
		}
	}

	// 检查缓冲区是否满
	if len(jb.packets) >= jb.config.MaxPackets {
		// 丢弃最旧的包
		oldest := heap.Pop(&jb.packets).(*BufferedPacket)
		_ = oldest
		jb.packetsDropped++
	}

	// 添加到缓冲区
	buffered := &BufferedPacket{
		Packet:       packet,
		ReceivedTime: now,
	}
	heap.Push(&jb.packets, buffered)
}

// Start 启动输出
func (jb *JitterBuffer) Start() {
	if !jb.config.Enabled {
		return
	}

	go jb.outputLoop()
}

// outputLoop 输出循环
func (jb *JitterBuffer) outputLoop() {
	// 以固定间隔输出包（假设 90kHz 时钟，30fps 视频 = 每 33ms 一帧）
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-jb.stopCh:
			return
		case <-ticker.C:
			jb.tryOutput()
		}
	}
}

// tryOutput 尝试输出包
func (jb *JitterBuffer) tryOutput() {
	jb.mu.Lock()
	defer jb.mu.Unlock()

	if len(jb.packets) == 0 {
		return
	}

	now := time.Now()

	for len(jb.packets) > 0 {
		// 查看最早的包
		oldest := jb.packets[0]
		age := now.Sub(oldest.ReceivedTime)

		// 如果已经达到目标延迟，输出
		if age >= jb.currentDelay {
			packet := heap.Pop(&jb.packets).(*BufferedPacket)

			// 更新序号
			if !jb.initialized || int16(packet.Packet.SequenceNumber-jb.lastSeqNum) > 0 {
				jb.lastSeqNum = packet.Packet.SequenceNumber
				jb.initialized = true
			}

			// 发送到输出通道
			select {
			case jb.outputCh <- packet.Packet:
			default:
				// 通道满，丢弃
				jb.packetsDropped++
			}
		} else {
			// 还没到时间，停止输出
			break
		}
	}
}

// Output 获取输出通道
func (jb *JitterBuffer) Output() <-chan *rtp.Packet {
	return jb.outputCh
}

// Pop 直接获取一个包（同步方式）
func (jb *JitterBuffer) Pop() *rtp.Packet {
	if !jb.config.Enabled {
		return nil
	}

	jb.mu.Lock()
	defer jb.mu.Unlock()

	if len(jb.packets) == 0 {
		return nil
	}

	packet := heap.Pop(&jb.packets).(*BufferedPacket)
	jb.lastSeqNum = packet.Packet.SequenceNumber
	jb.initialized = true
	return packet.Packet
}

// SetDelay 设置目标延迟
func (jb *JitterBuffer) SetDelay(delay time.Duration) {
	jb.mu.Lock()
	defer jb.mu.Unlock()

	if delay < jb.config.MinDelay {
		delay = jb.config.MinDelay
	}
	if delay > jb.config.MaxDelay {
		delay = jb.config.MaxDelay
	}
	jb.currentDelay = delay
}

// Enable 启用/禁用
func (jb *JitterBuffer) Enable(enabled bool) {
	jb.mu.Lock()
	defer jb.mu.Unlock()
	jb.config.Enabled = enabled
}

// IsEnabled 是否启用
func (jb *JitterBuffer) IsEnabled() bool {
	jb.mu.Lock()
	defer jb.mu.Unlock()
	return jb.config.Enabled
}

// Stats 统计信息
type JitterBufferStats struct {
	Enabled         bool   `json:"enabled"`
	BufferedPackets int    `json:"buffered_packets"`
	CurrentDelay    int64  `json:"current_delay_ms"`
	Jitter          int64  `json:"jitter_ms"`
	PacketsReceived uint64 `json:"packets_received"`
	PacketsDropped  uint64 `json:"packets_dropped"`
	PacketsReorder  uint64 `json:"packets_reorder"`
}

// GetStats 获取统计
func (jb *JitterBuffer) GetStats() JitterBufferStats {
	jb.mu.Lock()
	defer jb.mu.Unlock()

	return JitterBufferStats{
		Enabled:         jb.config.Enabled,
		BufferedPackets: len(jb.packets),
		CurrentDelay:    jb.currentDelay.Milliseconds(),
		Jitter:          jb.jitter.Milliseconds(),
		PacketsReceived: jb.packetsReceived,
		PacketsDropped:  jb.packetsDropped,
		PacketsReorder:  jb.packetsReorder,
	}
}

// Flush 清空缓冲区
func (jb *JitterBuffer) Flush() {
	jb.mu.Lock()
	defer jb.mu.Unlock()

	jb.packets = make(PacketHeap, 0, jb.config.MaxPackets)
	heap.Init(&jb.packets)
}

// Close 关闭
func (jb *JitterBuffer) Close() {
	jb.mu.Lock()
	if jb.closed {
		jb.mu.Unlock()
		return
	}
	jb.closed = true
	jb.mu.Unlock()

	close(jb.stopCh)
	close(jb.outputCh)
}
