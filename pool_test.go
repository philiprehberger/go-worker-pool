package workerpool

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestSubmitAndWait(t *testing.T) {
	p := New(4)
	var count atomic.Int64

	for i := 0; i < 100; i++ {
		p.Submit(func() {
			count.Add(1)
		})
	}

	p.Wait()

	if got := count.Load(); got != 100 {
		t.Fatalf("expected 100 tasks completed, got %d", got)
	}
}

func TestConcurrencyLimit(t *testing.T) {
	const maxWorkers = 2
	p := New(maxWorkers)

	var running atomic.Int64
	var maxRunning atomic.Int64
	var mu sync.Mutex

	for i := 0; i < 10; i++ {
		p.Submit(func() {
			cur := running.Add(1)

			mu.Lock()
			if cur > maxRunning.Load() {
				maxRunning.Store(cur)
			}
			mu.Unlock()

			time.Sleep(10 * time.Millisecond)
			running.Add(-1)
		})
	}

	p.Wait()

	if got := maxRunning.Load(); got > maxWorkers {
		t.Fatalf("max concurrent goroutines was %d, expected at most %d", got, maxWorkers)
	}
	if got := maxRunning.Load(); got == 0 {
		t.Fatal("expected at least some concurrency, got 0")
	}
}

func TestSubmitCtxCancelled(t *testing.T) {
	p := New(1)

	// Fill the single worker slot with a long-running task.
	started := make(chan struct{})
	p.Submit(func() {
		close(started)
		time.Sleep(200 * time.Millisecond)
	})
	<-started // ensure the worker is busy

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	err := p.SubmitCtx(ctx, func() {
		t.Fatal("should not have run")
	})

	if err != context.Canceled {
		t.Fatalf("expected context.Canceled, got %v", err)
	}

	p.Wait()
}

func TestSubmitCtxSuccess(t *testing.T) {
	p := New(2)
	var done atomic.Bool

	err := p.SubmitCtx(context.Background(), func() {
		done.Store(true)
	})

	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	p.Wait()

	if !done.Load() {
		t.Fatal("expected task to complete")
	}
}

func TestStopPreventsSubmit(t *testing.T) {
	p := New(2)
	p.Stop()

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic on Submit after Stop")
		}
	}()

	p.Submit(func() {})
}

func TestStopPreventsSubmitCtx(t *testing.T) {
	p := New(2)
	p.Stop()

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic on SubmitCtx after Stop")
		}
	}()

	_ = p.SubmitCtx(context.Background(), func() {})
}

func TestStopWaitsForCompletion(t *testing.T) {
	p := New(2)
	var done atomic.Bool

	p.Submit(func() {
		time.Sleep(50 * time.Millisecond)
		done.Store(true)
	})

	p.Stop()

	if !done.Load() {
		t.Fatal("Stop returned before work completed")
	}
}

func TestRunning(t *testing.T) {
	p := New(4)
	started := make(chan struct{})
	release := make(chan struct{})

	for i := 0; i < 3; i++ {
		p.Submit(func() {
			started <- struct{}{}
			<-release
		})
	}

	// Wait for all 3 to start.
	for i := 0; i < 3; i++ {
		<-started
	}

	r := p.Running()
	if r != 3 {
		t.Fatalf("expected 3 running, got %d", r)
	}

	close(release)
	p.Wait()

	r = p.Running()
	if r != 0 {
		t.Fatalf("expected 0 running after Wait, got %d", r)
	}
}

func TestNewPanicsOnZero(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for concurrency 0")
		}
	}()
	New(0)
}

func TestNewPanicsOnNegative(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for negative concurrency")
		}
	}()
	New(-1)
}

func TestBackpressure(t *testing.T) {
	p := New(1)
	release := make(chan struct{})
	started := make(chan struct{})

	// Submit a task that blocks until released.
	p.Submit(func() {
		close(started)
		<-release
	})
	<-started // ensure the worker is busy

	// Submit in a goroutine — it should block because the pool is full.
	submitted := make(chan struct{})
	go func() {
		p.Submit(func() {})
		close(submitted)
	}()

	// Give the goroutine a moment to potentially submit (it should not).
	select {
	case <-submitted:
		t.Fatal("Submit should have blocked due to backpressure")
	case <-time.After(50 * time.Millisecond):
		// Expected: Submit is blocked.
	}

	// Release the first task so the second can proceed.
	close(release)

	select {
	case <-submitted:
		// Good: second task was submitted after first completed.
	case <-time.After(2 * time.Second):
		t.Fatal("Submit did not unblock after worker slot freed")
	}

	p.Wait()
}
