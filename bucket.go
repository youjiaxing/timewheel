package timewheel

import (
	"container/list"
	"sync/atomic"
)

type Bucket struct {
	timers *list.List // *Timer
}

func NewBucket() *Bucket {
	return &Bucket{
		timers: list.New(),
	}
}

func (b *Bucket) AddTimer(t *Timer) {
	t.bucket = b
	b.timers.PushBack(t)
}

func (b *Bucket) Flush() []*Timer {
	if b.timers.Len() == 0 {
		return []*Timer{}
	}

	ret := make([]*Timer, 0, b.timers.Len())
	for ele := b.timers.Front(); ele != nil; ele = ele.Next() {
		ret = append(ret, ele.Value.(*Timer))
	}
	b.timers = list.New()
	return ret
}

type Timer struct {
	canceled atomic.Bool // 是否已被取消
	expireMS int64       // 过期时间
	cb       func()      // 执行函数
	wheel    *TimeWheel
	bucket   *Bucket
}

func (t *Timer) IsCanceled() bool {
	return t.canceled.Load()
}

// Cancel 标记取消
func (t *Timer) Cancel() {
	t.canceled.Store(true)
}
