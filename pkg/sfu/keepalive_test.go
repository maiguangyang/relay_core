/*
 * @Author: Marlon.M
 * @Email: maiguangyang@163.com
 * @Date: 2025-12-24
 *
 * Keepalive Tests
 */
package sfu

import (
	"testing"
	"time"
)

func TestKeepaliveCreate(t *testing.T) {
	config := DefaultKeepaliveConfig()
	km := NewKeepaliveManager(config)
	defer km.Stop()

	if km == nil {
		t.Fatal("KeepaliveManager should not be nil")
	}
}

func TestKeepaliveAddPeer(t *testing.T) {
	config := DefaultKeepaliveConfig()
	km := NewKeepaliveManager(config)
	defer km.Stop()

	km.AddPeer("peer-1")
	km.AddPeer("peer-2")

	statuses := km.GetAllPeerStatus()
	if len(statuses) != 2 {
		t.Errorf("Expected 2 peers, got %d", len(statuses))
	}
}

func TestKeepaliveRemovePeer(t *testing.T) {
	config := DefaultKeepaliveConfig()
	km := NewKeepaliveManager(config)
	defer km.Stop()

	km.AddPeer("peer-1")
	km.AddPeer("peer-2")
	km.RemovePeer("peer-1")

	statuses := km.GetAllPeerStatus()
	if len(statuses) != 1 {
		t.Errorf("Expected 1 peer, got %d", len(statuses))
	}
}

func TestKeepaliveHandlePong(t *testing.T) {
	config := DefaultKeepaliveConfig()
	km := NewKeepaliveManager(config)
	defer km.Stop()

	km.AddPeer("peer-1")

	// 初始状态应该是 Online
	status := km.GetPeerStatus("peer-1")
	if status != PeerStatusOnline {
		t.Errorf("Expected Online status, got %v", status)
	}

	// 处理 Pong
	km.HandlePong("peer-1")

	// RTT 应该有值
	rtt := km.GetPeerRTT("peer-1")
	if rtt < 0 {
		t.Error("RTT should not be negative")
	}
}

func TestKeepaliveOfflineCallback(t *testing.T) {
	config := KeepaliveConfig{
		Interval:      50 * time.Millisecond,
		Timeout:       100 * time.Millisecond,
		SlowThreshold: 30 * time.Millisecond,
		MaxRetries:    2,
	}
	km := NewKeepaliveManager(config)

	offlineCalled := make(chan string, 1)
	km.SetOnPeerOffline(func(peerID string) {
		offlineCalled <- peerID
	})

	// 设置空的 ping 回调（不发送 ping）
	km.SetOnPing(func(peerID string) {})

	km.AddPeer("peer-1")
	km.Start()
	defer km.Stop()

	// 等待超时
	select {
	case peer := <-offlineCalled:
		if peer != "peer-1" {
			t.Errorf("Expected peer-1, got %s", peer)
		}
	case <-time.After(500 * time.Millisecond):
		t.Error("Offline callback not called")
	}
}

func TestKeepalivePeerInfo(t *testing.T) {
	config := DefaultKeepaliveConfig()
	km := NewKeepaliveManager(config)
	defer km.Stop()

	km.AddPeer("peer-1")

	info := km.GetPeerInfo("peer-1")
	if info == nil {
		t.Fatal("PeerInfo should not be nil")
	}

	if info.PeerID != "peer-1" {
		t.Errorf("Expected peer-1, got %s", info.PeerID)
	}
}

func TestKeepaliveAllPeerInfo(t *testing.T) {
	config := DefaultKeepaliveConfig()
	km := NewKeepaliveManager(config)
	defer km.Stop()

	km.AddPeer("peer-1")
	km.AddPeer("peer-2")
	km.AddPeer("peer-3")

	infos := km.GetAllPeerInfo()
	if len(infos) != 3 {
		t.Errorf("Expected 3 peer infos, got %d", len(infos))
	}
}

// ==========================================
// Benchmarks
// ==========================================

func BenchmarkKeepaliveHandlePong(b *testing.B) {
	config := DefaultKeepaliveConfig()
	km := NewKeepaliveManager(config)
	defer km.Stop()

	km.AddPeer("peer-1")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		km.HandlePong("peer-1")
	}
}

func BenchmarkKeepaliveGetStatus(b *testing.B) {
	config := DefaultKeepaliveConfig()
	km := NewKeepaliveManager(config)
	defer km.Stop()

	// 添加多个 peer
	for i := 0; i < 100; i++ {
		km.AddPeer(string(rune('a' + i%26)))
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		km.GetAllPeerStatus()
	}
}
