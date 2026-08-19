package classify

import (
	"errors"
	"os"
	"syscall"
)

type Kind int

const (
	Success Kind = iota
	Retryable
	Terminal
)

func (k Kind) String() string {
	switch k {
	case Success:
		return "success"
	case Retryable:
		return "retryable"
	case Terminal:
		return "terminal"
	default:
		return "unknown"
	}
}

func HTTPStatus(code int) Kind {
	if code >= 200 && code <= 299 {
		return Success
	}
	switch code {
	case 408, 429:
		return Retryable
	}
	if code >= 500 && code <= 599 {
		return Retryable
	}
	if code >= 400 && code <= 499 {
		return Terminal
	}
	if code == 0 {
		return Retryable
	}
	return Terminal
}

// timeoutError is the minimal contract that net.Error, context.DeadlineExceeded,
// and *url.Error all satisfy. Matching on this interface (rather than a fixed
// sentinel) lets a wrapped timeout survive any number of fmt.Errorf("%w", ...)
// layers and still be recognised as retryable.
type timeoutError interface {
	Timeout() bool
}

func NetError(err error) Kind {
	if err == nil {
		return Success
	}
	var terr timeoutError
	if errors.As(err, &terr) && terr.Timeout() {
		return Retryable
	}
	if errors.Is(err, syscall.ECONNREFUSED) ||
		errors.Is(err, syscall.ECONNRESET) ||
		errors.Is(err, syscall.EPIPE) ||
		errors.Is(err, os.ErrDeadlineExceeded) {
		return Retryable
	}
	return Terminal
}

func Combine(status int, err error) Kind {
	if err != nil {
		return NetError(err)
	}
	return HTTPStatus(status)
}
