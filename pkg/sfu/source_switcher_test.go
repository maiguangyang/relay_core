/*
 * @Author: Marlon.M
 * @Email: maiguangyang@163.com
 * @Date: 2025-12-24
 *
 * SourceSwitcher Tests
 */
package sfu

import (
	"testing"
	"time"
)

func TestSourceSwitcherCreate(t *testing.T) {
	switcher, err := NewSourceSwitcher("test-room")
	if err != nil {
		t.Fatalf("Failed to create SourceSwitcher: %v", err)
	}
	defer switcher.Close()

	if switcher.GetActiveSource() != SourceTypeSFU {
		t.Error("Default source should be SFU")
	}

	if switcher.IsLocalSharing() {
		t.Error("Should not be local sharing by default")
	}
}

func TestSourceSwitcherInjectSFU(t *testing.T) {
	switcher, err := NewSourceSwitcher("test-room")
	if err != nil {
		t.Fatalf("Failed to create SourceSwitcher: %v", err)
	}
	defer switcher.Close()

	// 创建模拟 RTP 包（至少 12 字节的 RTP 头）
	rtpPacket := make([]byte, 100)
	// RTP 版本 2
	rtpPacket[0] = 0x80
	// Payload type
	rtpPacket[1] = 96
	// Sequence number
	rtpPacket[2] = 0x00
	rtpPacket[3] = 0x01
	// Timestamp
	rtpPacket[4] = 0x00
	rtpPacket[5] = 0x00
	rtpPacket[6] = 0x00
	rtpPacket[7] = 0x01
	// SSRC
	rtpPacket[8] = 0x00
	rtpPacket[9] = 0x00
	rtpPacket[10] = 0x00
	rtpPacket[11] = 0x01

	// 当前源是 SFU，注入 SFU 包应该成功
	err = switcher.InjectSFUPacket(true, rtpPacket)
	if err != nil {
		t.Errorf("Failed to inject SFU packet: %v", err)
	}
}

func TestSourceSwitcherLocalShare(t *testing.T) {
	switcher, err := NewSourceSwitcher("test-room")
	if err != nil {
		t.Fatalf("Failed to create SourceSwitcher: %v", err)
	}
	defer switcher.Close()

	// 开始本地分享
	switcher.StartLocalShare("sharer-1")

	if !switcher.IsLocalSharing() {
		t.Error("Should be local sharing after StartLocalShare")
	}

	if switcher.GetActiveSource() != SourceTypeLocal {
		t.Error("Current source should be Local")
	}

	status := switcher.GetStatus()
	if status.LocalSharerID != "sharer-1" {
		t.Errorf("Expected sharer ID 'sharer-1', got '%s'", status.LocalSharerID)
	}

	// 停止本地分享
	switcher.StopLocalShare()

	if switcher.IsLocalSharing() {
		t.Error("Should not be local sharing after StopLocalShare")
	}

	if switcher.GetActiveSource() != SourceTypeSFU {
		t.Error("Current source should be back to SFU")
	}
}

func TestSourceSwitcherCallback(t *testing.T) {
	switcher, err := NewSourceSwitcher("test-room")
	if err != nil {
		t.Fatalf("Failed to create SourceSwitcher: %v", err)
	}
	defer switcher.Close()

	callbackCalled := make(chan SourceType, 2)
	switcher.SetOnSourceChanged(func(roomID string, source SourceType, sharerID string) {
		callbackCalled <- source
	})

	// 切换到本地分享
	switcher.StartLocalShare("sharer")

	select {
	case src := <-callbackCalled:
		if src != SourceTypeLocal {
			t.Errorf("Expected Local source in callback, got %v", src)
		}
	case <-time.After(time.Second):
		t.Error("Callback not called for StartLocalShare")
	}

	// 切换回 SFU
	switcher.StopLocalShare()

	select {
	case src := <-callbackCalled:
		if src != SourceTypeSFU {
			t.Errorf("Expected SFU source in callback, got %v", src)
		}
	case <-time.After(time.Second):
		t.Error("Callback not called for StopLocalShare")
	}
}

func TestSourceSwitcherTracks(t *testing.T) {
	switcher, err := NewSourceSwitcher("test-room")
	if err != nil {
		t.Fatalf("Failed to create SourceSwitcher: %v", err)
	}
	defer switcher.Close()

	videoTrack := switcher.GetVideoTrack()
	audioTrack := switcher.GetAudioTrack()

	if videoTrack == nil {
		t.Error("Video track should not be nil")
	}

	if audioTrack == nil {
		t.Error("Audio track should not be nil")
	}
}

// ==========================================
// Benchmarks
// ==========================================

func BenchmarkSourceSwitcherInjectSFU(b *testing.B) {
	switcher, err := NewSourceSwitcher("bench-room")
	if err != nil {
		b.Fatalf("Failed to create SourceSwitcher: %v", err)
	}
	defer switcher.Close()

	// 创建 RTP 包
	rtpPacket := make([]byte, 1200)
	rtpPacket[0] = 0x80
	rtpPacket[1] = 96

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// 更新序号避免重复
		rtpPacket[2] = byte(i >> 8)
		rtpPacket[3] = byte(i)
		switcher.InjectSFUPacket(true, rtpPacket)
	}
}

func BenchmarkSourceSwitcherInjectParallel(b *testing.B) {
	switcher, err := NewSourceSwitcher("bench-room")
	if err != nil {
		b.Fatalf("Failed to create SourceSwitcher: %v", err)
	}
	defer switcher.Close()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		rtpPacket := make([]byte, 1200)
		rtpPacket[0] = 0x80
		rtpPacket[1] = 96
		seq := 0

		for pb.Next() {
			rtpPacket[2] = byte(seq >> 8)
			rtpPacket[3] = byte(seq)
			seq++
			switcher.InjectSFUPacket(true, rtpPacket)
		}
	})
}

func BenchmarkSourceSwitcherSwitch(b *testing.B) {
	switcher, err := NewSourceSwitcher("bench-room")
	if err != nil {
		b.Fatalf("Failed to create SourceSwitcher: %v", err)
	}
	defer switcher.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if i%2 == 0 {
			switcher.StartLocalShare("sharer")
		} else {
			switcher.StopLocalShare()
		}
	}
}
