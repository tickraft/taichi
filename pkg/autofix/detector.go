// Package autofix provides error detection and auto-fix capabilities.
//
// It inspects failing cases, classifies the underlying error, and applies predefined fix
// strategies before re-running the affected cases.
package autofix

import (
	"net/http"
	"sync"
)

// serviceDownThreshold is the number of consecutive failures that must accumulate before the detector classifies a service as down.
const serviceDownThreshold = 3

// ErrorType classifies the category of error observed while probing a service.
type ErrorType int

const (
	// ErrorTypeNone indicates no service-level error was detected.
	ErrorTypeNone ErrorType = iota
	// ErrorTypeServiceDown indicates the service is unresponsive (health checks repeatedly fail or there is no response).
	ErrorTypeServiceDown
	// ErrorTypeRateLimited indicates the service returned HTTP 429 Too Many Requests.
	ErrorTypeRateLimited
	// ErrorTypeServerError indicates the service returned HTTP 5xx.
	ErrorTypeServerError
	// ErrorTypeUnknown indicates an unclassified error.
	ErrorTypeUnknown
)

// String returns a human-readable name for ErrorType.
func (e ErrorType) String() string {
	switch e {
	case ErrorTypeNone:
		return "none"
	case ErrorTypeServiceDown:
		return "service_down"
	case ErrorTypeRateLimited:
		return "rate_limited"
	case ErrorTypeServerError:
		return "server_error"
	case ErrorTypeUnknown:
		return "unknown"
	default:
		return "unknown"
	}
}

// ErrorDetector inspects HTTP responses and errors, classifying the nature of failures
// observed while interacting with a service.
// It tracks consecutive failure counts to avoid transient network jitter being misclassified as a service outage.
//
// All methods are concurrency-safe.
type ErrorDetector struct {
	consecutiveFailures int
	mu                  sync.Mutex
}

// NewErrorDetector returns an ErrorDetector with no failure records.
func NewErrorDetector() *ErrorDetector {
	return &ErrorDetector{}
}

// Detect classifies the result of an HTTP request. Network errors or nil responses increment the consecutive failure count;
// when the count reaches the threshold (3), it is classified as ErrorTypeServiceDown. Receiving any HTTP response (even 4xx/5xx)
// resets the counter because the service is at least reachable.
func (d *ErrorDetector) Detect(resp *http.Response, err error) ErrorType {
	d.mu.Lock()
	defer d.mu.Unlock()

	if err != nil {
		d.consecutiveFailures++
		if d.consecutiveFailures >= serviceDownThreshold {
			return ErrorTypeServiceDown
		}
		return ErrorTypeNone
	}

	if resp == nil {
		d.consecutiveFailures++
		if d.consecutiveFailures >= serviceDownThreshold {
			return ErrorTypeServiceDown
		}
		return ErrorTypeNone
	}

	// The service responded, so it is at least reachable. Reset the failure count.
	d.consecutiveFailures = 0

	switch {
	case resp.StatusCode == http.StatusTooManyRequests:
		return ErrorTypeRateLimited
	case resp.StatusCode >= 500:
		return ErrorTypeServerError
	default:
		// 4xx (except 429) are expected client-side test scenarios, not service errors.
		return ErrorTypeNone
	}
}

// DetectFromHealth classifies the result of a health-check request. Unlike Detect,
// any non-200 response (or network error) is treated as a health failure and increments the count.
// A 200 success response resets the count and returns ErrorTypeNone.
func (d *ErrorDetector) DetectFromHealth(resp *http.Response, err error) ErrorType {
	d.mu.Lock()
	defer d.mu.Unlock()

	if err != nil || resp == nil || resp.StatusCode != http.StatusOK {
		d.consecutiveFailures++
		if d.consecutiveFailures >= serviceDownThreshold {
			return ErrorTypeServiceDown
		}
		return ErrorTypeNone
	}

	d.consecutiveFailures = 0
	return ErrorTypeNone
}

// ConsecutiveFailures returns the current consecutive failure count.
func (d *ErrorDetector) ConsecutiveFailures() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.consecutiveFailures
}

// Reset zeroes the consecutive failure count.
func (d *ErrorDetector) Reset() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.consecutiveFailures = 0
}
