/*
 * @Author: Marlon.M
 * @Email: maiguangyang@163.com
 * @Date: 2025-12-24
 *
 * Coordinator Tests
 * 测试一键自动代理模式的完整生命周期
 */
package sfu

import (
	"sync/atomic"
	"testing"
	"time"
)

func TestCoordinatorCreate(t *testing.T) {
	config := DefaultCoordinatorConfig()
	pmc, err := NewProxyModeCoordinator("test-room", "local-peer", config)
	if err != nil {
		t.Fatalf("Failed to create coordinator: %v", err)
	}
	defer pmc.Close()

	if pmc.IsRelay() {
		t.Error("Should not be relay initially")
	}

	status := pmc.GetStatus()
	if status["room_id"] != "test-room" {
		t.Errorf("Expected room_id 'test-room', got %v", status["room_id"])
	}
}

func TestCoordinatorAddPeer(t *testing.T) {
	config := DefaultCoordinatorConfig()
	pmc, err := NewProxyModeCoordinator("test-room", "local-peer", config)
	if err != nil {
		t.Fatalf("Failed to create coordinator: %v", err)
	}
	defer pmc.Close()

	pmc.AddPeer("peer-1", 1, 1, 1) // PC, Ethernet, PluggedIn
	pmc.AddPeer("peer-2", 3, 2, 2) // Mobile, WiFi, Battery

	status := pmc.GetStatus()
	peerCount := status["peer_count"].(int)
	if peerCount != 2 {
		t.Errorf("Expected 2 peers, got %d", peerCount)
	}
}

func TestCoordinatorRemovePeer(t *testing.T) {
	config := DefaultCoordinatorConfig()
	pmc, err := NewProxyModeCoordinator("test-room", "local-peer", config)
	if err != nil {
		t.Fatalf("Failed to create coordinator: %v", err)
	}
	defer pmc.Close()

	pmc.AddPeer("peer-1", 1, 1, 1)
	pmc.AddPeer("peer-2", 3, 2, 2)
	pmc.RemovePeer("peer-1")

	status := pmc.GetStatus()
	peerCount := status["peer_count"].(int)
	if peerCount != 1 {
		t.Errorf("Expected 1 peer after removal, got %d", peerCount)
	}
}

func TestCoordinatorSetCurrentRelay(t *testing.T) {
	config := DefaultCoordinatorConfig()
	pmc, err := NewProxyModeCoordinator("test-room", "local-peer", config)
	if err != nil {
		t.Fatalf("Failed to create coordinator: %v", err)
	}
	defer pmc.Close()

	pmc.SetCurrentRelay("relay-1", 1)

	status := pmc.GetStatus()
	if status["current_relay"] != "relay-1" {
		t.Errorf("Expected current_relay 'relay-1', got %v", status["current_relay"])
	}

	if status["epoch"].(uint64) != 1 {
		t.Errorf("Expected epoch 1, got %v", status["epoch"])
	}
}

func TestCoordinatorBecomeRelayEvent(t *testing.T) {
	config := DefaultCoordinatorConfig()
	pmc, err := NewProxyModeCoordinator("test-room", "local-peer", config)
	if err != nil {
		t.Fatalf("Failed to create coordinator: %v", err)
	}
	defer pmc.Close()

	var becameRelay int32
	pmc.SetOnEvent(func(event CoordinatorEvent) {
		if event.Type == CoordinatorEventBecomeRelay {
			atomic.AddInt32(&becameRelay, 1)
		}
	})

	// 模拟成为 Relay
	pmc.SetCurrentRelay("local-peer", 1)

	// 检查是否是 Relay
	if !pmc.IsRelay() {
		t.Error("Should be relay when current_relay == local_peer")
	}
}

func TestCoordinatorReceiveClaim(t *testing.T) {
	config := DefaultCoordinatorConfig()
	pmc, err := NewProxyModeCoordinator("test-room", "local-peer", config)
	if err != nil {
		t.Fatalf("Failed to create coordinator: %v", err)
	}
	defer pmc.Close()

	pmc.SetCurrentRelay("relay-1", 1)

	// 接收更高 epoch 的声明
	pmc.ReceiveRelayClaim("relay-2", 2, 80.0)

	status := pmc.GetStatus()
	if status["current_relay"] != "relay-2" {
		t.Errorf("Expected current_relay 'relay-2', got %v", status["current_relay"])
	}
}

func TestCoordinatorLocalShare(t *testing.T) {
	config := DefaultCoordinatorConfig()
	pmc, err := NewProxyModeCoordinator("test-room", "local-peer", config)
	if err != nil {
		t.Fatalf("Failed to create coordinator: %v", err)
	}
	defer pmc.Close()

	pmc.StartLocalShare("sharer-1")

	switcher := pmc.GetSourceSwitcher()
	if !switcher.IsLocalSharing() {
		t.Error("Should be local sharing")
	}

	pmc.StopLocalShare()

	if switcher.IsLocalSharing() {
		t.Error("Should not be local sharing after stop")
	}
}

func TestCoordinatorUpdateLocalDeviceInfo(t *testing.T) {
	config := DefaultCoordinatorConfig()
	pmc, err := NewProxyModeCoordinator("test-room", "local-peer", config)
	if err != nil {
		t.Fatalf("Failed to create coordinator: %v", err)
	}
	defer pmc.Close()

	pmc.UpdateLocalDeviceInfo(1, 1, 1) // PC, Ethernet, PluggedIn

	// 分数应该更新（内部会更新 elector 和 failover）
	// 这个测试主要验证不崩溃
}

func TestCoordinatorHandlePong(t *testing.T) {
	config := DefaultCoordinatorConfig()
	pmc, err := NewProxyModeCoordinator("test-room", "local-peer", config)
	if err != nil {
		t.Fatalf("Failed to create coordinator: %v", err)
	}
	defer pmc.Close()

	pmc.AddPeer("peer-1", 1, 1, 1)

	// 处理 Pong
	pmc.HandlePong("peer-1")

	// 主要验证不崩溃
}

func TestCoordinatorGetStatusJSON(t *testing.T) {
	config := DefaultCoordinatorConfig()
	pmc, err := NewProxyModeCoordinator("test-room", "local-peer", config)
	if err != nil {
		t.Fatalf("Failed to create coordinator: %v", err)
	}
	defer pmc.Close()

	json := pmc.GetStatusJSON()
	if json == "" {
		t.Error("Status JSON should not be empty")
	}

	t.Logf("Status JSON: %s", json)
}

func TestCoordinatorStart(t *testing.T) {
	config := DefaultCoordinatorConfig()
	config.KeepaliveInterval = 100 * time.Millisecond
	config.ElectionInterval = 100 * time.Millisecond

	pmc, err := NewProxyModeCoordinator("test-room", "local-peer", config)
	if err != nil {
		t.Fatalf("Failed to create coordinator: %v", err)
	}
	defer pmc.Close()

	pmc.Start()

	// 让它运行一会
	time.Sleep(200 * time.Millisecond)

	// 验证没有崩溃
	status := pmc.GetStatus()
	t.Logf("Status after start: %v", status)
}

// ==========================================
// Benchmarks
// ==========================================

func BenchmarkCoordinatorAddPeer(b *testing.B) {
	config := DefaultCoordinatorConfig()
	pmc, err := NewProxyModeCoordinator("bench-room", "local-peer", config)
	if err != nil {
		b.Fatalf("Failed to create coordinator: %v", err)
	}
	defer pmc.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pmc.AddPeer("peer-1", 1, 1, 1)
	}
}

func BenchmarkCoordinatorHandlePong(b *testing.B) {
	config := DefaultCoordinatorConfig()
	pmc, err := NewProxyModeCoordinator("bench-room", "local-peer", config)
	if err != nil {
		b.Fatalf("Failed to create coordinator: %v", err)
	}
	defer pmc.Close()

	pmc.AddPeer("peer-1", 1, 1, 1)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pmc.HandlePong("peer-1")
	}
}
