// Package errcode attaches xTeVe error codes to errors, so that a code reaches the caller that finally reports the
// error.
package errcode

import "errors"

type codedError struct {
	err  error
	code int
}

func (e *codedError) Error() string { return e.err.Error() }

func (e *codedError) Unwrap() error { return e.err }

// With attaches an xTeVe error code to err. It returns nil if err is nil.
func With(err error, code int) error {
	if err == nil {
		return nil
	}
	return &codedError{err: err, code: code}
}

// Of returns the xTeVe error code attached to err with With, or 0.
func Of(err error) int {
	if coded, ok := errors.AsType[*codedError](err); ok {
		return coded.code
	}
	return 0
}
