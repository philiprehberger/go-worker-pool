package workerpool

import (
	"errors"
	"sync"
	"testing"
	"time"
)

func TestFutureGetValue(t *testing.T) {
	p := New(2)

	f := Go(p, func() (int, error) {
		return 42, nil
	})

	val, err := f.Get()
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if val != 42 {
		t.Fatalf("expected 42, got %d", val)
	}

	p.Wait()
}

func TestFutureGetError(t *testing.T) {
	p := New(2)
	expectedErr := errors.New("something went wrong")

	f := Go(p, func() (string, error) {
		return "", expectedErr
	})

	val, err := f.Get()
	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected %v, got %v", expectedErr, err)
	}
	if val != "" {
		t.Fatalf("expected empty string, got %q", val)
	}

	p.Wait()
}

func TestFutureDone(t *testing.T) {
	p := New(1)
	release := make(chan struct{})

	f := Go(p, func() (int, error) {
		<-release
		return 1, nil
	})

	// Should not be done yet.
	if f.Done() {
		t.Fatal("expected Done() == false before completion")
	}

	close(release)

	// Wait for result and verify Done is true.
	f.Get()
	if !f.Done() {
		t.Fatal("expected Done() == true after Get()")
	}

	p.Wait()
}

func TestMultipleFuturesInParallel(t *testing.T) {
	p := New(4)

	futures := make([]*Future[int], 10)
	for i := 0; i < 10; i++ {
		n := i
		futures[i] = Go(p, func() (int, error) {
			time.Sleep(5 * time.Millisecond)
			return n * 2, nil
		})
	}

	// Collect results concurrently.
	var mu sync.Mutex
	results := make(map[int]int)
	var wg sync.WaitGroup

	for i, f := range futures {
		wg.Add(1)
		go func(idx int, fut *Future[int]) {
			defer wg.Done()
			val, err := fut.Get()
			if err != nil {
				t.Errorf("future %d: unexpected error: %v", idx, err)
				return
			}
			mu.Lock()
			results[idx] = val
			mu.Unlock()
		}(i, f)
	}

	wg.Wait()

	for i := 0; i < 10; i++ {
		expected := i * 2
		if got, ok := results[i]; !ok {
			t.Errorf("missing result for future %d", i)
		} else if got != expected {
			t.Errorf("future %d: expected %d, got %d", i, expected, got)
		}
	}

	p.Wait()
}

func TestFutureGetCalledMultipleTimes(t *testing.T) {
	p := New(2)

	f := Go(p, func() (string, error) {
		return "hello", nil
	})

	val1, err1 := f.Get()
	val2, err2 := f.Get()

	if err1 != nil || err2 != nil {
		t.Fatalf("expected nil errors, got %v and %v", err1, err2)
	}
	if val1 != "hello" || val2 != "hello" {
		t.Fatalf("expected 'hello' both times, got %q and %q", val1, val2)
	}

	p.Wait()
}

func TestGoTimeoutSuccess(t *testing.T) {
	p := New(2)

	f, err := GoTimeout(p, func() (int, error) {
		return 99, nil
	}, time.Second)

	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	val, ferr := f.Get()
	if ferr != nil {
		t.Fatalf("expected nil future error, got %v", ferr)
	}
	if val != 99 {
		t.Fatalf("expected 99, got %d", val)
	}

	p.Wait()
}

func TestGoTimeoutExpires(t *testing.T) {
	p := New(1)
	started := make(chan struct{})
	release := make(chan struct{})

	// Fill the single worker slot.
	p.Submit(func() {
		close(started)
		<-release
	})
	<-started

	f, err := GoTimeout(p, func() (int, error) {
		return 0, nil
	}, 20*time.Millisecond)

	if !errors.Is(err, ErrSubmitTimeout) {
		t.Fatalf("expected ErrSubmitTimeout, got %v", err)
	}
	if f != nil {
		t.Fatal("expected nil future on timeout")
	}

	close(release)
	p.Wait()
}

func TestGoTimeoutWithError(t *testing.T) {
	p := New(2)
	expectedErr := errors.New("computation failed")

	f, err := GoTimeout(p, func() (string, error) {
		return "", expectedErr
	}, time.Second)

	if err != nil {
		t.Fatalf("expected nil submit error, got %v", err)
	}

	val, ferr := f.Get()
	if !errors.Is(ferr, expectedErr) {
		t.Fatalf("expected %v, got %v", expectedErr, ferr)
	}
	if val != "" {
		t.Fatalf("expected empty string, got %q", val)
	}

	p.Wait()
}
