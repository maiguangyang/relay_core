/*
 * @Author: Marlon.M
 * @Email: maiguangyang@163.com
 * @Date: 2025-12-24
 *
 * Mock WebRTC Tests
 * 使用本地 Loopback 连接测试 RelayRoom 的完整连接流程
 */
package sfu

import (
	"testing"

	"github.com/pion/webrtc/v4"
)

func TestMock_RelayRoomConnection(t *testing.T) {
	// 1. 创建 RelayRoom
	relay, err := NewRelayRoom("mock-room", nil)
	if err != nil {
		t.Fatalf("Failed to create RelayRoom: %v", err)
	}
	defer relay.Close()

	relay.BecomeRelay("relay-node")
	relay.GetSourceSwitcher().StartLocalShare("local-sharer")

	// 2. 创建模拟客户端 PeerConnection
	m := &webrtc.MediaEngine{}
	if err := m.RegisterDefaultCodecs(); err != nil {
		t.Fatal(err)
	}
	api := webrtc.NewAPI(webrtc.WithMediaEngine(m))

	clientPC, err := api.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatal(err)
	}
	defer clientPC.Close()

	// 监听 Track
	trackReceived := make(chan struct{})
	clientPC.OnTrack(func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		t.Logf("Client received track: %s", track.ID())
		close(trackReceived)
	})

	// 3. 客户端创建 Offer (只接收 RecvOnly)
	_, err = clientPC.AddTransceiverFromKind(webrtc.RTPCodecTypeVideo, webrtc.RTPTransceiverInit{
		Direction: webrtc.RTPTransceiverDirectionRecvonly,
	})
	if err != nil {
		t.Fatal(err)
	}

	offer, err := clientPC.CreateOffer(nil)
	if err != nil {
		t.Fatal(err)
	}

	if err := clientPC.SetLocalDescription(offer); err != nil {
		t.Fatal(err)
	}

	// 4. Relay 处理 Offer
	answerSDP, err := relay.AddSubscriber("client-1", offer.SDP)
	if err != nil {
		t.Fatalf("AddSubscriber failed: %v", err)
	}

	// 5. 客户端设置 Answer
	answer := webrtc.SessionDescription{
		Type: webrtc.SDPTypeAnswer,
		SDP:  answerSDP,
	}
	if err := clientPC.SetRemoteDescription(answer); err != nil {
		t.Fatalf("SetRemoteDescription failed: %v", err)
	}

	// 6. 验证连接状态 (在非 vnet 环境下，ICE 可能无法连通，但这验证了 SDP 协商流程)
	// 如果需要验证 ICE 连通，我们需要手动交换 ICE candidate 或者使用 vnet
	// 这里我们主要验证逻辑流程是否跑通，SDP 是否正确生成
	t.Log("SDP exchange completed successfully")

	status := relay.GetStatus()
	if status.SubscriberCount != 1 {
		t.Errorf("Expected 1 subscriber, got %d", status.SubscriberCount)
	}
}
