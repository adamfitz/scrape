// Package ratelimit provides a token bucket rate limiter for API call throttling.
package ratelimit

import (
	"context"
	"time"
)

// RateLimiter uses a token bucket algorithm to enforce request rate limits.
type RateLimiter struct {
	ticker *time.Ticker
	tokens chan struct{}
	done   chan struct{}
}

// New creates a rate limiter that allows requestsPerSecond requests per second.
// Burst capacity is 1 (requests are evenly spaced).
func New(requestsPerSecond float64) *RateLimiter {
	if requestsPerSecond <= 0 {
		requestsPerSecond = 4
	}

	interval := time.Duration(float64(time.Second) / requestsPerSecond)
	rl := &RateLimiter{
		ticker: time.NewTicker(interval),
		tokens: make(chan struct{}, 1),
		done:   make(chan struct{}),
	}

	// Seed with one token so the first request doesn't wait.
	rl.tokens <- struct{}{}

	go rl.refill()

	return rl
}

func (r *RateLimiter) refill() {
	for {
		select {
		case <-r.ticker.C:
			select {
			case r.tokens <- struct{}{}:
			default:
				// Token slot full, drop.
			}
		case <-r.done:
			return
		}
	}
}

// Wait blocks until a token is available or ctx is cancelled.
// If ctx is nil, context.Background() is used.
func (r *RateLimiter) Wait(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-r.tokens:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Allow returns true if a token is available without blocking.
func (r *RateLimiter) Allow() bool {
	select {
	case <-r.tokens:
		return true
	default:
		return false
	}
}

// Stop releases the rate limiter's goroutine and ticker.
func (r *RateLimiter) Stop() {
	close(r.done)
	r.ticker.Stop()
}
