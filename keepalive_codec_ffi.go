/*
 * @Author: Marlon.M
 * @Email: maiguangyang@163.com
 * @Date: 2025-12-24
 *
 * P1 Features FFI Exports
 * 编解码器协商和心跳保活的 C 导出函数
 */
package main

/*
#include <stdlib.h>
#include <stdint.h>

// Ping 回调函数类型
typedef void (*PingCallback)(const char* peerID);

// 存储 ping 回调
static PingCallback pingCallback = NULL;

static void setPingCallback(PingCallback cb) {
    pingCallback = cb;
}

static void callPingCallback(const char* peerID) {
    if (pingCallback != NULL) {
        pingCallback(peerID);
    }
}
*/
import "C"

import (
	"encoding/json"
	"sync"
	"time"
	"unsafe"

	"github.com/maiguangyang/relay_core/pkg/sfu"
	"github.com/maiguangyang/relay_core/pkg/utils"
)

// KeepaliveManager 实例管理
var (
	keepaliveManagers sync.Map // roomID -> *sfu.KeepaliveManager
)

// 事件类型扩展
const (
	EventTypePeerOnline  = 20 // Peer 上线
	EventTypePeerSlow    = 21 // Peer 响应缓慢
	EventTypePeerOffline = 22 // Peer 离线
	EventTypePing        = 23 // 需要发送 Ping
)

// registerKeepaliveManager 注册 KeepaliveManager
func registerKeepaliveManager(roomID string, km *sfu.KeepaliveManager) {
	keepaliveManagers.Store(roomID, km)
}

// getKeepaliveManager 获取 KeepaliveManager
func getKeepaliveManager(roomID string) *sfu.KeepaliveManager {
	if v, ok := keepaliveManagers.Load(roomID); ok {
		return v.(*sfu.KeepaliveManager)
	}
	return nil
}

// unregisterKeepaliveManager 注销 KeepaliveManager
func unregisterKeepaliveManager(roomID string) {
	if v, ok := keepaliveManagers.Load(roomID); ok {
		v.(*sfu.KeepaliveManager).Stop()
		keepaliveManagers.Delete(roomID)
	}
}

// ==========================================
// Keepalive 心跳管理
// ==========================================

// KeepaliveCreate 创建心跳管理器
// intervalMs: 心跳间隔（毫秒）
// timeoutMs: 超时时间（毫秒）
//
//export KeepaliveCreate
func KeepaliveCreate(roomID *C.char, intervalMs C.int, timeoutMs C.int) C.int {
	goRoomID := C.GoString(roomID)

	config := sfu.DefaultKeepaliveConfig()
	if intervalMs > 0 {
		config.Interval = time.Duration(intervalMs) * time.Millisecond
	}
	if timeoutMs > 0 {
		config.Timeout = time.Duration(timeoutMs) * time.Millisecond
	}

	km := sfu.NewKeepaliveManager(config)

	// 设置回调
	km.SetOnPeerOnline(func(peerID string) {
		emitEvent(EventTypePeerOnline, goRoomID, peerID, "")
	})

	km.SetOnPeerSlow(func(peerID string, rtt time.Duration) {
		data, _ := json.Marshal(map[string]interface{}{
			"rtt_ms": rtt.Milliseconds(),
		})
		emitEvent(EventTypePeerSlow, goRoomID, peerID, string(data))
	})

	km.SetOnPeerOffline(func(peerID string) {
		emitEvent(EventTypePeerOffline, goRoomID, peerID, "")
	})

	km.SetOnPing(func(peerID string) {
		// 通过事件通知 Dart 层发送 ping
		emitEvent(EventTypePing, goRoomID, peerID, "")

		// 同时调用 C 回调（如果设置了）
		cPeerID := C.CString(peerID)
		defer C.free(unsafe.Pointer(cPeerID))
		C.callPingCallback(cPeerID)
	})

	registerKeepaliveManager(goRoomID, km)
	utils.Info("KeepaliveManager created for room: %s", goRoomID)
	return C.int(0)
}

// KeepaliveDestroy 销毁心跳管理器
//
//export KeepaliveDestroy
func KeepaliveDestroy(roomID *C.char) C.int {
	goRoomID := C.GoString(roomID)
	unregisterKeepaliveManager(goRoomID)
	utils.Info("KeepaliveManager destroyed for room: %s", goRoomID)
	return C.int(0)
}

// KeepaliveStart 启动心跳检测
//
//export KeepaliveStart
func KeepaliveStart(roomID *C.char) C.int {
	goRoomID := C.GoString(roomID)

	km := getKeepaliveManager(goRoomID)
	if km == nil {
		return C.int(-1)
	}

	km.Start()
	utils.Info("Keepalive started for room: %s", goRoomID)
	return C.int(0)
}

// KeepaliveStop 停止心跳检测
//
//export KeepaliveStop
func KeepaliveStop(roomID *C.char) C.int {
	goRoomID := C.GoString(roomID)

	km := getKeepaliveManager(goRoomID)
	if km == nil {
		return C.int(-1)
	}

	km.Stop()
	utils.Info("Keepalive stopped for room: %s", goRoomID)
	return C.int(0)
}

// KeepaliveAddPeer 添加需要监控的 Peer
//
//export KeepaliveAddPeer
func KeepaliveAddPeer(roomID *C.char, peerID *C.char) C.int {
	goRoomID := C.GoString(roomID)
	goPeerID := C.GoString(peerID)

	km := getKeepaliveManager(goRoomID)
	if km == nil {
		return C.int(-1)
	}

	km.AddPeer(goPeerID)
	return C.int(0)
}

// KeepaliveRemovePeer 移除 Peer
//
//export KeepaliveRemovePeer
func KeepaliveRemovePeer(roomID *C.char, peerID *C.char) C.int {
	goRoomID := C.GoString(roomID)
	goPeerID := C.GoString(peerID)

	km := getKeepaliveManager(goRoomID)
	if km == nil {
		return C.int(-1)
	}

	km.RemovePeer(goPeerID)
	return C.int(0)
}

// KeepaliveHandlePong 处理收到的 Pong
// 当 Dart 层收到对方的 pong 响应时调用此函数
//
//export KeepaliveHandlePong
func KeepaliveHandlePong(roomID *C.char, peerID *C.char) C.int {
	goRoomID := C.GoString(roomID)
	goPeerID := C.GoString(peerID)

	km := getKeepaliveManager(goRoomID)
	if km == nil {
		return C.int(-1)
	}

	km.HandlePong(goPeerID)
	return C.int(0)
}

// KeepaliveGetPeerStatus 获取 Peer 状态
// 返回: 0=Unknown, 1=Online, 2=Slow, 3=Offline
//
//export KeepaliveGetPeerStatus
func KeepaliveGetPeerStatus(roomID *C.char, peerID *C.char) C.int {
	goRoomID := C.GoString(roomID)
	goPeerID := C.GoString(peerID)

	km := getKeepaliveManager(goRoomID)
	if km == nil {
		return C.int(0)
	}

	return C.int(km.GetPeerStatus(goPeerID))
}

// KeepaliveGetPeerRTT 获取 Peer 的 RTT（毫秒）
//
//export KeepaliveGetPeerRTT
func KeepaliveGetPeerRTT(roomID *C.char, peerID *C.char) C.int64_t {
	goRoomID := C.GoString(roomID)
	goPeerID := C.GoString(peerID)

	km := getKeepaliveManager(goRoomID)
	if km == nil {
		return C.int64_t(0)
	}

	return C.int64_t(km.GetPeerRTT(goPeerID).Milliseconds())
}

// KeepaliveGetPeerInfo 获取 Peer 心跳详细信息
//
//export KeepaliveGetPeerInfo
func KeepaliveGetPeerInfo(roomID *C.char, peerID *C.char) *C.char {
	goRoomID := C.GoString(roomID)
	goPeerID := C.GoString(peerID)

	km := getKeepaliveManager(goRoomID)
	if km == nil {
		return nil
	}

	info := km.GetPeerInfo(goPeerID)
	if info == nil {
		return nil
	}

	data, _ := json.Marshal(info)
	return C.CString(string(data))
}

// KeepaliveGetAllPeerInfo 获取所有 Peer 心跳信息
//
//export KeepaliveGetAllPeerInfo
func KeepaliveGetAllPeerInfo(roomID *C.char) *C.char {
	goRoomID := C.GoString(roomID)

	km := getKeepaliveManager(goRoomID)
	if km == nil {
		return C.CString("[]")
	}

	info := km.GetAllPeerInfo()
	data, _ := json.Marshal(info)
	return C.CString(string(data))
}

// SetPingCallback 设置 Ping 回调
// 当需要发送 ping 时会调用此回调
//
//export SetPingCallback
func SetPingCallback(callback C.PingCallback) {
	C.setPingCallback(callback)
	utils.Info("Ping callback registered")
}

// ==========================================
// Codec 编解码器管理
// ==========================================

// CodecGetSupportedVideo 获取支持的视频编解码器列表
//
//export CodecGetSupportedVideo
func CodecGetSupportedVideo() *C.char {
	creator := sfu.NewDynamicTrackCreator()
	codecs := creator.GetRegistry().GetVideoCodecs()

	result := make([]map[string]interface{}, len(codecs))
	for i, codec := range codecs {
		result[i] = map[string]interface{}{
			"type":       string(codec.Type),
			"mime_type":  codec.MimeType,
			"clock_rate": codec.ClockRate,
		}
	}

	data, _ := json.Marshal(result)
	return C.CString(string(data))
}

// CodecGetSupportedAudio 获取支持的音频编解码器列表
//
//export CodecGetSupportedAudio
func CodecGetSupportedAudio() *C.char {
	creator := sfu.NewDynamicTrackCreator()
	codecs := creator.GetRegistry().GetAudioCodecs()

	result := make([]map[string]interface{}, len(codecs))
	for i, codec := range codecs {
		result[i] = map[string]interface{}{
			"type":       string(codec.Type),
			"mime_type":  codec.MimeType,
			"clock_rate": codec.ClockRate,
			"channels":   codec.Channels,
		}
	}

	data, _ := json.Marshal(result)
	return C.CString(string(data))
}

// CodecParseType 解析 MimeType 获取编解码器类型
//
//export CodecParseType
func CodecParseType(mimeType *C.char) *C.char {
	goMimeType := C.GoString(mimeType)
	codecType := sfu.ParseMimeType(goMimeType)
	return C.CString(string(codecType))
}

// CodecIsVideo 判断是否是视频编解码器
//
//export CodecIsVideo
func CodecIsVideo(codecType *C.char) C.int {
	goCodecType := C.GoString(codecType)
	if sfu.IsVideoCodec(sfu.CodecType(goCodecType)) {
		return C.int(1)
	}
	return C.int(0)
}

// CodecIsAudio 判断是否是音频编解码器
//
//export CodecIsAudio
func CodecIsAudio(codecType *C.char) C.int {
	goCodecType := C.GoString(codecType)
	if sfu.IsAudioCodec(sfu.CodecType(goCodecType)) {
		return C.int(1)
	}
	return C.int(0)
}
