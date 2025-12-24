/*
 * @Author: Marlon.M
 * @Email: maiguangyang@163.com
 * @Date: 2025-12-24
 *
 * SourceSwitcher - 源切换器
 * 实现"代理模式(Proxy Mode)"的核心组件
 * 支持双输入源（SFU流 / 本地分享流）的无缝切换
 */
package sfu

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4"
)

// SourceType 表示数据源类型
type SourceType int32

const (
	// SourceTypeSFU 表示来自远端 SFU 的流 (Input A)
	SourceTypeSFU SourceType = iota
	// SourceTypeLocal 表示来自本地分享者的流 (Input B)
	SourceTypeLocal
)

func (s SourceType) String() string {
	switch s {
	case SourceTypeSFU:
		return "SFU"
	case SourceTypeLocal:
		return "Local"
	default:
		return "Unknown"
	}
}

// SourceSwitcher 实现双输入源切换
// 像导播台一样，可以在 SFU 流和本地分享流之间切换
// 切换时不断开下游订阅者的连接
type SourceSwitcher struct {
	mu sync.RWMutex

	roomID string

	// 当前活跃的源类型
	activeSource atomic.Int32

	// 音视频 Track 的本地代理
	// 订阅者连接到这些 Track，源切换对他们透明
	videoTrack *webrtc.TrackLocalStaticRTP
	audioTrack *webrtc.TrackLocalStaticRTP

	// 源状态
	sfuActive   bool // SFU 源是否活跃
	localActive bool // 本地源是否活跃

	// 本地分享者信息
	localSharerID string

	// 统计
	packetsFromSFU   uint64
	packetsFromLocal uint64

	// 回调
	onSourceChanged func(roomID string, sourceType SourceType, sharerID string)

	closed bool
}

// NewSourceSwitcher 创建新的源切换器
func NewSourceSwitcher(roomID string) (*SourceSwitcher, error) {
	// 创建视频 Track (使用 VP8 作为默认编码)
	videoTrack, err := webrtc.NewTrackLocalStaticRTP(
		webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeVP8},
		"video-relay",
		"relay-stream",
	)
	if err != nil {
		return nil, err
	}

	// 创建音频 Track (使用 Opus 作为默认编码)
	audioTrack, err := webrtc.NewTrackLocalStaticRTP(
		webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus},
		"audio-relay",
		"relay-stream",
	)
	if err != nil {
		return nil, err
	}

	ss := &SourceSwitcher{
		roomID:     roomID,
		videoTrack: videoTrack,
		audioTrack: audioTrack,
	}
	ss.activeSource.Store(int32(SourceTypeSFU))

	return ss, nil
}

// SetOnSourceChanged 设置源切换回调
func (ss *SourceSwitcher) SetOnSourceChanged(fn func(roomID string, sourceType SourceType, sharerID string)) {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	ss.onSourceChanged = fn
}

// GetVideoTrack 返回视频 Track 供订阅者使用
func (ss *SourceSwitcher) GetVideoTrack() *webrtc.TrackLocalStaticRTP {
	return ss.videoTrack
}

// GetAudioTrack 返回音频 Track 供订阅者使用
func (ss *SourceSwitcher) GetAudioTrack() *webrtc.TrackLocalStaticRTP {
	return ss.audioTrack
}

// GetActiveSource 返回当前活跃的源类型
func (ss *SourceSwitcher) GetActiveSource() SourceType {
	return SourceType(ss.activeSource.Load())
}

// GetLocalSharerID 返回当前本地分享者 ID
func (ss *SourceSwitcher) GetLocalSharerID() string {
	ss.mu.RLock()
	defer ss.mu.RUnlock()
	return ss.localSharerID
}

// InjectSFUPacket 注入来自 SFU 的 RTP 包
// 当活跃源是 SFU 时，数据会被转发给订阅者
func (ss *SourceSwitcher) InjectSFUPacket(isVideo bool, data []byte) error {
	ss.mu.RLock()
	if ss.closed {
		ss.mu.RUnlock()
		return ErrForwarderClosed
	}
	ss.sfuActive = true
	ss.mu.RUnlock()

	// 只有当活跃源是 SFU 时才转发
	if ss.GetActiveSource() != SourceTypeSFU {
		return nil
	}

	return ss.writePacket(isVideo, data, true)
}

// InjectLocalPacket 注入来自本地分享者的 RTP 包
// 当活跃源是 Local 时，数据会被转发给订阅者
func (ss *SourceSwitcher) InjectLocalPacket(isVideo bool, data []byte) error {
	ss.mu.RLock()
	if ss.closed {
		ss.mu.RUnlock()
		return ErrForwarderClosed
	}
	ss.localActive = true
	ss.mu.RUnlock()

	// 只有当活跃源是 Local 时才转发
	if ss.GetActiveSource() != SourceTypeLocal {
		return nil
	}

	return ss.writePacket(isVideo, data, false)
}

// writePacket 写入 RTP 包到对应的 Track
func (ss *SourceSwitcher) writePacket(isVideo bool, data []byte, fromSFU bool) error {
	// 解析 RTP 包
	packet := &rtp.Packet{}
	if err := packet.Unmarshal(data); err != nil {
		return err
	}

	// 选择对应的 Track
	var track *webrtc.TrackLocalStaticRTP
	if isVideo {
		track = ss.videoTrack
	} else {
		track = ss.audioTrack
	}

	if track == nil {
		return nil
	}

	// 写入 Track（转发给所有订阅者）
	if err := track.WriteRTP(packet); err != nil {
		return err
	}

	// 更新统计
	if fromSFU {
		atomic.AddUint64(&ss.packetsFromSFU, 1)
	} else {
		atomic.AddUint64(&ss.packetsFromLocal, 1)
	}

	return nil
}

// StartLocalShare 开始本地分享（切换到 Local 源）
func (ss *SourceSwitcher) StartLocalShare(sharerID string) {
	ss.mu.Lock()
	ss.localSharerID = sharerID
	ss.mu.Unlock()

	// 原子切换源
	ss.activeSource.Store(int32(SourceTypeLocal))

	// 触发回调
	ss.mu.RLock()
	fn := ss.onSourceChanged
	ss.mu.RUnlock()
	if fn != nil {
		fn(ss.roomID, SourceTypeLocal, sharerID)
	}
}

// StopLocalShare 停止本地分享（切换回 SFU 源）
func (ss *SourceSwitcher) StopLocalShare() {
	ss.mu.Lock()
	sharerID := ss.localSharerID
	ss.localSharerID = ""
	ss.localActive = false
	ss.mu.Unlock()

	// 原子切换源
	ss.activeSource.Store(int32(SourceTypeSFU))

	// 触发回调
	ss.mu.RLock()
	fn := ss.onSourceChanged
	ss.mu.RUnlock()
	if fn != nil {
		fn(ss.roomID, SourceTypeSFU, sharerID)
	}
}

// SwitchToSource 手动切换到指定源
func (ss *SourceSwitcher) SwitchToSource(sourceType SourceType) {
	oldSource := ss.GetActiveSource()
	if oldSource == sourceType {
		return
	}

	ss.activeSource.Store(int32(sourceType))

	// 触发回调
	ss.mu.RLock()
	fn := ss.onSourceChanged
	sharerID := ss.localSharerID
	ss.mu.RUnlock()
	if fn != nil {
		fn(ss.roomID, sourceType, sharerID)
	}
}

// IsLocalSharing 返回是否正在本地分享
func (ss *SourceSwitcher) IsLocalSharing() bool {
	return ss.GetActiveSource() == SourceTypeLocal
}

// Stats 返回统计信息
func (ss *SourceSwitcher) Stats() (sfuPackets, localPackets uint64) {
	return atomic.LoadUint64(&ss.packetsFromSFU), atomic.LoadUint64(&ss.packetsFromLocal)
}

// Close 关闭源切换器
func (ss *SourceSwitcher) Close() {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	ss.closed = true
}

// SourceSwitcherStatus 源切换器状态
type SourceSwitcherStatus struct {
	RoomID        string     `json:"room_id"`
	ActiveSource  SourceType `json:"active_source"`
	SourceName    string     `json:"source_name"`
	LocalSharerID string     `json:"local_sharer_id,omitempty"`
	SFUActive     bool       `json:"sfu_active"`
	LocalActive   bool       `json:"local_active"`
	SFUPackets    uint64     `json:"sfu_packets"`
	LocalPackets  uint64     `json:"local_packets"`
}

// GetStatus 获取状态
func (ss *SourceSwitcher) GetStatus() SourceSwitcherStatus {
	ss.mu.RLock()
	defer ss.mu.RUnlock()

	sfuPackets, localPackets := ss.Stats()

	return SourceSwitcherStatus{
		RoomID:        ss.roomID,
		ActiveSource:  ss.GetActiveSource(),
		SourceName:    ss.GetActiveSource().String(),
		LocalSharerID: ss.localSharerID,
		SFUActive:     ss.sfuActive,
		LocalActive:   ss.localActive,
		SFUPackets:    sfuPackets,
		LocalPackets:  localPackets,
	}
}

// HealthCheck 健康检查 - 检测源是否正常产生数据
func (ss *SourceSwitcher) HealthCheck(timeout time.Duration) bool {
	ss.mu.RLock()
	defer ss.mu.RUnlock()

	activeSource := ss.GetActiveSource()
	if activeSource == SourceTypeSFU {
		return ss.sfuActive
	}
	return ss.localActive
}
