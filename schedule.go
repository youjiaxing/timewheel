package timewheel

import "time"

type Scheduler interface {
	// Next 返回下一次执行的时间
	// 若返回空则表示不再执行
	Next(time.Time) time.Time
}

// EverySecond 每秒
type EverySecond struct {
}

func (s EverySecond) Next(t time.Time) time.Time {
	return t.Add(time.Second)
}

type Every struct {
	Delay time.Duration
}

func NewEvery(delay time.Duration) *Every {
	return &Every{Delay: delay}
}

func (e Every) Next(t time.Time) time.Time {
	return t.Add(e.Delay)
}
