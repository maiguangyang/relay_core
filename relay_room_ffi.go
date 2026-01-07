/*
 * @Author: Marlon.M
 * @Email: maiguangyang@163.com
 * @Date: 2025-12-24
 *
 * Relay Room FFI Exports
 * RelayRoom 相关的 C 导出函数
 * 实现 Relay P2P 连接管理和 SDP 重协商
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

	"github.com/maiguangyang/relay_core/pkg/sfu"
	"github.com/maiguangyang/relay_core/pkg/signaling"
	"github.com/maiguangyang/relay_core/pkg/utils"
	"github.com/pion/webrtc/v4"
)

// RelayRoom 实例管理
var (
	relayRooms sync.Map // roomID -> *sfu.RelayRoom
)

// 事件类型扩展
const (
	EventTypeSubscriberJoined = 10 // 订阅者加入
	EventTypeSubscriberLeft   = 11 // 订阅者离开
	EventTypeRenegotiate      = 12 // 需要重协商
)

// registerRelayRoom 注册 RelayRoom
func registerRelayRoom(roomID string, room *sfu.RelayRoom) {
	relayRooms.Store(roomID, room)
}

// getRelayRoom 获取 RelayRoom
func getRelayRoom(roomID string) *sfu.RelayRoom {
	if v, ok := relayRooms.Load(roomID); ok {
		return v.(*sfu.RelayRoom)
	}
	return nil
}

// unregisterRelayRoom 注销 RelayRoom
func unregisterRelayRoom(roomID string) {
	if v, ok := relayRooms.Load(roomID); ok {
		v.(*sfu.RelayRoom).Close()
		relayRooms.Delete(roomID)
	}
}

// ==========================================
// RelayRoom 创建与销毁
// ==========================================

// RelayRoomCreate 创建代理房间
// iceServersJSON: ICE 服务器配置 JSON
//
//export RelayRoomCreate
func RelayRoomCreate(roomID *C.char, iceServersJSON *C.char) C.int {
	goRoomID := C.GoString(roomID)

	// 解析 ICE 服务器配置
	var iceServers []webrtc.ICEServer
	if iceServersJSON != nil {
		goICE := C.GoString(iceServersJSON)
		if goICE != "" {
			json.Unmarshal([]byte(goICE), &iceServers)
		}
	}

	if len(iceServers) == 0 {
		iceServers = []webrtc.ICEServer{
			{URLs: []string{"stun:stun.l.google.com:19302"}},
		}
	}

	// 准备选项 - 如果存在 Coordinator，使用其 SourceSwitcher
	var opts []sfu.RelayRoomOption
	if coord := getCoordinator(goRoomID); coord != nil {
		if ss := coord.GetSourceSwitcher(); ss != nil {
			opts = append(opts, sfu.WithSourceSwitcher(ss))
			utils.Info("RelayRoomCreate: using Coordinator's SourceSwitcher for room %s", goRoomID)
		}
	}

	room, err := sfu.NewRelayRoom(goRoomID, iceServers, opts...)
	if err != nil {
		utils.Error("Failed to create RelayRoom %s: %v", goRoomID, err)
		return C.int(-1)
	}

	// 设置回调
	room.SetCallbacks(
		// onSubscriberJoined
		func(rID, peerID string) {
			emitEvent(EventTypeSubscriberJoined, rID, peerID, "")
		},
		// onSubscriberLeft
		func(rID, peerID string) {
			emitEvent(EventTypeSubscriberLeft, rID, peerID, "")
		},
		// onICECandidate
		func(rID, peerID string, candidate *webrtc.ICECandidate) {
			if candidate != nil {
				candidateJSON := candidate.ToJSON()
				data, _ := json.Marshal(signaling.CandidateMessage{
					Candidate:        candidateJSON.Candidate,
					SDPMid:           candidateJSON.SDPMid,
					SDPMLineIndex:    candidateJSON.SDPMLineIndex,
					UsernameFragment: candidateJSON.UsernameFragment,
				})
				emitEvent(EventTypeICECandidate, rID, peerID, string(data))
			}
		},
		// onNeedRenegotiate
		func(rID, peerID string, offer string) {
			data, _ := json.Marshal(map[string]string{
				"type": "offer",
				"sdp":  offer,
			})
			emitEvent(EventTypeRenegotiate, rID, peerID, string(data))
		},
		// onError
		func(rID, peerID string, err error) {
			data, _ := json.Marshal(signaling.ErrorMessage{
				Code:    500,
				Message: err.Error(),
			})
			emitEvent(EventTypeError, rID, peerID, string(data))
		},
	)

	// 设置关键帧请求回调
	// 当新订阅者加入时，请求 SFU 发送关键帧，确保新订阅者能立即看到画面
	room.SetKeyframeRequestCallback(func(rID string) {
		if bridge := sfu.GetBridge(rID); bridge != nil {
			utils.Info("Requesting keyframe for new subscriber in room %s", rID)
			bridge.RequestKeyframe()
		}
	})

	registerRelayRoom(goRoomID, room)
	// 注册 SourceSwitcher，让 LiveKitBridge 能够获取到同一个实例
	registerSourceSwitcher(goRoomID, room.GetSourceSwitcher())
	utils.Info("RelayRoom created: %s", goRoomID)
	return C.int(0)
}

// RelayRoomDestroy 销毁代理房间
//
//export RelayRoomDestroy
func RelayRoomDestroy(roomID *C.char) C.int {
	goRoomID := C.GoString(roomID)
	unregisterRelayRoom(goRoomID)
	utils.Info("RelayRoom destroyed: %s", goRoomID)
	return C.int(0)
}

// ==========================================
// Relay 节点管理
// ==========================================

// RelayRoomBecomeRelay 成为 Relay 节点
//
//export RelayRoomBecomeRelay
func RelayRoomBecomeRelay(roomID *C.char, peerID *C.char) C.int {
	goRoomID := C.GoString(roomID)
	goPeerID := C.GoString(peerID)

	room := getRelayRoom(goRoomID)
	if room == nil {
		return C.int(-1)
	}

	room.BecomeRelay(goPeerID)
	utils.Info("Became Relay for room %s: %s", goRoomID, goPeerID)
	return C.int(0)
}

// RelayRoomIsRelay 检查是否是 Relay 节点
//
//export RelayRoomIsRelay
func RelayRoomIsRelay(roomID *C.char) C.int {
	goRoomID := C.GoString(roomID)

	room := getRelayRoom(goRoomID)
	if room == nil {
		return C.int(-1)
	}

	if room.IsRelay() {
		return C.int(1)
	}
	return C.int(0)
}

// ==========================================
// 订阅者管理
// ==========================================

// RelayRoomAddSubscriber 添加订阅者
// 使用远端 Offer 创建 P2P 连接，返回 Answer SDP
//
//export RelayRoomAddSubscriber
func RelayRoomAddSubscriber(roomID *C.char, peerID *C.char, offerSDP *C.char) *C.char {
	goRoomID := C.GoString(roomID)
	goPeerID := C.GoString(peerID)
	goOfferSDP := C.GoString(offerSDP)

	room := getRelayRoom(goRoomID)
	if room == nil {
		utils.Error("RelayRoom not found: %s", goRoomID)
		return nil
	}

	answerSDP, err := room.AddSubscriber(goPeerID, goOfferSDP)
	if err != nil {
		utils.Error("Failed to add subscriber %s: %v", goPeerID, err)
		return nil
	}

	utils.Info("Subscriber added: %s to room %s", goPeerID, goRoomID)
	return C.CString(answerSDP)
}

// RelayRoomRemoveSubscriber 移除订阅者
//
//export RelayRoomRemoveSubscriber
func RelayRoomRemoveSubscriber(roomID *C.char, peerID *C.char) C.int {
	goRoomID := C.GoString(roomID)
	goPeerID := C.GoString(peerID)

	room := getRelayRoom(goRoomID)
	if room == nil {
		return C.int(-1)
	}

	if err := room.RemoveSubscriber(goPeerID); err != nil {
		utils.Error("Failed to remove subscriber %s: %v", goPeerID, err)
		return C.int(-1)
	}

	utils.Info("Subscriber removed: %s from room %s", goPeerID, goRoomID)
	return C.int(0)
}

// RelayRoomGetSubscribers 获取订阅者列表
//
//export RelayRoomGetSubscribers
func RelayRoomGetSubscribers(roomID *C.char) *C.char {
	goRoomID := C.GoString(roomID)

	room := getRelayRoom(goRoomID)
	if room == nil {
		return C.CString("[]")
	}

	subscribers := room.GetSubscribers()
	data, _ := json.Marshal(subscribers)
	return C.CString(string(data))
}

// RelayRoomGetSubscriberCount 获取订阅者数量
//
//export RelayRoomGetSubscriberCount
func RelayRoomGetSubscriberCount(roomID *C.char) C.int {
	goRoomID := C.GoString(roomID)

	room := getRelayRoom(goRoomID)
	if room == nil {
		return C.int(0)
	}

	return C.int(room.GetSubscriberCount())
}

// ==========================================
// ICE 候选处理
// ==========================================

// RelayRoomAddICECandidate 添加 ICE 候选
//
//export RelayRoomAddICECandidate
func RelayRoomAddICECandidate(roomID *C.char, peerID *C.char, candidateJSON *C.char) C.int {
	goRoomID := C.GoString(roomID)
	goPeerID := C.GoString(peerID)
	goCandidateJSON := C.GoString(candidateJSON)

	room := getRelayRoom(goRoomID)
	if room == nil {
		return C.int(-1)
	}

	var candidateMsg signaling.CandidateMessage
	if err := json.Unmarshal([]byte(goCandidateJSON), &candidateMsg); err != nil {
		utils.Error("Failed to parse ICE candidate: %v", err)
		return C.int(-1)
	}

	candidateInit := webrtc.ICECandidateInit{
		Candidate:        candidateMsg.Candidate,
		SDPMid:           candidateMsg.SDPMid,
		SDPMLineIndex:    candidateMsg.SDPMLineIndex,
		UsernameFragment: candidateMsg.UsernameFragment,
	}

	if err := room.AddICECandidate(goPeerID, candidateInit); err != nil {
		utils.Error("Failed to add ICE candidate for %s: %v", goPeerID, err)
		return C.int(-1)
	}

	return C.int(0)
}

// ==========================================
// SDP 重协商
// ==========================================

// RelayRoomTriggerRenegotiation 触发重协商
// 为所有已连接的订阅者生成新的 Offer
// 返回 JSON: {"peerID1": "offer1", "peerID2": "offer2", ...}
//
//export RelayRoomTriggerRenegotiation
func RelayRoomTriggerRenegotiation(roomID *C.char) *C.char {
	goRoomID := C.GoString(roomID)

	room := getRelayRoom(goRoomID)
	if room == nil {
		return C.CString("{}")
	}

	offers := room.TriggerRenegotiation()
	data, _ := json.Marshal(offers)

	utils.Info("Renegotiation triggered for room %s, %d offers generated", goRoomID, len(offers))
	return C.CString(string(data))
}

// RelayRoomCreateOffer 为指定订阅者创建 Offer
//
//export RelayRoomCreateOffer
func RelayRoomCreateOffer(roomID *C.char, peerID *C.char) *C.char {
	goRoomID := C.GoString(roomID)
	goPeerID := C.GoString(peerID)

	room := getRelayRoom(goRoomID)
	if room == nil {
		return nil
	}

	offer, err := room.CreateOfferForSubscriber(goPeerID)
	if err != nil {
		utils.Error("Failed to create offer for %s: %v", goPeerID, err)
		return nil
	}

	return C.CString(offer)
}

// RelayRoomHandleAnswer 处理订阅者的 Answer（重协商响应）
//
//export RelayRoomHandleAnswer
func RelayRoomHandleAnswer(roomID *C.char, peerID *C.char, answerSDP *C.char) C.int {
	goRoomID := C.GoString(roomID)
	goPeerID := C.GoString(peerID)
	goAnswerSDP := C.GoString(answerSDP)

	room := getRelayRoom(goRoomID)
	if room == nil {
		return C.int(-1)
	}

	if err := room.HandleSubscriberAnswer(goPeerID, goAnswerSDP); err != nil {
		utils.Error("Failed to handle answer from %s: %v", goPeerID, err)
		return C.int(-1)
	}

	utils.Debug("Answer handled for %s in room %s", goPeerID, goRoomID)
	return C.int(0)
}

// ==========================================
// 状态查询
// ==========================================

// RelayRoomGetStatus 获取房间状态
//
//export RelayRoomGetStatus
func RelayRoomGetStatus(roomID *C.char) *C.char {
	goRoomID := C.GoString(roomID)

	room := getRelayRoom(goRoomID)
	if room == nil {
		return nil
	}

	status := room.GetStatus()
	return C.CString(status.ToJSON())
}

// ==========================================
// SourceSwitcher 集成（便捷方法）
// ==========================================

// RelayRoomInjectSFU 注入 SFU RTP 包（通过 RelayRoom）
//
//export RelayRoomInjectSFU
func RelayRoomInjectSFU(roomID *C.char, isVideo C.int, data unsafe.Pointer, dataLen C.int) C.int {
	goRoomID := C.GoString(roomID)

	room := getRelayRoom(goRoomID)
	if room == nil {
		return C.int(-1)
	}

	switcher := room.GetSourceSwitcher()
	if switcher == nil {
		return C.int(-1)
	}

	goData := C.GoBytes(data, dataLen)
	if err := switcher.InjectSFUPacket(isVideo != 0, goData); err != nil {
		return C.int(-1)
	}

	return C.int(0)
}

// RelayRoomInjectLocal 注入本地分享 RTP 包（通过 RelayRoom）
//
//export RelayRoomInjectLocal
func RelayRoomInjectLocal(roomID *C.char, isVideo C.int, data unsafe.Pointer, dataLen C.int) C.int {
	goRoomID := C.GoString(roomID)

	room := getRelayRoom(goRoomID)
	if room == nil {
		return C.int(-1)
	}

	switcher := room.GetSourceSwitcher()
	if switcher == nil {
		return C.int(-1)
	}

	goData := C.GoBytes(data, dataLen)
	if err := switcher.InjectLocalPacket(isVideo != 0, goData); err != nil {
		return C.int(-1)
	}

	return C.int(0)
}

// RelayRoomStartLocalShare 开始本地分享
//
//export RelayRoomStartLocalShare
func RelayRoomStartLocalShare(roomID *C.char, sharerID *C.char) C.int {
	goRoomID := C.GoString(roomID)
	goSharerID := C.GoString(sharerID)

	room := getRelayRoom(goRoomID)
	if room == nil {
		return C.int(-1)
	}

	switcher := room.GetSourceSwitcher()
	if switcher == nil {
		return C.int(-1)
	}

	switcher.StartLocalShare(goSharerID)

	// 触发重协商（通知订阅者源已切换）
	room.TriggerRenegotiation()

	utils.Info("Local share started in room %s by %s", goRoomID, goSharerID)
	return C.int(0)
}

// RelayRoomStopLocalShare 停止本地分享
//
//export RelayRoomStopLocalShare
func RelayRoomStopLocalShare(roomID *C.char) C.int {
	goRoomID := C.GoString(roomID)

	room := getRelayRoom(goRoomID)
	if room == nil {
		return C.int(-1)
	}

	switcher := room.GetSourceSwitcher()
	if switcher == nil {
		return C.int(-1)
	}

	switcher.StopLocalShare()

	// 触发重协商（通知订阅者源已切换回 SFU）
	room.TriggerRenegotiation()

	utils.Info("Local share stopped in room %s", goRoomID)
	return C.int(0)
}

// unsafePointer 辅助函数
func unsafePointer(p *C.char) (ptr interface{}) {
	return p
}
