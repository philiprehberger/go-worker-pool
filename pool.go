// Package workerpool provides a bounded goroutine pool for Go.
package workerpool

import (
	"context"
	"sync"
)

// Pool is a bounded goroutine pool that limits the number of concurrently
// running goroutines. It uses a buffered channel as a semaphore to enforce
// backpressure — when all worker slots are occupied, Submit blocks until
// a slot becomes available.
type Pool struct {
	workers chan struct{}
	wg      sync.WaitGroup
	mu      sync.Mutex
	stopped bool
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
	p.mu.Unlock()

	p.workers <- struct{}{} // acquire semaphore slot (blocks if full)
	p.wg.Add(1)
	go func() {
		defer func() {
			<-p.workers // release semaphore slot
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
	p.mu.Unlock()

	select {
	case p.workers <- struct{}{}: // acquire semaphore slot
		p.wg.Add(1)
		go func() {
			defer func() {
				<-p.workers
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
	return len(p.workers)
}
