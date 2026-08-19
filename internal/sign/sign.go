package sign

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/lacsar712/spillway/internal/clock"
	"github.com/lacsar712/spillway/internal/hashutil"
)

var (
	ErrMissingHeader = errors.New("missing signature header")
	ErrBadScheme     = errors.New("signature scheme must be v1")
	ErrBadHex        = errors.New("signature is not hex")
	ErrSkew          = errors.New("timestamp outside allowed window")
	ErrMismatch      = errors.New("signature mismatch")
	ErrEmptySecret   = errors.New("empty hmac secret")
)

const SchemePrefix = "v1="

type Headers struct {
	Timestamp int64
	Nonce     string
	Signature string
}

func Sign(secret string, unixSec int64, nonce string, body []byte) (string, error) {
	if secret == "" {
		return "", ErrEmptySecret
	}
	if err := hashutil.ValidNonce(nonce); err != nil {
		return "", err
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(hashutil.CanonicalV1(unixSec, nonce, body)))
	return SchemePrefix + hex.EncodeToString(mac.Sum(nil)), nil
}

func ParseSignatureHeader(h string) (hexPart string, err error) {
	h = strings.TrimSpace(h)
	if h == "" {
		return "", ErrMissingHeader
	}
	if !strings.HasPrefix(h, SchemePrefix) {
		return "", ErrBadScheme
	}
	hexPart = h[len(SchemePrefix):]
	if len(hexPart) != 64 {
		return "", fmt.Errorf("%w: got %d hex chars, want 64", ErrBadHex, len(hexPart))
	}
	if _, err := hex.DecodeString(hexPart); err != nil {
		return "", ErrBadHex
	}
	return hexPart, nil
}

// Verify checks timestamp skew and HMAC against any of the provided secrets
// (current + overlapping previous secret during rotation).
func Verify(clk clock.Clock, window time.Duration, secrets []string, hdr Headers, body []byte) error {
	if len(secrets) == 0 {
		return ErrEmptySecret
	}
	if err := hashutil.ValidNonce(hdr.Nonce); err != nil {
		return err
	}
	gotHex, err := ParseSignatureHeader(hdr.Signature)
	if err != nil {
		return err
	}
	now := clk.Now().Unix()
	delta := now - hdr.Timestamp
	if delta < 0 {
		delta = -delta
	}
	if window <= 0 {
		window = 5 * time.Minute
	}
	if delta > int64(window.Seconds()) {
		return fmt.Errorf("%w: skew=%ds window=%s", ErrSkew, delta, window)
	}
	canonical := []byte(hashutil.CanonicalV1(hdr.Timestamp, hdr.Nonce, body))
	got, err := hex.DecodeString(gotHex)
	if err != nil {
		return ErrBadHex
	}
	for _, secret := range secrets {
		if secret == "" {
			continue
		}
		mac := hmac.New(sha256.New, []byte(secret))
		_, _ = mac.Write(canonical)
		if hmac.Equal(got, mac.Sum(nil)) {
			return nil
		}
	}
	return ErrMismatch
}

func HeaderMap(unixSec int64, nonce, signature string) map[string]string {
	return map[string]string{
		"X-Spill-Timestamp": fmt.Sprintf("%d", unixSec),
		"X-Spill-Nonce":     nonce,
		"X-Spill-Signature": signature,
	}
}
