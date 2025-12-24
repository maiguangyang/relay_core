/*
 * @Author: Marlon.M
 * @Email: maiguangyang@163.com
 * @Date: 2025-12-24
 *
 * JitterBuffer Tests
 * 测试抖动缓冲的核心功能
 */
package sfu

import (
	"testing"
	"time"

	"github.com/pion/rtp"
)

func TestJitterBufferCreate(t *testing.T) {
	config := JitterBufferConfig{
		Enabled:     true,
		TargetDelay: 50 * time.Millisecond,
		MinDelay:    20 * time.Millisecond,
		MaxDelay:    200 * time.Millisecond,
		MaxPackets:  100,
	}

	jb := NewJitterBuffer(config)
	defer jb.Close()

	if jb == nil {
		t.Fatal("JitterBuffer should not be nil")
	}

	if !jb.IsEnabled() {
		t.Error("JitterBuffer should be enabled")
	}
}

func TestJitterBufferDisabled(t *testing.T) {
	config := JitterBufferConfig{
		Enabled:    false,
		MaxPackets: 100,
	}

	jb := NewJitterBuffer(config)
	defer jb.Close()

	// 禁用时直接输出
	packet := &rtp.Packet{
		Header: rtp.Header{
			SequenceNumber: 1,
			Timestamp:      3000,
		},
	}

	jb.Push(packet)

	// 应该直接在输出通道
	select {
	case out := <-jb.Output():
		if out.SequenceNumber != 1 {
			t.Errorf("Expected seq 1, got %d", out.SequenceNumber)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Packet should be immediately output when disabled")
	}
}

func TestJitterBufferReordering(t *testing.T) {
	config := JitterBufferConfig{
		Enabled:     true,
		TargetDelay: 30 * time.Millisecond,
		MinDelay:    10 * time.Millisecond,
		MaxDelay:    100 * time.Millisecond,
		MaxPackets:  100,
	}

	jb := NewJitterBuffer(config)
	defer jb.Close()

	// 推入乱序包
	packets := []*rtp.Packet{
		{Header: rtp.Header{SequenceNumber: 3, Timestamp: 9000}},
		{Header: rtp.Header{SequenceNumber: 1, Timestamp: 3000}},
		{Header: rtp.Header{SequenceNumber: 2, Timestamp: 6000}},
	}

	for _, p := range packets {
		jb.Push(p)
	}

	stats := jb.GetStats()
	if stats.PacketsReceived != 3 {
		t.Errorf("Expected 3 packets received, got %d", stats.PacketsReceived)
	}

	if stats.BufferedPackets != 3 {
		t.Errorf("Expected 3 buffered packets, got %d", stats.BufferedPackets)
	}
}

func TestJitterBufferDrop(t *testing.T) {
	config := JitterBufferConfig{
		Enabled:     true,
		TargetDelay: 30 * time.Millisecond,
		MinDelay:    10 * time.Millisecond,
		MaxDelay:    100 * time.Millisecond,
		MaxPackets:  5, // 很小的缓冲区
	}

	jb := NewJitterBuffer(config)
	defer jb.Close()

	// 推入超过缓冲区容量的包
	for i := uint16(0); i < 10; i++ {
		jb.Push(&rtp.Packet{
			Header: rtp.Header{
				SequenceNumber: i,
				Timestamp:      uint32(i) * 3000,
			},
		})
	}

	stats := jb.GetStats()
	if stats.BufferedPackets > 5 {
		t.Errorf("Buffer should not exceed max, got %d", stats.BufferedPackets)
	}

	if stats.PacketsDropped < 5 {
		t.Errorf("Should have dropped at least 5 packets, got %d", stats.PacketsDropped)
	}
}

func TestJitterBufferJitterCalculation(t *testing.T) {
	config := JitterBufferConfig{
		Enabled:     true,
		TargetDelay: 50 * time.Millisecond,
		MinDelay:    20 * time.Millisecond,
		MaxDelay:    200 * time.Millisecond,
		MaxPackets:  100,
	}

	jb := NewJitterBuffer(config)
	defer jb.Close()

	// 模拟稳定的包到达（每 33ms 一个，30fps）
	for i := uint16(0); i < 30; i++ {
		jb.Push(&rtp.Packet{
			Header: rtp.Header{
				SequenceNumber: i,
				Timestamp:      uint32(i) * 3000, // 90kHz, 30fps
			},
		})
		time.Sleep(33 * time.Millisecond)
	}

	stats := jb.GetStats()
	// 抖动应该被计算
	t.Logf("Jitter: %d ms", stats.Jitter)
	t.Logf("Current delay: %d ms", stats.CurrentDelay)
}

func TestJitterBufferFlush(t *testing.T) {
	config := JitterBufferConfig{
		Enabled:     true,
		TargetDelay: 50 * time.Millisecond,
		MaxPackets:  100,
	}

	jb := NewJitterBuffer(config)
	defer jb.Close()

	// 添加一些包
	for i := uint16(0); i < 10; i++ {
		jb.Push(&rtp.Packet{
			Header: rtp.Header{SequenceNumber: i},
		})
	}

	stats := jb.GetStats()
	if stats.BufferedPackets == 0 {
		t.Error("Should have buffered packets")
	}

	// 清空
	jb.Flush()

	stats = jb.GetStats()
	if stats.BufferedPackets != 0 {
		t.Errorf("Should have 0 packets after flush, got %d", stats.BufferedPackets)
	}
}

func TestJitterBufferEnable(t *testing.T) {
	config := JitterBufferConfig{
		Enabled:    false,
		MaxPackets: 100,
	}

	jb := NewJitterBuffer(config)
	defer jb.Close()

	if jb.IsEnabled() {
		t.Error("Should be disabled initially")
	}

	jb.Enable(true)

	if !jb.IsEnabled() {
		t.Error("Should be enabled after Enable(true)")
	}
}

func TestJitterBufferSetDelay(t *testing.T) {
	config := JitterBufferConfig{
		Enabled:     true,
		TargetDelay: 50 * time.Millisecond,
		MinDelay:    20 * time.Millisecond,
		MaxDelay:    200 * time.Millisecond,
		MaxPackets:  100,
	}

	jb := NewJitterBuffer(config)
	defer jb.Close()

	jb.SetDelay(100 * time.Millisecond)

	// 验证延迟已更新（通过统计）
	stats := jb.GetStats()
	t.Logf("Current delay after set: %d ms", stats.CurrentDelay)
}

func TestJitterBufferPop(t *testing.T) {
	config := JitterBufferConfig{
		Enabled:     true,
		TargetDelay: 10 * time.Millisecond,
		MinDelay:    5 * time.Millisecond,
		MaxDelay:    100 * time.Millisecond,
		MaxPackets:  100,
	}

	jb := NewJitterBuffer(config)
	defer jb.Close()

	// 添加包
	jb.Push(&rtp.Packet{
		Header: rtp.Header{SequenceNumber: 1, Timestamp: 3000},
	})

	// Pop 应该返回包
	packet := jb.Pop()
	if packet == nil {
		t.Error("Pop should return packet")
	} else if packet.SequenceNumber != 1 {
		t.Errorf("Expected seq 1, got %d", packet.SequenceNumber)
	}
}

func TestJitterBufferDefaultConfig(t *testing.T) {
	config := DefaultJitterBufferConfig()

	if config.Enabled {
		t.Error("Default config should have Enabled=false")
	}

	if config.MaxPackets != 100 {
		t.Errorf("Expected MaxPackets 100, got %d", config.MaxPackets)
	}
}

// ==========================================
// Benchmarks
// ==========================================

func BenchmarkJitterBufferPush(b *testing.B) {
	config := JitterBufferConfig{
		Enabled:     true,
		TargetDelay: 50 * time.Millisecond,
		MaxPackets:  1000,
	}

	jb := NewJitterBuffer(config)
	defer jb.Close()

	packet := &rtp.Packet{
		Header: rtp.Header{
			SequenceNumber: 0,
			Timestamp:      0,
		},
		Payload: make([]byte, 1200),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		packet.SequenceNumber = uint16(i)
		packet.Timestamp = uint32(i) * 3000
		jb.Push(packet)
	}
}

func BenchmarkJitterBufferGetStats(b *testing.B) {
	config := JitterBufferConfig{
		Enabled:     true,
		TargetDelay: 50 * time.Millisecond,
		MaxPackets:  100,
	}

	jb := NewJitterBuffer(config)
	defer jb.Close()

	// 添加一些包
	for i := uint16(0); i < 50; i++ {
		jb.Push(&rtp.Packet{
			Header: rtp.Header{SequenceNumber: i, Timestamp: uint32(i) * 3000},
		})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		jb.GetStats()
	}
}
