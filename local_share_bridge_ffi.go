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
	"github.com/maiguangyang/relay_core/pkg/sfu"
	"github.com/maiguangyang/relay_core/pkg/utils"
)

// ========================================
// LocalShareBridge FFI 导出
// ========================================

// LocalShareConnect 建立本地分享 Loopback 连接
// 接收 Offer SDP，返回 Answer SDP
// 返回: Answer SDP 字符串 (需要 FreeString), 或者 NULL (失败)
//
//export LocalShareConnect
func LocalShareConnect(roomID *C.char, offerSdp *C.char) *C.char {
	rid := C.GoString(roomID)
	offer := C.GoString(offerSdp)

	utils.Info("[LocalShareFFI] Connect requested for room %s", rid)

	// 1. Get SourceSwitcher
	var switcher *sfu.SourceSwitcher
	if coord := getCoordinator(rid); coord != nil {
		switcher = coord.GetSourceSwitcher()
	}
	if switcher == nil {
		switcher = getSourceSwitcher(rid)
	}
	if switcher == nil {
		utils.Error("[LocalShareFFI] No SourceSwitcher found for room %s", rid)
		return nil
	}

	// 2. Create LoopbackPeer
	peer, err := sfu.CreateLoopbackPeer(rid, switcher)
	if err != nil {
		utils.Error("[LocalShareFFI] Failed to create LoopbackPeer: %v", err)
		return nil
	}

	// 3. Handle Offer
	answer, err := peer.HandleOffer(offer)
	if err != nil {
		utils.Error("[LocalShareFFI] Failed to handle offer: %v", err)
		sfu.DestroyLoopbackPeer(rid) // Cleanup on error
		return nil
	}

	return C.CString(answer)
}
