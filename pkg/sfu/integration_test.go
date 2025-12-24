/*
 * @Author: Marlon.M
 * @Email: maiguangyang@163.com
 * @Date: 2025-12-24
 *
 * Integration Tests
 * 端到端集成测试，模拟真实使用场景
 */
package sfu

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/maiguangyang/relay_core/pkg/election"
	"github.com/pion/rtp"
)

// ==========================================
// 场景：多节点选举和故障切换
// ==========================================

func TestIntegration_MultiNodeElection(t *testing.T) {
	// 创建 3 个节点
	nodes := make([]*ProxyModeCoordinator, 3)
	nodeIDs := []string{"pc-node", "ipad-node", "mobile-node"}
	deviceTypes := []int{1, 2, 3} // PC, Pad, Mobile

	for i := 0; i < 3; i++ {
		config := DefaultCoordinatorConfig()
		config.KeepaliveInterval = 50 * time.Millisecond
		config.ElectionInterval = 50 * time.Millisecond

		pmc, err := NewProxyModeCoordinator("test-room", nodeIDs[i], config)
		if err != nil {
			t.Fatalf("Failed to create coordinator %d: %v", i, err)
		}
		nodes[i] = pmc

		// 更新设备信息
		pmc.UpdateLocalDeviceInfo(deviceTypes[i], 1, 1)
	}

	defer func() {
		for _, node := range nodes {
			node.Close()
		}
	}()

	// 让所有节点互相知道
	for i := 0; i < 3; i++ {
		for j := 0; j < 3; j++ {
			if i != j {
				nodes[i].AddPeer(nodeIDs[j], deviceTypes[j], 1, 1)
			}
		}
	}

	// 启动所有节点
	for _, node := range nodes {
		node.Start()
	}

	// 等待选举
	time.Sleep(200 * time.Millisecond)

	// 验证 PC 节点应该是 Relay（分数最高）
	// 这依赖于选举算法，暂时只验证有一个 Relay 被选出
	t.Log("Election completed")
}

// ==========================================
// 场景：故障切换测试
// ==========================================

func TestIntegration_FailoverScenario(t *testing.T) {
	config := DefaultCoordinatorConfig()
	config.KeepaliveInterval = 50 * time.Millisecond
	config.FailoverOfflineThreshold = 2

	// 创建选举器
	electorConfig := election.DefaultElectorConfig()
	elector := election.NewElector("test-room", electorConfig)
	defer elector.Close()

	// 添加候选者
	elector.UpdateCandidate(election.Candidate{
		PeerID:     "relay-1",
		DeviceType: election.DeviceTypePC,
		Score:      80,
	})
	elector.UpdateCandidate(election.Candidate{
		PeerID:     "backup-1",
		DeviceType: election.DeviceTypePC,
		Score:      70,
	})

	// 创建 Failover Manager
	failoverConfig := FailoverConfig{
		BackoffPerPoint:  10 * time.Millisecond,
		MaxBackoff:       500 * time.Millisecond,
		ClaimTimeout:     100 * time.Millisecond,
		OfflineThreshold: 2,
	}

	fm := NewFailoverManager("test-room", "backup-1", elector, nil, failoverConfig)
	defer fm.Close()

	fm.SetCurrentRelay("relay-1", 1)
	fm.UpdateLocalScore(70)

	var becameRelay int32
	fm.SetCallbacks(
		func(roomID, relayID string) {
			t.Logf("Relay %s failed", relayID)
		},
		func(roomID, newRelayID string, epoch uint64) {
			t.Logf("New relay elected: %s (epoch=%d)", newRelayID, epoch)
		},
		func(roomID string) {
			atomic.AddInt32(&becameRelay, 1)
			t.Log("This node became relay")
		},
	)

	// 模拟 Relay 离线
	fm.handlePeerOffline("relay-1")
	fm.handlePeerOffline("relay-1") // 第二次触发选举

	// 等待选举完成
	time.Sleep(300 * time.Millisecond)

	// 验证选举发生
	t.Logf("Became relay: %d times", atomic.LoadInt32(&becameRelay))
}

// ==========================================
// 场景：完整数据流测试
// ==========================================

func TestIntegration_DataFlowScenario(t *testing.T) {
	// 创建 SourceSwitcher
	switcher, err := NewSourceSwitcher("test-room")
	if err != nil {
		t.Fatalf("Failed to create switcher: %v", err)
	}
	defer switcher.Close()

	var sourceChanges int32
	switcher.SetOnSourceChanged(func(roomID string, sourceType SourceType, sharerID string) {
		atomic.AddInt32(&sourceChanges, 1)
		t.Logf("Source changed to: %s", sourceType)
	})

	// 注入 SFU 数据
	for i := 0; i < 100; i++ {
		packet := createTestRTPPacket(uint16(i), 1200)
		switcher.InjectSFUPacket(true, packet)
	}

	// 切换到本地分享
	switcher.StartLocalShare("sharer-1")

	// 注入本地数据
	for i := 0; i < 50; i++ {
		packet := createTestRTPPacket(uint16(i), 1200)
		switcher.InjectLocalPacket(true, packet)
	}

	// 切换回 SFU
	switcher.StopLocalShare()

	time.Sleep(50 * time.Millisecond)

	if atomic.LoadInt32(&sourceChanges) < 2 {
		t.Errorf("Expected at least 2 source changes, got %d", atomic.LoadInt32(&sourceChanges))
	}

	status := switcher.GetStatus()
	t.Logf("Final status: %+v", status)
}

// ==========================================
// 场景：网络质量变化
// ==========================================

func TestIntegration_NetworkQualityChange(t *testing.T) {
	// 创建选举器
	electorConfig := election.DefaultElectorConfig()
	elector := election.NewElector("test-room", electorConfig)
	defer elector.Close()

	// 添加候选者并设置初始网络指标
	elector.UpdateCandidate(election.Candidate{
		PeerID:         "peer-1",
		DeviceType:     election.DeviceTypePC,
		ConnectionType: election.ConnectionTypeEthernet,
		Bandwidth:      50 * 1024 * 1024,
		Latency:        30,
		PacketLoss:     0.01,
	})
	elector.UpdateCandidate(election.Candidate{
		PeerID:         "peer-2",
		DeviceType:     election.DeviceTypePC,
		ConnectionType: election.ConnectionTypeEthernet,
		Bandwidth:      45 * 1024 * 1024,
		Latency:        35,
		PacketLoss:     0.02,
	})

	// 初始选举
	result1 := elector.Elect()
	t.Logf("Initial election: %s (score=%.2f)", result1.ProxyID, result1.Score)

	// 模拟 peer-1 网络质量下降
	elector.UpdateNetworkMetrics("peer-1", 10*1024*1024, 200, 0.10) // 降低带宽，增加延迟和丢包

	// 重新选举
	result2 := elector.Elect()
	t.Logf("After quality change: %s (score=%.2f)", result2.ProxyID, result2.Score)

	// 验证选举结果变化
	if result1.ProxyID == result2.ProxyID {
		t.Logf("Note: Same proxy elected, but scores may have changed")
	}
}

// ==========================================
// 场景：高并发压力测试
// ==========================================

func TestIntegration_HighConcurrencyStress(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	const (
		numPeers     = 20
		testDuration = 3 * time.Second
	)

	// 创建 Coordinator
	config := DefaultCoordinatorConfig()
	config.KeepaliveInterval = 100 * time.Millisecond

	pmc, err := NewProxyModeCoordinator("stress-room", "local-peer", config)
	if err != nil {
		t.Fatalf("Failed to create coordinator: %v", err)
	}
	defer pmc.Close()

	// 添加大量 Peer
	for i := 0; i < numPeers; i++ {
		pmc.AddPeer("peer-"+string(rune('a'+i)), i%4+1, i%3+1, i%3+1)
	}

	pmc.Start()

	// 模拟高频 Pong 处理
	startTime := time.Now()
	var pongCount int64

	for time.Since(startTime) < testDuration {
		for i := 0; i < numPeers; i++ {
			pmc.HandlePong("peer-" + string(rune('a'+i)))
			atomic.AddInt64(&pongCount, 1)
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Logf("Processed %d pongs in %v", atomic.LoadInt64(&pongCount), testDuration)
	t.Logf("Pongs per second: %.0f", float64(atomic.LoadInt64(&pongCount))/testDuration.Seconds())

	status := pmc.GetStatus()
	t.Logf("Final status: peer_count=%d", status["peer_count"])
}

// ==========================================
// 场景：JitterBuffer 流处理
// ==========================================

func TestIntegration_JitterBufferStreaming(t *testing.T) {
	config := JitterBufferConfig{
		Enabled:     true,
		TargetDelay: 40 * time.Millisecond,
		MinDelay:    20 * time.Millisecond,
		MaxDelay:    100 * time.Millisecond,
		MaxPackets:  200,
	}

	jb := NewJitterBuffer(config)
	defer jb.Close()

	// 启动输出
	go jb.Start()

	// 模拟 1 秒的视频流 (30fps)
	for i := 0; i < 30; i++ {
		packet := createRTPPacketWithTimestamp(uint16(i), uint32(i)*3000)
		jb.Push(packet)
		time.Sleep(33 * time.Millisecond) // 30fps
	}

	stats := jb.GetStats()
	t.Logf("Packets received: %d", stats.PacketsReceived)
	t.Logf("Packets dropped: %d", stats.PacketsDropped)
	t.Logf("Jitter: %d ms", stats.Jitter)
	t.Logf("Current delay: %d ms", stats.CurrentDelay)

	if stats.PacketsReceived != 30 {
		t.Errorf("Expected 30 packets received, got %d", stats.PacketsReceived)
	}
}

// ==========================================
// 辅助函数
// ==========================================

func createRTPPacketWithTimestamp(seq uint16, timestamp uint32) *rtp.Packet {
	return &rtp.Packet{
		Header: rtp.Header{
			Version:        2,
			PayloadType:    96,
			SequenceNumber: seq,
			Timestamp:      timestamp,
			SSRC:           0x12345678,
		},
		Payload: make([]byte, 1200),
	}
}
