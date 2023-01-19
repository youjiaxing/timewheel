package timewheel

import (
	"sync"
	"time"
)

// TimeWheel 分层时间轮
// TODO 限制最大过期时间（比如超过3年的就不允许放入之类的）
type TimeWheel struct {
	curTicketMS int64      // 当前层的时间轮刻度, 计算方式见 CalcCurTicketMS
	ticketMS    int64      // 槽的单位时间
	wheelSize   int        // 槽数量
	intervalMS  int64      // 时间轮最大间隔 = ticketMS * wheelSize
	buckets     []*Bucket  // 槽
	curBucket   int        // 当前所在槽
	overflow    *TimeWheel // 下一层时间轮
	sync.Locker

	wg       *sync.WaitGroup
	exitC    chan struct{}
	stopSign bool
}

func NewTimeWheel(ticket time.Duration, wheelSize int) *TimeWheel {
	ticketMS := ticket.Milliseconds()
	if ticketMS < 0 {
		panic("ticket must greater equal than 1ms")
	}
	if wheelSize < 0 {
		panic("wheelSize must greater than 0")
	}

	buckets := make([]*Bucket, 0, wheelSize)
	for i := 0; i < wheelSize; i++ {
		buckets = append(buckets, NewBucket())
	}

	tw := &TimeWheel{
		curTicketMS: CalcCurTicketMS(ticketMS),
		ticketMS:    ticketMS,
		wheelSize:   wheelSize,
		intervalMS:  ticketMS * int64(wheelSize),
		buckets:     buckets,
		curBucket:   0,
		overflow:    nil,
		Locker:      &sync.Mutex{},
		wg:          &sync.WaitGroup{},
		exitC:       make(chan struct{}),
	}
	return tw
}

func (w *TimeWheel) Start() {
	w.wg.Add(1)
	go func() {
		defer w.wg.Done()

		ticker := time.NewTicker(time.Duration(w.ticketMS) * time.Millisecond)
		defer ticker.Stop()
		select {
		case <-w.exitC:
			return
		case <-ticker.C:
			w.Lock()
			defer w.Unlock()
			now := time.Now()
			w.advanceClock(now)
		}
	}()
}

func (w *TimeWheel) advanceClock(now time.Time) {
	nowMS := now.UnixMilli()

	// 时间不足以跨一个刻度
	if nowMS < w.curTicketMS+w.ticketMS {
		return
	}

	passedTick := int((nowMS - w.curTicketMS) / w.ticketMS)
	if int64(passedTick) >= w.intervalMS {
		// 避免时间跳跃过大导致浪费
		passedTick = w.wheelSize + (passedTick % w.wheelSize)
	}

	for i := 0; i < passedTick; i++ {
		w.curBucket = (w.curBucket + 1) % w.wheelSize
		trigger := w.buckets[w.curBucket].Flush()
		if len(trigger) == 0 {
			continue
		}
		w.wg.Add(len(trigger))
		for _, o := range trigger {
			o := o
			go func() {
				defer w.wg.Done()
				o.cb()
			}()
		}
	}

	w.getNextTW().advanceClock(now)
}

// Stop 可重复调用
func (w *TimeWheel) Stop() {
	w.Lock()
	if !w.stopSign {
		w.stopSign = true
		close(w.exitC)
	}
	w.Unlock()

	w.wg.Wait()
}

func (w *TimeWheel) getNextTW() *TimeWheel {
	if w.overflow == nil {
		tw := NewTimeWheel(time.Millisecond*time.Duration(w.ticketMS*int64(w.wheelSize)), w.wheelSize)
		tw.curTicketMS = w.curTicketMS
		w.overflow = tw
	}
	return w.overflow
}

func (w *TimeWheel) addOrRun(t *Timer) {
	// 已取消
	if t.IsCanceled() {
		return
	}

	if !w.add(t) {
		w.wg.Add(1)
		go func() {
			defer w.wg.Done()
			t.cb()
		}()
	}
}

func (w *TimeWheel) add(t *Timer) bool {
	curTicketMS := w.curTicketMS

	// 已超时
	if t.expireMS < curTicketMS+w.ticketMS {
		return false
	}

	// 属于当前层时间范围
	if t.expireMS < curTicketMS+w.intervalMS {
		targetIdx := (int(t.expireMS/w.ticketMS) + w.curBucket) % w.wheelSize
		t.wheel = w
		w.buckets[targetIdx].AddTimer(t)
		return true
	}

	// 往更高层放
	return w.getNextTW().add(t)
}

func (w *TimeWheel) AfterFunc(after time.Duration, cb func()) *Timer {
	w.Lock()
	defer w.Unlock()

	t := &Timer{
		expireMS: time.Now().Add(after).UnixMilli(),
		cb:       cb,
	}
	w.addOrRun(t)
	return t
}

func (w *TimeWheel) ScheduleFunc(scheduler Scheduler, cb func()) (t *Timer) {
	w.Lock()
	defer w.Unlock()

	next := scheduler.Next(time.Now())
	if next.IsZero() {
		return nil
	}

	t = &Timer{
		expireMS: next.UnixMilli(),
		cb: func() {
			next := scheduler.Next(time.Now())

			cb()

			if !next.IsZero() {
				w.Lock()
				defer w.Unlock()
				w.addOrRun(t)
			}
		},
	}
	w.addOrRun(t)
	return t
}
