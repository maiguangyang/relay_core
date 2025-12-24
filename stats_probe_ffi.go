/*
 * @Author: Marlon.M
 * @Email: maiguangyang@163.com
 * @Date: 2025-12-24
 *
 * P2/P3 Features FFI Exports
 * 缓冲池、流量统计、网络探测、抖动缓冲的 C 导出函数
 */
package main

/*
#include <stdlib.h>
#include <stdint.h>
*/
import "C"

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/maiguangyang/relay_core/pkg/sfu"
	"github.com/maiguangyang/relay_core/pkg/utils"
)

// 实例管理
var (
	roomStats            sync.Map // roomID -> *sfu.RoomStats
	networkProbeManagers sync.Map // roomID -> *sfu.NetworkProbeManager
	jitterBuffers        sync.Map // key -> *sfu.JitterBuffer
)

// ==========================================
// Buffer Pool - RTP 缓冲池
// ==========================================

// BufferPoolGetStats 获取全局缓冲池统计
//
//export BufferPoolGetStats
func BufferPoolGetStats() *C.char {
	stats := sfu.GetGlobalBufferPoolStats()
	data, _ := json.Marshal(stats)
	return C.CString(string(data))
}

// BufferPoolResetStats 重置缓冲池统计
//
//export BufferPoolResetStats
func BufferPoolResetStats() {
	sfu.ResetGlobalBufferPoolStats()
}

// ==========================================
// Stats - 流量统计
// ==========================================

// StatsCreate 创建房间统计
//
//export StatsCreate
func StatsCreate(roomID *C.char) C.int {
	goRoomID := C.GoString(roomID)

	stats := sfu.NewRoomStats(goRoomID)
	roomStats.Store(goRoomID, stats)

	utils.Info("RoomStats created for: %s", goRoomID)
	return C.int(0)
}

// StatsDestroy 销毁房间统计
//
//export StatsDestroy
func StatsDestroy(roomID *C.char) C.int {
	goRoomID := C.GoString(roomID)
	roomStats.Delete(goRoomID)
	return C.int(0)
}

// StatsAddBytesIn 添加接收字节数
//
//export StatsAddBytesIn
func StatsAddBytesIn(roomID *C.char, peerID *C.char, bytes C.uint64_t) C.int {
	goRoomID := C.GoString(roomID)
	goPeerID := C.GoString(peerID)

	v, ok := roomStats.Load(goRoomID)
	if !ok {
		return C.int(-1)
	}
	stats := v.(*sfu.RoomStats)

	// 更新总体统计
	stats.GetTraffic().AddBytesIn(uint64(bytes))
	stats.GetTraffic().AddPacketIn()

	// 更新 Peer 统计
	if goPeerID != "" {
		peerStats := stats.GetOrCreatePeerStats(goPeerID)
		peerStats.AddBytesIn(uint64(bytes))
		peerStats.AddPacketIn()
	}

	return C.int(0)
}

// StatsAddBytesOut 添加发送字节数
//
//export StatsAddBytesOut
func StatsAddBytesOut(roomID *C.char, peerID *C.char, bytes C.uint64_t) C.int {
	goRoomID := C.GoString(roomID)
	goPeerID := C.GoString(peerID)

	v, ok := roomStats.Load(goRoomID)
	if !ok {
		return C.int(-1)
	}
	stats := v.(*sfu.RoomStats)

	stats.GetTraffic().AddBytesOut(uint64(bytes))
	stats.GetTraffic().AddPacketOut()

	if goPeerID != "" {
		peerStats := stats.GetOrCreatePeerStats(goPeerID)
		peerStats.AddBytesOut(uint64(bytes))
		peerStats.AddPacketOut()
	}

	return C.int(0)
}

// StatsAddPacketLost 添加丢包数
//
//export StatsAddPacketLost
func StatsAddPacketLost(roomID *C.char) C.int {
	goRoomID := C.GoString(roomID)

	v, ok := roomStats.Load(goRoomID)
	if !ok {
		return C.int(-1)
	}
	stats := v.(*sfu.RoomStats)
	stats.GetTraffic().AddPacketLost()

	return C.int(0)
}

// StatsCalculateBitrate 计算码率（每秒调用一次）
//
//export StatsCalculateBitrate
func StatsCalculateBitrate(roomID *C.char) C.int {
	goRoomID := C.GoString(roomID)

	v, ok := roomStats.Load(goRoomID)
	if !ok {
		return C.int(-1)
	}
	stats := v.(*sfu.RoomStats)
	stats.CalculateAllBitrates()

	return C.int(0)
}

// StatsGetSnapshot 获取统计快照
//
//export StatsGetSnapshot
func StatsGetSnapshot(roomID *C.char) *C.char {
	goRoomID := C.GoString(roomID)

	v, ok := roomStats.Load(goRoomID)
	if !ok {
		return nil
	}
	stats := v.(*sfu.RoomStats)
	snapshot := stats.Snapshot()

	return C.CString(snapshot.ToJSON())
}

// StatsGetTraffic 获取总体流量统计
//
//export StatsGetTraffic
func StatsGetTraffic(roomID *C.char) *C.char {
	goRoomID := C.GoString(roomID)

	v, ok := roomStats.Load(goRoomID)
	if !ok {
		return nil
	}
	stats := v.(*sfu.RoomStats)
	snapshot := stats.GetTraffic().Snapshot()

	return C.CString(snapshot.ToJSON())
}

// StatsGetBitrateIn 获取入站码率 (bps)
//
//export StatsGetBitrateIn
func StatsGetBitrateIn(roomID *C.char) C.double {
	goRoomID := C.GoString(roomID)

	v, ok := roomStats.Load(goRoomID)
	if !ok {
		return C.double(0)
	}
	stats := v.(*sfu.RoomStats)
	snapshot := stats.GetTraffic().Snapshot()

	return C.double(snapshot.BitrateIn)
}

// StatsGetBitrateOut 获取出站码率 (bps)
//
//export StatsGetBitrateOut
func StatsGetBitrateOut(roomID *C.char) C.double {
	goRoomID := C.GoString(roomID)

	v, ok := roomStats.Load(goRoomID)
	if !ok {
		return C.double(0)
	}
	stats := v.(*sfu.RoomStats)
	snapshot := stats.GetTraffic().Snapshot()

	return C.double(snapshot.BitrateOut)
}

// StatsGetLossRate 获取丢包率
//
//export StatsGetLossRate
func StatsGetLossRate(roomID *C.char) C.double {
	goRoomID := C.GoString(roomID)

	v, ok := roomStats.Load(goRoomID)
	if !ok {
		return C.double(0)
	}
	stats := v.(*sfu.RoomStats)

	return C.double(stats.GetTraffic().GetLossRate())
}

// ==========================================
// Network Probe - 网络质量探测
// ==========================================

// NetworkProbeCreate 创建网络探测管理器
//
//export NetworkProbeCreate
func NetworkProbeCreate(roomID *C.char) C.int {
	goRoomID := C.GoString(roomID)

	manager := sfu.NewNetworkProbeManager()
	networkProbeManagers.Store(goRoomID, manager)

	utils.Info("NetworkProbeManager created for: %s", goRoomID)
	return C.int(0)
}

// NetworkProbeDestroy 销毁网络探测管理器
//
//export NetworkProbeDestroy
func NetworkProbeDestroy(roomID *C.char) C.int {
	goRoomID := C.GoString(roomID)

	v, ok := networkProbeManagers.Load(goRoomID)
	if ok {
		manager := v.(*sfu.NetworkProbeManager)
		manager.StopAll()
		networkProbeManagers.Delete(goRoomID)
	}

	return C.int(0)
}

// NetworkProbeGetMetrics 获取指定 Peer 的网络指标
//
//export NetworkProbeGetMetrics
func NetworkProbeGetMetrics(roomID *C.char, peerID *C.char) *C.char {
	goRoomID := C.GoString(roomID)
	goPeerID := C.GoString(peerID)

	v, ok := networkProbeManagers.Load(goRoomID)
	if !ok {
		return nil
	}
	manager := v.(*sfu.NetworkProbeManager)

	metrics := manager.GetMetrics(goPeerID)
	if metrics == nil {
		return nil
	}

	data, _ := json.Marshal(map[string]interface{}{
		"peer_id":       goPeerID,
		"rtt_ms":        metrics.RTT.Milliseconds(),
		"jitter_ms":     metrics.Jitter.Milliseconds(),
		"packet_loss":   metrics.PacketLoss,
		"available_bw":  metrics.AvailableBandwidth,
		"quality_score": metrics.QualityScore,
		"timestamp":     metrics.Timestamp.Unix(),
	})
	return C.CString(string(data))
}

// NetworkProbeGetAllMetrics 获取所有 Peer 的网络指标
//
//export NetworkProbeGetAllMetrics
func NetworkProbeGetAllMetrics(roomID *C.char) *C.char {
	goRoomID := C.GoString(roomID)

	v, ok := networkProbeManagers.Load(goRoomID)
	if !ok {
		return C.CString("{}")
	}
	manager := v.(*sfu.NetworkProbeManager)

	allMetrics := manager.GetAllMetrics()
	result := make(map[string]interface{}, len(allMetrics))

	for peerID, metrics := range allMetrics {
		result[peerID] = map[string]interface{}{
			"rtt_ms":        metrics.RTT.Milliseconds(),
			"jitter_ms":     metrics.Jitter.Milliseconds(),
			"packet_loss":   metrics.PacketLoss,
			"available_bw":  metrics.AvailableBandwidth,
			"quality_score": metrics.QualityScore,
		}
	}

	data, _ := json.Marshal(result)
	return C.CString(string(data))
}

// ==========================================
// Jitter Buffer - 抖动缓冲
// ==========================================

// JitterBufferCreate 创建抖动缓冲
// enabled: 是否启用
// targetDelayMs: 目标延迟（毫秒）
//
//export JitterBufferCreate
func JitterBufferCreate(key *C.char, enabled C.int, targetDelayMs C.int) C.int {
	goKey := C.GoString(key)

	config := sfu.DefaultJitterBufferConfig()
	config.Enabled = enabled != 0
	if targetDelayMs > 0 {
		config.TargetDelay = time.Duration(targetDelayMs) * time.Millisecond
	}

	jb := sfu.NewJitterBuffer(config)
	if config.Enabled {
		jb.Start()
	}

	jitterBuffers.Store(goKey, jb)
	utils.Info("JitterBuffer created: %s, enabled=%v, delay=%dms", goKey, config.Enabled, targetDelayMs)
	return C.int(0)
}

// JitterBufferDestroy 销毁抖动缓冲
//
//export JitterBufferDestroy
func JitterBufferDestroy(key *C.char) C.int {
	goKey := C.GoString(key)

	v, ok := jitterBuffers.Load(goKey)
	if ok {
		jb := v.(*sfu.JitterBuffer)
		jb.Close()
		jitterBuffers.Delete(goKey)
	}

	return C.int(0)
}

// JitterBufferEnable 启用/禁用抖动缓冲
//
//export JitterBufferEnable
func JitterBufferEnable(key *C.char, enabled C.int) C.int {
	goKey := C.GoString(key)

	v, ok := jitterBuffers.Load(goKey)
	if !ok {
		return C.int(-1)
	}
	jb := v.(*sfu.JitterBuffer)
	jb.Enable(enabled != 0)

	if enabled != 0 {
		jb.Start()
	}

	return C.int(0)
}

// JitterBufferSetDelay 设置目标延迟
//
//export JitterBufferSetDelay
func JitterBufferSetDelay(key *C.char, delayMs C.int) C.int {
	goKey := C.GoString(key)

	v, ok := jitterBuffers.Load(goKey)
	if !ok {
		return C.int(-1)
	}
	jb := v.(*sfu.JitterBuffer)
	jb.SetDelay(time.Duration(delayMs) * time.Millisecond)

	return C.int(0)
}

// JitterBufferFlush 清空缓冲区
//
//export JitterBufferFlush
func JitterBufferFlush(key *C.char) C.int {
	goKey := C.GoString(key)

	v, ok := jitterBuffers.Load(goKey)
	if !ok {
		return C.int(-1)
	}
	jb := v.(*sfu.JitterBuffer)
	jb.Flush()

	return C.int(0)
}

// JitterBufferGetStats 获取抖动缓冲统计
//
//export JitterBufferGetStats
func JitterBufferGetStats(key *C.char) *C.char {
	goKey := C.GoString(key)

	v, ok := jitterBuffers.Load(goKey)
	if !ok {
		return nil
	}
	jb := v.(*sfu.JitterBuffer)
	stats := jb.GetStats()

	data, _ := json.Marshal(stats)
	return C.CString(string(data))
}

// JitterBufferIsEnabled 检查是否启用
//
//export JitterBufferIsEnabled
func JitterBufferIsEnabled(key *C.char) C.int {
	goKey := C.GoString(key)

	v, ok := jitterBuffers.Load(goKey)
	if !ok {
		return C.int(-1)
	}
	jb := v.(*sfu.JitterBuffer)

	if jb.IsEnabled() {
		return C.int(1)
	}
	return C.int(0)
}
