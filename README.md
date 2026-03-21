# go-worker-pool

[![CI](https://github.com/philiprehberger/go-worker-pool/actions/workflows/ci.yml/badge.svg)](https://github.com/philiprehberger/go-worker-pool/actions/workflows/ci.yml) [![Go Reference](https://pkg.go.dev/badge/github.com/philiprehberger/go-worker-pool.svg)](https://pkg.go.dev/github.com/philiprehberger/go-worker-pool) [![License](https://img.shields.io/github/license/philiprehberger/go-worker-pool)](LICENSE)

Bounded goroutine pool with backpressure and futures for Go

## Installation

```bash
go get github.com/philiprehberger/go-worker-pool
```

## Usage

### Basic Pool

```go
import "github.com/philiprehberger/go-worker-pool"

p := workerpool.New(4) // max 4 concurrent goroutines

for i := 0; i < 100; i++ {
    p.Submit(func() {
        // do work
    })
}

p.Wait() // block until all tasks complete
```

### Future

```go
f := workerpool.Go(p, func() (int, error) {
    return computeExpensiveValue(), nil
})

// do other work...

val, err := f.Get() // block until result is ready
```

### Context-Aware Submit

```go
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()

err := p.SubmitCtx(ctx, func() {
    // do work
})
if err != nil {
    // context was cancelled while waiting for a worker slot
}
```

## API

| Function / Type | Description |
|-----------------|-------------|
| `New(concurrency int) *Pool` | Create a new pool with the given concurrency limit |
| `(*Pool) Submit(fn func())` | Submit work; blocks if all workers are busy |
| `(*Pool) SubmitCtx(ctx, fn) error` | Submit with context; returns `ctx.Err()` if cancelled while waiting |
| `(*Pool) Wait()` | Block until all submitted work completes |
| `(*Pool) Stop()` | Mark stopped and wait; further submits panic |
| `(*Pool) Running() int` | Approximate number of active goroutines |
| `Go[T](p, fn) *Future[T]` | Submit work that returns a value; returns a Future |
| `(*Future[T]) Get() (T, error)` | Block until result is ready |
| `(*Future[T]) Done() bool` | Non-blocking check if complete |

## Development

```bash
go test ./...
go vet ./...
```

## License

MIT
