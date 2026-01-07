/*
 * @Author: Marlon.M
 * @Email: maiguangyang@163.com
 * @Date: 2025-12-26
 *
 * LiveKitBridge - 连接 LiveKit SFU 并接收 RTP 包
 *
 * 用于 Relay 设备直接从 LiveKit SFU 获取 RTP 包，
 * 然后通过 SourceSwitcher 转发给局域网订阅者。
 */
package sfu

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/livekit/protocol/livekit"
	lksdk "github.com/livekit/server-sdk-go/v2"
	"github.com/pion/webrtc/v4"
)

// LiveKitBridgeState 桥接器状态
type LiveKitBridgeState int

const (
	LiveKitBridgeStateIdle LiveKitBridgeState = iota
	LiveKitBridgeStateConnecting
	LiveKitBridgeStateConnected
	LiveKitBridgeStateDisconnected
	LiveKitBridgeStateFailed
)

func (s LiveKitBridgeState) String() string {
	switch s {
	case LiveKitBridgeStateIdle:
		return "idle"
	case LiveKitBridgeStateConnecting:
		return "connecting"
	case LiveKitBridgeStateConnected:
		return "connected"
	case LiveKitBridgeStateDisconnected:
		return "disconnected"
	case LiveKitBridgeStateFailed:
		return "failed"
	default:
		return "unknown"
	}
}

// LiveKitBridge 连接 LiveKit SFU 并接收 RTP 包
type LiveKitBridge struct {
	mu sync.RWMutex

	roomID   string
	room     *lksdk.Room
	switcher *SourceSwitcher

	state LiveKitBridgeState

	// 统计
	videoPacketsReceived uint64
	audioPacketsReceived uint64
	tracksSubscribed     int32

	// 回调
	onStateChanged func(roomID string, state LiveKitBridgeState)
	onError        func(roomID string, err error)

	closed bool
}

// NewLiveKitBridge 创建新的桥接器
func NewLiveKitBridge(roomID string, switcher *SourceSwitcher) *LiveKitBridge {
	return &LiveKitBridge{
		roomID:   roomID,
		switcher: switcher,
		state:    LiveKitBridgeStateIdle,
	}
}

// SetCallbacks 设置回调
func (b *LiveKitBridge) SetCallbacks(
	onStateChanged func(roomID string, state LiveKitBridgeState),
	onError func(roomID string, err error),
) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.onStateChanged = onStateChanged
	b.onError = onError
}

// Connect 连接到 LiveKit 房间
func (b *LiveKitBridge) Connect(url, token string) error {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return fmt.Errorf("bridge is closed")
	}
	if b.room != nil {
		b.mu.Unlock()
		return fmt.Errorf("already connected")
	}
	b.state = LiveKitBridgeStateConnecting
	b.mu.Unlock()

	b.emitStateChanged(LiveKitBridgeStateConnecting)

	// 创建房间回调
	roomCB := &lksdk.RoomCallback{
		ParticipantCallback: lksdk.ParticipantCallback{
			OnTrackSubscribed:   b.onTrackSubscribed,
			OnTrackUnsubscribed: b.onTrackUnsubscribed,
			OnTrackPublished: func(pub *lksdk.RemoteTrackPublication, rp *lksdk.RemoteParticipant) {
				fmt.Printf("[Bridge] Track published: %s (kind: %s, source: %s, participant: %s)\n", pub.SID(), pub.Kind(), pub.Source(), rp.Identity())
				if !pub.IsSubscribed() {
					fmt.Printf("[Bridge] Track %s not subscribed, attempting manual subscription\n", pub.SID())
					pub.SetSubscribed(true)
				}
			},
		},
		OnDisconnected: func() {
			b.handleDisconnected()
		},
		OnReconnecting: func() {
			b.emitStateChanged(LiveKitBridgeStateConnecting)
		},
		OnReconnected: func() {
			b.emitStateChanged(LiveKitBridgeStateConnected)
		},
	}

	// 连接到房间
	// 连接到房间，启用自动订阅
	room, err := lksdk.ConnectToRoomWithToken(url, token, roomCB, lksdk.WithAutoSubscribe(true))
	if err != nil {
		b.mu.Lock()
		b.state = LiveKitBridgeStateFailed
		b.mu.Unlock()
		b.emitStateChanged(LiveKitBridgeStateFailed)
		b.emitError(err)
		return err
	}

	b.mu.Lock()
	b.room = room
	b.state = LiveKitBridgeStateConnected
	b.mu.Unlock()

	b.emitStateChanged(LiveKitBridgeStateConnected)

	return nil
}

// onTrackSubscribed 轨道订阅回调
func (b *LiveKitBridge) onTrackSubscribed(
	track *webrtc.TrackRemote,
	pub *lksdk.RemoteTrackPublication,
	rp *lksdk.RemoteParticipant,
) {
	atomic.AddInt32(&b.tracksSubscribed, 1)
	fmt.Printf("[Bridge] Track subscribed: %s (kind: %s, participant: %s)\n", track.ID(), track.Kind().String(), rp.Identity())

	isVideo := track.Kind() == webrtc.RTPCodecTypeVideo

	// 对视频轨道请求最高质量（解决屏幕共享模糊问题）
	// 注意: Go SDK 的 RemoteTrackPublication 不支持 SetVideoFPS，只能设置 Quality
	if isVideo {
		// 立即请求 HIGH 质量
		pub.SetVideoQuality(livekit.VideoQuality_HIGH)
		fmt.Printf("[Bridge] Video quality requested: HIGH for track %s (source: %s)\n", track.ID(), pub.Source())

		// 延迟再次请求，确保 SFU 切换到最高质量
		go func() {
			// 500ms 后再次请求
			time.Sleep(500 * time.Millisecond)
			pub.SetVideoQuality(livekit.VideoQuality_HIGH)
			fmt.Printf("[Bridge] Video quality re-requested (500ms): HIGH for track %s\n", track.ID())

			// 2秒后再次请求，确保稳定
			time.Sleep(1500 * time.Millisecond)
			pub.SetVideoQuality(livekit.VideoQuality_HIGH)
			fmt.Printf("[Bridge] Video quality re-requested (2s): HIGH for track %s\n", track.ID())
		}()
	}

	// 获取远端轨道的完整编码参数，并更新 SourceSwitcher 的 Track
	// 这是解决画质模糊问题的关键：使用远端轨道的真实编码参数
	if b.switcher != nil {
		codec := track.Codec()
		codecCap := codec.RTPCodecCapability
		fmt.Printf("[Bridge] Track Codec: MimeType=%s, ClockRate=%d, Channels=%d, SDPFmtpLine=%s, PayloadType=%d\n",
			codecCap.MimeType, codecCap.ClockRate, codecCap.Channels, codecCap.SDPFmtpLine, codec.PayloadType)

		if isVideo {
			if err := b.switcher.SetVideoCodec(codec.RTPCodecCapability); err != nil {
				fmt.Printf("[Bridge] ERROR: SetVideoCodec failed: %v\n", err)
			} else {
				fmt.Printf("[Bridge] SetVideoCodec success: %s\n", codecCap.MimeType)
			}
		} else {
			if err := b.switcher.SetAudioCodec(codec.RTPCodecCapability); err != nil {
				// 设置编码器失败，记录但继续转发
			}
		}
	}

	// 启动 RTP 读取循环
	go b.readRTPLoop(track, isVideo, rp.Identity())
}

// onTrackUnsubscribed 轨道取消订阅回调
func (b *LiveKitBridge) onTrackUnsubscribed(
	track *webrtc.TrackRemote,
	pub *lksdk.RemoteTrackPublication,
	rp *lksdk.RemoteParticipant,
) {
	atomic.AddInt32(&b.tracksSubscribed, -1)
}

// readRTPLoop 循环读取 RTP 包并转发到 SourceSwitcher
func (b *LiveKitBridge) readRTPLoop(track *webrtc.TrackRemote, isVideo bool, participantID string) {
	packetCount := uint64(0)

	for {
		// 检查是否已关闭
		b.mu.RLock()
		if b.closed {
			b.mu.RUnlock()
			return
		}
		b.mu.RUnlock()

		// 读取 RTP 包
		pkt, _, err := track.ReadRTP()
		if err != nil {
			// 轨道结束或连接断开
			fmt.Printf("[Bridge] ReadRTP error for track %s: %v\n", track.ID(), err)
			return
		}

		packetCount++
		if packetCount == 1 {
			fmt.Printf("[Bridge] First RTP packet received for track %s\n", track.ID())
		}

		// 每 100 个包打印一次
		if packetCount%100 == 1 {
		}

		// 序列化 RTP 包
		data, err := pkt.Marshal()
		if err != nil {
			continue
		}

		// 注入到 SourceSwitcher
		if b.switcher != nil {
			b.switcher.InjectSFUPacket(isVideo, data)
		} else if packetCount == 1 {
		}

		// 更新统计
		if isVideo {
			atomic.AddUint64(&b.videoPacketsReceived, 1)
		} else {
			atomic.AddUint64(&b.audioPacketsReceived, 1)
		}
	}
}

// handleDisconnected 处理断开连接
func (b *LiveKitBridge) handleDisconnected() {
	b.mu.Lock()
	b.state = LiveKitBridgeStateDisconnected
	b.room = nil
	b.mu.Unlock()

	b.emitStateChanged(LiveKitBridgeStateDisconnected)
}

// Disconnect 断开连接
func (b *LiveKitBridge) Disconnect() {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return
	}
	b.closed = true // 先标记关闭，让 readRTPLoop 尽快退出
	room := b.room
	b.room = nil
	b.state = LiveKitBridgeStateDisconnected
	b.mu.Unlock()

	if room != nil {
		// 异步断开，避免阻塞调用线程
		go room.Disconnect()
	}

	b.emitStateChanged(LiveKitBridgeStateDisconnected)
}

// Close 关闭桥接器
func (b *LiveKitBridge) Close() {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return
	}
	b.closed = true
	room := b.room
	b.room = nil
	b.mu.Unlock()

	if room != nil {
		room.Disconnect()
	}
}

// RequestKeyframe 请求关键帧
// 通过重新设置视频质量来触发 SFU 发送关键帧
// 当新订阅者加入时调用，确保新订阅者能立即看到画面
func (b *LiveKitBridge) RequestKeyframe() {
	b.mu.RLock()
	room := b.room
	b.mu.RUnlock()

	if room == nil {
		return
	}

	// 遍历所有远程参与者的视频轨道，重新请求 HIGH 质量
	// 这会触发 SFU 发送新的关键帧
	for _, p := range room.GetRemoteParticipants() {
		for _, pub := range p.TrackPublications() {
			if remotePub, ok := pub.(*lksdk.RemoteTrackPublication); ok {
				if remotePub.Kind() == lksdk.TrackKindVideo {
					// 重新请求 HIGH 质量会触发 SFU 发送关键帧
					remotePub.SetVideoQuality(livekit.VideoQuality_HIGH)
					fmt.Printf("[Bridge] Keyframe requested for track %s\n", remotePub.SID())
				}
			}
		}
	}
}

// GetState 获取当前状态
func (b *LiveKitBridge) GetState() LiveKitBridgeState {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.state
}

// IsConnected 是否已连接
func (b *LiveKitBridge) IsConnected() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.state == LiveKitBridgeStateConnected && b.room != nil
}

// LiveKitBridgeStatus 桥接器状态信息
type LiveKitBridgeStatus struct {
	RoomID               string `json:"room_id"`
	State                string `json:"state"`
	TracksSubscribed     int32  `json:"tracks_subscribed"`
	VideoPacketsReceived uint64 `json:"video_packets_received"`
	AudioPacketsReceived uint64 `json:"audio_packets_received"`
}

// GetStatus 获取状态信息
func (b *LiveKitBridge) GetStatus() LiveKitBridgeStatus {
	b.mu.RLock()
	defer b.mu.RUnlock()

	return LiveKitBridgeStatus{
		RoomID:               b.roomID,
		State:                b.state.String(),
		TracksSubscribed:     atomic.LoadInt32(&b.tracksSubscribed),
		VideoPacketsReceived: atomic.LoadUint64(&b.videoPacketsReceived),
		AudioPacketsReceived: atomic.LoadUint64(&b.audioPacketsReceived),
	}
}

// 回调触发函数
func (b *LiveKitBridge) emitStateChanged(state LiveKitBridgeState) {
	b.mu.RLock()
	fn := b.onStateChanged
	b.mu.RUnlock()

	if fn != nil {
		go fn(b.roomID, state)
	}
}

func (b *LiveKitBridge) emitError(err error) {
	b.mu.RLock()
	fn := b.onError
	b.mu.RUnlock()

	if fn != nil {
		go fn(b.roomID, err)
	}
}

// ========================================
// 全局桥接器管理
// ========================================

var (
	bridgesMu sync.RWMutex
	bridges   = make(map[string]*LiveKitBridge)
)

// GetBridge 获取桥接器
func GetBridge(roomID string) *LiveKitBridge {
	bridgesMu.RLock()
	defer bridgesMu.RUnlock()
	return bridges[roomID]
}

// CreateBridge 创建桥接器
func CreateBridge(roomID string, switcher *SourceSwitcher) *LiveKitBridge {
	bridgesMu.Lock()
	defer bridgesMu.Unlock()

	// 如果已存在，先关闭
	if existing, ok := bridges[roomID]; ok {
		existing.Close()
	}

	bridge := NewLiveKitBridge(roomID, switcher)
	bridges[roomID] = bridge
	return bridge
}

// DestroyBridge 销毁桥接器
func DestroyBridge(roomID string) {
	bridgesMu.Lock()
	bridge, ok := bridges[roomID]
	if ok {
		delete(bridges, roomID)
	}
	bridgesMu.Unlock()

	if bridge != nil {
		bridge.Close()
	}
}

// 防止 time 包未使用警告
var _ = time.Now
