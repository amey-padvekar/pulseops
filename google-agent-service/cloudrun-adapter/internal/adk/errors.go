package adk

import (
	"context"
	"errors"
	"fmt"
	"net"
)

// TransportError wraps upstream HTTP/status failures from ADK boundaries.
type TransportError struct {
	StatusCode int
	Err        error
}

func (e *TransportError) Error() string {
	if e == nil {
		return "transport error"
	}
	if e.Err == nil {
		return fmt.Sprintf("transport error status=%d", e.StatusCode)
	}
	return fmt.Sprintf("transport error status=%d: %v", e.StatusCode, e.Err)
}

func (e *TransportError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// TransientError marks a retriable transport failure.
type TransientError struct {
	Err error
}

func (e *TransientError) Error() string {
	if e == nil || e.Err == nil {
		return "transient error"
	}
	return e.Err.Error()
}

func (e *TransientError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func IsTransient(err error) bool {
	if err == nil {
		return false
	}

	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}

	var marked *TransientError
	if errors.As(err, &marked) {
		return true
	}

	var tr *TransportError
	if errors.As(err, &tr) {
		return tr.StatusCode >= 500
	}

	var netErr net.Error
	if errors.As(err, &netErr) {
		return netErr.Timeout() || netErr.Temporary()
	}

	return false
}
