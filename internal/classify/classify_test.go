package classify_test

import (
	"fmt"
	"testing"

	"github.com/lacsar712/spillway/internal/classify"
)

type timeoutErr struct{}

func (timeoutErr) Error() string   { return "deadline exceeded" }
func (timeoutErr) Timeout() bool   { return true }
func (timeoutErr) Temporary() bool { return true }

func TestHTTPStatus(t *testing.T) {
	cases := []struct {
		code int
		kind classify.Kind
	}{
		{200, classify.Success},
		{204, classify.Success},
		{408, classify.Retryable},
		{429, classify.Retryable},
		{500, classify.Retryable},
		{503, classify.Retryable},
		{400, classify.Terminal},
		{404, classify.Terminal},
		{422, classify.Terminal},
	}
	for _, tc := range cases {
		if got := classify.HTTPStatus(tc.code); got != tc.kind {
			t.Fatalf("status %d: got %s want %s", tc.code, got, tc.kind)
		}
	}
}

func TestHTTPStatus429IsRetryable(t *testing.T) {
	if classify.HTTPStatus(429) != classify.Retryable {
		t.Fatalf("429: got %s want retryable", classify.HTTPStatus(429))
	}
}

func TestHTTPStatus202IsSuccess(t *testing.T) {
	if classify.HTTPStatus(202) != classify.Success {
		t.Fatalf("202: got %s want success", classify.HTTPStatus(202))
	}
}

func TestNetErrorTimeoutUnwraps(t *testing.T) {
	err := fmt.Errorf("plc write: %w", timeoutErr{})
	if got := classify.NetError(err); got != classify.Retryable {
		t.Fatalf("wrapped timeout: got %s want retryable", got)
	}
}
