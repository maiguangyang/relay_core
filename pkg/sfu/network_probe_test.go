/*
 * @Author: Marlon.M
 * @Email: maiguangyang@163.com
 * @Date: 2025-12-24
 *
 * NetworkProbe Tests
 * 测试网络质量探测功能
 * 注意：需要 mock PeerConnection 或使用 NetworkProbeManager
 */
package sfu

import (
	"testing"
	"time"
)

func TestNetworkProbeManagerCreate(t *testing.T) {
	manager := NewNetworkProbeManager()
	if manager == nil {
		t.Fatal("NetworkProbeManager should not be nil")
	}
	manager.StopAll()
}

func TestNetworkProbeManagerGetMetrics(t *testing.T) {
	manager := NewNetworkProbeManager()
	defer manager.StopAll()

	// 没有添加任何探测器，GetMetrics 应该返回 nil
	metrics := manager.GetMetrics("non-existent")
	if metrics != nil {
		t.Error("Should return nil for non-existent peer")
	}
}

func TestNetworkProbeManagerGetAllMetrics(t *testing.T) {
	manager := NewNetworkProbeManager()
	defer manager.StopAll()

	// 没有添加任何探测器，应该返回空 map
	allMetrics := manager.GetAllMetrics()
	if len(allMetrics) != 0 {
		t.Errorf("Should return empty map, got %d items", len(allMetrics))
	}
}

func TestNetworkMetricsStruct(t *testing.T) {
	// 测试 NetworkMetrics 结构体
	metrics := NetworkMetrics{
		RTT:             50 * time.Millisecond,
		Jitter:          10 * time.Millisecond,
		PacketLoss:      0.01,
		PacketsSent:     1000,
		PacketsReceived: 998,
		BytesSent:       1000000,
		BytesReceived:   2000000,
		QualityScore:    85.0,
		Timestamp:       time.Now(),
	}

	if metrics.PacketLoss != 0.01 {
		t.Errorf("Expected packet loss 0.01, got %f", metrics.PacketLoss)
	}

	if metrics.QualityScore != 85.0 {
		t.Errorf("Expected quality score 85.0, got %f", metrics.QualityScore)
	}
}

// ==========================================
// 测试使用 NetworkProbe 的基本逻辑（不需要真实连接）
// ==========================================

func TestNetworkProbeWithNilPC(t *testing.T) {
	// 使用 nil PeerConnection 创建探测器（用于测试基本结构）
	probe := NewNetworkProbe(nil)
	if probe == nil {
		t.Fatal("NetworkProbe should not be nil even with nil PC")
	}

	// 验证基本方法不崩溃
	if probe.IsRunning() {
		t.Error("Should not be running initially")
	}

	latest := probe.GetLatest()
	t.Logf("Latest metrics: %+v", latest)

	history := probe.GetHistory()
	if len(history) != 0 {
		t.Errorf("History should be empty, got %d items", len(history))
	}

	avg := probe.GetAverage()
	t.Logf("Average metrics: %+v", avg)

	// Stop 应该安全
	probe.Stop()
}

func TestNetworkProbeSetCallbacks(t *testing.T) {
	probe := NewNetworkProbe(nil)
	defer probe.Stop()

	called := false
	probe.SetOnMetricsUpdated(func(metrics NetworkMetrics) {
		called = true
	})

	// 回调设置成功（不会直接触发）
	if called {
		t.Error("Callback should not be called yet")
	}
}

func TestNetworkProbeSetInterval(t *testing.T) {
	probe := NewNetworkProbe(nil)
	defer probe.Stop()

	// 设置间隔应该不崩溃
	probe.SetInterval(1 * time.Second)
}

// ==========================================
// Benchmarks
// ==========================================

func BenchmarkNetworkProbeManagerCreate(b *testing.B) {
	for i := 0; i < b.N; i++ {
		manager := NewNetworkProbeManager()
		manager.StopAll()
	}
}

func BenchmarkNetworkProbeManagerGetAllMetrics(b *testing.B) {
	manager := NewNetworkProbeManager()
	defer manager.StopAll()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		manager.GetAllMetrics()
	}
}

func BenchmarkNetworkProbeGetLatest(b *testing.B) {
	probe := NewNetworkProbe(nil)
	defer probe.Stop()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		probe.GetLatest()
	}
}
