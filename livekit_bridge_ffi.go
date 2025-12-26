/*
 * @Author: Marlon.M
 * @Email: maiguangyang@163.com
 * @Date: 2025-12-26
 *
 * LiveKitBridge FFI 导出
 *
 * 提供 Dart FFI 调用接口，用于控制 LiveKit 桥接器。
 */
package main

/*
#include <stdlib.h>
*/
import "C"

import (
	"encoding/json"
	"unsafe"

	"github.com/maiguangyang/relay_core/pkg/sfu"
)

// ========================================
// LiveKitBridge FFI 导出
// ========================================

// LiveKitBridgeCreate 创建 LiveKit 桥接器
// 返回: 0 成功, -1 失败
//
//export LiveKitBridgeCreate
func LiveKitBridgeCreate(roomID *C.char) C.int {
	rid := C.GoString(roomID)

	// 获取对应的 SourceSwitcher
	// 需要先有 Coordinator 或 SourceSwitcher
	coord := getCoordinator(rid)
	var switcher *sfu.SourceSwitcher
	if coord != nil {
		switcher = coord.GetSourceSwitcher()
	}

	if switcher == nil {
		// 尝试获取独立的 SourceSwitcher
		switcher = getSourceSwitcher(rid)
	}

	// 创建桥接器
	sfu.CreateBridge(rid, switcher)
	return 0
}

// LiveKitBridgeConnect 连接到 LiveKit 房间
// 返回: 0 成功, -1 失败
//
//export LiveKitBridgeConnect
func LiveKitBridgeConnect(roomID, url, token *C.char) C.int {
	rid := C.GoString(roomID)
	u := C.GoString(url)
	t := C.GoString(token)

	bridge := sfu.GetBridge(rid)
	if bridge == nil {
		return -1
	}

	err := bridge.Connect(u, t)
	if err != nil {
		return -1
	}

	return 0
}

// LiveKitBridgeDisconnect 断开 LiveKit 连接
// 返回: 0 成功, -1 失败
//
//export LiveKitBridgeDisconnect
func LiveKitBridgeDisconnect(roomID *C.char) C.int {
	rid := C.GoString(roomID)

	bridge := sfu.GetBridge(rid)
	if bridge == nil {
		return -1
	}

	bridge.Disconnect()
	return 0
}

// LiveKitBridgeDestroy 销毁 LiveKit 桥接器
// 返回: 0 成功
//
//export LiveKitBridgeDestroy
func LiveKitBridgeDestroy(roomID *C.char) C.int {
	rid := C.GoString(roomID)
	sfu.DestroyBridge(rid)
	return 0
}

// LiveKitBridgeGetStatus 获取桥接器状态
// 返回: JSON 字符串，需要调用 FreeString 释放
//
//export LiveKitBridgeGetStatus
func LiveKitBridgeGetStatus(roomID *C.char) *C.char {
	rid := C.GoString(roomID)

	bridge := sfu.GetBridge(rid)
	if bridge == nil {
		return C.CString("{}")
	}

	status := bridge.GetStatus()
	data, _ := json.Marshal(status)
	return C.CString(string(data))
}

// LiveKitBridgeIsConnected 检查是否已连接
// 返回: 1 已连接, 0 未连接
//
//export LiveKitBridgeIsConnected
func LiveKitBridgeIsConnected(roomID *C.char) C.int {
	rid := C.GoString(roomID)

	bridge := sfu.GetBridge(rid)
	if bridge == nil {
		return 0
	}

	if bridge.IsConnected() {
		return 1
	}
	return 0
}

// ========================================
// 辅助函数
// ========================================

// getCoordinator 获取 Coordinator (使用 proxy_mode_ffi.go 中的 coordinators)
func getCoordinator(roomID string) *sfu.ProxyModeCoordinator {
	if v, ok := coordinators.Load(roomID); ok {
		return v.(*sfu.ProxyModeCoordinator)
	}
	return nil
}

// 确保 unsafe 包被使用
var _ = unsafe.Pointer(nil)
