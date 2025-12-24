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

// SourceSwitcher, FailoverManager 和 Coordinator 实例管理
var (
	sourceSwitchers  sync.Map // roomID -> *sfu.SourceSwitcher
	failoverManagers sync.Map // roomID -> *sfu.FailoverManager
	coordinators     sync.Map // roomID -> *sfu.ProxyModeCoordinator
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

	// 故障切换状态
	if v, ok := failoverManagers.Load(goRoomID); ok {
		fm := v.(*sfu.FailoverManager)
		relayID, epoch := fm.GetCurrentRelay()
		status["failover"] = map[string]interface{}{
			"state":         fm.GetState().String(),
			"current_relay": relayID,
			"epoch":         epoch,
		}
	}

	data, _ := json.Marshal(status)
	return C.CString(string(data))
}

// ==========================================
// 故障切换 (Auto Failover)
// ==========================================

// FailoverEnable 启用自动故障切换
// localPeerID: 本机 Peer ID
//
//export FailoverEnable
func FailoverEnable(roomID *C.char, localPeerID *C.char) C.int {
	goRoomID := C.GoString(roomID)
	goLocalPeerID := C.GoString(localPeerID)

	// 获取选举器
	elector := getElector(goRoomID)
	if elector == nil {
		utils.Warn("Elector not found for room %s, creating one", goRoomID)
		return C.int(-1)
	}

	// 获取或创建 Keepalive
	var km *sfu.KeepaliveManager
	if v, ok := keepaliveManagers.Load(goRoomID); ok {
		km = v.(*sfu.KeepaliveManager)
	}

	// 创建 FailoverManager
	config := sfu.DefaultFailoverConfig()
	fm := sfu.NewFailoverManager(goRoomID, goLocalPeerID, elector, km, config)

	// 设置回调
	fm.SetCallbacks(
		// Relay 失效
		func(rID, relayID string) {
			data, _ := json.Marshal(map[string]interface{}{
				"relay_id": relayID,
				"reason":   "offline",
			})
			emitEvent(EventTypePeerOffline, rID, relayID, string(data))
		},
		// 新 Relay 选出
		func(rID, newRelayID string, epoch uint64) {
			data, _ := json.Marshal(map[string]interface{}{
				"proxy_id": newRelayID,
				"epoch":    epoch,
			})
			emitEvent(EventTypeProxyChange, rID, newRelayID, string(data))
		},
		// 本机成为 Relay
		func(rID string) {
			data, _ := json.Marshal(map[string]interface{}{
				"action":  "become_relay",
				"peer_id": goLocalPeerID,
			})
			emitEvent(EventTypeProxyChange, rID, goLocalPeerID, string(data))
		},
	)

	failoverManagers.Store(goRoomID, fm)
	utils.Info("Failover enabled for room: %s, local=%s", goRoomID, goLocalPeerID)
	return C.int(0)
}

// FailoverDisable 禁用自动故障切换
//
//export FailoverDisable
func FailoverDisable(roomID *C.char) C.int {
	goRoomID := C.GoString(roomID)

	if v, ok := failoverManagers.Load(goRoomID); ok {
		v.(*sfu.FailoverManager).Close()
		failoverManagers.Delete(goRoomID)
	}

	utils.Info("Failover disabled for room: %s", goRoomID)
	return C.int(0)
}

// FailoverSetCurrentRelay 设置当前 Relay
//
//export FailoverSetCurrentRelay
func FailoverSetCurrentRelay(roomID *C.char, relayID *C.char, epoch C.uint64_t) C.int {
	goRoomID := C.GoString(roomID)
	goRelayID := C.GoString(relayID)

	v, ok := failoverManagers.Load(goRoomID)
	if !ok {
		return C.int(-1)
	}

	fm := v.(*sfu.FailoverManager)
	fm.SetCurrentRelay(goRelayID, uint64(epoch))

	utils.Info("Failover: current relay set to %s (epoch=%d)", goRelayID, epoch)
	return C.int(0)
}

// FailoverUpdateLocalScore 更新本机分数
//
//export FailoverUpdateLocalScore
func FailoverUpdateLocalScore(roomID *C.char, score C.double) C.int {
	goRoomID := C.GoString(roomID)

	v, ok := failoverManagers.Load(goRoomID)
	if !ok {
		return C.int(-1)
	}

	fm := v.(*sfu.FailoverManager)
	fm.UpdateLocalScore(float64(score))

	return C.int(0)
}

// FailoverReceiveClaim 接收其他节点的 Relay 声明
// epoch: 选举纪元号
// score: 声明者的分数（用于同 epoch 冲突解决）
//
//export FailoverReceiveClaim
func FailoverReceiveClaim(roomID *C.char, peerID *C.char, epoch C.uint64_t, score C.double) C.int {
	goRoomID := C.GoString(roomID)
	goPeerID := C.GoString(peerID)

	v, ok := failoverManagers.Load(goRoomID)
	if !ok {
		return C.int(-1)
	}

	fm := v.(*sfu.FailoverManager)
	fm.ReceiveRelayClaim(goPeerID, uint64(epoch), float64(score))

	utils.Debug("Failover: received claim from %s (epoch=%d, score=%.2f)", goPeerID, epoch, score)
	return C.int(0)
}

// FailoverGetState 获取故障切换状态
//
//export FailoverGetState
func FailoverGetState(roomID *C.char) *C.char {
	goRoomID := C.GoString(roomID)

	v, ok := failoverManagers.Load(goRoomID)
	if !ok {
		return C.CString("{}")
	}

	fm := v.(*sfu.FailoverManager)
	relayID, epoch := fm.GetCurrentRelay()

	status := map[string]interface{}{
		"state":         fm.GetState().String(),
		"current_relay": relayID,
		"epoch":         epoch,
	}

	data, _ := json.Marshal(status)
	return C.CString(string(data))
}

// ==========================================
// 一键自动代理模式 (Coordinator)
// ==========================================

// CoordinatorEnable 一键启用自动代理模式
// 自动管理：心跳、选举、故障切换、Relay 接管
// 用户无需关心内部细节，完全自动
//
//export CoordinatorEnable
func CoordinatorEnable(roomID *C.char, localPeerID *C.char) C.int {
	goRoomID := C.GoString(roomID)
	goLocalPeerID := C.GoString(localPeerID)

	// 检查是否已存在
	if _, ok := coordinators.Load(goRoomID); ok {
		utils.Warn("Coordinator already exists for room %s", goRoomID)
		return C.int(0)
	}

	config := sfu.DefaultCoordinatorConfig()
	pmc, err := sfu.NewProxyModeCoordinator(goRoomID, goLocalPeerID, config)
	if err != nil {
		utils.Error("Failed to create Coordinator: %v", err)
		return C.int(-1)
	}

	// 设置事件回调，转发给 Dart
	pmc.SetOnEvent(func(event sfu.CoordinatorEvent) {
		data, _ := json.Marshal(event.Data)

		var eventType int
		switch event.Type {
		case sfu.CoordinatorEventRelayChanged:
			eventType = EventTypeProxyChange
		case sfu.CoordinatorEventBecomeRelay:
			eventType = EventTypeProxyChange
		case sfu.CoordinatorEventRelayFailed:
			eventType = EventTypePeerOffline
		case sfu.CoordinatorEventPeerJoined:
			eventType = EventTypePeerOnline
		case sfu.CoordinatorEventPeerLeft:
			eventType = EventTypePeerOffline
		default:
			eventType = EventTypeProxyChange
		}

		emitEvent(eventType, event.RoomID, event.PeerID, string(data))
	})

	coordinators.Store(goRoomID, pmc)
	pmc.Start()

	utils.Info("Coordinator enabled: room=%s, local=%s (auto-failover active)", goRoomID, goLocalPeerID)
	return C.int(0)
}

// CoordinatorDisable 禁用自动代理模式
//
//export CoordinatorDisable
func CoordinatorDisable(roomID *C.char) C.int {
	goRoomID := C.GoString(roomID)

	if v, ok := coordinators.Load(goRoomID); ok {
		v.(*sfu.ProxyModeCoordinator).Close()
		coordinators.Delete(goRoomID)
	}

	utils.Info("Coordinator disabled: room=%s", goRoomID)
	return C.int(0)
}

// CoordinatorAddPeer 添加 Peer 到自动管理
//
//export CoordinatorAddPeer
func CoordinatorAddPeer(roomID *C.char, peerID *C.char, deviceType, connectionType, powerState C.int) C.int {
	goRoomID := C.GoString(roomID)
	goPeerID := C.GoString(peerID)

	v, ok := coordinators.Load(goRoomID)
	if !ok {
		return C.int(-1)
	}

	pmc := v.(*sfu.ProxyModeCoordinator)
	pmc.AddPeer(goPeerID, int(deviceType), int(connectionType), int(powerState))

	return C.int(0)
}

// CoordinatorRemovePeer 移除 Peer
//
//export CoordinatorRemovePeer
func CoordinatorRemovePeer(roomID *C.char, peerID *C.char) C.int {
	goRoomID := C.GoString(roomID)
	goPeerID := C.GoString(peerID)

	v, ok := coordinators.Load(goRoomID)
	if !ok {
		return C.int(-1)
	}

	pmc := v.(*sfu.ProxyModeCoordinator)
	pmc.RemovePeer(goPeerID)

	return C.int(0)
}

// CoordinatorHandlePong 处理 Pong（心跳响应）
//
//export CoordinatorHandlePong
func CoordinatorHandlePong(roomID *C.char, peerID *C.char) C.int {
	goRoomID := C.GoString(roomID)
	goPeerID := C.GoString(peerID)

	v, ok := coordinators.Load(goRoomID)
	if !ok {
		return C.int(-1)
	}

	pmc := v.(*sfu.ProxyModeCoordinator)
	pmc.HandlePong(goPeerID)

	return C.int(0)
}

// CoordinatorSetRelay 设置当前 Relay（收到外部通知时）
//
//export CoordinatorSetRelay
func CoordinatorSetRelay(roomID *C.char, relayID *C.char, epoch C.uint64_t) C.int {
	goRoomID := C.GoString(roomID)
	goRelayID := C.GoString(relayID)

	v, ok := coordinators.Load(goRoomID)
	if !ok {
		return C.int(-1)
	}

	pmc := v.(*sfu.ProxyModeCoordinator)
	pmc.SetCurrentRelay(goRelayID, uint64(epoch))

	return C.int(0)
}

// CoordinatorReceiveClaim 接收 Relay 声明
// epoch: 选举纪元号
// score: 声明者分数（用于同 epoch 冲突解决）
//
//export CoordinatorReceiveClaim
func CoordinatorReceiveClaim(roomID *C.char, peerID *C.char, epoch C.uint64_t, score C.double) C.int {
	goRoomID := C.GoString(roomID)
	goPeerID := C.GoString(peerID)

	v, ok := coordinators.Load(goRoomID)
	if !ok {
		return C.int(-1)
	}

	pmc := v.(*sfu.ProxyModeCoordinator)
	pmc.ReceiveRelayClaim(goPeerID, uint64(epoch), float64(score))

	return C.int(0)
}

// CoordinatorUpdateLocalDevice 更新本机设备信息
//
//export CoordinatorUpdateLocalDevice
func CoordinatorUpdateLocalDevice(roomID *C.char, deviceType, connectionType, powerState C.int) C.int {
	goRoomID := C.GoString(roomID)

	v, ok := coordinators.Load(goRoomID)
	if !ok {
		return C.int(-1)
	}

	pmc := v.(*sfu.ProxyModeCoordinator)
	pmc.UpdateLocalDeviceInfo(int(deviceType), int(connectionType), int(powerState))

	return C.int(0)
}

// CoordinatorInjectSFU 注入 SFU RTP 包
//
//export CoordinatorInjectSFU
func CoordinatorInjectSFU(roomID *C.char, isVideo C.int, data unsafe.Pointer, dataLen C.int) C.int {
	goRoomID := C.GoString(roomID)

	v, ok := coordinators.Load(goRoomID)
	if !ok {
		return C.int(-1)
	}

	pmc := v.(*sfu.ProxyModeCoordinator)
	goData := C.GoBytes(data, dataLen)
	if err := pmc.InjectSFUPacket(isVideo != 0, goData); err != nil {
		return C.int(-1)
	}
	return C.int(0)
}

// CoordinatorInjectLocal 注入本地分享 RTP 包
//
//export CoordinatorInjectLocal
func CoordinatorInjectLocal(roomID *C.char, isVideo C.int, data unsafe.Pointer, dataLen C.int) C.int {
	goRoomID := C.GoString(roomID)

	v, ok := coordinators.Load(goRoomID)
	if !ok {
		return C.int(-1)
	}

	pmc := v.(*sfu.ProxyModeCoordinator)
	goData := C.GoBytes(data, dataLen)
	if err := pmc.InjectLocalPacket(isVideo != 0, goData); err != nil {
		return C.int(-1)
	}
	return C.int(0)
}

// CoordinatorStartLocalShare 开始本地分享
//
//export CoordinatorStartLocalShare
func CoordinatorStartLocalShare(roomID *C.char, sharerID *C.char) C.int {
	goRoomID := C.GoString(roomID)
	goSharerID := C.GoString(sharerID)

	v, ok := coordinators.Load(goRoomID)
	if !ok {
		return C.int(-1)
	}

	pmc := v.(*sfu.ProxyModeCoordinator)
	pmc.StartLocalShare(goSharerID)

	return C.int(0)
}

// CoordinatorStopLocalShare 停止本地分享
//
//export CoordinatorStopLocalShare
func CoordinatorStopLocalShare(roomID *C.char) C.int {
	goRoomID := C.GoString(roomID)

	v, ok := coordinators.Load(goRoomID)
	if !ok {
		return C.int(-1)
	}

	pmc := v.(*sfu.ProxyModeCoordinator)
	pmc.StopLocalShare()

	return C.int(0)
}

// CoordinatorGetStatus 获取协调器状态
//
//export CoordinatorGetStatus
func CoordinatorGetStatus(roomID *C.char) *C.char {
	goRoomID := C.GoString(roomID)

	v, ok := coordinators.Load(goRoomID)
	if !ok {
		return C.CString("{}")
	}

	pmc := v.(*sfu.ProxyModeCoordinator)
	return C.CString(pmc.GetStatusJSON())
}

// CoordinatorIsRelay 检查本机是否是 Relay
//
//export CoordinatorIsRelay
func CoordinatorIsRelay(roomID *C.char) C.int {
	goRoomID := C.GoString(roomID)

	v, ok := coordinators.Load(goRoomID)
	if !ok {
		return C.int(-1)
	}

	pmc := v.(*sfu.ProxyModeCoordinator)
	if pmc.IsRelay() {
		return C.int(1)
	}
	return C.int(0)
}
