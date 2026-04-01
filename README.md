# go-worker-pool

[![CI](https://github.com/philiprehberger/go-worker-pool/actions/workflows/ci.yml/badge.svg)](https://github.com/philiprehberger/go-worker-pool/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/philiprehberger/go-worker-pool.svg)](https://pkg.go.dev/github.com/philiprehberger/go-worker-pool)
[![Last updated](https://img.shields.io/github/last-commit/philiprehberger/go-worker-pool)](https://github.com/philiprehberger/go-worker-pool/commits/main)

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

### Timeout Submit

```go
err := p.SubmitTimeout(func() {
    // do work
}, 2*time.Second)
if errors.Is(err, workerpool.ErrSubmitTimeout) {
    // no worker slot available within 2 seconds
}

// With futures
f, err := workerpool.GoTimeout(p, func() (int, error) {
    return computeValue(), nil
}, 2*time.Second)
if err != nil {
    // timed out waiting for a worker slot
}
val, err := f.Get()
```

### Stats

```go
s := p.Stats()
fmt.Printf("workers=%d active=%d completed=%d\n",
    s.Workers, s.Active, s.Completed)
```

### Resize

```go
p := workerpool.New(4)

// Scale up under load
p.Resize(8)

// Scale down — excess workers drain naturally
p.Resize(2)
```

### Drain

```go
// Wait for all active tasks to finish, then resume accepting work
p.Drain()
```

## API

| Function / Type | Description |
|-----------------|-------------|
| `New(concurrency int) *Pool` | Create a new pool with the given concurrency limit |
| `(*Pool) Submit(fn func())` | Submit work; blocks if all workers are busy |
| `(*Pool) SubmitCtx(ctx, fn) error` | Submit with context; returns `ctx.Err()` if cancelled while waiting |
| `(*Pool) SubmitTimeout(fn, d) error` | Submit with timeout; returns `ErrSubmitTimeout` if deadline expires |
| `(*Pool) Wait()` | Block until all submitted work completes |
| `(*Pool) Stop()` | Mark stopped and wait; further submits panic |
| `(*Pool) Running() int` | Approximate number of active goroutines |
| `(*Pool) Stats() PoolStats` | Snapshot of workers, active, queued, and completed counts |
| `(*Pool) Resize(n int)` | Dynamically adjust max concurrency |
| `(*Pool) Drain()` | Wait for active tasks to finish, then resume accepting work |
| `Go[T](p, fn) *Future[T]` | Submit work that returns a value; returns a Future |
| `GoTimeout[T](p, fn, d) (*Future[T], error)` | Submit for result with timeout on submission |
| `(*Future[T]) Get() (T, error)` | Block until result is ready |
| `(*Future[T]) Done() bool` | Non-blocking check if complete |
| `ErrSubmitTimeout` | Sentinel error for timed-out submissions |
| `PoolStats` | Struct: `Workers`, `Active`, `Queued`, `Completed` |

## Development

```bash
go test ./...
go vet ./...
```

## Support

If you find this project useful:

⭐ [Star the repo](https://github.com/philiprehberger/go-worker-pool)

🐛 [Report issues](https://github.com/philiprehberger/go-worker-pool/issues?q=is%3Aissue+is%3Aopen+label%3Abug)

💡 [Suggest features](https://github.com/philiprehberger/go-worker-pool/issues?q=is%3Aissue+is%3Aopen+label%3Aenhancement)

❤️ [Sponsor development](https://github.com/sponsors/philiprehberger)

🌐 [All Open Source Projects](https://philiprehberger.com/open-source-packages)

💻 [GitHub Profile](https://github.com/philiprehberger)

🔗 [LinkedIn Profile](https://www.linkedin.com/in/philiprehberger)

## License

[MIT](LICENSE)
