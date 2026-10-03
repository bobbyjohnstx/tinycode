package provider

import (
	"math"
	"math/rand/v2"
	"regexp"
	"strings"
	"time"
)

const (
	MaxRetries      = 5
	RetryInitDelay  = 2 * time.Second
	RetryBackoff    = 2.0
	RetryMaxDelay   = 30 * time.Second
	RetryJitter     = 0.25
)

var retryablePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)fetch failed`),
	regexp.MustCompile(`(?i)connection refused`),
	regexp.MustCompile(`(?i)ECONNRESET`),
	regexp.MustCompile(`(?i)ECONNREFUSED`),
	regexp.MustCompile(`(?i)ETIMEDOUT`),
	regexp.MustCompile(`(?i)ENOTFOUND`),
	regexp.MustCompile(`(?i)socket hang up`),
	regexp.MustCompile(`(?i)network error`),
	regexp.MustCompile(`(?i)request timeout`),
	regexp.MustCompile(`(?i)gateway timeout`),
	regexp.MustCompile(`(?i)service unavailable`),
	regexp.MustCompile(`(?i)bad gateway`),
	regexp.MustCompile(`(?i)internal server error`),
	regexp.MustCompile(`(?i)server error`),
	regexp.MustCompile(`(?i)overloaded`),
	regexp.MustCompile(`(?i)rate limit`),
	regexp.MustCompile(`(?i)too many requests`),
	regexp.MustCompile(`(?i)try again later`),
	regexp.MustCompile(`(?i)at capacity`),
	regexp.MustCompile(`(?i)temporarily unavailable`),
	regexp.MustCompile(`(?i)resource exhausted`),
	regexp.MustCompile(`(?i)deadline exceeded`),
	regexp.MustCompile(`(?i)unavailable`),
	regexp.MustCompile(`(?i)connection reset`),
	regexp.MustCompile(`(?i)broken pipe`),
	regexp.MustCompile(`(?i)EPIPE`),
	regexp.MustCompile(`(?i)aborted`),
	regexp.MustCompile(`(?i)stream error`),
	regexp.MustCompile(`(?i)unexpected EOF`),
	regexp.MustCompile(`(?i)incomplete chunked encoding`),
}

// IsRetryable returns true if the error message matches known retryable patterns.
func IsRetryable(errMsg string) bool {
	for _, p := range retryablePatterns {
		if p.MatchString(errMsg) {
			return true
		}
	}
	return false
}

// IsRetryableStatus returns true for HTTP status codes that are retryable.
func IsRetryableStatus(status int) bool {
	return status == 429 || status == 500 || status == 502 || status == 503 || status == 504
}

// RetryDelay calculates the delay for a given attempt number (0-indexed).
// Uses exponential backoff with 25% jitter.
func RetryDelay(attempt int) time.Duration {
	base := float64(RetryInitDelay) * math.Pow(RetryBackoff, float64(attempt))
	if base > float64(RetryMaxDelay) {
		base = float64(RetryMaxDelay)
	}

	jitter := base * RetryJitter * (2*rand.Float64() - 1)
	delay := time.Duration(base + jitter)

	if delay < 0 {
		delay = RetryInitDelay
	}
	return delay
}

var overflowPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)prompt is too long`),
	regexp.MustCompile(`(?i)input is too long for requested model`),
	regexp.MustCompile(`(?i)exceeds the context window`),
	regexp.MustCompile(`(?i)input token count.*exceeds the maximum`),
	regexp.MustCompile(`(?i)maximum prompt length is \d+`),
	regexp.MustCompile(`(?i)reduce the length of the messages`),
	regexp.MustCompile(`(?i)maximum context length is \d+ tokens`),
	regexp.MustCompile(`(?i)exceeds the limit of \d+`),
	regexp.MustCompile(`(?i)exceeds the available context size`),
	regexp.MustCompile(`(?i)greater than the context length`),
	regexp.MustCompile(`(?i)context window exceeds limit`),
	regexp.MustCompile(`(?i)exceeded model token limit`),
	regexp.MustCompile(`(?i)context[_ ]length[_ ]exceeded`),
	regexp.MustCompile(`(?i)request entity too large`),
	regexp.MustCompile(`(?i)context length is only \d+ tokens`),
	regexp.MustCompile(`(?i)input length.*exceeds.*context length`),
	regexp.MustCompile(`(?i)prompt too long; exceeded (?:max )?context length`),
	regexp.MustCompile(`(?i)too large for model with \d+ maximum context length`),
	regexp.MustCompile(`(?i)model_context_window_exceeded`),
}

// IsOverflow returns true if the error message indicates a context overflow.
func IsOverflow(errMsg string) bool {
	for _, p := range overflowPatterns {
		if p.MatchString(errMsg) {
			return true
		}
	}
	return strings.HasPrefix(errMsg, "HTTP 413:")
}
