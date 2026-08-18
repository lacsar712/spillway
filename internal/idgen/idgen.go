package idgen

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"strings"
	"time"
)

const crockford = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

// New returns a 26-character Crockford-base32 id: 48-bit timestamp (ms)
// followed by 80 bits of entropy. Prefix is a short type tag such as "cmd".
func New(prefix string, now time.Time) string {
	var raw [16]byte
	ms := uint64(now.UnixMilli())
	raw[0] = byte(ms >> 40)
	raw[1] = byte(ms >> 32)
	raw[2] = byte(ms >> 24)
	raw[3] = byte(ms >> 16)
	raw[4] = byte(ms >> 8)
	raw[5] = byte(ms)
	if _, err := rand.Read(raw[6:]); err != nil {
		binary.BigEndian.PutUint64(raw[8:], uint64(now.UnixNano()))
	}
	enc := encodeCrockford(raw[:])
	if prefix == "" {
		return enc
	}
	return prefix + "_" + enc
}

func encodeCrockford(src []byte) string {
	// 16 bytes = 128 bits -> 26 chars of 5 bits (last char uses 3 padding bits).
	var n uint64
	var nbits int
	var b strings.Builder
	b.Grow(26)
	for _, by := range src {
		n = (n << 8) | uint64(by)
		nbits += 8
		for nbits >= 5 {
			nbits -= 5
			idx := (n >> nbits) & 31
			b.WriteByte(crockford[idx])
		}
	}
	if nbits > 0 {
		idx := (n << (5 - nbits)) & 31
		b.WriteByte(crockford[idx])
	}
	s := b.String()
	if len(s) > 26 {
		s = s[:26]
	}
	for len(s) < 26 {
		s += "0"
	}
	return s
}

func ValidatePrefix(id, prefix string) error {
	if prefix == "" {
		return nil
	}
	want := prefix + "_"
	if !strings.HasPrefix(id, want) {
		return fmt.Errorf("id %q missing prefix %q", id, prefix)
	}
	if len(id) != len(want)+26 {
		return fmt.Errorf("id %q has unexpected length", id)
	}
	body := id[len(want):]
	for i := 0; i < len(body); i++ {
		c := body[i]
		if strings.IndexByte(crockford, c) < 0 {
			return fmt.Errorf("id %q has invalid character %q", id, c)
		}
	}
	return nil
}
