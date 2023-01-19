package timewheel

import (
	"sync"
	"testing"
	"time"
)

func TestNewTimeWheel(t *testing.T) {
	go func() {
		for {
			time.Sleep(time.Second)
		}
	}()

	tw := NewTimeWheel(time.Millisecond, 10)
	tw.Start()

	wg := sync.WaitGroup{}

	printNow := func(msg string) func() {
		return func() {
			t.Logf("%s\t%s", time.Now().Format("2006-01-02 15:04:05.000"), msg)
			wg.Done()
		}
	}

	wg.Add(2)
	tw.AfterFunc(time.Millisecond*50, printNow("50ms"))
	tw.AfterFunc(time.Millisecond*150, printNow("150ms"))
	wg.Wait()
}
