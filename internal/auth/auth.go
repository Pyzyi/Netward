// Package auth implements password hashing (Argon2id), login throttling
// and session tokens.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"golang.org/x/crypto/argon2"
)

// Argon2id parameters (OWASP Password Storage Cheat Sheet, second option:
// 19 MiB, 2 iterations, 1 lane). Stored with every hash so they can be raised later.
const (
	argonMemory  = 19 * 1024
	argonTime    = 2
	argonThreads = 1
	argonKeyLen  = 32
	saltLen      = 16
)

// ErrBusy means the hashing slot was not free in time.
var ErrBusy = errors.New("authentication busy, try again")

// hashSlot allows one Argon2 computation at a time to bound memory use and
// make concurrent guessing pointless.
var hashSlot = make(chan struct{}, 1)

func acquire(ctx context.Context) error {
	t := time.NewTimer(3 * time.Second)
	defer t.Stop()
	select {
	case hashSlot <- struct{}{}:
		return nil
	case <-t.C:
		return ErrBusy
	case <-ctx.Done():
		return ctx.Err()
	}
}

func release() { <-hashSlot }

// HashPassword returns an encoded Argon2id hash.
func HashPassword(ctx context.Context, password string) (string, error) {
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	if err := acquire(ctx); err != nil {
		return "", err
	}
	key := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	release()
	b64 := base64.RawStdEncoding
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s", argon2.Version, argonMemory, argonTime, argonThreads,
		b64.EncodeToString(salt), b64.EncodeToString(key)), nil
}

// dummyHash is verified when the user does not exist, so the response time
// does not reveal valid user names.
var dummyHash = "$argon2id$v=19$m=19456,t=2,p=1$c29tZXNhbHRzb21lc2FsdA$4Xq0Yk3d3m5lE9nqGq2Yb0w8H8B0b6Vb3t0b0v1m2Kc"

// VerifyPassword checks password against an encoded hash in constant time.
func VerifyPassword(ctx context.Context, encoded, password string) (bool, error) {
	if encoded == "" {
		encoded = dummyHash
	}
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false, errors.New("unsupported hash format")
	}
	var version int
	var mem, iters uint32
	var threads uint8
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return false, errors.New("unsupported argon2 version")
	}
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &mem, &iters, &threads); err != nil {
		return false, errors.New("invalid hash parameters")
	}
	if mem > 256*1024 || iters > 10 || threads > 8 || mem == 0 || iters == 0 || threads == 0 {
		return false, errors.New("hash parameters out of range")
	}
	b64 := base64.RawStdEncoding
	salt, err := b64.DecodeString(parts[4])
	if err != nil {
		return false, err
	}
	want, err := b64.DecodeString(parts[5])
	if err != nil || len(want) == 0 || len(want) > 64 {
		return false, errors.New("invalid hash")
	}
	if err := acquire(ctx); err != nil {
		return false, err
	}
	got := argon2.IDKey([]byte(password), salt, iters, mem, threads, uint32(len(want)))
	release()
	return subtle.ConstantTimeCompare(got, want) == 1, nil
}

// Password policy errors.
var (
	ErrPasswordShort  = errors.New("password_too_short")
	ErrPasswordLong   = errors.New("password_too_long")
	ErrPasswordCommon = errors.New("password_too_common")
	ErrPasswordUser   = errors.New("password_equals_username")
)

var common = map[string]bool{
	"1234567890": true, "0123456789": true, "passwort123": true, "password123": true, "qwertzuiop": true,
	"qwertyuiop": true, "1q2w3e4r5t": true, "passwort12": true, "password12": true, "administrator": true,
	"netzwart123": true, "1111111111": true, "abcdefghij": true, "iloveyou12": true, "hallo12345": true,
	"letmein123": true, "12345678910": true, "asdfghjkl1": true, "zimaos1234": true, "changeme123": true,
}

// CheckPassword enforces the password policy.
func CheckPassword(username, password string) error {
	n := utf8.RuneCountInString(password)
	switch {
	case n < 10:
		return ErrPasswordShort
	case n > 128:
		return ErrPasswordLong
	case strings.EqualFold(password, username):
		return ErrPasswordUser
	case common[strings.ToLower(password)]:
		return ErrPasswordCommon
	}
	distinct := map[rune]bool{}
	for _, r := range password {
		distinct[r] = true
	}
	if len(distinct) < 4 {
		return ErrPasswordCommon
	}
	return nil
}

// ValidUsername checks the user name format.
func ValidUsername(u string) bool {
	if utf8.RuneCountInString(u) < 2 || utf8.RuneCountInString(u) > 32 {
		return false
	}
	for _, r := range u {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '.' || r == '_' || r == '-') {
			return false
		}
	}
	return true
}

// NewToken returns a random 256-bit URL-safe token.
func NewToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err) // crypto/rand never fails on Linux
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// HashToken returns the SHA-256 of a session token.
func HashToken(t string) []byte {
	h := sha256.Sum256([]byte(t))
	return h[:]
}

// Limiter throttles failed logins per client with exponential backoff.
type Limiter struct {
	mu    sync.Mutex
	state map[string]*attempts
	now   func() time.Time
}

type attempts struct {
	failures int
	until    time.Time
	last     time.Time
}

// NewLimiter returns an empty limiter.
func NewLimiter() *Limiter { return &Limiter{state: map[string]*attempts{}, now: time.Now} }

const freeAttempts = 5

// Allow reports whether key may try to log in, and if not, for how long it must wait.
func (l *Limiter) Allow(key string) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	a := l.state[key]
	if a == nil {
		return true, 0
	}
	if wait := a.until.Sub(l.now()); wait > 0 {
		return false, wait
	}
	return true, 0
}

// Fail records a failed attempt.
func (l *Limiter) Fail(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	if len(l.state) > 10000 {
		for k, a := range l.state {
			if now.Sub(a.last) > time.Hour {
				delete(l.state, k)
			}
		}
	}
	a := l.state[key]
	if a == nil || now.Sub(a.last) > 24*time.Hour {
		a = &attempts{}
		l.state[key] = a
	}
	a.failures++
	a.last = now
	if a.failures >= freeAttempts {
		shift := min(a.failures-freeAttempts, 5)
		d := min(30*time.Second<<shift, 15*time.Minute)
		a.until = now.Add(d)
	}
}

// Succeed clears the failures of key.
func (l *Limiter) Succeed(key string) {
	l.mu.Lock()
	delete(l.state, key)
	l.mu.Unlock()
}
