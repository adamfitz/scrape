# ratelimiting Specification

## Purpose

Enforce MangaDex API rate limits to comply with their acceptable use policy. The global limit is approximately 5 requests per second per IP address.

## Requirements

### Requirement: Token bucket rate limiter

The rate limiter SHALL use a token bucket algorithm to smooth request rates.

#### Scenario: Requests within limit

- GIVEN a rate limiter configured for 5 requests/second
- WHEN 5 requests are made within 1 second
- THEN all requests SHALL proceed without delay

#### Scenario: Requests exceeding limit

- GIVEN a rate limiter configured for 5 requests/second
- WHEN 6 requests are made within 1 second
- THEN the 6th request SHALL be delayed until a token is available

### Requirement: Conservative default rate

The default rate SHALL be 4 requests/second (80% of the 5 req/s limit) to provide headroom and avoid hitting the limit.

#### Scenario: Default configuration

- GIVEN no custom rate configuration
- WHEN the rate limiter is created
- THEN it SHALL allow 4 requests per second

### Requirement: 429 response handling

When the MangaDex API returns HTTP 429 (Too Many Requests), the client SHALL:
1. Read the `X-RateLimit-Retry-After` header if present
2. Wait for the specified duration
3. Retry the request once

#### Scenario: Retry-After header present

- GIVEN MangaDex returns 429 with `X-RateLimit-Retry-After: 2`
- WHEN the client processes the response
- THEN the client SHALL wait 2 seconds and retry

#### Scenario: No Retry-After header

- GIVEN MangaDex returns 429 without a Retry-After header
- WHEN the client processes the response
- THEN the client SHALL wait 1 second (default) and retry

#### Scenario: Persistent 429

- GIVEN MangaDex returns 429 on the retry
- WHEN the client processes the second 429
- THEN the error SHALL be returned to the caller
- AND no further automatic retries SHALL be attempted

### Requirement: Batch request spacing

For batch lookups, the rate limiter SHALL space requests evenly to maintain the target rate.

#### Scenario: 10 titles in batch

- GIVEN 10 titles to look up
- WHEN processed through the rate limiter
- THEN each request SHALL be spaced at least 250ms apart (4 req/s)

### Requirement: Configurable rate

The rate limiter SHALL accept a custom requests-per-second value.

#### Scenario: Custom rate

- GIVEN a rate limiter configured for 2 requests/second
- WHEN requests are made
- THEN they SHALL be spaced at least 500ms apart

## Implementation Notes

```go
type RateLimiter struct {
    ticker    *time.Ticker
    tokens    chan struct{}
    requests  float64
    perSecond float64
}
```

- Use a goroutine with a time.Ticker to refill tokens
- Buffer channel to capacity = burst size (default 1)
- The `Wait()` method blocks until a token is available
- The `Allow()` method returns immediately with true/false
- The rate limiter MUST be shared across all MangaDex API calls in a session
