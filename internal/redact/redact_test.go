package redact_test

import (
	"bytes"
	"testing"

	"github.com/lacsar712/spillway/internal/redact"
)

func TestJSONMasksSecrets(t *testing.T) {
	in := []byte(`{"order_id":"A","password":"hunter2","nested":{"token":"abc"}}`)
	out := redact.JSON(in)
	if bytes.Contains(out, []byte("hunter2")) || bytes.Contains(out, []byte("abc")) {
		t.Fatalf("secret leaked: %s", out)
	}
	if !bytes.Contains(out, []byte("A")) {
		t.Fatalf("non-sensitive field missing: %s", out)
	}
}

func TestJSONMasksNestedToken(t *testing.T) {
	in := []byte(`{"payload":{"nested":{"token":"abc"}}}`)
	out := redact.JSON(in)
	if bytes.Contains(out, []byte("abc")) {
		t.Fatalf("nested token leaked: %s", out)
	}
}
