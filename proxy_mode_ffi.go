/*
 * @Author: Marlon.M
 * @Email: maiguangyang@163.com
 * @Date: 2025-12-24
 *
 * Proxy Mode FFI Exports
 * 代理模式相关的 C 导出函数
 * 支持 SourceSwitcher 和增强选举
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
	"unsafe"

	"github.com/maiguangyang/relay_core/pkg/election"
	"github.com/maiguangyang/relay_core/pkg/sfu"
	"github.com/maiguangyang/relay_core/pkg/utils"
)

// SourceSwitcher 实例管理
var (
	sourceSwitchers sync.Map // roomID -> *sfu.SourceSwitcher
)

// registerSourceSwitcher 注册 SourceSwitcher
func registerSourceSwitcher(roomID string, ss *sfu.SourceSwitcher) {
	sourceSwitchers.Store(roomID, ss)
}

// getSourceSwitcher 获取 SourceSwitcher
func getSourceSwitcher(roomID string) *sfu.SourceSwitcher {
	if v, ok := sourceSwitchers.Load(roomID); ok {
		return v.(*sfu.SourceSwitcher)
	}
	return nil
}

// unregisterSourceSwitcher 注销 SourceSwitcher
func unregisterSourceSwitcher(roomID string) {
	if v, ok := sourceSwitchers.Load(roomID); ok {
		v.(*sfu.SourceSwitcher).Close()
		sourceSwitchers.Delete(roomID)
	}
}

// ==========================================
// SourceSwitcher (代理模式核心)
// ==========================================

// SourceSwitcherCreate 创建源切换器
//
//export SourceSwitcherCreate
func SourceSwitcherCreate(roomID *C.char) C.int {
	goRoomID := C.GoString(roomID)

	ss, err := sfu.NewSourceSwitcher(goRoomID)
	if err != nil {
		utils.Error("Failed to create SourceSwitcher for room %s: %v", goRoomID, err)
		return C.int(-1)
	}

	// 设置源切换回调
	ss.SetOnSourceChanged(func(rID string, sourceType sfu.SourceType, sharerID string) {
		data, _ := json.Marshal(map[string]interface{}{
			"room_id":     rID,
			"source_type": int(sourceType),
			"source_name": sourceType.String(),
			"sharer_id":   sharerID,
		})
		emitEvent(EventTypeProxyChange, rID, sharerID, string(data))
	})

	registerSourceSwitcher(goRoomID, ss)
	utils.Info("SourceSwitcher created for room: %s", goRoomID)
	return C.int(0)
}

// SourceSwitcherDestroy 销毁源切换器
//
//export SourceSwitcherDestroy
func SourceSwitcherDestroy(roomID *C.char) C.int {
	goRoomID := C.GoString(roomID)
	unregisterSourceSwitcher(goRoomID)
	utils.Info("SourceSwitcher destroyed for room: %s", goRoomID)
	return C.int(0)
}

// SourceSwitcherInjectSFU 注入来自 SFU 的 RTP 包
// isVideo: 1 = video, 0 = audio
// data: RTP 原始数据
// dataLen: 数据长度
//
//export SourceSwitcherInjectSFU
func SourceSwitcherInjectSFU(roomID *C.char, isVideo C.int, data unsafe.Pointer, dataLen C.int) C.int {
	goRoomID := C.GoString(roomID)
	ss := getSourceSwitcher(goRoomID)
	if ss == nil {
		return C.int(-1)
	}

	// 转换 C 数据为 Go slice
	goData := C.GoBytes(data, dataLen)

	if err := ss.InjectSFUPacket(isVideo != 0, goData); err != nil {
		return C.int(-1)
	}
	return C.int(0)
}

// SourceSwitcherInjectLocal 注入来自本地分享者的 RTP 包
//
//export SourceSwitcherInjectLocal
func SourceSwitcherInjectLocal(roomID *C.char, isVideo C.int, data unsafe.Pointer, dataLen C.int) C.int {
	goRoomID := C.GoString(roomID)
	ss := getSourceSwitcher(goRoomID)
	if ss == nil {
		return C.int(-1)
	}

	goData := C.GoBytes(data, dataLen)

	if err := ss.InjectLocalPacket(isVideo != 0, goData); err != nil {
		return C.int(-1)
	}
	return C.int(0)
}

// SourceSwitcherStartLocalShare 开始本地分享
//
//export SourceSwitcherStartLocalShare
func SourceSwitcherStartLocalShare(roomID *C.char, sharerID *C.char) C.int {
	goRoomID := C.GoString(roomID)
	goSharerID := C.GoString(sharerID)

	ss := getSourceSwitcher(goRoomID)
	if ss == nil {
		return C.int(-1)
	}

	ss.StartLocalShare(goSharerID)
	utils.Info("Local share started: room=%s, sharer=%s", goRoomID, goSharerID)
	return C.int(0)
}

// SourceSwitcherStopLocalShare 停止本地分享
//
//export SourceSwitcherStopLocalShare
func SourceSwitcherStopLocalShare(roomID *C.char) C.int {
	goRoomID := C.GoString(roomID)

	ss := getSourceSwitcher(goRoomID)
	if ss == nil {
		return C.int(-1)
	}

	ss.StopLocalShare()
	utils.Info("Local share stopped: room=%s", goRoomID)
	return C.int(0)
}

// SourceSwitcherGetStatus 获取源切换器状态
//
//export SourceSwitcherGetStatus
func SourceSwitcherGetStatus(roomID *C.char) *C.char {
	goRoomID := C.GoString(roomID)

	ss := getSourceSwitcher(goRoomID)
	if ss == nil {
		return nil
	}

	status := ss.GetStatus()
	data, _ := json.Marshal(status)
	return C.CString(string(data))
}

// SourceSwitcherIsLocalSharing 检查是否正在本地分享
//
//export SourceSwitcherIsLocalSharing
func SourceSwitcherIsLocalSharing(roomID *C.char) C.int {
	goRoomID := C.GoString(roomID)

	ss := getSourceSwitcher(goRoomID)
	if ss == nil {
		return C.int(-1)
	}

	if ss.IsLocalSharing() {
		return C.int(1)
	}
	return C.int(0)
}

// ==========================================
// 增强选举 - 设备信息更新
// ==========================================

// ElectionUpdateDeviceInfo 更新设备信息
// deviceType: 0=Unknown, 1=PC, 2=Pad, 3=Mobile, 4=TV
// connectionType: 0=Unknown, 1=Ethernet, 2=WiFi, 3=Cellular
// powerState: 0=Unknown, 1=PluggedIn, 2=Battery, 3=LowBattery
//
//export ElectionUpdateDeviceInfo
func ElectionUpdateDeviceInfo(relayID C.int64_t, roomID *C.char, peerID *C.char,
	deviceType C.int, connectionType C.int, powerState C.int) C.int {
	goRoomID := C.GoString(roomID)
	goPeerID := C.GoString(peerID)

	elector := getElector(goRoomID)
	if elector == nil {
		return C.int(-1)
	}

	elector.UpdateDeviceInfo(
		goPeerID,
		election.DeviceType(deviceType),
		election.ConnectionType(connectionType),
		election.PowerState(powerState),
	)

	utils.Debug("Device info updated: peer=%s, device=%d, conn=%d, power=%d",
		goPeerID, deviceType, connectionType, powerState)
	return C.int(0)
}

// ElectionUpdateNetworkMetrics 更新网络指标
//
//export ElectionUpdateNetworkMetrics
func ElectionUpdateNetworkMetrics(relayID C.int64_t, roomID *C.char, peerID *C.char,
	bandwidth C.int64_t, latency C.int64_t, packetLoss C.double) C.int {
	goRoomID := C.GoString(roomID)
	goPeerID := C.GoString(peerID)

	elector := getElector(goRoomID)
	if elector == nil {
		return C.int(-1)
	}

	elector.UpdateNetworkMetrics(goPeerID, int64(bandwidth), int64(latency), float64(packetLoss))

	utils.Debug("Network metrics updated: peer=%s, bw=%d, lat=%d, loss=%.2f",
		goPeerID, bandwidth, latency, packetLoss)
	return C.int(0)
}

// ElectionGetCandidates 获取所有候选者列表
//
//export ElectionGetCandidates
func ElectionGetCandidates(relayID C.int64_t, roomID *C.char) *C.char {
	goRoomID := C.GoString(roomID)

	elector := getElector(goRoomID)
	if elector == nil {
		return C.CString("[]")
	}

	candidates := elector.GetCandidates()

	// 转换为 JSON 友好的格式
	result := make([]map[string]interface{}, len(candidates))
	for i, c := range candidates {
		result[i] = map[string]interface{}{
			"peer_id":         c.PeerID,
			"score":           c.Score,
			"device_type":     c.DeviceType.String(),
			"connection_type": c.ConnectionType.String(),
			"power_state":     c.PowerState.String(),
			"bandwidth":       c.Bandwidth,
			"latency":         c.Latency,
			"packet_loss":     c.PacketLoss,
			"is_proxy":        c.IsProxy,
			"device_name":     c.DeviceName,
		}
	}

	data, _ := json.Marshal(result)
	return C.CString(string(data))
}

// ==========================================
// 便捷组合函数
// ==========================================

// ProxyModeInit 初始化代理模式（创建 SourceSwitcher + 启用选举）
//
//export ProxyModeInit
func ProxyModeInit(relayID C.int64_t, roomID *C.char) C.int {
	goRoomID := C.GoString(roomID)

	// 1. 创建 SourceSwitcher
	if SourceSwitcherCreate(roomID) != 0 {
		return C.int(-1)
	}

	// 2. 启用选举
	if ElectionEnable(relayID, roomID) != 0 {
		SourceSwitcherDestroy(roomID)
		return C.int(-1)
	}

	utils.Info("Proxy mode initialized for room: %s", goRoomID)
	return C.int(0)
}

// ProxyModeCleanup 清理代理模式
//
//export ProxyModeCleanup
func ProxyModeCleanup(relayID C.int64_t, roomID *C.char) C.int {
	goRoomID := C.GoString(roomID)

	// 1. 禁用选举
	ElectionDisable(relayID, roomID)

	// 2. 销毁 SourceSwitcher
	SourceSwitcherDestroy(roomID)

	utils.Info("Proxy mode cleaned up for room: %s", goRoomID)
	return C.int(0)
}

// ProxyModeGetStatus 获取代理模式综合状态
//
//export ProxyModeGetStatus
func ProxyModeGetStatus(relayID C.int64_t, roomID *C.char) *C.char {
	goRoomID := C.GoString(roomID)

	status := map[string]interface{}{
		"room_id": goRoomID,
	}

	// SourceSwitcher 状态
	ss := getSourceSwitcher(goRoomID)
	if ss != nil {
		status["source_switcher"] = ss.GetStatus()
	}

	// 选举状态
	elector := getElector(goRoomID)
	if elector != nil {
		status["election"] = map[string]interface{}{
			"current_proxy":   elector.GetCurrentProxy(),
			"candidate_count": elector.GetCandidateCount(),
		}
	}

	data, _ := json.Marshal(status)
	return C.CString(string(data))
}
