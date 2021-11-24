package nfigure

import (
	"github.com/pkg/errors"
)

type usageError struct {
	cause error
}

// UsageError annotates an error as being a usage error (messed up
// flags/command invocation).  When you have a usage error, you should display
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

type environmentError struct {
	cause error
}

// EnvironmentError annotates an error as being caused by an invalid
// environment variable.  When you have an environment error, you may
// want to display appropriate help.
func EnvironmentError(err error) error {
	if err == nil {
		return nil
	}
	return environmentError{
		cause: errors.WithStack(err),
	}
}

func (u environmentError) Error() string { return u.cause.Error() }
func (u environmentError) Unwrap() error { return u.cause }
func (u environmentError) Cause() error  { return u.cause }
func (u environmentError) Is(err error) bool {
	_, ok := err.(environmentError)
	return ok
}

func IsEnvironmentError(err error) bool {
	var u environmentError
	return errors.Is(err, u)
}

type configurationError struct {
	cause error
}

// ConfigurationError annotates an error as being a configuration error (messed up
// flags/command invocation).  When you have a configuration error, you should display
// the program configuration help text.
func ConfigurationError(err error) error {
	if err == nil {
		return nil
	}
	return configurationError{
		cause: errors.WithStack(err),
	}
}

func (u configurationError) Error() string { return u.cause.Error() }
func (u configurationError) Unwrap() error { return u.cause }
func (u configurationError) Cause() error  { return u.cause }
func (u configurationError) Is(err error) bool {
	_, ok := err.(configurationError)
	return ok
}

func IsConfigurationError(err error) bool {
	var u configurationError
	return errors.Is(err, u)
}

type programmerError struct {
	cause error
}

// ProgrammerError annotates an error as being made by a user (importer) of this
// package.
func ProgrammerError(err error) error {
	if err == nil {
		return nil
	}
	return programmerError{
		cause: errors.WithStack(err),
	}
}

func (u programmerError) Error() string { return u.cause.Error() }
func (u programmerError) Unwrap() error { return u.cause }
func (u programmerError) Cause() error  { return u.cause }
func (u programmerError) Is(err error) bool {
	_, ok := err.(programmerError)
	return ok
}

func IsProgrammerError(err error) bool {
	var u programmerError
	return errors.Is(err, u)
}

type nFigureError struct {
	cause error
}

// NFigureError annotates an error as being caused by a bug in this package.  Please
// submit a fix or at least raise an issue.
func NFigureError(err error) error {
	if err == nil {
		return nil
	}
	return nFigureError{
		cause: errors.WithStack(err),
	}
}

func (u nFigureError) Error() string { return u.cause.Error() }
func (u nFigureError) Unwrap() error { return u.cause }
func (u nFigureError) Cause() error  { return u.cause }
func (u nFigureError) Is(err error) bool {
	_, ok := err.(nFigureError)
	return ok
}

func IsNFigureError(err error) bool {
	var u nFigureError
	return errors.Is(err, u)
}

type validationError struct {
	cause error
}

// ValidationError annotates an error as being caused by a bug in this package.  Please
// submit a fix or at least raise an issue.
func ValidationError(err error) error {
	if err == nil {
		return nil
	}
	return validationError{
		cause: errors.WithStack(err),
	}
}

func (u validationError) Error() string { return u.cause.Error() }
func (u validationError) Unwrap() error { return u.cause }
func (u validationError) Cause() error  { return u.cause }
func (u validationError) Is(err error) bool {
	_, ok := err.(validationError)
	return ok
}

func IsValidationError(err error) bool {
	var u validationError
	return errors.Is(err, u)
}
