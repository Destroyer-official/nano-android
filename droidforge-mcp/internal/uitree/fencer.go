package uitree

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/anthropic/droidforge-mcp/internal/types"
)

const (
	// DefaultMaxNodeTextBytes is the maximum allowed byte length for a single UI node's text.
	DefaultMaxNodeTextBytes = 256
	// DefaultMaxLogcatLineBytes is the maximum allowed byte length for one logcat line.
	DefaultMaxLogcatLineBytes = 512
)

var (
	// Regex matching potential prompt injection spoofing tags like </DF_UNTRUSTED_...> or <DF_UNTRUSTED_...>
	fakeTagRegex = regexp.MustCompile(`(?i)</?DF_UNTRUSTED_[a-fA-F0-9_]*>`)

	// Regex matching ANSI terminal escape sequences
	ansiRegex = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)
)

// GenerateNonce creates a cryptographic per-call nonce in the format "DF_UNTRUSTED_xxxxxx".
func GenerateNonce() string {
	b := make([]byte, 3)
	if _, err := rand.Read(b); err != nil {
		return "DF_UNTRUSTED_000000"
	}
	return "DF_UNTRUSTED_" + hex.EncodeToString(b)
}

// SanitizeDeviceText removes invisible control characters, bidi overrides, ANSI escapes, and spoofed fence tags.
func SanitizeDeviceText(raw string, maxBytes int) string {
	if raw == "" {
		return ""
	}

	// 1. Strip fake/injected untrusted tags
	clean := fakeTagRegex.ReplaceAllString(raw, "")

	// 2. Strip ANSI escape sequences
	clean = ansiRegex.ReplaceAllString(clean, "")

	// 2. Filter out zero-width and bidirectional override characters
	var b strings.Builder
	b.Grow(len(clean))
	for _, r := range clean {
		// Zero-width characters: U+200B (ZWSP), U+200C (ZWNJ), U+200D (ZWJ), U+FEFF (BOM), U+200E, U+200F
		if r == '\u200B' || r == '\u200C' || r == '\u200D' || r == '\uFEFF' || r == '\u200E' || r == '\u200F' {
			continue
		}
		// Bidi control characters: U+202A through U+202E, U+2066 through U+2069
		if (r >= '\u202A' && r <= '\u202E') || (r >= '\u2066' && r <= '\u2069') {
			continue
		}
		// Control characters except tab and newline
		if unicode.IsControl(r) && r != '\t' && r != '\n' && r != '\r' {
			continue
		}
		b.WriteRune(r)
	}

	res := b.String()
	// 3. Truncate byte length safely without cutting multi-byte UTF-8 runes
	if maxBytes > 0 && len(res) > maxBytes {
		res = res[:maxBytes]
		for len(res) > 0 && !utf8.ValidString(res) {
			res = res[:len(res)-1]
		}
	}

	return res
}

// FenceText wraps sanitized device text within opening and closing nonce tags.
func FenceText(raw string, nonce string, maxBytes int) string {
	if raw == "" {
		return ""
	}
	clean := SanitizeDeviceText(raw, maxBytes)
	if clean == "" {
		return ""
	}
	return fmt.Sprintf("<%s>%s</%s>", nonce, clean, nonce)
}

// FenceNodes creates a copy of pruned nodes where text and content-desc are nonce-fenced.
func FenceNodes(nodes []types.PrunedNode, nonce string, maxBytes int) []types.PrunedNode {
	if maxBytes <= 0 {
		maxBytes = DefaultMaxNodeTextBytes
	}
	fenced := make([]types.PrunedNode, len(nodes))
	for i, n := range nodes {
		f := n
		if n.Text != "" {
			f.Text = FenceText(n.Text, nonce, maxBytes)
		}
		if n.ContentDesc != "" {
			f.ContentDesc = FenceText(n.ContentDesc, nonce, maxBytes)
		}
		fenced[i] = f
	}
	return fenced
}

// FenceLogcatLines fences an array of logcat error lines with the given nonce.
func FenceLogcatLines(lines []string, nonce string, maxBytes int) []string {
	if maxBytes <= 0 {
		maxBytes = DefaultMaxLogcatLineBytes
	}
	fenced := make([]string, len(lines))
	for i, l := range lines {
		fenced[i] = FenceText(l, nonce, maxBytes)
	}
	return fenced
}
