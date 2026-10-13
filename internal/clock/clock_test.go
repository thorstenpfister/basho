package clock

import (
	"sync"
	"testing"
	"time"
)

func TestFakeAdvance(t *testing.T) {
	start := time.Date(2026, 11, 8, 5, 0, 0, 0, time.UTC)
	f := NewFake(start)
	// Concurrent use, checked by `make test-race`.
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			f.Advance(time.Second)
			_ = f.Now()
		})
	}
	wg.Wait()
	if got := f.Now(); !got.Equal(start.Add(8 * time.Second)) {
		t.Errorf("Now = %v, want %v", got, start.Add(8*time.Second))
	}
}
