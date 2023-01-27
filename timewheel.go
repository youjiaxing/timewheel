package timewheel

import (
	"fmt"
	"strconv"
	"strings"
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
	overflow    *TimeWheel // 下一层时间轮（单位时间更大）
	prevTW      *TimeWheel // 上一层时间轮
	firstTW     *TimeWheel // 首层时间轮
	level       int
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
		firstTW:     nil,
		prevTW:      nil,
		level:       1,
		Locker:      &sync.Mutex{},
		wg:          &sync.WaitGroup{},
		exitC:       make(chan struct{}),
	}
	tw.firstTW = tw
	return tw
}

func (w *TimeWheel) Start() {
	w.wg.Add(1)
	go func() {
		defer w.wg.Done()

		ticker := time.NewTicker(time.Duration(w.ticketMS) * time.Millisecond)
		defer ticker.Stop()

		startTime := w.curTicketMS
		_ = startTime

		for {
			select {
			case <-w.exitC:
				return
			case <-ticker.C:
				w.Lock()
				now := time.Now().Truncate(time.Millisecond)
				elapsed := now.Sub(time.UnixMilli(startTime))
				elapsedMS := elapsed.Milliseconds()
				_ = elapsedMS
				//fmt.Printf("Tick %s\tElapse:%s\n", now, elapsed)
				w.advanceClock(now)
				//w.printDebug()
				w.Unlock()
			}
		}
	}()
}

func (w *TimeWheel) printDebug() {
	bucketStat := make([]string, 0, len(w.buckets))
	for _, o := range w.buckets {
		bucketStat = append(bucketStat, strconv.Itoa(o.timers.Len()))
	}
	bucketStat[w.curBucket] = fmt.Sprintf("[%s]", bucketStat[w.curBucket])
	fmt.Printf("TimeWheel【%d-%d-%d】%s\n", w.level, w.ticketMS, w.wheelSize, strings.Join(bucketStat, ","))
	if w.overflow != nil {
		w.overflow.printDebug()
	}
}

func (w *TimeWheel) advanceClock(now time.Time) {
	elapseMS := now.UnixMilli() - w.curTicketMS

	// 时间不足以跨一个刻度
	if elapseMS < w.ticketMS {
		return
	}

	elapseTick := int(elapseMS / w.ticketMS)
	w.curTicketMS = CalcCurTicketMS(w.ticketMS, now)
	if int64(elapseTick) >= w.intervalMS {
		// 避免时间跳跃过大导致浪费
		elapseTick = w.wheelSize + (elapseTick % w.wheelSize)
	}

	for i := 0; i < elapseTick; i++ {
		w.curBucket = (w.curBucket + 1) % w.wheelSize

		trigger := w.buckets[w.curBucket].Flush()
		if len(trigger) == 0 {
			continue
		}

		for _, timer := range trigger {
			w.firstTW.addTimer(timer)
		}
	}

	// 仅在一个循环后再推动下层
	if w.overflow != nil {
		w.overflow.advanceClock(now)
	}
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
		tw.curTicketMS = CalcCurTicketMS(tw.ticketMS, time.UnixMilli(w.curTicketMS))
		tw.wg = w.wg
		tw.Locker = w
		tw.exitC = w.exitC
		tw.firstTW = w.firstTW
		tw.prevTW = w
		tw.level = w.level + 1

		w.overflow = tw
	}
	return w.overflow
}

func (w *TimeWheel) addTimer(t *Timer) {
	// 已取消
	if t.IsCanceled() {
		return
	}

	if w.stopSign {
		return
	}

	delay := t.expireMS - w.curTicketMS

	// 已超时
	if delay < w.ticketMS {
		w.wg.Add(1)
		go func() {
			defer w.wg.Done()
			t.cb()
		}()
		return
	}

	// 属于当前层时间范围
	if delay < w.intervalMS {
		targetIdx := (int(delay/w.ticketMS) + w.curBucket) % w.wheelSize
		t.wheel = w
		t.bucket = w.buckets[targetIdx]
		w.buckets[targetIdx].AddTimer(t)
		return
	}

	// 往下一层放
	w.getNextTW().addTimer(t)
}

func (w *TimeWheel) AfterFunc(after time.Duration, cb func()) *Timer {
	w.Lock()
	defer w.Unlock()

	t := &Timer{
		expireMS: time.Now().UnixMilli() + after.Milliseconds(),
		cb:       cb,
	}
	w.addTimer(t)
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
			cb()

			next := scheduler.Next(time.UnixMilli(t.expireMS))
			if !next.IsZero() {
				// 防止执行间隔过短导致的额外重复执行
				if next.UnixMilli()-w.curTicketMS < w.ticketMS {
					next = time.UnixMilli(w.curTicketMS + w.ticketMS)
				}
				t.expireMS = next.UnixMilli()

				w.Lock()
				defer w.Unlock()
				w.firstTW.addTimer(t)
			}
		},
	}
	w.addTimer(t)
	return t
}
