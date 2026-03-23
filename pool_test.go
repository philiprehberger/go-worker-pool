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

func TestStatsInitial(t *testing.T) {
	p := New(4)
	s := p.Stats()
	if s.Workers != 4 {
		t.Fatalf("expected Workers=4, got %d", s.Workers)
	}
	if s.Active != 0 {
		t.Fatalf("expected Active=0, got %d", s.Active)
	}
	if s.Completed != 0 {
		t.Fatalf("expected Completed=0, got %d", s.Completed)
	}
}

func TestStatsTracking(t *testing.T) {
	p := New(2)
	started := make(chan struct{})
	release := make(chan struct{})

	p.Submit(func() {
		close(started)
		<-release
	})
	<-started

	s := p.Stats()
	if s.Active != 1 {
		t.Fatalf("expected Active=1, got %d", s.Active)
	}
	if s.Completed != 0 {
		t.Fatalf("expected Completed=0, got %d", s.Completed)
	}

	close(release)
	p.Wait()

	s = p.Stats()
	if s.Active != 0 {
		t.Fatalf("expected Active=0, got %d", s.Active)
	}
	if s.Completed != 1 {
		t.Fatalf("expected Completed=1, got %d", s.Completed)
	}
}

func TestStatsCompletedAccumulates(t *testing.T) {
	p := New(4)
	for i := 0; i < 50; i++ {
		p.Submit(func() {})
	}
	p.Wait()

	s := p.Stats()
	if s.Completed != 50 {
		t.Fatalf("expected Completed=50, got %d", s.Completed)
	}
}

func TestSubmitTimeoutSuccess(t *testing.T) {
	p := New(2)
	var done atomic.Bool

	err := p.SubmitTimeout(func() {
		done.Store(true)
	}, time.Second)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	p.Wait()
	if !done.Load() {
		t.Fatal("expected task to complete")
	}
}

func TestSubmitTimeoutExpires(t *testing.T) {
	p := New(1)
	started := make(chan struct{})
	release := make(chan struct{})

	// Fill the single worker slot.
	p.Submit(func() {
		close(started)
		<-release
	})
	<-started

	err := p.SubmitTimeout(func() {
		t.Fatal("should not have run")
	}, 20*time.Millisecond)

	if err != ErrSubmitTimeout {
		t.Fatalf("expected ErrSubmitTimeout, got %v", err)
	}

	close(release)
	p.Wait()
}

func TestSubmitTimeoutPanicsOnStopped(t *testing.T) {
	p := New(2)
	p.Stop()

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on SubmitTimeout after Stop")
		}
	}()

	_ = p.SubmitTimeout(func() {}, time.Second)
}

func TestResizeIncrease(t *testing.T) {
	p := New(2)

	p.Resize(4)

	s := p.Stats()
	if s.Workers != 4 {
		t.Fatalf("expected Workers=4 after resize, got %d", s.Workers)
	}

	// Verify the pool works with new capacity.
	var count atomic.Int64
	for i := 0; i < 10; i++ {
		p.Submit(func() {
			count.Add(1)
		})
	}
	p.Wait()

	if got := count.Load(); got != 10 {
		t.Fatalf("expected 10 tasks completed, got %d", got)
	}
}

func TestResizeDecrease(t *testing.T) {
	p := New(4)

	p.Resize(2)

	s := p.Stats()
	if s.Workers != 2 {
		t.Fatalf("expected Workers=2 after resize, got %d", s.Workers)
	}

	// Verify the pool still works.
	var count atomic.Int64
	for i := 0; i < 10; i++ {
		p.Submit(func() {
			count.Add(1)
		})
	}
	p.Wait()

	if got := count.Load(); got != 10 {
		t.Fatalf("expected 10 tasks completed, got %d", got)
	}
}

func TestResizePanicsOnZero(t *testing.T) {
	p := New(2)
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for resize to 0")
		}
	}()
	p.Resize(0)
}

func TestResizeSameSize(t *testing.T) {
	p := New(3)
	p.Resize(3) // should be a no-op

	s := p.Stats()
	if s.Workers != 3 {
		t.Fatalf("expected Workers=3, got %d", s.Workers)
	}
}

func TestDrain(t *testing.T) {
	p := New(2)
	var count atomic.Int64

	// Submit some tasks.
	for i := 0; i < 5; i++ {
		p.Submit(func() {
			time.Sleep(10 * time.Millisecond)
			count.Add(1)
		})
	}

	// Drain waits for active tasks and temporarily stops accepting new ones.
	p.Drain()

	completed := count.Load()
	if completed != 5 {
		t.Fatalf("expected 5 tasks completed after drain, got %d", completed)
	}

	// Pool should be usable again after Drain.
	p.Submit(func() {
		count.Add(1)
	})
	p.Wait()

	if got := count.Load(); got != 6 {
		t.Fatalf("expected 6 total tasks, got %d", got)
	}
}

func TestDrainWithActiveTasks(t *testing.T) {
	p := New(2)
	var done atomic.Bool

	p.Submit(func() {
		time.Sleep(50 * time.Millisecond)
		done.Store(true)
	})

	p.Drain()

	if !done.Load() {
		t.Fatal("Drain returned before active task completed")
	}
}
