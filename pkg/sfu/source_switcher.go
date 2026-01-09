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

	"github.com/maiguangyang/relay_core/pkg/utils"
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

	// RTP Rewriting 状态 (用于保证输出流的连续性)
	videoSnOffset uint16
	videoTsOffset uint32
	lastVideoSn   uint16 // Output SN
	lastVideoTs   uint32 // Output TS
	videoSynced   bool   // 是否已同步过至少一个包
	videoReset    bool   // 标记是否需要重置同步

	audioSnOffset uint16
	audioTsOffset uint32
	lastAudioSn   uint16
	lastAudioTs   uint32
	audioSynced   bool
	audioReset    bool

	// 统计
	packetsFromSFU   uint64
	packetsFromLocal uint64

	// 下游错误日志节流
	lastWriteErrorTime int64 // UnixNano, atomic

	// 回调
	onSourceChanged func(roomID string, sourceType SourceType, sharerID string)
	onTrackChanged  func(videoTrack, audioTrack *webrtc.TrackLocalStaticRTP)

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

// SetOnTrackChanged 设置 Track 变更回调
func (ss *SourceSwitcher) SetOnTrackChanged(fn func(videoTrack, audioTrack *webrtc.TrackLocalStaticRTP)) {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	ss.onTrackChanged = fn
}

// GetVideoTrack 返回视频 Track 供订阅者使用
func (ss *SourceSwitcher) GetVideoTrack() *webrtc.TrackLocalStaticRTP {
	return ss.videoTrack
}

// GetAudioTrack 返回音频 Track 供订阅者使用
func (ss *SourceSwitcher) GetAudioTrack() *webrtc.TrackLocalStaticRTP {
	return ss.audioTrack
}

// SetVideoCodec 设置视频编解码器（从远端轨道获取完整参数）
// 解决画质模糊问题：使用远端轨道的完整编码参数而不是默认的最小配置
// 优化：只在 codec 真正变化时才创建新 track，避免 SSRC 变化导致黑屏
func (ss *SourceSwitcher) SetVideoCodec(codec webrtc.RTPCodecCapability) error {
	var callback func(*webrtc.TrackLocalStaticRTP, *webrtc.TrackLocalStaticRTP)
	var videoTrack, audioTrack *webrtc.TrackLocalStaticRTP
	var needNewTrack bool

	ss.mu.Lock()
	if ss.closed {
		ss.mu.Unlock()
		return ErrForwarderClosed
	}

	// 始终创建新 track，确保 ReplaceTrack 能正常工作
	// 之前的优化（复用相同 codec 的 track）导致了黑屏问题：
	// 当 ReplaceTrack 替换同一个 track 对象时，接收端不会收到任何通知，
	// 解码器无法知道需要重新初始化。
	//
	// 每次屏幕共享都创建新 track：
	// 1. ReplaceTrack 会替换到新的 track 对象
	// 2. 新 track 有新的内部状态
	// 3. 接收端的解码器能正确处理新的视频流
	// 检查 codec 是否真的变化了
	// 如果当前 track 的 MimeType 和新 codec 相同，则不需要创建新 track
	currentCodec := ss.videoTrack.Codec()
	if currentCodec.MimeType == codec.MimeType {
		// codec 没有变化，不需要创建新 track
		// 但我们需要标记流已重置，以便 writePacket 重新计算 RTP offset，保证 SN/TS 连续
		utils.Info("[Switcher] Video codec unchanged (%s), reusing track but triggering sync reset", codec.MimeType)

		ss.videoReset = true // 标记重置

		callback = ss.onTrackChanged
		videoTrack = ss.videoTrack
		audioTrack = ss.audioTrack
		ss.mu.Unlock()

		// 触发回调，这会导致 ReplaceTrack（替换同一个 track，无操作）并请求关键帧
		if callback != nil {
			callback(videoTrack, audioTrack)
		}
		return nil
	}

	// codec 变化了，需要创建新 track
	needNewTrack = true
	utils.Info("[Switcher] Video codec changed from %s to %s, creating new track",
		currentCodec.MimeType, codec.MimeType)
	newTrack, err := webrtc.NewTrackLocalStaticRTP(
		codec,
		"video-relay",
		"relay-stream",
	)
	if err != nil {
		ss.mu.Unlock()
		return err
	}

	ss.videoTrack = newTrack
	// 新 track 意味着全新的流，重置状态
	ss.videoSynced = false
	ss.videoReset = false
	ss.videoSnOffset = 0
	ss.videoTsOffset = 0

	// 复制回调和 track 引用，以便在锁外调用
	callback = ss.onTrackChanged
	videoTrack = ss.videoTrack
	audioTrack = ss.audioTrack
	ss.mu.Unlock()

	// 在锁外触发回调，避免死锁
	if needNewTrack && callback != nil {
		utils.Info("[Switcher] Triggering OnTrackChanged callback for video codec: %s", codec.MimeType)
		callback(videoTrack, audioTrack)
	}

	return nil
}

// SetAudioCodec 设置音频编解码器（从远端轨道获取完整参数）
func (ss *SourceSwitcher) SetAudioCodec(codec webrtc.RTPCodecCapability) error {
	var callback func(*webrtc.TrackLocalStaticRTP, *webrtc.TrackLocalStaticRTP)
	var videoTrack, audioTrack *webrtc.TrackLocalStaticRTP

	ss.mu.Lock()
	if ss.closed {
		ss.mu.Unlock()
		return ErrForwarderClosed
	}

	// 创建新的音频 Track，使用完整的编码参数
	newTrack, err := webrtc.NewTrackLocalStaticRTP(
		codec,
		"audio-relay",
		"relay-stream",
	)
	if err != nil {
		ss.mu.Unlock()
		return err
	}

	ss.audioTrack = newTrack
	// 复制回调和 track 引用，以便在锁外调用
	callback = ss.onTrackChanged
	videoTrack = ss.videoTrack
	audioTrack = ss.audioTrack
	ss.mu.Unlock()

	// 在锁外触发回调，避免死锁
	if callback != nil {
		utils.Info("[Switcher] Triggering OnTrackChanged callback for audio codec: %s", codec.MimeType)
		callback(videoTrack, audioTrack)
	}

	return nil
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

// IngestLocalRTP 注入来自本地分享者的 RTP 包 (直接使用 *rtp.Packet)
// 当活跃源是 Local 时，数据会被转发给订阅者 (供 Local Loopback P2P 使用)
func (ss *SourceSwitcher) IngestLocalRTP(isVideo bool, packet *rtp.Packet) error {
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

	return ss.processRTPPacket(isVideo, packet, false)
}

// writePacket 写入 RTP 包到对应的 Track (From Bytes)
func (ss *SourceSwitcher) writePacket(isVideo bool, data []byte, fromSFU bool) error {
	// 解析 RTP 包
	packet := &rtp.Packet{}
	if err := packet.Unmarshal(data); err != nil {
		return err
	}

	return ss.processRTPPacket(isVideo, packet, fromSFU)
}

// processRTPPacket 处理并转发 RTP 包
func (ss *SourceSwitcher) processRTPPacket(isVideo bool, packet *rtp.Packet, fromSFU bool) error {
	// 获取当前的 Track 引用（需要加锁，因为 SetVideoCodec 可能正在更新）
	ss.mu.RLock()
	var track *webrtc.TrackLocalStaticRTP
	if isVideo {
		track = ss.videoTrack
	} else {
		track = ss.audioTrack
	}
	ss.mu.RUnlock()

	if track == nil {
		return nil
	}

	// 写入 Track（转发给所有订阅者）
	// RTP Rewriting 核心逻辑
	if isVideo {
		// 处理同步重置
		if ss.videoReset {
			if ss.videoSynced {
				// 如果之前同步过，计算新的 offset 以连接上一次的 output
				// new_output_sn = old_output_sn + 1
				// new_output_sn = input_sn + new_offset
				// => new_offset = old_output_sn + 1 - input_sn
				ss.videoSnOffset = ss.lastVideoSn + 1 - packet.SequenceNumber

				// 时间戳同理，增加一个小 delta (比如 3000 samples @ 90k = 33ms)
				// 这个 delta 不是很关键，只要不回滚即可
				ss.videoTsOffset = ss.lastVideoTs + 3000 - packet.Timestamp

				utils.Info("[Switcher] Video stream reset recovered: sn_offset=%d, ts_offset=%d", ss.videoSnOffset, ss.videoTsOffset)
			} else {
				// 第一次同步，offset 设为 0 (或者随机，这里选 0 简单)
				ss.videoSnOffset = 0
				ss.videoTsOffset = 0
			}
			ss.videoReset = false
			ss.videoSynced = true
		} else if !ss.videoSynced {
			ss.videoSnOffset = 0
			ss.videoTsOffset = 0
			ss.videoSynced = true
		}

		// 应用 Offset
		packet.SequenceNumber += ss.videoSnOffset
		packet.Timestamp += ss.videoTsOffset

		// 更新最后状态
		ss.lastVideoSn = packet.SequenceNumber
		ss.lastVideoTs = packet.Timestamp

	} else {
		// 音频同理 (简化处理，音频通常容忍度高一些，但为了完美也加上)
		if ss.audioReset {
			if ss.audioSynced {
				ss.audioSnOffset = ss.lastAudioSn + 1 - packet.SequenceNumber
				ss.audioTsOffset = ss.lastAudioTs + 960 - packet.Timestamp // 20ms @ 48k
			} else {
				ss.audioSnOffset = 0
				ss.audioTsOffset = 0
			}
			ss.audioReset = false
			ss.audioSynced = true
		} else if !ss.audioSynced {
			ss.audioSnOffset = 0
			ss.audioTsOffset = 0
			ss.audioSynced = true
		}

		packet.SequenceNumber += ss.audioSnOffset
		packet.Timestamp += ss.audioTsOffset

		ss.lastAudioSn = packet.SequenceNumber
		ss.lastAudioTs = packet.Timestamp
	}

	if err := track.WriteRTP(packet); err != nil {
		// 节流错误日志：每秒只打印一次
		now := time.Now().UnixNano()
		last := atomic.LoadInt64(&ss.lastWriteErrorTime)
		if now-last > int64(time.Second) {
			if atomic.CompareAndSwapInt64(&ss.lastWriteErrorTime, last, now) {
				utils.Error("[Switcher] WriteRTP error: %v (isVideo: %v)", err, isVideo)
			}
		}
		return err
	}

	if ss.packetsFromSFU%100 == 0 {
		// fmt.Printf("[Switcher] Wrote packet to track (isVideo: %v, fromSFU: %v)\n", isVideo, fromSFU)
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
