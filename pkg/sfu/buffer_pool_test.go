/*
 * @Author: Marlon.M
 * @Email: maiguangyang@163.com
 * @Date: 2025-12-24
 *
 * Buffer Pool Tests
 */
package sfu

import (
	"sync"
	"testing"
)

func TestBufferPoolGetPut(t *testing.T) {
	pool := NewBufferPool()

	// 获取缓冲区
	buf := pool.GetBuffer()
	if len(buf) != DefaultRTPBufferSize {
		t.Errorf("Expected buffer size %d, got %d", DefaultRTPBufferSize, len(buf))
	}

	// 归还缓冲区
	pool.PutBuffer(buf)

	// 再次获取应该复用
	buf2 := pool.GetBuffer()
	if len(buf2) != DefaultRTPBufferSize {
		t.Errorf("Expected buffer size %d, got %d", DefaultRTPBufferSize, len(buf2))
	}
}

func TestBufferPoolLargeBuffer(t *testing.T) {
	pool := NewBufferPool()

	buf := pool.GetLargeBuffer()
	if len(buf) != LargeRTPBufferSize {
		t.Errorf("Expected large buffer size %d, got %d", LargeRTPBufferSize, len(buf))
	}

	pool.PutLargeBuffer(buf)
}

func TestBufferPoolStats(t *testing.T) {
	pool := NewBufferPool()

	// 获取一些缓冲区
	for i := 0; i < 10; i++ {
		buf := pool.GetBuffer()
		pool.PutBuffer(buf)
	}

	stats := pool.GetStats()
	if stats.StandardReuses == 0 && stats.StandardAllocs == 0 {
		t.Error("Expected some allocations or reuses")
	}

	// 重置统计
	pool.ResetStats()
	stats = pool.GetStats()
	if stats.StandardReuses != 0 || stats.StandardAllocs != 0 {
		t.Error("Stats should be reset")
	}
}

func TestBufferPoolConcurrency(t *testing.T) {
	pool := NewBufferPool()
	var wg sync.WaitGroup

	// 并发获取和归还
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				buf := pool.GetBuffer()
				// 模拟使用
				buf[0] = byte(j)
				pool.PutBuffer(buf)
			}
		}()
	}

	wg.Wait()
}

func TestGlobalBufferPool(t *testing.T) {
	buf := GetRTPBuffer()
	if len(buf) != DefaultRTPBufferSize {
		t.Errorf("Expected buffer size %d, got %d", DefaultRTPBufferSize, len(buf))
	}
	PutRTPBuffer(buf)

	largeBuf := GetRTPLargeBuffer()
	if len(largeBuf) != LargeRTPBufferSize {
		t.Errorf("Expected large buffer size %d, got %d", LargeRTPBufferSize, len(largeBuf))
	}
	PutRTPLargeBuffer(largeBuf)
}

// ==========================================
// Benchmarks
// ==========================================

func BenchmarkBufferPoolGet(b *testing.B) {
	pool := NewBufferPool()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf := pool.GetBuffer()
		pool.PutBuffer(buf)
	}
}

func BenchmarkBufferPoolGetParallel(b *testing.B) {
	pool := NewBufferPool()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			buf := pool.GetBuffer()
			pool.PutBuffer(buf)
		}
	})
}

func BenchmarkMakeSlice(b *testing.B) {
	// 对比：不使用 pool 直接分配
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf := make([]byte, DefaultRTPBufferSize)
		_ = buf
	}
}

func BenchmarkMakeSliceParallel(b *testing.B) {
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			buf := make([]byte, DefaultRTPBufferSize)
			_ = buf
		}
	})
}
