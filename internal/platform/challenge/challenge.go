// Package challenge issues single-use, short-lived challenges in Valkey. A device attestation is made for one of them,
// so a token captured once cannot be replayed for another sign-in.
package challenge

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"time"

	"github.com/valkey-io/valkey-go"

	"github.com/itsivali/4draws-lottery-platform/internal/platform/clock"
)

// Store issues and consumes challenges.
type Store struct {
	vk  valkey.Client
	ttl time.Duration
}

// NewStore returns a store whose challenges live for ttl.
func NewStore(vk valkey.Client, ttl time.Duration) *Store { return &Store{vk: vk, ttl: ttl} }

func key(c string) string { return "attest:challenge:" + c }

// Issue creates a challenge (32 random bytes, base64url) and returns it with its expiry.
func (s *Store) Issue(ctx context.Context) (string, time.Time, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", time.Time{}, err
	}
	c := base64.RawURLEncoding.EncodeToString(b)
	now, _ := clock.Wall{}.Now(ctx, nil) // the expiry shown to the client; Valkey enforces the real one
	exp := now.Add(s.ttl)
	err := s.vk.Do(ctx, s.vk.B().Set().Key(key(c)).Value("1").Nx().Px(s.ttl).Build()).Error()
	return c, exp, err
}

// Consume reports whether c was issued, is still fresh and had not been used, and uses it up (atomically).
func (s *Store) Consume(ctx context.Context, c string) (bool, error) {
	if c == "" || len(c) > 128 {
		return false, nil
	}
	err := s.vk.Do(ctx, s.vk.B().Getdel().Key(key(c)).Build()).Error()
	if valkey.IsValkeyNil(err) {
		return false, nil
	}
	if err != nil {
		return false, errors.Join(errors.New("challenge: consume"), err)
	}
	return true, nil
}
