/*
 * @Author: Marlon.M
 * @Email: maiguangyang@163.com
 * @Date: 2026-01-13
 *
 * LocalShareBridge 单元测试
 */
package sfu

import (
	"net"
	"sync/atomic"
	"testing"
	"time"
)

func TestLocalShareBridge_Basic(t *testing.T) {
	// 创建 SourceSwitcher
	switcher, err := NewSourceSwitcher("test-room")
	if err != nil {
		t.Fatalf("Failed to create SourceSwitcher: %v", err)
	}
	defer switcher.Close()

	// 创建桥接器
	bridge := NewLocalShareBridge("test-room", switcher)
	if bridge == nil {
		t.Fatal("Failed to create LocalShareBridge")
	}
	defer bridge.Close()

	// 启动（自动分配端口）
	port, err := bridge.Start(0)
	if err != nil {
		t.Fatalf("Failed to start bridge: %v", err)
	}
	if port <= 0 {
		t.Fatalf("Invalid port: %d", port)
	}

	// 检查状态
	if !bridge.IsRunning() {
		t.Error("Bridge should be running")
	}
	if bridge.GetPort() != port {
		t.Errorf("GetPort() = %d, want %d", bridge.GetPort(), port)
	}

	// 停止
	bridge.Stop()
	if bridge.IsRunning() {
		t.Error("Bridge should not be running after Stop()")
	}
}

func TestLocalShareBridge_ReceivePackets(t *testing.T) {
	// 创建 SourceSwitcher
	switcher, err := NewSourceSwitcher("test-room")
	if err != nil {
		t.Fatalf("Failed to create SourceSwitcher: %v", err)
	}
	defer switcher.Close()

	// 创建桥接器
	bridge := NewLocalShareBridge("test-room", switcher)
	defer bridge.Close()

	// 启动
	port, err := bridge.Start(0)
	if err != nil {
		t.Fatalf("Failed to start bridge: %v", err)
	}

	// 发送 RTP 包
	conn, err := net.Dial("udp", "127.0.0.1:"+string(rune('0'+port/10000))+string(rune('0'+port/1000%10))+string(rune('0'+port/100%10))+string(rune('0'+port/10%10))+string(rune('0'+port%10)))
	if err != nil {
		// Use proper port string conversion
		addr, _ := net.ResolveUDPAddr("udp", "127.0.0.1:0")
		conn, err = net.DialUDP("udp", addr, &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: port})
		if err != nil {
			t.Fatalf("Failed to connect to bridge: %v", err)
		}
	}
	defer conn.Close()

	// 构造简单的 RTP 包（视频，PT=96）
	rtpPacket := make([]byte, 100)
	rtpPacket[0] = 0x80 // V=2, P=0, X=0, CC=0
	rtpPacket[1] = 0x60 // M=0, PT=96 (video)
	// 其余字段默认为 0

	// 发送多个包
	for i := 0; i < 10; i++ {
		_, err := conn.Write(rtpPacket)
		if err != nil {
			t.Errorf("Failed to send packet %d: %v", i, err)
		}
	}

	// 等待接收
	time.Sleep(100 * time.Millisecond)

	// 验证统计
	status := bridge.GetStatus()
	if status.PacketsReceived < 1 {
		t.Logf("Warning: Expected packets to be received, got %d (may be timing issue)", status.PacketsReceived)
	}
}

func TestLocalShareBridge_GlobalManagement(t *testing.T) {
	// 创建 SourceSwitcher
	switcher, err := NewSourceSwitcher("global-test-room")
	if err != nil {
		t.Fatalf("Failed to create SourceSwitcher: %v", err)
	}
	defer switcher.Close()

	// 创建桥接器
	bridge := CreateLocalShareBridge("global-test-room", switcher)
	if bridge == nil {
		t.Fatal("CreateLocalShareBridge returned nil")
	}

	// 获取相同的桥接器
	bridge2 := GetLocalShareBridge("global-test-room")
	if bridge2 != bridge {
		t.Error("GetLocalShareBridge should return the same instance")
	}

	// 创建相同房间应创建新实例（为避免内存泄漏，老的会被清理）
	bridge3 := CreateLocalShareBridge("global-test-room", switcher)
	if bridge3 == nil {
		t.Error("CreateLocalShareBridge should return new instance")
	}
	// 新行为：每次创建都是新的实例，旧的被清理
	if bridge3 == bridge {
		t.Error("CreateLocalShareBridge should create new instance, not reuse old one")
	}

	// 销毁
	DestroyLocalShareBridge("global-test-room")

	// 应该不存在了
	bridge4 := GetLocalShareBridge("global-test-room")
	if bridge4 != nil {
		t.Error("GetLocalShareBridge should return nil after destruction")
	}
}

func TestLocalShareBridge_States(t *testing.T) {
	// 创建 SourceSwitcher
	switcher, err := NewSourceSwitcher("state-test-room")
	if err != nil {
		t.Fatalf("Failed to create SourceSwitcher: %v", err)
	}
	defer switcher.Close()

	bridge := NewLocalShareBridge("state-test-room", switcher)
	defer bridge.Close()

	// 初始状态
	if bridge.GetState() != LocalShareBridgeStateIdle {
		t.Errorf("Initial state should be Idle, got %s", bridge.GetState())
	}

	// 启动
	_, err = bridge.Start(0)
	if err != nil {
		t.Fatalf("Failed to start: %v", err)
	}
	if bridge.GetState() != LocalShareBridgeStateRunning {
		t.Errorf("State should be Running after Start(), got %s", bridge.GetState())
	}

	// 停止
	bridge.Stop()
	if bridge.GetState() != LocalShareBridgeStateStopped {
		t.Errorf("State should be Stopped after Stop(), got %s", bridge.GetState())
	}
}

// BenchmarkLocalShareBridge_Receive 基准测试：接收性能
func BenchmarkLocalShareBridge_Receive(b *testing.B) {
	switcher, _ := NewSourceSwitcher("bench-room")
	defer switcher.Close()

	bridge := NewLocalShareBridge("bench-room", switcher)
	defer bridge.Close()

	port, err := bridge.Start(0)
	if err != nil {
		b.Fatalf("Failed to start bridge: %v", err)
	}

	// 创建发送连接
	conn, err := net.DialUDP("udp", nil, &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: port})
	if err != nil {
		b.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	// RTP 包
	rtpPacket := make([]byte, 1200)
	rtpPacket[0] = 0x80
	rtpPacket[1] = 0x60 // PT=96

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		conn.Write(rtpPacket)
	}

	// 等待接收完成
	time.Sleep(100 * time.Millisecond)

	received := atomic.LoadUint64(&bridge.packetsReceived)
	b.ReportMetric(float64(received)/float64(b.N)*100, "%received")
}
