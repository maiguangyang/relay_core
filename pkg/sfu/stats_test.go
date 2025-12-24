/*
 * @Author: Marlon.M
 * @Email: maiguangyang@163.com
 * @Date: 2025-12-24
 *
 * Stats Tests
 * 测试流量统计功能
 */
package sfu

import (
	"testing"
	"time"
)

func TestTrafficStatsCreate(t *testing.T) {
	stats := NewTrafficStats()
	if stats == nil {
		t.Fatal("TrafficStats should not be nil")
	}
}

func TestTrafficStatsAddBytes(t *testing.T) {
	stats := NewTrafficStats()

	stats.AddBytesIn(1000)
	stats.AddBytesIn(2000)
	stats.AddBytesOut(500)

	snapshot := stats.Snapshot()
	if snapshot.TotalBytesIn != 3000 {
		t.Errorf("Expected 3000 bytes in, got %d", snapshot.TotalBytesIn)
	}

	if snapshot.TotalBytesOut != 500 {
		t.Errorf("Expected 500 bytes out, got %d", snapshot.TotalBytesOut)
	}
}

func TestTrafficStatsAddPacketLoss(t *testing.T) {
	stats := NewTrafficStats()

	stats.AddPacketLost()
	stats.AddPacketLost()
	stats.AddPacketLost()

	snapshot := stats.Snapshot()
	if snapshot.PacketsLost != 3 {
		t.Errorf("Expected 3 packets lost, got %d", snapshot.PacketsLost)
	}
}

func TestTrafficStatsBitrateCalculation(t *testing.T) {
	stats := NewTrafficStats()

	// 模拟 1 秒内传输 1 MB
	stats.AddBytesIn(1024 * 1024)

	// 等待一段时间
	time.Sleep(100 * time.Millisecond)

	// 计算码率
	stats.CalculateBitrate()

	snapshot := stats.Snapshot()
	t.Logf("Bitrate In: %.2f Mbps", snapshot.BitrateIn/1000000)
	t.Logf("Bitrate Out: %.2f Mbps", snapshot.BitrateOut/1000000)
}

func TestTrafficStatsLossRate(t *testing.T) {
	stats := NewTrafficStats()

	// 模拟接收 1000 个包，丢失 10 个
	for i := 0; i < 1000; i++ {
		stats.AddPacketIn()
	}
	for i := 0; i < 10; i++ {
		stats.AddPacketLost()
	}

	lossRate := stats.GetLossRate()
	t.Logf("Loss rate: %.2f%%", lossRate*100)

	// 丢包率应该接近 1%
	if lossRate < 0.005 || lossRate > 0.015 {
		t.Errorf("Expected loss rate around 1%%, got %.2f%%", lossRate*100)
	}
}

func TestTrafficStatsWindow(t *testing.T) {
	stats := NewTrafficStats()

	stats.AddBytesIn(1000)
	stats.AddBytesOut(500)
	stats.AddPacketIn()
	stats.AddPacketOut()

	duration, bytesIn, bytesOut, packetsIn, packetsOut := stats.GetWindowStats()
	t.Logf("Window duration: %v", duration)
	t.Logf("Bytes in: %d, out: %d", bytesIn, bytesOut)
	t.Logf("Packets in: %d, out: %d", packetsIn, packetsOut)

	if bytesIn != 1000 {
		t.Errorf("Expected 1000 bytes in window, got %d", bytesIn)
	}

	// 重置窗口
	stats.ResetWindow()

	_, bytesIn2, _, _, _ := stats.GetWindowStats()
	if bytesIn2 != 0 {
		t.Errorf("Expected 0 bytes after reset, got %d", bytesIn2)
	}
}

func TestRoomStatsCreate(t *testing.T) {
	rs := NewRoomStats("test-room")
	if rs == nil {
		t.Fatal("RoomStats should not be nil")
	}
}

func TestRoomStatsGetOrCreatePeerStats(t *testing.T) {
	rs := NewRoomStats("test-room")

	stats1 := rs.GetOrCreatePeerStats("peer-1")
	stats2 := rs.GetOrCreatePeerStats("peer-2")

	if stats1 == nil || stats2 == nil {
		t.Fatal("Peer stats should not be nil")
	}

	// 同一个 PeerID 应返回同一个实例
	stats1Again := rs.GetOrCreatePeerStats("peer-1")
	if stats1 != stats1Again {
		t.Error("Should return same instance for same peerID")
	}
}

func TestRoomStatsRemovePeerStats(t *testing.T) {
	rs := NewRoomStats("test-room")

	rs.GetOrCreatePeerStats("peer-1")
	rs.GetOrCreatePeerStats("peer-2")

	snapshot := rs.Snapshot()
	if snapshot.PeerCount != 2 {
		t.Errorf("Expected 2 peers, got %d", snapshot.PeerCount)
	}

	rs.RemovePeerStats("peer-1")

	snapshot = rs.Snapshot()
	if snapshot.PeerCount != 1 {
		t.Errorf("Expected 1 peer after removal, got %d", snapshot.PeerCount)
	}
}

func TestRoomStatsSnapshot(t *testing.T) {
	rs := NewRoomStats("test-room")

	peerStats := rs.GetOrCreatePeerStats("peer-1")
	peerStats.AddBytesIn(1000)
	peerStats.AddBytesOut(500)

	rs.GetTraffic().AddBytesIn(2000)

	snapshot := rs.Snapshot()

	if snapshot.RoomID != "test-room" {
		t.Errorf("Expected room ID 'test-room', got %s", snapshot.RoomID)
	}

	if snapshot.Traffic.TotalBytesIn != 2000 {
		t.Errorf("Expected 2000 total bytes in, got %d", snapshot.Traffic.TotalBytesIn)
	}

	if len(snapshot.PeerStats) != 1 {
		t.Errorf("Expected 1 peer stat, got %d", len(snapshot.PeerStats))
	}

	t.Logf("Snapshot JSON: %s", snapshot.ToJSON())
}

func TestRoomStatsCalculateAllBitrates(t *testing.T) {
	rs := NewRoomStats("test-room")

	rs.GetTraffic().AddBytesIn(1000000)
	peerStats := rs.GetOrCreatePeerStats("peer-1")
	peerStats.AddBytesIn(500000)

	time.Sleep(100 * time.Millisecond)

	rs.CalculateAllBitrates()

	snapshot := rs.Snapshot()
	t.Logf("Total bitrate in: %.2f Mbps", snapshot.Traffic.BitrateIn/1000000)
}

// ==========================================
// Benchmarks
// ==========================================

func BenchmarkTrafficStatsAddBytesIn(b *testing.B) {
	stats := NewTrafficStats()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		stats.AddBytesIn(1200)
	}
}

func BenchmarkTrafficStatsAddBytesParallel(b *testing.B) {
	stats := NewTrafficStats()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			stats.AddBytesIn(1200)
		}
	})
}

func BenchmarkTrafficStatsSnapshot(b *testing.B) {
	stats := NewTrafficStats()
	stats.AddBytesIn(1000000)
	stats.AddBytesOut(500000)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		stats.Snapshot()
	}
}

func BenchmarkRoomStatsGetOrCreatePeerStats(b *testing.B) {
	rs := NewRoomStats("bench-room")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rs.GetOrCreatePeerStats("peer-1")
	}
}
