/*
 * @Author: Marlon.M
 * @Email: maiguangyang@163.com
 * @Date: 2026-01-09
 *
 * Buffer Pool - 字节切片缓存池
 * 用于减少高频 FFI 调用时的内存分配和 GC 压力
 */
package utils

import (
	"sync"
)

// 默认缓冲区大小 (UDP MTU 1500)
// 对于大于此大小的包，Pool 会自动适应
const defaultBufferSize = 2048

var bufferPool = sync.Pool{
	New: func() interface{} {
		// 预分配稍大一点的 buffer，覆盖绝大多数 RTP 包
		return make([]byte, defaultBufferSize)
	},
}

// GetBuffer 获取一个长度为 length 的切片
// 可能会返回复用的切片，也可能分配新的
func GetBuffer(length int) []byte {
	buf := bufferPool.Get().([]byte)

	// 如果请求的长度超过了 cap，我们需要一个新的切片
	// 并将其放回 pool (虽然这会导致 pool 里存了大对象，但对于 RTP 来说通常是 OK 的)
	// 或者您可以选择不复用大对象，直接分配。
	// 这里我们简单的策略：如果 cap 不够，直接分配新的且不从 pool 取。
	// 下次 Put 时会更新 pool 中的对象大小吗？sync.Pool 存的是 interface{}。

	if cap(buf) < length {
		// 如果原有 buffer 太小，直接丢弃（GC 会回收），分配新的更大的
		// 这样 pool 可能会慢慢适应更大的包，或者我们直接分配一次性的
		return make([]byte, length)
	}

	// 复用，重置长度
	return buf[:length]
}

// PutBuffer 将切片放回池中
func PutBuffer(buf []byte) {
	// 如果 slice 太小或太大，可以选择不放回，以保持 pool 的健康
	// 这里我们只放回 cap >= defaultBufferSize 的，避免存入太小的碎片
	if cap(buf) < defaultBufferSize {
		return
	}

	// 也不要存太大的，防止内存占用过高 (比如 > 4KB)
	if cap(buf) > 4096 {
		return
	}

	bufferPool.Put(buf)
}
