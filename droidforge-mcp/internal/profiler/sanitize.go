package profiler

import (
	"regexp"
	"strings"
	"unicode"
)

var (
	// reSafePkg permits only valid Android package identifiers.
	reSafePkg = regexp.MustCompile(`^[a-zA-Z0-9._]+$`)
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

// SanitizeDeviceText cleans device-sourced string output and limits maximum bytes.
func SanitizeDeviceText(raw string, maxBytes int) string {
	cleaned := StripZeroWidthAndBidi(raw)
	if maxBytes > 0 && len(cleaned) > maxBytes {
		return cleaned[:maxBytes] + "...[TRUNCATED]"
	}
	return cleaned
}

// ValidatePackageID validates an Android package ID.
func ValidatePackageID(pkg string) bool {
	pkg = strings.TrimSpace(pkg)
	return reSafePkg.MatchString(pkg)
}
