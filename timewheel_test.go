package timewheel

import (
	"sync"
	"testing"
	"time"
)

func TestTimeWheelAfterFunc(t *testing.T) {
	go func() {
		for {
			time.Sleep(time.Second)
		}
	}()

	startTime := time.Now()
	tw := NewTimeWheel(time.Millisecond*10, 10)
	tw.Start()

	wg := &sync.WaitGroup{}

	printNow := func(msg string) func() {
		return func() {
			now := time.Now()
			t.Logf(
				"%s\t%s\telapse:%s",
				now.Format("2006-01-02 15:04:05.000"),
				msg,
				now.Sub(startTime),
			)
			wg.Done()
		}
	}

	tests := []struct {
		name  string
		after time.Duration
	}{
		{
			name:  "1ms",
			after: time.Millisecond * 1,
		},
		{
			name:  "4ms",
			after: time.Millisecond * 4,
		},
		{
			name:  "10ms",
			after: time.Millisecond * 10,
		},
		{
			name:  "14ms",
			after: time.Millisecond * 14,
		},
		{
			name:  "18ms",
			after: time.Millisecond * 18,
		},
		{
			name:  "28ms",
			after: time.Millisecond * 28,
		},
		{
			name:  "99ms",
			after: time.Millisecond * 99,
		},
		{
			name:  "100ms",
			after: time.Millisecond * 100,
		},
		{
			name:  "101ms",
			after: time.Millisecond * 101,
		},
		{
			name:  "143ms",
			after: time.Millisecond * 143,
		},
		{
			name:  "234ms",
			after: time.Millisecond * 234,
		},
		{
			name:  "1000ms",
			after: time.Millisecond * 1000,
		},
		{
			name:  "1234ms",
			after: time.Millisecond * 1234,
		},
	}
	wg.Add(len(tests))
	for _, test := range tests {
		tw.AfterFunc(test.after, printNow(test.name))
	}
	t.Logf("StartTime: %s", startTime)
	wg.Wait()
	tw.Stop()
}

func TestTimeWheelScheduleFunc(t *testing.T) {
	go func() {
		for {
			time.Sleep(time.Second)
		}
	}()

	var wait = make(chan struct{}, 1)
	startTime := time.Now()
	tw := NewTimeWheel(time.Millisecond*1, 10)

	tw.Start()
	tw.ScheduleFunc(NewEvery(time.Millisecond*10), func() {
		now := time.Now()
		t.Logf("%s\telapse:%s", now, now.Sub(startTime))
		select {
		case wait <- struct{}{}:
		default:

		}
	})

	for i := 0; i < 100; i++ {
		<-wait
	}
	tw.Stop()
}
