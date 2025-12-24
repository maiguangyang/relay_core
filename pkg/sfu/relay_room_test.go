/*
 * @Author: Marlon.M
 * @Email: maiguangyang@163.com
 * @Date: 2025-12-24
 *
 * RelayRoom Tests
 * 测试 Relay P2P 连接管理
 * 注意：WebRTC 连接测试需要 mock，这里主要测试逻辑
 */
package sfu

import (
	"testing"

	"github.com/pion/webrtc/v4"
)

func TestRelayRoomCreate(t *testing.T) {
	iceServers := []webrtc.ICEServer{
		{URLs: []string{"stun:stun.l.google.com:19302"}},
	}

	room, err := NewRelayRoom("test-room", iceServers)
	if err != nil {
		t.Fatalf("Failed to create RelayRoom: %v", err)
	}
	defer room.Close()

	if room.IsRelay() {
		t.Error("Should not be relay initially")
	}

	status := room.GetStatus()
	if status.RoomID != "test-room" {
		t.Errorf("Expected room ID 'test-room', got %s", status.RoomID)
	}
}

func TestRelayRoomBecomeRelay(t *testing.T) {
	room, err := NewRelayRoom("test-room", nil)
	if err != nil {
		t.Fatalf("Failed to create RelayRoom: %v", err)
	}
	defer room.Close()

	room.BecomeRelay("relay-peer")

	if !room.IsRelay() {
		t.Error("Should be relay after BecomeRelay")
	}

	status := room.GetStatus()
	if status.RelayPeerID != "relay-peer" {
		t.Errorf("Expected relay peer 'relay-peer', got %s", status.RelayPeerID)
	}
}

func TestRelayRoomGetSubscriberCount(t *testing.T) {
	room, err := NewRelayRoom("test-room", nil)
	if err != nil {
		t.Fatalf("Failed to create RelayRoom: %v", err)
	}
	defer room.Close()

	room.BecomeRelay("relay-peer")

	count := room.GetSubscriberCount()
	if count != 0 {
		t.Errorf("Expected 0 subscribers initially, got %d", count)
	}
}

func TestRelayRoomGetSubscribers(t *testing.T) {
	room, err := NewRelayRoom("test-room", nil)
	if err != nil {
		t.Fatalf("Failed to create RelayRoom: %v", err)
	}
	defer room.Close()

	room.BecomeRelay("relay-peer")

	subscribers := room.GetSubscribers()
	if len(subscribers) != 0 {
		t.Errorf("Expected 0 subscribers, got %d", len(subscribers))
	}
}

func TestRelayRoomRemoveSubscriber(t *testing.T) {
	room, err := NewRelayRoom("test-room", nil)
	if err != nil {
		t.Fatalf("Failed to create RelayRoom: %v", err)
	}
	defer room.Close()

	room.BecomeRelay("relay-peer")

	// 移除不存在的订阅者应该不报错
	room.RemoveSubscriber("non-existent")
}

func TestRelayRoomCallbacks(t *testing.T) {
	room, err := NewRelayRoom("test-room", nil)
	if err != nil {
		t.Fatalf("Failed to create RelayRoom: %v", err)
	}
	defer room.Close()

	room.SetCallbacks(
		func(roomID, peerID string) { t.Log("joined") },
		func(roomID, peerID string) { t.Log("left") },
		nil, nil, nil,
	)

	// 回调应该设置成功（不会直接触发）
}

func TestRelayRoomSourceSwitcher(t *testing.T) {
	room, err := NewRelayRoom("test-room", nil)
	if err != nil {
		t.Fatalf("Failed to create RelayRoom: %v", err)
	}
	defer room.Close()

	room.BecomeRelay("relay-peer")

	// 获取 SourceSwitcher
	switcher := room.GetSourceSwitcher()
	if switcher == nil {
		t.Error("SourceSwitcher should not be nil")
	}
}

func TestRelayRoomGetStatus(t *testing.T) {
	room, err := NewRelayRoom("test-room", nil)
	if err != nil {
		t.Fatalf("Failed to create RelayRoom: %v", err)
	}
	defer room.Close()

	room.BecomeRelay("relay-peer")

	status := room.GetStatus()
	if status.RoomID != "test-room" {
		t.Errorf("Expected room ID 'test-room', got %s", status.RoomID)
	}

	if !status.IsRelay {
		t.Error("Status should show IsRelay = true")
	}

	t.Logf("Status: %+v", status)
}

func TestRelayRoomClose(t *testing.T) {
	room, err := NewRelayRoom("test-room", nil)
	if err != nil {
		t.Fatalf("Failed to create RelayRoom: %v", err)
	}

	room.BecomeRelay("relay-peer")
	room.Close()

	// 关闭后再次调用应该安全
	room.Close()
}

// ==========================================
// Benchmarks
// ==========================================

func BenchmarkRelayRoomCreate(b *testing.B) {
	for i := 0; i < b.N; i++ {
		room, err := NewRelayRoom("bench-room", nil)
		if err != nil {
			b.Fatalf("Failed to create: %v", err)
		}
		room.Close()
	}
}

func BenchmarkRelayRoomGetStatus(b *testing.B) {
	room, err := NewRelayRoom("bench-room", nil)
	if err != nil {
		b.Fatalf("Failed to create: %v", err)
	}
	defer room.Close()

	room.BecomeRelay("relay-peer")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		room.GetStatus()
	}
}
