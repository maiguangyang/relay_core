/*
 * @Author: Marlon.M
 * @Email: maiguangyang@163.com
 * @Date: 2025-12-24
 *
 * Failover Manager Tests
 */
package sfu

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/maiguangyang/relay_core/pkg/election"
)

func TestFailoverManagerCreate(t *testing.T) {
	config := DefaultFailoverConfig()
	elector := election.NewElector("test-room", election.DefaultElectorConfig())
	defer elector.Close()

	km := NewKeepaliveManager(DefaultKeepaliveConfig())
	defer km.Stop()

	fm := NewFailoverManager("test-room", "local-peer", elector, km, config)
	defer fm.Close()

	if fm.GetState() != FailoverStateIdle {
		t.Error("Initial state should be idle")
	}
}

func TestFailoverManagerSetCurrentRelay(t *testing.T) {
	config := DefaultFailoverConfig()
	fm := NewFailoverManager("test-room", "local-peer", nil, nil, config)
	defer fm.Close()

	fm.SetCurrentRelay("relay-1", 1)

	relayID, epoch := fm.GetCurrentRelay()
	if relayID != "relay-1" {
		t.Errorf("Expected relay-1, got %s", relayID)
	}
	if epoch != 1 {
		t.Errorf("Expected epoch 1, got %d", epoch)
	}
}

func TestFailoverManagerReceiveClaim(t *testing.T) {
	config := DefaultFailoverConfig()
	fm := NewFailoverManager("test-room", "local-peer", nil, nil, config)
	defer fm.Close()

	fm.SetCurrentRelay("relay-1", 1)

	// 接收更高 epoch 的声明
	fm.ReceiveRelayClaim("relay-2", 2, 80.0)

	relayID, epoch := fm.GetCurrentRelay()
	if relayID != "relay-2" {
		t.Errorf("Expected relay-2, got %s", relayID)
	}
	if epoch != 2 {
		t.Errorf("Expected epoch 2, got %d", epoch)
	}
}

func TestFailoverManagerBackoffCalculation(t *testing.T) {
	config := DefaultFailoverConfig()

	testCases := []struct {
		score           float64
		expectedBackoff time.Duration
	}{
		{100, 0},                       // 最高分，无退避
		{50, 500 * time.Millisecond},   // 中等分
		{0, 1000 * time.Millisecond},   // 最低分
		{-10, 1100 * time.Millisecond}, // 负分，(100-(-10))*10 = 1100ms
	}

	for _, tc := range testCases {
		backoff := time.Duration(100-tc.score) * config.BackoffPerPoint
		if backoff > config.MaxBackoff {
			backoff = config.MaxBackoff
		}
		if backoff < 0 {
			backoff = 0
		}

		// 允许一定误差
		if backoff != tc.expectedBackoff && (backoff < tc.expectedBackoff-time.Millisecond || backoff > tc.expectedBackoff+time.Millisecond) {
			t.Errorf("Score %.0f: expected backoff %v, got %v", tc.score, tc.expectedBackoff, backoff)
		}
	}
}

func TestFailoverManagerOfflineThreshold(t *testing.T) {
	config := DefaultFailoverConfig()
	config.OfflineThreshold = 3

	// 创建模拟选举器
	elector := election.NewElector("test-room", election.DefaultElectorConfig())
	defer elector.Close()

	elector.UpdateCandidate(election.Candidate{PeerID: "local-peer", DeviceType: election.DeviceTypePC})

	fm := NewFailoverManager("test-room", "local-peer", elector, nil, config)
	defer fm.Close()

	fm.SetCurrentRelay("relay-1", 1)
	fm.UpdateLocalScore(80)

	var failedCalled int32

	fm.SetCallbacks(
		func(roomID, relayID string) {
			atomic.AddInt32(&failedCalled, 1)
		},
		nil, nil,
	)

	// 第一次离线，不触发
	fm.handlePeerOffline("relay-1")
	if atomic.LoadInt32(&failedCalled) != 0 {
		t.Error("Should not trigger failover on first offline")
	}

	// 第二次离线，仍不触发
	fm.handlePeerOffline("relay-1")
	if atomic.LoadInt32(&failedCalled) != 0 {
		t.Error("Should not trigger failover on second offline")
	}

	// 第三次离线，触发
	fm.handlePeerOffline("relay-1")
	time.Sleep(50 * time.Millisecond) // 等待异步回调
	if atomic.LoadInt32(&failedCalled) != 1 {
		t.Error("Should trigger failover on third offline")
	}
}

func TestFailoverManagerConflictPrevention(t *testing.T) {
	config := DefaultFailoverConfig()
	config.BackoffPerPoint = 50 * time.Millisecond

	// 创建两个节点，模拟竞争
	elector1 := election.NewElector("test-room", election.DefaultElectorConfig())
	defer elector1.Close()
	elector1.UpdateCandidate(election.Candidate{PeerID: "node-1", DeviceType: election.DeviceTypePC, Score: 90})
	elector1.UpdateCandidate(election.Candidate{PeerID: "node-2", DeviceType: election.DeviceTypeMobile, Score: 40})

	elector2 := election.NewElector("test-room", election.DefaultElectorConfig())
	defer elector2.Close()
	elector2.UpdateCandidate(election.Candidate{PeerID: "node-1", DeviceType: election.DeviceTypePC, Score: 90})
	elector2.UpdateCandidate(election.Candidate{PeerID: "node-2", DeviceType: election.DeviceTypeMobile, Score: 40})

	fm1 := NewFailoverManager("test-room", "node-1", elector1, nil, config)
	defer fm1.Close()
	fm1.SetCurrentRelay("old-relay", 1)
	fm1.UpdateLocalScore(90) // 高分，短退避

	fm2 := NewFailoverManager("test-room", "node-2", elector2, nil, config)
	defer fm2.Close()
	fm2.SetCurrentRelay("old-relay", 1)
	fm2.UpdateLocalScore(40) // 低分，长退避

	var node1BecameRelay, node2BecameRelay int32

	fm1.SetCallbacks(nil, nil, func(roomID string) {
		atomic.AddInt32(&node1BecameRelay, 1)
		// 通知 fm2 已有人声明
		fm2.ReceiveRelayClaim("node-1", 2, 90.0)
	})

	fm2.SetCallbacks(nil, nil, func(roomID string) {
		atomic.AddInt32(&node2BecameRelay, 1)
	})

	// 两个节点同时检测到离线
	go fm1.startFailoverProcess()
	go fm2.startFailoverProcess()

	// 等待选举完成
	time.Sleep(time.Second)

	// 由于 node-1 分数高，退避短，应该先声明
	// node-2 收到声明后应该放弃
	if atomic.LoadInt32(&node1BecameRelay) != 1 {
		t.Error("Node-1 (higher score) should become relay")
	}
	if atomic.LoadInt32(&node2BecameRelay) != 0 {
		t.Error("Node-2 (lower score) should NOT become relay")
	}
}
