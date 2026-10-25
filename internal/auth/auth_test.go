package auth

import (
	"context"
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
	for i := 0; i < 4; i++ {
		l.Fail("1.2.3.4")
		if ok, _ := l.Allow("1.2.3.4"); !ok {
			t.Fatalf("blocked after %d failures", i+1)
		}
	}
	l.Fail("1.2.3.4")
	ok, wait := l.Allow("1.2.3.4")
	if ok || wait != 30*time.Second {
		t.Fatalf("after 5 failures: ok=%v wait=%v", ok, wait)
	}
	if ok, _ := l.Allow("5.6.7.8"); !ok {
		t.Fatal("other clients must not be affected")
	}
	for i := 0; i < 20; i++ {
		l.Fail("1.2.3.4")
	}
	_, wait = l.Allow("1.2.3.4")
	if wait != 15*time.Minute {
		t.Fatalf("backoff cap: %v", wait)
	}
	now = now.Add(16 * time.Minute)
	if ok, _ := l.Allow("1.2.3.4"); !ok {
		t.Fatal("still blocked after backoff")
	}
	l.Succeed("1.2.3.4")
	l.Fail("1.2.3.4")
	if ok, _ := l.Allow("1.2.3.4"); !ok {
		t.Fatal("success must reset the counter")
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
