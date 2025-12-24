/*
 * @Author: Marlon.M
 * @Email: maiguangyang@163.com
 * @Date: 2025-12-24
 *
 * Buffer Pool - RTP 包缓冲池
 * 使用 sync.Pool 复用内存，减少 GC 压力
 * 适用于高频 RTP 包转发场景
 */
package sfu

import (
	"sync"
	"sync/atomic"
)

const (
	// DefaultRTPBufferSize RTP 包默认缓冲区大小（MTU）
	DefaultRTPBufferSize = 1500
	// LargeRTPBufferSize 大包缓冲区大小
	LargeRTPBufferSize = 65535
)

// BufferPool RTP 包缓冲池
type BufferPool struct {
	// 标准大小池（1500 bytes，适用于大多数 RTP 包）
	standardPool sync.Pool
	// 大包池（65535 bytes，适用于聚合包）
	largePool sync.Pool

	// 统计
	standardAllocs uint64
	standardReuses uint64
	largeAllocs    uint64
	largeReuses    uint64
}

// 全局缓冲池实例
var globalBufferPool = NewBufferPool()

// NewBufferPool 创建缓冲池
func NewBufferPool() *BufferPool {
	return &BufferPool{
		standardPool: sync.Pool{
			New: func() interface{} {
				return make([]byte, DefaultRTPBufferSize)
			},
		},
		largePool: sync.Pool{
			New: func() interface{} {
				return make([]byte, LargeRTPBufferSize)
			},
		},
	}
}

// GetBuffer 获取标准缓冲区
func (p *BufferPool) GetBuffer() []byte {
	buf := p.standardPool.Get().([]byte)
	if cap(buf) >= DefaultRTPBufferSize {
		atomic.AddUint64(&p.standardReuses, 1)
	} else {
		atomic.AddUint64(&p.standardAllocs, 1)
	}
	return buf[:DefaultRTPBufferSize]
}

// PutBuffer 归还标准缓冲区
func (p *BufferPool) PutBuffer(buf []byte) {
	if cap(buf) >= DefaultRTPBufferSize {
		p.standardPool.Put(buf[:cap(buf)])
	}
}

// GetLargeBuffer 获取大缓冲区
func (p *BufferPool) GetLargeBuffer() []byte {
	buf := p.largePool.Get().([]byte)
	if cap(buf) >= LargeRTPBufferSize {
		atomic.AddUint64(&p.largeReuses, 1)
	} else {
		atomic.AddUint64(&p.largeAllocs, 1)
	}
	return buf[:LargeRTPBufferSize]
}

// PutLargeBuffer 归还大缓冲区
func (p *BufferPool) PutLargeBuffer(buf []byte) {
	if cap(buf) >= LargeRTPBufferSize {
		p.largePool.Put(buf[:cap(buf)])
	}
}

// GetBufferWithSize 获取指定大小的缓冲区
func (p *BufferPool) GetBufferWithSize(size int) []byte {
	if size <= DefaultRTPBufferSize {
		return p.GetBuffer()[:size]
	}
	return p.GetLargeBuffer()[:size]
}

// PutBufferWithSize 归还缓冲区（自动判断大小）
func (p *BufferPool) PutBufferWithSize(buf []byte) {
	if cap(buf) >= LargeRTPBufferSize {
		p.PutLargeBuffer(buf)
	} else if cap(buf) >= DefaultRTPBufferSize {
		p.PutBuffer(buf)
	}
	// 太小的 buffer 不回收
}

// Stats 统计信息
type BufferPoolStats struct {
	StandardAllocs uint64  `json:"standard_allocs"`
	StandardReuses uint64  `json:"standard_reuses"`
	LargeAllocs    uint64  `json:"large_allocs"`
	LargeReuses    uint64  `json:"large_reuses"`
	ReuseRatio     float64 `json:"reuse_ratio"`
}

// GetStats 获取统计信息
func (p *BufferPool) GetStats() BufferPoolStats {
	standardAllocs := atomic.LoadUint64(&p.standardAllocs)
	standardReuses := atomic.LoadUint64(&p.standardReuses)
	largeAllocs := atomic.LoadUint64(&p.largeAllocs)
	largeReuses := atomic.LoadUint64(&p.largeReuses)

	totalOps := standardAllocs + standardReuses + largeAllocs + largeReuses
	totalReuses := standardReuses + largeReuses

	var reuseRatio float64
	if totalOps > 0 {
		reuseRatio = float64(totalReuses) / float64(totalOps)
	}

	return BufferPoolStats{
		StandardAllocs: standardAllocs,
		StandardReuses: standardReuses,
		LargeAllocs:    largeAllocs,
		LargeReuses:    largeReuses,
		ReuseRatio:     reuseRatio,
	}
}

// ResetStats 重置统计
func (p *BufferPool) ResetStats() {
	atomic.StoreUint64(&p.standardAllocs, 0)
	atomic.StoreUint64(&p.standardReuses, 0)
	atomic.StoreUint64(&p.largeAllocs, 0)
	atomic.StoreUint64(&p.largeReuses, 0)
}

// 全局便捷函数
func GetRTPBuffer() []byte {
	return globalBufferPool.GetBuffer()
}

func PutRTPBuffer(buf []byte) {
	globalBufferPool.PutBuffer(buf)
}

func GetRTPLargeBuffer() []byte {
	return globalBufferPool.GetLargeBuffer()
}

func PutRTPLargeBuffer(buf []byte) {
	globalBufferPool.PutLargeBuffer(buf)
}

func GetGlobalBufferPoolStats() BufferPoolStats {
	return globalBufferPool.GetStats()
}

func ResetGlobalBufferPoolStats() {
	globalBufferPool.ResetStats()
}
