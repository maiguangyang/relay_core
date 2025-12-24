/*
 * @Author: Marlon.M
 * @Email: maiguangyang@163.com
 * @Date: 2025-12-24
 *
 * Realistic RTP Scenario Benchmarks
 * 模拟真实 RTP 转发场景的性能测试
 */
package sfu

import (
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ==========================================
// 场景 1: 持续视频流转发
// 模拟 1080p @ 30fps 视频流（约 3 Mbps）
// ==========================================

func TestScenario_ContinuousVideoStream(t *testing.T) {
	// 配置
	const (
		duration    = 5 * time.Second // 测试持续时间
		fps         = 30              // 帧率
		bitrateKbps = 3000            // 码率 3 Mbps
		packetSize  = 1200            // 每个 RTP 包大小
	)

	packetsPerSecond := (bitrateKbps * 1000 / 8) / packetSize // 约 312 包/秒
	packetInterval := time.Second / time.Duration(packetsPerSecond)

	t.Logf("=== 持续视频流测试 ===")
	t.Logf("码率: %d Kbps", bitrateKbps)
	t.Logf("包大小: %d bytes", packetSize)
	t.Logf("每秒包数: %d", packetsPerSecond)
	t.Logf("发送间隔: %v", packetInterval)

	// 创建 SourceSwitcher
	switcher, err := NewSourceSwitcher("benchmark-room")
	if err != nil {
		t.Fatalf("创建失败: %v", err)
	}
	defer switcher.Close()

	// 统计
	var (
		packetsSent  uint64
		totalLatency int64 // 纳秒
		maxLatency   int64
		minLatency   int64 = int64(time.Hour)
	)

	// 记录内存使用
	var memBefore, memAfter runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&memBefore)

	startTime := time.Now()
	ticker := time.NewTicker(packetInterval)
	defer ticker.Stop()

	seq := uint16(0)

	for {
		select {
		case <-ticker.C:
			// 创建模拟 RTP 包
			rtpPacket := createTestRTPPacket(seq, packetSize)
			seq++

			// 测量注入延迟
			injectStart := time.Now()
			err := switcher.InjectSFUPacket(true, rtpPacket)
			latency := time.Since(injectStart).Nanoseconds()

			if err == nil {
				atomic.AddUint64(&packetsSent, 1)
				atomic.AddInt64(&totalLatency, latency)

				// 更新最大延迟
				for {
					current := atomic.LoadInt64(&maxLatency)
					if latency <= current || atomic.CompareAndSwapInt64(&maxLatency, current, latency) {
						break
					}
				}
				// 更新最小延迟
				for {
					current := atomic.LoadInt64(&minLatency)
					if latency >= current || atomic.CompareAndSwapInt64(&minLatency, current, latency) {
						break
					}
				}
			}

		default:
			if time.Since(startTime) >= duration {
				goto done
			}
		}
	}

done:
	runtime.ReadMemStats(&memAfter)

	// 输出结果
	sent := atomic.LoadUint64(&packetsSent)
	avgLatency := time.Duration(atomic.LoadInt64(&totalLatency) / int64(sent))
	actualBitrate := float64(sent*uint64(packetSize)*8) / duration.Seconds() / 1000

	t.Logf("\n=== 测试结果 ===")
	t.Logf("持续时间: %v", duration)
	t.Logf("发送包数: %d", sent)
	t.Logf("实际吞吐: %.2f Kbps", actualBitrate)
	t.Logf("平均延迟: %v", avgLatency)
	t.Logf("最小延迟: %v", time.Duration(atomic.LoadInt64(&minLatency)))
	t.Logf("最大延迟: %v", time.Duration(atomic.LoadInt64(&maxLatency)))
	t.Logf("内存增长: %d KB", (memAfter.Alloc-memBefore.Alloc)/1024)
}

// ==========================================
// 场景 2: 多订阅者压力测试
// 模拟 1 个 Relay 同时向多个订阅者转发
// ==========================================

func TestScenario_MultipleSubscribers(t *testing.T) {
	subscriberCounts := []int{1, 5, 10, 20}

	for _, count := range subscriberCounts {
		t.Run(fmt.Sprintf("%d_subscribers", count), func(t *testing.T) {
			testMultiSubscriber(t, count)
		})
	}
}

func testMultiSubscriber(t *testing.T, subscriberCount int) {
	const (
		duration   = 3 * time.Second
		fps        = 30
		packetSize = 1200
	)

	packetsPerSecond := fps * 10 // 约 300 包/秒（视频+音频）
	packetInterval := time.Second / time.Duration(packetsPerSecond)

	// 创建 RelayRoom（模拟多订阅者场景）
	// 注意：由于 RelayRoom 需要真正的 WebRTC 连接，
	// 这里我们模拟多个 SourceSwitcher 并行写入来测试性能

	switchers := make([]*SourceSwitcher, subscriberCount)
	for i := 0; i < subscriberCount; i++ {
		sw, err := NewSourceSwitcher(fmt.Sprintf("room-%d", i))
		if err != nil {
			t.Fatalf("创建 SourceSwitcher 失败: %v", err)
		}
		switchers[i] = sw
	}
	defer func() {
		for _, sw := range switchers {
			sw.Close()
		}
	}()

	var (
		totalPackets uint64
		totalBytes   uint64
	)

	var wg sync.WaitGroup
	startTime := time.Now()

	// 启动数据注入
	for _, sw := range switchers {
		wg.Add(1)
		go func(switcher *SourceSwitcher) {
			defer wg.Done()

			ticker := time.NewTicker(packetInterval)
			defer ticker.Stop()

			seq := uint16(0)
			for time.Since(startTime) < duration {
				<-ticker.C
				packet := createTestRTPPacket(seq, packetSize)
				seq++
				if switcher.InjectSFUPacket(true, packet) == nil {
					atomic.AddUint64(&totalPackets, 1)
					atomic.AddUint64(&totalBytes, uint64(packetSize))
				}
			}
		}(sw)
	}

	wg.Wait()

	packets := atomic.LoadUint64(&totalPackets)
	bytes := atomic.LoadUint64(&totalBytes)
	packetsPerSec := float64(packets) / duration.Seconds()
	bytesPerSec := float64(bytes) / duration.Seconds()

	t.Logf("订阅者数量: %d", subscriberCount)
	t.Logf("总包数: %d", packets)
	t.Logf("包/秒: %.0f", packetsPerSec)
	t.Logf("吞吐量: %.2f Mbps", bytesPerSec*8/1000000)
}

// ==========================================
// 场景 3: 源切换延迟测试
// 测量在流转发过程中切换源的延迟和中断时间
// ==========================================

func TestScenario_SourceSwitchLatency(t *testing.T) {
	const (
		testRounds = 100 // 测试 100 次切换
	)

	switcher, err := NewSourceSwitcher("switch-test-room")
	if err != nil {
		t.Fatalf("创建失败: %v", err)
	}
	defer switcher.Close()

	var (
		totalSwitchTime int64
		maxSwitchTime   int64
		minSwitchTime   int64 = int64(time.Hour)
	)

	// 记录切换时间
	switchDone := make(chan time.Duration, 1)
	switcher.SetOnSourceChanged(func(roomID string, source SourceType, sharerID string) {
		// 回调触发时记录
	})

	for i := 0; i < testRounds; i++ {
		// 切换到 Local
		startSwitch := time.Now()
		switcher.StartLocalShare(fmt.Sprintf("sharer-%d", i))
		switchLatency := time.Since(startSwitch)

		atomic.AddInt64(&totalSwitchTime, switchLatency.Nanoseconds())
		if switchLatency.Nanoseconds() > atomic.LoadInt64(&maxSwitchTime) {
			atomic.StoreInt64(&maxSwitchTime, switchLatency.Nanoseconds())
		}
		if switchLatency.Nanoseconds() < atomic.LoadInt64(&minSwitchTime) {
			atomic.StoreInt64(&minSwitchTime, switchLatency.Nanoseconds())
		}

		// 切换回 SFU
		switcher.StopLocalShare()
	}

	close(switchDone)

	avgSwitch := time.Duration(atomic.LoadInt64(&totalSwitchTime) / int64(testRounds))

	t.Logf("=== 源切换延迟测试 ===")
	t.Logf("测试次数: %d", testRounds)
	t.Logf("平均切换延迟: %v", avgSwitch)
	t.Logf("最小切换延迟: %v", time.Duration(atomic.LoadInt64(&minSwitchTime)))
	t.Logf("最大切换延迟: %v", time.Duration(atomic.LoadInt64(&maxSwitchTime)))

	// 断言切换延迟应该非常快（< 1ms）
	if avgSwitch > time.Millisecond {
		t.Errorf("平均切换延迟过高: %v (期望 < 1ms)", avgSwitch)
	}
}

// ==========================================
// 场景 4: 高频包转发压力测试
// 测试极限吞吐量
// ==========================================

func BenchmarkScenario_MaxThroughput(b *testing.B) {
	switcher, err := NewSourceSwitcher("throughput-room")
	if err != nil {
		b.Fatalf("创建失败: %v", err)
	}
	defer switcher.Close()

	packetSize := 1200
	packet := createTestRTPPacket(0, packetSize)

	var totalBytes int64

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		localPacket := make([]byte, packetSize)
		copy(localPacket, packet)
		seq := uint16(0)

		for pb.Next() {
			// 更新序列号
			localPacket[2] = byte(seq >> 8)
			localPacket[3] = byte(seq)
			seq++

			if switcher.InjectSFUPacket(true, localPacket) == nil {
				atomic.AddInt64(&totalBytes, int64(packetSize))
			}
		}
	})

	b.StopTimer()

	// 计算吞吐量
	bytes := atomic.LoadInt64(&totalBytes)
	bytesPerOp := float64(bytes) / float64(b.N)

	b.ReportMetric(bytesPerOp, "bytes/op")
	b.ReportMetric(bytesPerOp*8/1000000, "Mbits/op")
}

// ==========================================
// 场景 5: 长时间稳定性测试
// ==========================================

func TestScenario_LongRunningStability(t *testing.T) {
	if testing.Short() {
		t.Skip("跳过长时间测试")
	}

	const (
		duration   = 30 * time.Second
		fps        = 30
		packetSize = 1200
	)

	t.Logf("=== 长时间稳定性测试 (%v) ===", duration)

	switcher, err := NewSourceSwitcher("stability-room")
	if err != nil {
		t.Fatalf("创建失败: %v", err)
	}
	defer switcher.Close()

	var (
		packetsSent  uint64
		packetsError uint64
	)

	// 记录内存
	var memStart runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&memStart)

	packetsPerSecond := fps * 10
	packetInterval := time.Second / time.Duration(packetsPerSecond)
	ticker := time.NewTicker(packetInterval)
	defer ticker.Stop()

	startTime := time.Now()
	seq := uint16(0)

	// 定期采样
	sampleInterval := 5 * time.Second
	lastSample := startTime
	lastPackets := uint64(0)

	for time.Since(startTime) < duration {
		<-ticker.C

		packet := createTestRTPPacket(seq, packetSize)
		seq++

		if switcher.InjectSFUPacket(true, packet) == nil {
			atomic.AddUint64(&packetsSent, 1)
		} else {
			atomic.AddUint64(&packetsError, 1)
		}

		// 每 5 秒输出采样
		if time.Since(lastSample) >= sampleInterval {
			currentPackets := atomic.LoadUint64(&packetsSent)
			pps := float64(currentPackets-lastPackets) / sampleInterval.Seconds()
			t.Logf("[%v] 包/秒: %.0f", time.Since(startTime).Round(time.Second), pps)
			lastPackets = currentPackets
			lastSample = time.Now()
		}
	}

	// 最终统计
	var memEnd runtime.MemStats
	runtime.ReadMemStats(&memEnd)

	sent := atomic.LoadUint64(&packetsSent)
	errors := atomic.LoadUint64(&packetsError)

	t.Logf("\n=== 最终结果 ===")
	t.Logf("总包数: %d", sent)
	t.Logf("错误数: %d", errors)
	t.Logf("平均包/秒: %.0f", float64(sent)/duration.Seconds())
	t.Logf("内存增长: %d KB", (memEnd.Alloc-memStart.Alloc)/1024)
	t.Logf("GC 次数: %d", memEnd.NumGC-memStart.NumGC)

	// 断言无错误
	if errors > 0 {
		t.Errorf("发现 %d 个错误", errors)
	}
}

// ==========================================
// 辅助函数
// ==========================================

func createTestRTPPacket(seq uint16, size int) []byte {
	if size < 12 {
		size = 12
	}
	packet := make([]byte, size)

	// RTP Header (12 bytes)
	packet[0] = 0x80 // Version 2, no padding, no extension
	packet[1] = 96   // Payload type (dynamic)
	packet[2] = byte(seq >> 8)
	packet[3] = byte(seq)

	// Timestamp (假设 90kHz 时钟)
	ts := uint32(seq) * 3000 // 30fps = 每帧 3000 ticks
	packet[4] = byte(ts >> 24)
	packet[5] = byte(ts >> 16)
	packet[6] = byte(ts >> 8)
	packet[7] = byte(ts)

	// SSRC
	packet[8] = 0x12
	packet[9] = 0x34
	packet[10] = 0x56
	packet[11] = 0x78

	// Payload (填充随机数据)
	for i := 12; i < size; i++ {
		packet[i] = byte(i % 256)
	}

	return packet
}
