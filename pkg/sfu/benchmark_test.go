/*
 * @Author: Marlon.M
 * @Email: maiguangyang@163.com
 * @Date: 2025-12-24
 *
 * Comprehensive Benchmarks
 * 全面基准测试：覆盖关键路径、并发场景和内存分配
 */
package sfu

import (
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/maiguangyang/relay_core/pkg/election"
	"github.com/pion/rtp"
)

// ==========================================
// RTP 转发路径基准测试
// ==========================================

func BenchmarkRTPPacketForwarding(b *testing.B) {
	switcher, _ := NewSourceSwitcher("bench-room")
	defer switcher.Close()

	packet := &rtp.Packet{
		Header: rtp.Header{
			Version:        2,
			PayloadType:    96,
			SequenceNumber: 0,
			Timestamp:      0,
			SSRC:           0x12345678,
		},
		Payload: make([]byte, 1200),
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		packet.SequenceNumber = uint16(i)
		packet.Timestamp = uint32(i) * 3000
		data, _ := packet.Marshal()
		switcher.InjectSFUPacket(true, data)
	}
}

func BenchmarkRTPPacketForwarding_Parallel(b *testing.B) {
	switcher, _ := NewSourceSwitcher("bench-room")
	defer switcher.Close()

	b.ResetTimer()
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		packet := &rtp.Packet{
			Header: rtp.Header{
				Version:     2,
				PayloadType: 96,
				SSRC:        0x12345678,
			},
			Payload: make([]byte, 1200),
		}
		seq := uint16(0)
		for pb.Next() {
			seq++
			packet.SequenceNumber = seq
			data, _ := packet.Marshal()
			switcher.InjectSFUPacket(true, data)
		}
	})
}

// ==========================================
// 选举算法基准测试
// ==========================================

func BenchmarkElectionScoring(b *testing.B) {
	config := election.DefaultElectorConfig()
	elector := election.NewElector("bench-room", config)
	defer elector.Close()

	// 添加 10 个候选者
	for i := 0; i < 10; i++ {
		elector.UpdateCandidate(election.Candidate{
			PeerID:         string(rune('A' + i)),
			DeviceType:     election.DeviceType(i%4 + 1),
			ConnectionType: election.ConnectionType(i%3 + 1),
			Bandwidth:      int64((10 - i) * 10 * 1024 * 1024),
			Latency:        int64(i * 10),
			PacketLoss:     float64(i) * 0.01,
		})
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		elector.Elect()
	}
}

func BenchmarkElectionWithNetworkUpdate(b *testing.B) {
	config := election.DefaultElectorConfig()
	elector := election.NewElector("bench-room", config)
	defer elector.Close()

	for i := 0; i < 10; i++ {
		elector.UpdateCandidate(election.Candidate{
			PeerID:     string(rune('A' + i)),
			DeviceType: election.DeviceTypePC,
		})
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// 模拟网络指标更新
		elector.UpdateNetworkMetrics("A", 50*1024*1024, int64(i%100), float64(i%10)*0.01)
		elector.Elect()
	}
}

// ==========================================
// Failover 基准测试
// ==========================================

func BenchmarkFailoverClaimProcessing(b *testing.B) {
	config := election.DefaultElectorConfig()
	elector := election.NewElector("bench-room", config)
	defer elector.Close()

	failoverConfig := FailoverConfig{
		BackoffPerPoint:  10 * time.Millisecond,
		MaxBackoff:       500 * time.Millisecond,
		ClaimTimeout:     100 * time.Millisecond,
		OfflineThreshold: 2,
	}

	fm := NewFailoverManager("bench-room", "local-peer", elector, nil, failoverConfig)
	defer fm.Close()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		fm.ReceiveRelayClaim("peer-"+string(rune('A'+i%26)), uint64(i), 80.0)
	}
}

// ==========================================
// Coordinator 并发基准测试
// ==========================================

func BenchmarkCoordinatorConcurrentPeerManagement(b *testing.B) {
	config := DefaultCoordinatorConfig()
	pmc, _ := NewProxyModeCoordinator("bench-room", "local-peer", config)
	defer pmc.Close()

	b.ResetTimer()
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		id := 0
		for pb.Next() {
			id++
			peerID := "peer-" + string(rune('A'+id%26))
			pmc.AddPeer(peerID, 1, 1, 1)
			pmc.HandlePong(peerID)
			pmc.RemovePeer(peerID)
		}
	})
}

func BenchmarkCoordinatorGetStatus(b *testing.B) {
	config := DefaultCoordinatorConfig()
	pmc, _ := NewProxyModeCoordinator("bench-room", "local-peer", config)
	defer pmc.Close()

	// 添加 20 个 Peer
	for i := 0; i < 20; i++ {
		pmc.AddPeer("peer-"+string(rune('A'+i)), i%4+1, i%3+1, i%3+1)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		pmc.GetStatus()
	}
}

// ==========================================
// JitterBuffer 高吞吐基准测试
// ==========================================

func BenchmarkJitterBufferHighThroughput(b *testing.B) {
	config := JitterBufferConfig{
		Enabled:     true,
		TargetDelay: 50 * time.Millisecond,
		MinDelay:    20 * time.Millisecond,
		MaxDelay:    200 * time.Millisecond,
		MaxPackets:  500,
	}

	jb := NewJitterBuffer(config)
	defer jb.Close()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		packet := &rtp.Packet{
			Header: rtp.Header{
				SequenceNumber: uint16(i),
				Timestamp:      uint32(i) * 3000,
			},
			Payload: make([]byte, 1200),
		}
		jb.Push(packet)
	}
}

func BenchmarkJitterBufferPopThroughput(b *testing.B) {
	config := JitterBufferConfig{
		Enabled:     true,
		TargetDelay: 10 * time.Millisecond,
		MinDelay:    5 * time.Millisecond,
		MaxDelay:    50 * time.Millisecond,
		MaxPackets:  500,
	}

	jb := NewJitterBuffer(config)
	defer jb.Close()

	// 预填充
	for i := 0; i < 500; i++ {
		jb.Push(&rtp.Packet{
			Header: rtp.Header{SequenceNumber: uint16(i), Timestamp: uint32(i) * 3000},
		})
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		jb.Pop()
		// 补充
		jb.Push(&rtp.Packet{
			Header: rtp.Header{SequenceNumber: uint16(500 + i), Timestamp: uint32(500+i) * 3000},
		})
	}
}

// ==========================================
// 内存分配基准测试
// ==========================================

func BenchmarkMemoryAllocation_RTPPacket(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		packet := &rtp.Packet{
			Header: rtp.Header{
				Version:        2,
				PayloadType:    96,
				SequenceNumber: uint16(i),
				Timestamp:      uint32(i) * 3000,
				SSRC:           0x12345678,
			},
			Payload: make([]byte, 1200),
		}
		_, _ = packet.Marshal()
	}
}

func BenchmarkMemoryAllocation_BufferPool(b *testing.B) {
	pool := NewBufferPool()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		buf := pool.GetBuffer()
		pool.PutBuffer(buf)
	}
}

// ==========================================
// 并发竞争基准测试
// ==========================================

func BenchmarkContention_SourceSwitcher(b *testing.B) {
	switcher, _ := NewSourceSwitcher("contention-room")
	defer switcher.Close()

	var ops int64

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		packet := make([]byte, 1200)
		for pb.Next() {
			op := atomic.AddInt64(&ops, 1)
			if op%2 == 0 {
				switcher.InjectSFUPacket(true, packet)
			} else {
				switcher.InjectLocalPacket(true, packet)
			}
		}
	})
}

func BenchmarkContention_TrafficStats(b *testing.B) {
	stats := NewTrafficStats()

	b.ResetTimer()
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			stats.AddBytesIn(1200)
			stats.AddPacketIn()
		}
	})
}

func BenchmarkContention_RoomStats(b *testing.B) {
	rs := NewRoomStats("contention-room")

	// 预创建 Peer
	for i := 0; i < 20; i++ {
		rs.GetOrCreatePeerStats("peer-" + string(rune('A'+i)))
	}

	b.ResetTimer()
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			i++
			peerStats := rs.GetOrCreatePeerStats("peer-" + string(rune('A'+i%20)))
			peerStats.AddBytesIn(1200)
		}
	})
}

// ==========================================
// 吞吐量极限测试
// ==========================================

func BenchmarkThroughput_1Mbps(b *testing.B) {
	benchmarkThroughput(b, 1*1024*1024)
}

func BenchmarkThroughput_10Mbps(b *testing.B) {
	benchmarkThroughput(b, 10*1024*1024)
}

func BenchmarkThroughput_100Mbps(b *testing.B) {
	benchmarkThroughput(b, 100*1024*1024)
}

func benchmarkThroughput(b *testing.B, targetBitsPerSec int) {
	switcher, _ := NewSourceSwitcher("throughput-room")
	defer switcher.Close()

	packetSize := 1200
	packetsPerSec := targetBitsPerSec / 8 / packetSize
	interval := time.Second / time.Duration(packetsPerSec)

	packet := make([]byte, packetSize)

	b.ResetTimer()

	start := time.Now()
	sent := 0
	for i := 0; i < b.N && time.Since(start) < time.Second; i++ {
		switcher.InjectSFUPacket(true, packet)
		sent++
		time.Sleep(interval)
	}

	elapsed := time.Since(start)
	actualBps := float64(sent*packetSize*8) / elapsed.Seconds()
	b.ReportMetric(actualBps/1024/1024, "Mbps")
}

// ==========================================
// GC 压力测试
// ==========================================

func BenchmarkGCPressure(b *testing.B) {
	var m1, m2 runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&m1)

	switcher, _ := NewSourceSwitcher("gc-room")
	jb := NewJitterBuffer(JitterBufferConfig{Enabled: true, MaxPackets: 100})
	stats := NewTrafficStats()

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		packet := &rtp.Packet{
			Header:  rtp.Header{SequenceNumber: uint16(i), Timestamp: uint32(i) * 3000},
			Payload: make([]byte, 1200),
		}
		data, _ := packet.Marshal()
		switcher.InjectSFUPacket(true, data)
		jb.Push(packet)
		stats.AddBytesIn(uint64(len(data)))
	}

	runtime.GC()
	runtime.ReadMemStats(&m2)

	b.ReportMetric(float64(m2.NumGC-m1.NumGC), "GCs")
	b.ReportMetric(float64(m2.TotalAlloc-m1.TotalAlloc)/1024/1024, "MB_alloc")

	switcher.Close()
	jb.Close()
}

// ==========================================
// Latency 分布测试
// ==========================================

func BenchmarkLatencyDistribution(b *testing.B) {
	switcher, _ := NewSourceSwitcher("latency-room")
	defer switcher.Close()

	packet := make([]byte, 1200)
	latencies := make([]time.Duration, 0, b.N)
	var mu sync.Mutex

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		start := time.Now()
		switcher.InjectSFUPacket(true, packet)
		elapsed := time.Since(start)

		mu.Lock()
		latencies = append(latencies, elapsed)
		mu.Unlock()
	}

	if len(latencies) > 0 {
		var total time.Duration
		min := latencies[0]
		max := latencies[0]
		for _, l := range latencies {
			total += l
			if l < min {
				min = l
			}
			if l > max {
				max = l
			}
		}
		avg := total / time.Duration(len(latencies))
		b.ReportMetric(float64(avg.Nanoseconds()), "avg_ns")
		b.ReportMetric(float64(min.Nanoseconds()), "min_ns")
		b.ReportMetric(float64(max.Nanoseconds()), "max_ns")
	}
}
