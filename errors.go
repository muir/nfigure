package nfigure

import (
	"github.com/pkg/errors"
)

type usageError struct {
	cause error
}

// UsageError annotates an error as being a usage error (messed up
// flags invocation).  When you have a usage error, you should display
// the program usage help text.
func UsageError(err error) error {
	if err == nil {
		return nil
	}
	return usageError{
		cause: errors.WithStack(err),
	}
}

func (u usageError) Error() string { return u.cause.Error() }
func (u usageError) Unwrap() error { return u.cause }
func (u usageError) Cause() error  { return u.cause }
func (u usageError) Is(err error) bool {
	_, ok := err.(usageError)
	return ok
}
func IsUsageError(err error) bool {
	var u usageError
	return errors.Is(err, u)
}
