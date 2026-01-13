/*
 * @Author: Marlon.M
 * @Email: maiguangyang@163.com
 * @Date: 2026-01-13
 *
 * LocalShareBridge FFI 导出
 *
 * 提供 Dart FFI 调用接口，用于控制本地分享桥接器。
 */
package main

/*
#include <stdlib.h>
*/
import "C"

import (
	"encoding/json"

	"github.com/maiguangyang/relay_core/pkg/sfu"
	"github.com/maiguangyang/relay_core/pkg/utils"
)

// ========================================
// LocalShareBridge FFI 导出
// ========================================

// LocalShareBridgeCreate 创建本地分享桥接器
// 返回: 0 成功, -1 失败
//
//export LocalShareBridgeCreate
func LocalShareBridgeCreate(roomID *C.char) C.int {
	rid := C.GoString(roomID)

	// 获取对应的 SourceSwitcher
	var switcher *sfu.SourceSwitcher

	// 优先从 Coordinator 获取
	if coord := getCoordinator(rid); coord != nil {
		switcher = coord.GetSourceSwitcher()
		utils.Info("[LocalShareBridgeFFI] Got SourceSwitcher from Coordinator for room %s", rid)
	}

	// 备选：从独立的 SourceSwitcher 注册表获取
	if switcher == nil {
		switcher = getSourceSwitcher(rid)
		utils.Info("[LocalShareBridgeFFI] Got SourceSwitcher from registry for room %s", rid)
	}

	if switcher == nil {
		utils.Error("[LocalShareBridgeFFI] No SourceSwitcher found for room %s", rid)
		return -1
	}

	// 创建桥接器
	sfu.CreateLocalShareBridge(rid, switcher)
	return 0
}

// LocalShareBridgeStart 启动桥接器并开始监听 UDP
// preferredPort: 首选端口，0 表示自动分配
// 返回: 实际监听端口 (成功), -1 (失败)
//
//export LocalShareBridgeStart
func LocalShareBridgeStart(roomID *C.char, preferredPort C.int) C.int {
	rid := C.GoString(roomID)

	bridge := sfu.GetLocalShareBridge(rid)
	if bridge == nil {
		utils.Error("[LocalShareBridgeFFI] Bridge not found for room %s", rid)
		return -1
	}

	port, err := bridge.Start(int(preferredPort))
	if err != nil {
		utils.Error("[LocalShareBridgeFFI] Failed to start bridge for room %s: %v", rid, err)
		return -1
	}

	utils.Info("[LocalShareBridgeFFI] Bridge started on port %d for room %s", port, rid)
	return C.int(port)
}

// LocalShareBridgeStop 停止桥接器
// 返回: 0 成功, -1 失败
//
//export LocalShareBridgeStop
func LocalShareBridgeStop(roomID *C.char) C.int {
	rid := C.GoString(roomID)

	bridge := sfu.GetLocalShareBridge(rid)
	if bridge == nil {
		return -1
	}

	bridge.Stop()
	return 0
}

// LocalShareBridgeDestroy 销毁桥接器
// 返回: 0 成功
//
//export LocalShareBridgeDestroy
func LocalShareBridgeDestroy(roomID *C.char) C.int {
	rid := C.GoString(roomID)
	sfu.DestroyLocalShareBridge(rid)
	return 0
}

// LocalShareBridgeGetPort 获取监听端口
// 返回: 端口号 (成功), -1 (未启动)
//
//export LocalShareBridgeGetPort
func LocalShareBridgeGetPort(roomID *C.char) C.int {
	rid := C.GoString(roomID)

	bridge := sfu.GetLocalShareBridge(rid)
	if bridge == nil {
		return -1
	}

	port := bridge.GetPort()
	if port == 0 {
		return -1
	}
	return C.int(port)
}

// LocalShareBridgeIsRunning 检查是否正在运行
// 返回: 1 运行中, 0 未运行
//
//export LocalShareBridgeIsRunning
func LocalShareBridgeIsRunning(roomID *C.char) C.int {
	rid := C.GoString(roomID)

	bridge := sfu.GetLocalShareBridge(rid)
	if bridge == nil {
		return 0
	}

	if bridge.IsRunning() {
		return 1
	}
	return 0
}

// LocalShareBridgeGetStatus 获取桥接器状态
// 返回: JSON 字符串，需要调用 FreeString 释放
//
//export LocalShareBridgeGetStatus
func LocalShareBridgeGetStatus(roomID *C.char) *C.char {
	rid := C.GoString(roomID)

	bridge := sfu.GetLocalShareBridge(rid)
	if bridge == nil {
		return C.CString("{}")
	}

	status := bridge.GetStatus()
	data, _ := json.Marshal(status)
	return C.CString(string(data))
}
