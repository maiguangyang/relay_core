/*
 * @Author: Marlon.M
 * @Email: maiguangyang@163.com
 * @Date: 2025-12-24
 *
 * Election Tests
 */
package election

import (
	"testing"
	"time"
)

func TestElectorCreate(t *testing.T) {
	config := DefaultElectorConfig()
	elector := NewElector("test-room", config)
	defer elector.Close()

	if elector.GetCurrentProxy() != "" {
		t.Error("Initial proxy should be empty")
	}
}

func TestElectorAddCandidate(t *testing.T) {
	config := DefaultElectorConfig()
	elector := NewElector("test-room", config)
	defer elector.Close()

	elector.UpdateCandidate(Candidate{
		PeerID:     "peer-1",
		Bandwidth:  10000000, // 10 Mbps
		Latency:    20,       // 20ms
		PacketLoss: 0.001,    // 0.1%
	})

	candidates := elector.GetCandidates()
	if len(candidates) != 1 {
		t.Errorf("Expected 1 candidate, got %d", len(candidates))
	}

	if candidates[0].PeerID != "peer-1" {
		t.Errorf("Expected peer-1, got %s", candidates[0].PeerID)
	}
}

func TestElectorElect(t *testing.T) {
	config := DefaultElectorConfig()
	elector := NewElector("test-room", config)
	defer elector.Close()

	// 添加多个候选者
	elector.UpdateCandidate(Candidate{
		PeerID:         "peer-mobile",
		DeviceType:     DeviceTypeMobile,
		ConnectionType: ConnectionTypeWiFi,
		PowerState:     PowerStateBattery,
		Bandwidth:      5000000,
		Latency:        50,
		PacketLoss:     0.01,
	})

	elector.UpdateCandidate(Candidate{
		PeerID:         "peer-pc",
		DeviceType:     DeviceTypePC,
		ConnectionType: ConnectionTypeEthernet,
		PowerState:     PowerStatePluggedIn,
		Bandwidth:      100000000,
		Latency:        10,
		PacketLoss:     0.0,
	})

	result := elector.Elect()
	if result == nil {
		t.Fatal("Election result should not be nil")
	}

	// PC 应该赢得选举
	if result.ProxyID != "peer-pc" {
		t.Errorf("Expected peer-pc to win, got %s", result.ProxyID)
	}
}

func TestElectorScoring(t *testing.T) {
	config := DefaultElectorConfig()
	elector := NewElector("test-room", config)
	defer elector.Close()

	testCases := []struct {
		name      string
		candidate Candidate
		minScore  float64
		maxScore  float64
	}{
		{
			name: "PC+Ethernet+PluggedIn (best)",
			candidate: Candidate{
				PeerID:         "best",
				DeviceType:     DeviceTypePC,
				ConnectionType: ConnectionTypeEthernet,
				PowerState:     PowerStatePluggedIn,
				Bandwidth:      100000000,
				Latency:        10,
				PacketLoss:     0,
			},
			minScore: 60, // Realistic score based on algorithm weights
			maxScore: 100,
		},
		{
			name: "Mobile+Cellular+LowBattery (worst)",
			candidate: Candidate{
				PeerID:         "worst",
				DeviceType:     DeviceTypeMobile,
				ConnectionType: ConnectionTypeCellular,
				PowerState:     PowerStateLowBattery,
				Bandwidth:      1000000,
				Latency:        200,
				PacketLoss:     0.05,
			},
			minScore: 0,
			maxScore: 20,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			elector.UpdateCandidate(tc.candidate)
			candidates := elector.GetCandidates()

			var found *Candidate
			for i := range candidates {
				if candidates[i].PeerID == tc.candidate.PeerID {
					found = &candidates[i]
					break
				}
			}

			if found == nil {
				t.Fatal("Candidate not found")
			}

			if found.Score < tc.minScore || found.Score > tc.maxScore {
				t.Errorf("Score %f not in expected range [%f, %f]",
					found.Score, tc.minScore, tc.maxScore)
			}
		})
	}
}

func TestElectorUpdateDeviceInfo(t *testing.T) {
	config := DefaultElectorConfig()
	elector := NewElector("test-room", config)
	defer elector.Close()

	// 先添加基础信息
	elector.UpdateCandidate(Candidate{
		PeerID:    "peer-1",
		Bandwidth: 10000000,
	})

	// 更新设备信息
	elector.UpdateDeviceInfo("peer-1", DeviceTypePC, ConnectionTypeEthernet, PowerStatePluggedIn)

	candidates := elector.GetCandidates()
	if len(candidates) != 1 {
		t.Fatalf("Expected 1 candidate, got %d", len(candidates))
	}

	if candidates[0].DeviceType != DeviceTypePC {
		t.Errorf("Expected DeviceTypePC, got %d", candidates[0].DeviceType)
	}
}

func TestElectorCallback(t *testing.T) {
	config := DefaultElectorConfig()
	config.ElectionInterval = 100 * time.Millisecond
	elector := NewElector("test-room", config)
	defer elector.Close()

	callbackCalled := make(chan ElectionResult, 1)
	elector.SetOnElection(func(result ElectionResult) {
		callbackCalled <- result
	})

	elector.UpdateCandidate(Candidate{
		PeerID:     "peer-1",
		DeviceType: DeviceTypePC,
		Bandwidth:  10000000,
	})

	elector.Start()

	select {
	case result := <-callbackCalled:
		if result.ProxyID != "peer-1" {
			t.Errorf("Expected peer-1, got %s", result.ProxyID)
		}
	case <-time.After(500 * time.Millisecond):
		t.Error("Callback not called")
	}
}

func TestElectorRemoveCandidate(t *testing.T) {
	config := DefaultElectorConfig()
	elector := NewElector("test-room", config)
	defer elector.Close()

	elector.UpdateCandidate(Candidate{PeerID: "peer-1"})
	elector.UpdateCandidate(Candidate{PeerID: "peer-2"})

	if len(elector.GetCandidates()) != 2 {
		t.Error("Should have 2 candidates")
	}

	elector.RemoveCandidate("peer-1")

	if len(elector.GetCandidates()) != 1 {
		t.Error("Should have 1 candidate after removal")
	}
}

// ==========================================
// Benchmarks
// ==========================================

func BenchmarkElectorElect(b *testing.B) {
	config := DefaultElectorConfig()
	elector := NewElector("bench-room", config)
	defer elector.Close()

	// 添加 10 个候选者
	for i := 0; i < 10; i++ {
		elector.UpdateCandidate(Candidate{
			PeerID:     string(rune('a' + i)),
			DeviceType: DeviceType(i % 4),
			Bandwidth:  int64(10000000 * (i + 1)),
			Latency:    int64(10 + i*5),
		})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		elector.Elect()
	}
}

func BenchmarkElectorUpdateCandidate(b *testing.B) {
	config := DefaultElectorConfig()
	elector := NewElector("bench-room", config)
	defer elector.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		elector.UpdateCandidate(Candidate{
			PeerID:     "peer-1",
			Bandwidth:  int64(i * 1000),
			Latency:    int64(i % 100),
			PacketLoss: float64(i%10) / 100,
		})
	}
}
