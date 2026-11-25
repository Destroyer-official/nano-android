package harness

import (
	"regexp"
	"strings"
	"unicode"
)

var (
	// reSafePhone permits valid phone number characters, preventing telnet / CLI injection.
	reSafePhone = regexp.MustCompile(`^[+]?[0-9A-Za-z._\-]{1,32}$`)

	// reSafeIdentifier permits standard Android package and permission identifiers.
	reSafeIdentifier = regexp.MustCompile(`^[a-zA-Z0-9._]+$`)
)

// StripZeroWidthAndBidi removes invisible zero-width and bidirectional Unicode control characters
// (OWASP MCP06 defense against prompt injection / text obfuscation).
func StripZeroWidthAndBidi(s string) string {
	return strings.Map(func(r rune) rune {
		// Zero-width characters
		if r == '\u200B' || r == '\u200C' || r == '\u200D' || r == '\uFEFF' || r == '\u2060' {
			return -1
		}
		// Bidi control characters
		if r >= '\u200E' && r <= '\u200F' {
			return -1
		}
		if r >= '\u202A' && r <= '\u202E' {
			return -1
		}
		if r >= '\u2066' && r <= '\u2069' {
			return -1
		}
		// Strip unprintable control characters except newline and tab
		if unicode.IsControl(r) && r != '\n' && r != '\r' && r != '\t' {
			return -1
		}
		return r
	}, s)
}

// SanitizeTelnetParam removes newlines, carriage returns, and control characters to prevent
// console command injection in emulator telnet sessions.
func SanitizeTelnetParam(s string) string {
	cleaned := StripZeroWidthAndBidi(s)
	cleaned = strings.ReplaceAll(cleaned, "\r", " ")
	cleaned = strings.ReplaceAll(cleaned, "\n", " ")
	return strings.TrimSpace(cleaned)
}

// ValidatePhoneNumber checks if a phone number or sender ID contains only safe characters.
func ValidatePhoneNumber(phone string) bool {
	phone = strings.TrimSpace(phone)
	return reSafePhone.MatchString(phone)
}

// ValidateIdentifier validates package names, permission names, and appops keys.
func ValidateIdentifier(id string) bool {
	id = strings.TrimSpace(id)
	return reSafeIdentifier.MatchString(id)
}
