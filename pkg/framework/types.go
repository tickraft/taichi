// Package framework provides the core data models, assertion engine, reporter, and
// service lifecycle management for test orchestration.
package framework

import "time"

// TestCase represents a single executable test case.
type TestCase struct {
	// Name is the short identifier of the test case, unique within its owning suite.
	Name string
	// Description is a human-readable summary of what the test case validates.
	Description string
	// Fn is the function executed when the test case runs. A non-nil error indicates failure.
	Fn func() error
}

// TestResult captures the outcome of a single test case execution.
type TestResult struct {
	// Name is the identifier of the test case this result belongs to.
	Name string
	// Passed indicates whether the test case completed successfully.
	Passed bool
	// Skipped indicates whether the test case was skipped and not executed.
	Skipped bool
	// Message is an optional human-readable detail about the result.
	Message string
	// Duration is the execution time of the test case.
	Duration time.Duration
	// Error holds the error returned by the test case function (if any).
	Error error
}

// TestSuite groups related test cases together.
type TestSuite struct {
	// Name is the short identifier of the test suite.
	Name string
	// Cases is the list of test cases belonging to this suite.
	Cases []TestCase
	// Results holds the results of executed cases. Populated by the runner after the suite runs.
	Results []TestResult
}

// AssertResult captures the outcome of a single assertion.
type AssertResult struct {
	// Passed indicates whether the assertion succeeded.
	Passed bool
	// Message is a human-readable description of the assertion result.
	Message string
	// Expected is the expected value of the assertion.
	Expected interface{}
	// Actual is the value actually observed by the assertion.
	Actual interface{}
}
