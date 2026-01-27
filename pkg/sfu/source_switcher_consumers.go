package sfu

import (
	"sync/atomic"

	"github.com/maiguangyang/relay_core/pkg/utils"
)

func (ss *SourceSwitcher) SetConsumerCount(count int) {
	ss.mu.Lock()
	defer ss.mu.Unlock()

	oldCount := atomic.LoadInt32(&ss.consumerCount)
	newCount := int32(count)
	atomic.StoreInt32(&ss.consumerCount, newCount)

	oldHasConsumers := oldCount > 0
	newHasConsumers := newCount > 0

	if oldHasConsumers != newHasConsumers {
		utils.Info("[Switcher] Consumer state changed: %v -> %v (count: %d)", oldHasConsumers, newHasConsumers, count)
		if ss.onConsumerStateChanged != nil {
			// 异步调用回调，避免死锁
			go ss.onConsumerStateChanged(newHasConsumers)
		}
	}
}

// IncrementConsumerCount 增加消费者计数
func (ss *SourceSwitcher) IncrementConsumerCount() {
	ss.mu.Lock()
	defer ss.mu.Unlock()

	val := atomic.LoadInt32(&ss.consumerCount)
	ss.SetConsumerCount(int(val) + 1)
}

// DecrementConsumerCount 减少消费者计数
func (ss *SourceSwitcher) DecrementConsumerCount() {
	ss.mu.Lock()
	defer ss.mu.Unlock()

	val := atomic.LoadInt32(&ss.consumerCount)
	if val > 0 {
		ss.SetConsumerCount(int(val) - 1)
	}
}

// HasConsumers 是否有消费者
func (ss *SourceSwitcher) HasConsumers() bool {
	return atomic.LoadInt32(&ss.consumerCount) > 0
}

// SetOnConsumerStateChanged 设置消费者状态变更回调
func (ss *SourceSwitcher) SetOnConsumerStateChanged(f func(hasConsumers bool)) {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	ss.onConsumerStateChanged = f
}
