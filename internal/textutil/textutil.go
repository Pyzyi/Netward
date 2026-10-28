// Package textutil normalises untrusted text that arrives from the network
// (DHCP host names, mDNS instance names, UPnP descriptions, ...).
package textutil

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// MaxName is the maximum length, in runes, of any device-supplied name.
const MaxName = 128

// Clean returns s with invalid UTF-8, control and formatting characters
// (including bidirectional overrides) removed, whitespace collapsed and the
// result truncated to max runes. It never returns leading/trailing spaces.
func Clean(s string, max int) string {
	if max <= 0 {
		max = MaxName
	}
	var b strings.Builder
	b.Grow(min(len(s), max*4))
	n := 0
	space := false
	for len(s) > 0 && n < max {
		r, size := utf8.DecodeRuneInString(s)
		s = s[size:]
		switch {
		case r == utf8.RuneError && size <= 1:
			continue
		case unicode.IsSpace(r):
			space = true
			continue
		case unicode.Is(unicode.Cc, r), unicode.Is(unicode.Cf, r), unicode.Is(unicode.Co, r),
			unicode.Is(unicode.Cs, r), r == unicode.ReplacementChar:
			continue
		}
		if space && b.Len() > 0 {
			if n+2 > max { // no room for the separator and the next rune
				break
			}
			b.WriteByte(' ')
			n++
		}
		space = false
		b.WriteRune(r)
		n++
	}
	return b.String()
}

// Hostname cleans a DNS-style host name and strips well-known local domain
// suffixes so "anna-iphone.fritz.box." becomes "anna-iphone".
func Hostname(s string) string {
	s = strings.TrimSuffix(strings.TrimSpace(s), ".")
	if s == "" {
		return ""
	}
	lower := strings.ToLower(s)
	for _, suffix := range localSuffixes {
		if strings.HasSuffix(lower, suffix) && len(s) > len(suffix) {
			s = s[:len(s)-len(suffix)]
			break
		}
	}
	return Clean(s, MaxName)
}

var localSuffixes = []string{
	".fritz.box", ".speedport.ip", ".home.arpa", ".localdomain", ".local",
	".lan", ".home", ".box", ".router", ".internal", ".intranet", ".domain",
	".ip", ".dlink", ".gateway", ".netgear.com", ".hitronhub.home", ".station",
}
