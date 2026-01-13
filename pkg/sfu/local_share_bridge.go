/*
 * @Author: Marlon.M
 * @Email: maiguangyang@163.com
 * @Date: 2026-01-13
 *
 * LocalShareBridge - 本地分享专用 UDP 接收器
 *
 * 完全在 Go 层处理 UDP，不经过 FFI，实现零拷贝高性能数据路径。
 * 用于接收来自 Flutter 的本地屏幕共享 RTP 包。
 */
package sfu

import (
	"errors"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/maiguangyang/relay_core/pkg/utils"
)

// LocalShareBridgeState 桥接器状态
type LocalShareBridgeState int32

const (
	LocalShareBridgeStateIdle LocalShareBridgeState = iota
	LocalShareBridgeStateStarting
	LocalShareBridgeStateRunning
	LocalShareBridgeStateStopping
	LocalShareBridgeStateStopped
	LocalShareBridgeStateFailed
)

func (s LocalShareBridgeState) String() string {
	switch s {
	case LocalShareBridgeStateIdle:
		return "idle"
	case LocalShareBridgeStateStarting:
		return "starting"
	case LocalShareBridgeStateRunning:
		return "running"
	case LocalShareBridgeStateStopping:
		return "stopping"
	case LocalShareBridgeStateStopped:
		return "stopped"
	case LocalShareBridgeStateFailed:
		return "failed"
	default:
		return "unknown"
	}
}

// LocalShareBridge 本地分享专用 UDP 接收器
// 完全在 Go 层处理 UDP，避免 FFI 开销
type LocalShareBridge struct {
	mu sync.RWMutex

	roomID   string
	switcher *SourceSwitcher
	conn     *net.UDPConn
	port     int // actual listening port

	state  int32 // atomic LocalShareBridgeState
	closed bool

	// 缓冲池
	bufferPool *BufferPool

	// 统计
	packetsReceived uint64
	bytesReceived   uint64
	errorsCount     uint64

	// 回调
	onStateChanged func(roomID string, state LocalShareBridgeState)
	onError        func(roomID string, err error)

	// 停止信号
	stopCh chan struct{}
}

// NewLocalShareBridge 创建新的本地分享桥接器
func NewLocalShareBridge(roomID string, switcher *SourceSwitcher) *LocalShareBridge {
	return &LocalShareBridge{
		roomID:     roomID,
		switcher:   switcher,
		bufferPool: NewBufferPool(),
		stopCh:     make(chan struct{}),
	}
}

// SetCallbacks 设置回调
func (b *LocalShareBridge) SetCallbacks(
	onStateChanged func(roomID string, state LocalShareBridgeState),
	onError func(roomID string, err error),
) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.onStateChanged = onStateChanged
	b.onError = onError
}

// Start 启动 UDP 监听
// preferredPort: 首选端口，0 表示自动分配
// 返回: 实际监听端口
func (b *LocalShareBridge) Start(preferredPort int) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return 0, ErrBridgeClosed
	}

	if b.conn != nil {
		return b.port, nil // 已经启动
	}

	b.setState(LocalShareBridgeStateStarting)

	// 绑定 UDP 地址
	addr := &net.UDPAddr{
		IP:   net.ParseIP("127.0.0.1"), // 只监听 localhost
		Port: preferredPort,
	}

	conn, err := net.ListenUDP("udp4", addr)
	if err != nil {
		b.setState(LocalShareBridgeStateFailed)
		b.emitError(err)
		return 0, err
	}

	// 设置读缓冲区大小（2MB）
	if err := conn.SetReadBuffer(2 * 1024 * 1024); err != nil {
		utils.Warn("[LocalShareBridge] Failed to set read buffer: %v", err)
	}

	localAddr := conn.LocalAddr().(*net.UDPAddr)
	b.conn = conn
	b.port = localAddr.Port

	// 启动读取循环
	go b.readLoop()

	b.setState(LocalShareBridgeStateRunning)
	utils.Info("[LocalShareBridge] Started on port %d for room %s", b.port, b.roomID)

	return b.port, nil
}

// Stop 停止 UDP 监听
func (b *LocalShareBridge) Stop() {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.conn == nil {
		return
	}

	b.setState(LocalShareBridgeStateStopping)

	// 发送停止信号
	close(b.stopCh)

	// 设置读超时，让 readLoop 退出
	b.conn.SetReadDeadline(time.Now())

	// 关闭连接
	b.conn.Close()
	b.conn = nil

	b.setState(LocalShareBridgeStateStopped)
	utils.Info("[LocalShareBridge] Stopped for room %s", b.roomID)
}

// Close 关闭桥接器
func (b *LocalShareBridge) Close() {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return
	}
	b.closed = true
	b.mu.Unlock()

	b.Stop()
}

// GetPort 获取监听端口
func (b *LocalShareBridge) GetPort() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.port
}

// GetState 获取当前状态
func (b *LocalShareBridge) GetState() LocalShareBridgeState {
	return LocalShareBridgeState(atomic.LoadInt32(&b.state))
}

// IsRunning 是否正在运行
func (b *LocalShareBridge) IsRunning() bool {
	return b.GetState() == LocalShareBridgeStateRunning
}

// readLoop UDP 读取循环
func (b *LocalShareBridge) readLoop() {
	for {
		// 检查停止信号
		select {
		case <-b.stopCh:
			return
		default:
		}

		// 获取缓冲区（从池中复用）
		buf := b.bufferPool.GetBuffer()

		// 读取 UDP 包
		n, _, err := b.conn.ReadFromUDP(buf)
		if err != nil {
			b.bufferPool.PutBuffer(buf)

			// 检查是否是正常关闭
			if b.GetState() != LocalShareBridgeStateRunning {
				return
			}

			// 网络错误
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue // 超时重试
			}

			atomic.AddUint64(&b.errorsCount, 1)
			utils.Error("[LocalShareBridge] Read error: %v", err)
			continue
		}

		// 更新统计
		atomic.AddUint64(&b.packetsReceived, 1)
		atomic.AddUint64(&b.bytesReceived, uint64(n))

		// 解析 RTP 并注入到 SourceSwitcher
		if b.switcher != nil && n >= 12 { // 最小 RTP 头长度
			// 简单的 RTP/RTCP 区分
			// RTP payload type 在第 2 字节的低 7 位
			// 视频通常使用动态负载类型 96-127
			pt := buf[1] & 0x7F
			isVideo := pt >= 96 && pt <= 127

			if err := b.switcher.InjectLocalPacket(isVideo, buf[:n]); err != nil {
				atomic.AddUint64(&b.errorsCount, 1)
			}
		}

		// 归还缓冲区
		b.bufferPool.PutBuffer(buf)
	}
}

// setState 设置状态
func (b *LocalShareBridge) setState(state LocalShareBridgeState) {
	atomic.StoreInt32(&b.state, int32(state))
	b.emitStateChanged(state)
}

// emitStateChanged 触发状态变更回调
func (b *LocalShareBridge) emitStateChanged(state LocalShareBridgeState) {
	if b.onStateChanged != nil {
		go b.onStateChanged(b.roomID, state)
	}
}

// emitError 触发错误回调
func (b *LocalShareBridge) emitError(err error) {
	if b.onError != nil {
		go b.onError(b.roomID, err)
	}
}

// LocalShareBridgeStatus 桥接器状态信息
type LocalShareBridgeStatus struct {
	RoomID          string `json:"room_id"`
	State           string `json:"state"`
	Port            int    `json:"port"`
	PacketsReceived uint64 `json:"packets_received"`
	BytesReceived   uint64 `json:"bytes_received"`
	ErrorsCount     uint64 `json:"errors_count"`
}

// GetStatus 获取状态信息
func (b *LocalShareBridge) GetStatus() LocalShareBridgeStatus {
	return LocalShareBridgeStatus{
		RoomID:          b.roomID,
		State:           b.GetState().String(),
		Port:            b.GetPort(),
		PacketsReceived: atomic.LoadUint64(&b.packetsReceived),
		BytesReceived:   atomic.LoadUint64(&b.bytesReceived),
		ErrorsCount:     atomic.LoadUint64(&b.errorsCount),
	}
}

// ========================================
// 全局桥接器管理
// ========================================

var (
	localShareBridgesMu sync.RWMutex
	localShareBridges   = make(map[string]*LocalShareBridge)
)

// GetLocalShareBridge 获取桥接器
func GetLocalShareBridge(roomID string) *LocalShareBridge {
	localShareBridgesMu.RLock()
	defer localShareBridgesMu.RUnlock()
	return localShareBridges[roomID]
}

// CreateLocalShareBridge 创建桥接器
func CreateLocalShareBridge(roomID string, switcher *SourceSwitcher) *LocalShareBridge {
	localShareBridgesMu.Lock()
	defer localShareBridgesMu.Unlock()

	// 已存在则返回
	if bridge, ok := localShareBridges[roomID]; ok {
		return bridge
	}

	bridge := NewLocalShareBridge(roomID, switcher)
	localShareBridges[roomID] = bridge
	utils.Info("[LocalShareBridge] Created for room %s", roomID)
	return bridge
}

// DestroyLocalShareBridge 销毁桥接器
func DestroyLocalShareBridge(roomID string) {
	localShareBridgesMu.Lock()
	defer localShareBridgesMu.Unlock()

	if bridge, ok := localShareBridges[roomID]; ok {
		bridge.Close()
		delete(localShareBridges, roomID)
		utils.Info("[LocalShareBridge] Destroyed for room %s", roomID)
	}
}

// 错误定义
var (
	ErrBridgeClosed = errors.New("bridge closed")
)
