package fetchers

import (
	"errors"
	"fmt"
)

// Sentinel errors for callers to distinguish empty results from failures.
var (
	ErrNoData = errors.New("no data")
)

// FetchError wraps fetcher failures so callers can distinguish request failure from no data.
type FetchError struct {
	Op      string
	Err     error
	NoData  bool
	Message string
}

func (e *FetchError) Error() string {
	if e.Message != "" {
		if e.Err != nil {
			return fmt.Sprintf("%s: %s: %v", e.Op, e.Message, e.Err)
		}
		return fmt.Sprintf("%s: %s", e.Op, e.Message)
	}
	if e.Err != nil {
		return fmt.Sprintf("%s: %v", e.Op, e.Err)
	}
	return e.Op
}

func (e *FetchError) Unwrap() error {
	if e.NoData {
		return ErrNoData
	}
	return e.Err
}

func (e *FetchError) Is(target error) bool {
	if target == ErrNoData {
		return e.NoData
	}
	return false
}

// NewFetchError returns a request-failed FetchError.
func NewFetchError(op string, err error) *FetchError {
	return &FetchError{Op: op, Err: err}
}

// NewNoDataError returns a no-data FetchError.
func NewNoDataError(op string) *FetchError {
	return &FetchError{Op: op, NoData: true, Message: "no items in window", Err: ErrNoData}
}
