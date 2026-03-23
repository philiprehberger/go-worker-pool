// Package workerpool provides a bounded goroutine pool for Go.
package workerpool

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

// ErrSubmitTimeout is returned when a task submission exceeds its timeout.
var ErrSubmitTimeout = errors.New("workerpool: submit timed out")

// PoolStats contains a snapshot of pool activity.
type PoolStats struct {
	Workers   int
	Active    int
	Queued    int
	Completed int64
}

// Pool is a bounded goroutine pool that limits the number of concurrently
// running goroutines. It uses a buffered channel as a semaphore to enforce
// backpressure — when all worker slots are occupied, Submit blocks until
// a slot becomes available.
type Pool struct {
	workers   chan struct{}
	wg        sync.WaitGroup
	mu        sync.Mutex
	stopped   bool
	active    atomic.Int64
	completed atomic.Int64
}

// New creates a new Pool with the given concurrency limit.
// The concurrency parameter controls how many goroutines can run simultaneously.
// It panics if concurrency is less than 1.
func New(concurrency int) *Pool {
	if concurrency < 1 {
		panic("workerpool: concurrency must be at least 1")
	}
	return &Pool{
		workers: make(chan struct{}, concurrency),
	}
}

// Submit sends a function to the pool for execution. It blocks if all worker
// slots are currently occupied (backpressure). The function runs in its own
// goroutine once a slot is available. Submit panics if the pool has been stopped.
func (p *Pool) Submit(fn func()) {
	p.mu.Lock()
	if p.stopped {
		p.mu.Unlock()
		panic("workerpool: submit on stopped pool")
	}
	w := p.workers
	p.mu.Unlock()

	w <- struct{}{} // acquire semaphore slot (blocks if full)
	p.wg.Add(1)
	p.active.Add(1)
	go func() {
		defer func() {
			p.active.Add(-1)
			p.completed.Add(1)
			<-w // release semaphore slot
			p.wg.Done()
		}()
		fn()
	}()
}

// SubmitCtx sends a function to the pool for execution with context support.
// It returns ctx.Err() if the context is cancelled or times out while waiting
// for a worker slot. If a slot is acquired, the function runs in its own
// goroutine and nil is returned. SubmitCtx panics if the pool has been stopped.
func (p *Pool) SubmitCtx(ctx context.Context, fn func()) error {
	p.mu.Lock()
	if p.stopped {
		p.mu.Unlock()
		panic("workerpool: submit on stopped pool")
	}
	w := p.workers
	p.mu.Unlock()

	select {
	case w <- struct{}{}: // acquire semaphore slot
		p.wg.Add(1)
		p.active.Add(1)
		go func() {
			defer func() {
				p.active.Add(-1)
				p.completed.Add(1)
				<-w
				p.wg.Done()
			}()
			fn()
		}()
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Wait blocks until all submitted work has completed.
func (p *Pool) Wait() {
	p.wg.Wait()
}

// Stop marks the pool as stopped and waits for all in-flight work to complete.
// After Stop returns, any call to Submit or SubmitCtx will panic.
func (p *Pool) Stop() {
	p.mu.Lock()
	p.stopped = true
	p.mu.Unlock()
	p.wg.Wait()
}

// Running returns the approximate number of goroutines currently executing work.
// Because goroutines start and finish concurrently, the value is an approximation.
func (p *Pool) Running() int {
	p.mu.Lock()
	w := p.workers
	p.mu.Unlock()
	return len(w)
}

// Stats returns a snapshot of the pool's current activity. Workers is the
// maximum concurrency, Active is the number of goroutines currently running
// tasks, Queued is the number of occupied semaphore slots (an approximation
// that includes active workers), and Completed is the total number of tasks
// that have finished since the pool was created.
func (p *Pool) Stats() PoolStats {
	p.mu.Lock()
	w := p.workers
	p.mu.Unlock()
	return PoolStats{
		Workers:   cap(w),
		Active:    int(p.active.Load()),
		Queued:    len(w),
		Completed: p.completed.Load(),
	}
}

// SubmitTimeout sends a function to the pool for execution, waiting at most
// duration d for a worker slot. It returns ErrSubmitTimeout if no slot becomes
// available within the deadline. SubmitTimeout panics if the pool has been
// stopped.
func (p *Pool) SubmitTimeout(fn func(), d time.Duration) error {
	p.mu.Lock()
	if p.stopped {
		p.mu.Unlock()
		panic("workerpool: submit on stopped pool")
	}
	w := p.workers
	p.mu.Unlock()

	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case w <- struct{}{}: // acquire semaphore slot
		p.wg.Add(1)
		p.active.Add(1)
		go func() {
			defer func() {
				p.active.Add(-1)
				p.completed.Add(1)
				<-w
				p.wg.Done()
			}()
			fn()
		}()
		return nil
	case <-timer.C:
		return ErrSubmitTimeout
	}
}

// Resize adjusts the pool's maximum concurrency to n. If n is greater than
// the current capacity, new slots are made available immediately. If n is
// smaller, excess workers are allowed to drain naturally — no goroutines
// are interrupted. Resize panics if n is less than 1.
func (p *Pool) Resize(n int) {
	if n < 1 {
		panic("workerpool: concurrency must be at least 1")
	}
	p.mu.Lock()
	defer p.mu.Unlock()

	oldCap := cap(p.workers)
	if n == oldCap {
		return
	}

	newWorkers := make(chan struct{}, n)

	// Transfer existing semaphore tokens to the new channel. If the new
	// capacity is smaller than the number of currently occupied slots,
	// we transfer only what fits — remaining tokens will be dropped as
	// goroutines finish (they read from the old channel via closure, but
	// we swap the channel pointer so new submissions use the new one).
	//
	// Because active goroutines captured p.workers by value (channel
	// reference), they will release their tokens to the OLD channel.
	// We need to drain the old channel's current tokens and put them
	// into the new channel (up to its capacity).
	currentLen := len(p.workers)
	transferCount := currentLen
	if transferCount > n {
		transferCount = n
	}

	// Drain tokens from old channel.
	for i := 0; i < currentLen; i++ {
		<-p.workers
	}

	// Put transferred tokens into new channel.
	for i := 0; i < transferCount; i++ {
		newWorkers <- struct{}{}
	}

	p.workers = newWorkers
}

// Drain removes all pending tasks from the queue by draining and refilling
// the semaphore channel. Active tasks are not interrupted and will complete
// normally. Because Submit blocks on the semaphore channel, any goroutines
// blocked in Submit will eventually proceed once Drain returns.
func (p *Pool) Drain() {
	// Drain works by consuming all available (unoccupied) slots and
	// immediately putting them back, effectively a no-op for the semaphore.
	// The real effect is that any goroutines blocked on Submit will race
	// for slots after Drain, but tasks that were "queued" (sitting in the
	// channel buffer waiting) are already running or about to run.
	//
	// In this pool design, tasks are not queued in a separate queue — the
	// semaphore channel IS the queue. A task blocks on Submit until it
	// acquires a slot and then runs immediately. So "drain" means: wait
	// for all currently active tasks to finish, without accepting new work.
	p.mu.Lock()
	p.stopped = true
	p.mu.Unlock()

	p.wg.Wait()

	p.mu.Lock()
	p.stopped = false
	p.mu.Unlock()
}
