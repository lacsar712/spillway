package sign_test

import (
	"errors"
	"testing"
	"time"

	"github.com/lacsar712/spillway/internal/clock"
	"github.com/lacsar712/spillway/internal/sign"
)

func TestSignAndVerify(t *testing.T) {
	clk := clock.NewFrozen(time.Unix(1_700_000_000, 0))
	body := []byte(`{"type":"gate.raise","payload":{}}`)
	nonce := "abcdefghijklmnop"
	sig, err := sign.Sign("supersecret", clk.Now().Unix(), nonce, body)
	if err != nil {
		t.Fatal(err)
	}
	err = sign.Verify(clk, 5*time.Minute, []string{"supersecret"}, sign.Headers{
		Timestamp: clk.Now().Unix(),
		Nonce:     nonce,
		Signature: sig,
	}, body)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
}

func TestVerifyRejectsSkewAndWrongSecret(t *testing.T) {
	clk := clock.NewFrozen(time.Unix(1_700_000_000, 0))
	body := []byte(`{}`)
	nonce := "abcdefghijklmnop"
	sig, err := sign.Sign("supersecret", clk.Now().Unix(), nonce, body)
	if err != nil {
		t.Fatal(err)
	}
	clk.Advance(10 * time.Minute)
	err = sign.Verify(clk, 5*time.Minute, []string{"supersecret"}, sign.Headers{
		Timestamp: 1_700_000_000,
		Nonce:     nonce,
		Signature: sig,
	}, body)
	if err == nil {
		t.Fatal("expected skew error")
	}
	clk.Set(time.Unix(1_700_000_000, 0))
	err = sign.Verify(clk, 5*time.Minute, []string{"other"}, sign.Headers{
		Timestamp: 1_700_000_000,
		Nonce:     nonce,
		Signature: sig,
	}, body)
	if err == nil {
		t.Fatal("expected mismatch")
	}
}

func TestVerifySkewUnwraps(t *testing.T) {
	clk := clock.NewFrozen(time.Unix(1_700_000_000, 0))
	body := []byte(`{}`)
	nonce := "abcdefghijklmnop"
	sig, err := sign.Sign("supersecret", clk.Now().Unix(), nonce, body)
	if err != nil {
		t.Fatal(err)
	}
	clk.Advance(10 * time.Minute)
	err = sign.Verify(clk, 5*time.Minute, []string{"supersecret"}, sign.Headers{
		Timestamp: 1_700_000_000,
		Nonce:     nonce,
		Signature: sig,
	}, body)
	if !errors.Is(err, sign.ErrSkew) {
		t.Fatalf("want ErrSkew, got %v", err)
	}
}

func TestVerifyEmptySecretsNoPanic(t *testing.T) {
	clk := clock.NewFrozen(time.Unix(1_700_000_000, 0))
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("empty secrets panicked: %v", r)
		}
	}()
	err := sign.Verify(clk, 5*time.Minute, nil, sign.Headers{
		Timestamp: clk.Now().Unix(),
		Nonce:     "abcdefghijklmnop",
		Signature: "v1=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	}, []byte(`{}`))
	if err == nil {
		t.Fatal("expected error for empty secrets")
	}
}
