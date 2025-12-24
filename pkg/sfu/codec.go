/*
 * @Author: Marlon.M
 * @Email: maiguangyang@163.com
 * @Date: 2025-12-24
 *
 * Codec - 编解码器协商与管理
 * 支持动态匹配 SFU 发送的编码格式（VP8/VP9/H264/AV1 等）
 */
package sfu

import (
	"strings"
	"sync"

	"github.com/pion/webrtc/v4"
)

// CodecType 编解码器类型
type CodecType string

const (
	CodecTypeVP8  CodecType = "VP8"
	CodecTypeVP9  CodecType = "VP9"
	CodecTypeH264 CodecType = "H264"
	CodecTypeAV1  CodecType = "AV1"
	CodecTypeOpus CodecType = "opus"
	CodecTypeG722 CodecType = "G722"
	CodecTypePCMU CodecType = "PCMU"
	CodecTypePCMA CodecType = "PCMA"
)

// CodecInfo 编解码器信息
type CodecInfo struct {
	Type        CodecType
	MimeType    string
	ClockRate   uint32
	Channels    uint16
	SDPFmtpLine string
	PayloadType webrtc.PayloadType
}

// 预定义编解码器
var (
	// 视频编解码器
	CodecVP8 = CodecInfo{
		Type:      CodecTypeVP8,
		MimeType:  webrtc.MimeTypeVP8,
		ClockRate: 90000,
	}
	CodecVP9 = CodecInfo{
		Type:      CodecTypeVP9,
		MimeType:  webrtc.MimeTypeVP9,
		ClockRate: 90000,
	}
	CodecH264 = CodecInfo{
		Type:        CodecTypeH264,
		MimeType:    webrtc.MimeTypeH264,
		ClockRate:   90000,
		SDPFmtpLine: "level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=42e01f",
	}
	CodecAV1 = CodecInfo{
		Type:      CodecTypeAV1,
		MimeType:  webrtc.MimeTypeAV1,
		ClockRate: 90000,
	}

	// 音频编解码器
	CodecOpus = CodecInfo{
		Type:        CodecTypeOpus,
		MimeType:    webrtc.MimeTypeOpus,
		ClockRate:   48000,
		Channels:    2,
		SDPFmtpLine: "minptime=10;useinbandfec=1",
	}
	CodecG722 = CodecInfo{
		Type:      CodecTypeG722,
		MimeType:  webrtc.MimeTypeG722,
		ClockRate: 8000,
	}
)

// CodecRegistry 编解码器注册表
type CodecRegistry struct {
	mu          sync.RWMutex
	videoCodecs []CodecInfo
	audioCodecs []CodecInfo
}

// NewCodecRegistry 创建编解码器注册表
func NewCodecRegistry() *CodecRegistry {
	return &CodecRegistry{
		videoCodecs: []CodecInfo{CodecVP8, CodecVP9, CodecH264, CodecAV1},
		audioCodecs: []CodecInfo{CodecOpus, CodecG722},
	}
}

// GetVideoCodecs 获取支持的视频编解码器
func (r *CodecRegistry) GetVideoCodecs() []CodecInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]CodecInfo, len(r.videoCodecs))
	copy(result, r.videoCodecs)
	return result
}

// GetAudioCodecs 获取支持的音频编解码器
func (r *CodecRegistry) GetAudioCodecs() []CodecInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]CodecInfo, len(r.audioCodecs))
	copy(result, r.audioCodecs)
	return result
}

// FindVideoCodec 根据 MimeType 查找视频编解码器
func (r *CodecRegistry) FindVideoCodec(mimeType string) *CodecInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	mimeType = strings.ToLower(mimeType)
	for _, codec := range r.videoCodecs {
		if strings.ToLower(codec.MimeType) == mimeType {
			return &codec
		}
	}
	return nil
}

// FindAudioCodec 根据 MimeType 查找音频编解码器
func (r *CodecRegistry) FindAudioCodec(mimeType string) *CodecInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	mimeType = strings.ToLower(mimeType)
	for _, codec := range r.audioCodecs {
		if strings.ToLower(codec.MimeType) == mimeType {
			return &codec
		}
	}
	return nil
}

// SetPreferredVideoCodec 设置首选视频编解码器（放到列表首位）
func (r *CodecRegistry) SetPreferredVideoCodec(codecType CodecType) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for i, codec := range r.videoCodecs {
		if codec.Type == codecType {
			// 移到首位
			r.videoCodecs = append([]CodecInfo{codec}, append(r.videoCodecs[:i], r.videoCodecs[i+1:]...)...)
			return
		}
	}
}

// DynamicTrackCreator 动态 Track 创建器
type DynamicTrackCreator struct {
	registry *CodecRegistry
}

// NewDynamicTrackCreator 创建动态 Track 创建器
func NewDynamicTrackCreator() *DynamicTrackCreator {
	return &DynamicTrackCreator{
		registry: NewCodecRegistry(),
	}
}

// CreateVideoTrack 根据远端 Track 的编码格式创建本地 Track
func (c *DynamicTrackCreator) CreateVideoTrack(remoteTrack *webrtc.TrackRemote) (*webrtc.TrackLocalStaticRTP, error) {
	codec := remoteTrack.Codec()

	return webrtc.NewTrackLocalStaticRTP(
		codec.RTPCodecCapability,
		remoteTrack.ID(),
		remoteTrack.StreamID(),
	)
}

// CreateAudioTrack 根据远端 Track 的编码格式创建本地 Track
func (c *DynamicTrackCreator) CreateAudioTrack(remoteTrack *webrtc.TrackRemote) (*webrtc.TrackLocalStaticRTP, error) {
	codec := remoteTrack.Codec()

	return webrtc.NewTrackLocalStaticRTP(
		codec.RTPCodecCapability,
		remoteTrack.ID(),
		remoteTrack.StreamID(),
	)
}

// CreateVideoTrackWithCodec 使用指定编码创建视频 Track
func (c *DynamicTrackCreator) CreateVideoTrackWithCodec(codecInfo CodecInfo, trackID, streamID string) (*webrtc.TrackLocalStaticRTP, error) {
	capability := webrtc.RTPCodecCapability{
		MimeType:    codecInfo.MimeType,
		ClockRate:   codecInfo.ClockRate,
		SDPFmtpLine: codecInfo.SDPFmtpLine,
	}

	return webrtc.NewTrackLocalStaticRTP(capability, trackID, streamID)
}

// CreateAudioTrackWithCodec 使用指定编码创建音频 Track
func (c *DynamicTrackCreator) CreateAudioTrackWithCodec(codecInfo CodecInfo, trackID, streamID string) (*webrtc.TrackLocalStaticRTP, error) {
	capability := webrtc.RTPCodecCapability{
		MimeType:    codecInfo.MimeType,
		ClockRate:   codecInfo.ClockRate,
		Channels:    codecInfo.Channels,
		SDPFmtpLine: codecInfo.SDPFmtpLine,
	}

	return webrtc.NewTrackLocalStaticRTP(capability, trackID, streamID)
}

// GetRegistry 获取编解码器注册表
func (c *DynamicTrackCreator) GetRegistry() *CodecRegistry {
	return c.registry
}

// ParseMimeType 解析 MimeType 获取编解码器类型
func ParseMimeType(mimeType string) CodecType {
	mimeType = strings.ToLower(mimeType)

	switch {
	case strings.Contains(mimeType, "vp8"):
		return CodecTypeVP8
	case strings.Contains(mimeType, "vp9"):
		return CodecTypeVP9
	case strings.Contains(mimeType, "h264"):
		return CodecTypeH264
	case strings.Contains(mimeType, "av1"):
		return CodecTypeAV1
	case strings.Contains(mimeType, "opus"):
		return CodecTypeOpus
	case strings.Contains(mimeType, "g722"):
		return CodecTypeG722
	case strings.Contains(mimeType, "pcmu"):
		return CodecTypePCMU
	case strings.Contains(mimeType, "pcma"):
		return CodecTypePCMA
	default:
		return CodecType(mimeType)
	}
}

// IsVideoCodec 判断是否是视频编解码器
func IsVideoCodec(codecType CodecType) bool {
	switch codecType {
	case CodecTypeVP8, CodecTypeVP9, CodecTypeH264, CodecTypeAV1:
		return true
	default:
		return false
	}
}

// IsAudioCodec 判断是否是音频编解码器
func IsAudioCodec(codecType CodecType) bool {
	switch codecType {
	case CodecTypeOpus, CodecTypeG722, CodecTypePCMU, CodecTypePCMA:
		return true
	default:
		return false
	}
}
