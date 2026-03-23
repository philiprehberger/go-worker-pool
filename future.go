package workerpool

import "time"

// Future represents a value that will be available at some point in the future.
// It is safe to call Get and Done from multiple goroutines concurrently.
type Future[T any] struct {
	ch  chan struct{}
	val T
	err error
}

// Go submits a function that returns a value and an error to the pool,
// and returns a Future that can be used to retrieve the result. The function
// runs asynchronously in the pool. Use Get to block until the result is ready.
func Go[T any](p *Pool, fn func() (T, error)) *Future[T] {
	f := &Future[T]{
		ch: make(chan struct{}),
	}
	p.Submit(func() {
		f.val, f.err = fn()
		close(f.ch)
	})
	return f
}

// Get blocks until the future's result is available and returns the value
// and error produced by the submitted function.
func (f *Future[T]) Get() (T, error) {
	<-f.ch
	return f.val, f.err
}

// Done reports whether the future's result is available without blocking.
// It returns true if the submitted function has completed.
func (f *Future[T]) Done() bool {
	select {
	case <-f.ch:
		return true
	default:
		return false
	}
}

// GoTimeout submits a function that returns a value and an error to the pool,
// waiting at most duration d for a worker slot. It returns a Future and nil
// error on success, or nil and ErrSubmitTimeout if no slot becomes available
// within the deadline.
func GoTimeout[T any](p *Pool, fn func() (T, error), d time.Duration) (*Future[T], error) {
	f := &Future[T]{
		ch: make(chan struct{}),
	}
	err := p.SubmitTimeout(func() {
		f.val, f.err = fn()
		close(f.ch)
	}, d)
	if err != nil {
		return nil, err
	}
	return f, nil
}
