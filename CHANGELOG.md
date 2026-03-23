# Changelog

## 0.2.0

- Add `Stats()` method returning `PoolStats` with workers, active, queued, and completed counts
- Add `SubmitTimeout` for task submission with a deadline
- Add `GoTimeout` for future-based submission with a deadline
- Add `Resize` to dynamically adjust pool concurrency
- Add `Drain` to wait for active tasks and temporarily pause new submissions
- Add `ErrSubmitTimeout` sentinel error
- Track active and completed tasks using atomic counters

## 0.1.2

- Consolidate README badges onto single line

## 0.1.1

- Add badges and Development section to README

## 0.1.0

- Initial release
- Bounded goroutine pool with backpressure
- Context-aware `SubmitCtx`
- Generic `Future` for collecting results
