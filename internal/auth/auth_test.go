package auth

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"testing"
	"time"
)

func TestHashAndVerify(t *testing.T) {
	ctx := context.Background()
	h, err := HashPassword(ctx, "korrekt-pferd-batterie")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(h, "$argon2id$v=19$m=19456,t=2,p=1$") {
		t.Fatalf("unexpected encoding %q", h)
	}
	ok, err := VerifyPassword(ctx, h, "korrekt-pferd-batterie")
	if err != nil || !ok {
		t.Fatalf("valid password rejected: %v", err)
	}
	ok, _ = VerifyPassword(ctx, h, "korrekt-pferd-batteriE")
	if ok {
		t.Fatal("wrong password accepted")
	}
	h2, _ := HashPassword(ctx, "korrekt-pferd-batterie")
	if h == h2 {
		t.Fatal("salt not random")
	}
	// Unknown user: the dummy hash is verified and never matches.
	if ok, err := VerifyPassword(ctx, "", "anything-at-all"); ok || err != nil {
		t.Fatalf("dummy verification: %v %v", ok, err)
	}
	// Absurd parameters are refused instead of exhausting memory.
	if _, err := VerifyPassword(ctx, "$argon2id$v=19$m=99999999,t=2,p=1$c2FsdA$aGFzaA", "x"); err == nil {
		t.Fatal("oversized parameters accepted")
	}
}

func TestPasswordPolicy(t *testing.T) {
	cases := map[string]error{
		"short":                  ErrPasswordShort,
		"1234567890":             ErrPasswordCommon,
		"aaaaaaaaaaaa":           ErrPasswordCommon,
		"jonasjonas":             ErrPasswordUser,
		"Sonnenblume-im-Garten7": nil,
		strings.Repeat("x", 129): ErrPasswordLong,
	}
	for pw, want := range cases {
		if got := CheckPassword("jonasjonas", pw); got != want {
			t.Errorf("CheckPassword(%q) = %v, want %v", pw, got, want)
		}
	}
	if !ValidUsername("admin") || ValidUsername("a") || ValidUsername("ad min") || ValidUsername("<script>") {
		t.Error("username validation wrong")
	}
}

func TestLimiterBackoff(t *testing.T) {
	l := NewLimiter()
	now := time.Now()
	l.now = func() time.Time { return now }
	ip := IPKey("login", netip.MustParseAddr("1.2.3.4"))
	fail := func(k Key) {
		end, _, ok := l.Begin(k)
		if !ok {
			t.Fatal("unexpectedly blocked")
		}
		end(Failure)
	}
	for i := 0; i < 4; i++ {
		fail(ip)
	}
	fail(ip) // 5th failure starts the backoff
	if _, wait, ok := l.Begin(ip); ok || wait != 30*time.Second {
		t.Fatalf("after 5 failures: ok=%v wait=%v", ok, wait)
	}
	if end, _, ok := l.Begin(IPKey("login", netip.MustParseAddr("5.6.7.8"))); !ok {
		t.Fatal("other clients must not be affected")
	} else {
		end(Refund)
	}
	for i := 0; i < 20; i++ {
		now = now.Add(16 * time.Minute)
		fail(ip)
	}
	if _, wait, _ := l.Begin(ip); wait != 15*time.Minute {
		t.Fatalf("backoff cap: %v", wait)
	}
	now = now.Add(16 * time.Minute)
	end, _, ok := l.Begin(ip)
	if !ok {
		t.Fatal("still blocked after backoff")
	}
	end(Success)
	fail(ip)
	if end, _, ok := l.Begin(ip); !ok {
		t.Fatal("success must reset the counter")
	} else {
		end(Refund)
	}
}

// Parallel attempts are charged before the password check, so a burst
// cannot exceed the free budget (review 2, security M2).
func TestLimiterChargesInFlightAttempts(t *testing.T) {
	l := NewLimiter()
	ip := IPKey("login", netip.MustParseAddr("10.0.0.9"))
	var ends []func(Outcome) int
	admitted := 0
	for i := 0; i < 60; i++ {
		if end, _, ok := l.Begin(ip); ok {
			admitted++
			ends = append(ends, end)
		}
	}
	if admitted != 5 {
		t.Fatalf("%d parallel attempts admitted, want 5", admitted)
	}
	for _, end := range ends {
		end(Refund)
	}
	if end, _, ok := l.Begin(ip); !ok {
		t.Fatal("refunded attempts must not block")
	} else {
		end(Refund)
	}
}

// Rotating source addresses does not help against one account.
func TestLimiterPerUser(t *testing.T) {
	l := NewLimiter()
	user := UserKey("login", "Admin")
	blocked := false
	for i := 0; i < 30; i++ {
		end, _, ok := l.Begin(IPKey("login", netip.AddrFrom4([4]byte{10, 0, 0, byte(i)})), user)
		if !ok {
			blocked = true
			break
		}
		end(Failure)
	}
	if !blocked {
		t.Fatal("guessing one account from many addresses was not limited")
	}
	if _, wait, _ := l.Begin(UserKey("login", "admin")); wait > time.Minute {
		t.Fatalf("account lockout too long: %v", wait)
	}
}

func TestPriorityGetsHashSlot(t *testing.T) {
	hashSlot <- struct{}{} // occupied by a flood
	done := make(chan error, 1)
	go func() {
		_, err := VerifyPassword(WithPriority(context.Background()), "", "x")
		done <- err
	}()
	time.Sleep(50 * time.Millisecond)
	// While a trusted caller waits, ordinary callers yield.
	if err := acquire(context.Background()); !errors.Is(err, ErrBusy) {
		t.Fatalf("ordinary caller took the slot ahead of a trusted one: %v", err)
	}
	<-hashSlot
	if err := <-done; err != nil {
		t.Fatalf("trusted caller: %v", err)
	}
}

func TestTokens(t *testing.T) {
	a, b := NewToken(), NewToken()
	if a == b || len(a) != 43 {
		t.Fatalf("bad tokens %q %q", a, b)
	}
	if len(HashToken(a)) != 32 {
		t.Fatal("bad hash length")
	}
}

func TestLimiterBounded(t *testing.T) {
	l := NewLimiter()
	// "Busy" refunds leave nothing behind.
	for i := range 1000 {
		end, _, ok := l.Begin(IPKey("login", netip.AddrFrom4([4]byte{10, 1, byte(i >> 8), byte(i)})), UserKey("login", fmt.Sprint("u", i)))
		if !ok {
			t.Fatal("blocked")
		}
		end(Refund)
	}
	if n := len(l.state); n != 0 {
		t.Fatalf("%d entries after refunds", n)
	}
	// IPv6 addresses of one /64 share a budget.
	a, b := netip.MustParseAddr("2001:db8::1"), netip.MustParseAddr("2001:db8::ffff:1")
	if IPKey("login", a) != IPKey("login", b) {
		t.Fatal("IPv6 /64 not grouped")
	}
	// The table never grows beyond maxKeys; known keys keep their state.
	owner := UserKey("login", "admin")
	for range 11 { // the account budget is 10
		end, _, _ := l.Begin(owner)
		if end != nil {
			end(Failure)
		}
	}
	for i := range maxKeys + 500 {
		end, _, ok := l.Begin(UserKey("login", fmt.Sprint("x", i)))
		if ok {
			end(Failure)
		}
	}
	if n := len(l.state); n > maxKeys {
		t.Fatalf("%d entries, cap %d", n, maxKeys)
	}
	if _, _, ok := l.Begin(owner); ok {
		t.Fatal("backoff of a known key lost when the table was full")
	}
}
