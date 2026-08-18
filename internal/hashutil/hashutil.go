package hashutil

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

func SHA256Hex(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

// CanonicalV1 is the exact byte string HMAC'd for both inbound and outbound
// signatures: v1.{unix_seconds}.{nonce}.{sha256_hex(raw_body)}.
func CanonicalV1(unixSec int64, nonce string, body []byte) string {
	var b strings.Builder
	b.Grow(3 + 20 + 1 + len(nonce) + 1 + 64)
	b.WriteString("v1.")
	b.WriteString(strconv.FormatInt(unixSec, 10))
	b.WriteByte('.')
	b.WriteString(nonce)
	b.WriteByte('.')
	b.WriteString(SHA256Hex(body))
	return b.String()
}

func ValidNonce(nonce string) error {
	n := len(nonce)
	if n < 16 || n > 64 {
		return fmt.Errorf("nonce length %d not in [16,64]", n)
	}
	for i := 0; i < n; i++ {
		c := nonce[i]
		if c < 0x21 || c > 0x7e {
			return fmt.Errorf("nonce contains non-printable byte at %d", i)
		}
	}
	if !utf8.ValidString(nonce) {
		return fmt.Errorf("nonce is not valid utf-8")
	}
	return nil
}

func ValidIdempotencyKey(key string) error {
	n := len(key)
	if n < 8 || n > 128 {
		return fmt.Errorf("idempotency key length %d not in [8,128]", n)
	}
	for i := 0; i < n; i++ {
		c := key[i]
		ok := (c >= 'a' && c <= 'z') ||
			(c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') ||
			c == '-' || c == '_' || c == '.'
		if !ok {
			return fmt.Errorf("idempotency key has illegal character %q", c)
		}
	}
	return nil
}
