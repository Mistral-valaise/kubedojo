package ratelimit

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestLimiter(t *testing.T) {
	now := time.Unix(1000, 0)
	l := NewLimiter(10, time.Minute)
	l.SetClock(func() time.Time { return now })

	for i := range 10 {
		if ok, _ := l.Allow("a"); !ok {
			t.Fatalf("request %d of the burst was refused", i+1)
		}
	}
	ok, wait := l.Allow("a")
	if ok || wait <= 0 || wait > 6*time.Second {
		t.Fatalf("11th request: ok=%v wait=%s, want a refusal with a wait of up to 6s", ok, wait)
	}
	// Another key has its own bucket.
	if ok, _ := l.Allow("b"); !ok {
		t.Error("key b was refused because of key a")
	}
	// One token comes back every six seconds.
	now = now.Add(5 * time.Second)
	if ok, _ := l.Allow("a"); ok {
		t.Error("allowed after 5s")
	}
	now = now.Add(time.Second)
	if ok, _ := l.Allow("a"); !ok {
		t.Error("refused after 6s")
	}
	if ok, _ := l.Allow("a"); ok {
		t.Error("two requests on one token")
	}
	// A long pause refills to the burst and not beyond it.
	now = now.Add(time.Hour)
	n := 0
	for {
		if ok, _ := l.Allow("a"); !ok {
			break
		}
		n++
	}
	if n != 10 {
		t.Errorf("%d requests after a long pause, want 10", n)
	}
}

func TestLimiterForgetsIdleKeys(t *testing.T) {
	now := time.Unix(1000, 0)
	l := NewLimiter(2, time.Minute)
	l.SetClock(func() time.Time { return now })
	for i := range maxKeys {
		l.Allow(fmt.Sprint("k", i))
	}
	now = now.Add(time.Hour)
	l.Allow("one more")
	if len(l.buckets) > 2 {
		t.Errorf("%d buckets kept after every key was idle for an hour", len(l.buckets))
	}
}

func TestGate(t *testing.T) {
	g := NewGate(2)
	a, err := g.Acquire("a")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := g.Acquire("a"); !errors.Is(err, ErrBusy) {
		t.Errorf("second stream for the same key: %v", err)
	}
	b, err := g.Acquire("b")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := g.Acquire("c"); !errors.Is(err, ErrFull) {
		t.Errorf("third stream: %v", err)
	}
	a()
	a() // releasing twice frees one slot, not two
	if g.Running() != 1 {
		t.Errorf("running = %d", g.Running())
	}
	c, err := g.Acquire("c")
	if err != nil {
		t.Fatalf("after a release: %v", err)
	}
	b()
	c()
	if g.Running() != 0 {
		t.Errorf("running = %d at the end", g.Running())
	}
}

func TestGateConcurrently(t *testing.T) {
	g := NewGate(4)
	var wg sync.WaitGroup
	var mu sync.Mutex
	peak, cur := 0, 0
	for i := range 64 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			release, err := g.Acquire(fmt.Sprint(i % 8))
			if err != nil {
				return
			}
			mu.Lock()
			cur++
			peak = max(peak, cur)
			mu.Unlock()
			time.Sleep(time.Millisecond)
			mu.Lock()
			cur--
			mu.Unlock()
			release()
		}()
	}
	wg.Wait()
	if peak > 4 || g.Running() != 0 {
		t.Errorf("peak = %d (limit 4), running at the end = %d", peak, g.Running())
	}
}
