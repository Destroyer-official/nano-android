package uitree

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/anthropic/droidforge-mcp/internal/types"
)

// =============================================================================
// Benchmarks
// =============================================================================

func BenchmarkParseXML_Small(b *testing.B) {
	data := []byte(sample1SettingsMain)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := ParseXML(data)
		if err != nil {
			b.Fatalf("ParseXML error: %v", err)
		}
	}
}

func BenchmarkParseXML_Large2000Nodes(b *testing.B) {
	data := []byte(generate2000NodeXML())
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := ParseXML(data)
		if err != nil {
			b.Fatalf("ParseXML error: %v", err)
		}
	}
}

func BenchmarkPrune_Small(b *testing.B) {
	h, err := ParseXML([]byte(sample1SettingsMain))
	if err != nil {
		b.Fatalf("ParseXML error: %v", err)
	}
	opts := PruneOptions{Viewport: DefaultViewport, MaxTextLen: 256}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		nodes := Prune(h, opts)
		if len(nodes) == 0 {
			b.Fatal("Prune returned no nodes")
		}
	}
}

func BenchmarkPrune_Large2000Nodes(b *testing.B) {
	h, err := ParseXML([]byte(generate2000NodeXML()))
	if err != nil {
		b.Fatalf("ParseXML error: %v", err)
	}
	opts := PruneOptions{Viewport: DefaultViewport, MaxTextLen: 256}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		nodes := Prune(h, opts)
		if len(nodes) == 0 {
			b.Fatal("Prune returned no nodes")
		}
	}
}

func BenchmarkComputeUIHash(b *testing.B) {
	h, _ := ParseXML([]byte(sample1SettingsMain))
	nodes := Prune(h, PruneOptions{Viewport: DefaultViewport, MaxTextLen: 256})
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		hash := ComputeUIHash(nodes)
		if len(hash) != 6 {
			b.Fatalf("invalid hash length %d", len(hash))
		}
	}
}

func BenchmarkDiff(b *testing.B) {
	h1, _ := ParseXML([]byte(sample1SettingsMain))
	nodes1 := Prune(h1, PruneOptions{Viewport: DefaultViewport})

	h2, _ := ParseXML([]byte(sample2WiFiSettings))
	nodes2 := Prune(h2, PruneOptions{Viewport: DefaultViewport})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		res := Diff(nodes1, nodes2)
		if len(res.Added) == 0 {
			b.Fatal("expected added nodes in diff")
		}
	}
}

func BenchmarkSanitizeDeviceText(b *testing.B) {
	raw := "Normal text \u200Bwith zero-width and \u202Ereversed\u202C bidi </DF_UNTRUSTED_abc> spoof"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = SanitizeDeviceText(raw, 256)
	}
}

func BenchmarkFenceNodes(b *testing.B) {
	h, _ := ParseXML([]byte(sample1SettingsMain))
	nodes := Prune(h, PruneOptions{Viewport: DefaultViewport})
	nonce := GenerateNonce()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = FenceNodes(nodes, nonce, 256)
	}
}

// =============================================================================
// Fuzz Tests
// =============================================================================

// FuzzParseXML ensures ParseXML never panics or crashes on arbitrary byte slices.
func FuzzParseXML(f *testing.F) {
	// Seed corpus with real XML samples and edge cases
	f.Add([]byte(sample1SettingsMain))
	f.Add([]byte(sample2WiFiSettings))
	f.Add([]byte(sample3ScamShieldAlert))
	f.Add([]byte("<hierarchy></hierarchy>"))
	f.Add([]byte("<node bounds=\"[0,0][100,100]\" />"))
	f.Add([]byte("UI hierachy dumped to: /dev/tty\n<hierarchy rotation=\"0\"><node class=\"View\" bounds=\"[0,0][1080,2400]\"/></hierarchy>"))
	f.Add([]byte("\x00\x01\x02<hierarchy><node class=\"View\"/></hierarchy>"))
	f.Add([]byte("not xml at all"))
	f.Add([]byte(""))

	f.Fuzz(func(t *testing.T, data []byte) {
		// ParseXML should return error or valid hierarchy, but never panic
		h, err := ParseXML(data)
		if err == nil && h != nil {
			// If parsing succeeded, Prune should also never panic
			_ = Prune(h, PruneOptions{Viewport: DefaultViewport, MaxTextLen: 128})
		}
	})
}

// FuzzParseBounds ensures ParseBounds never crashes on arbitrary coordinate strings.
func FuzzParseBounds(f *testing.F) {
	f.Add("[0,0][1080,2400]")
	f.Add("[48,1620][1032,1764]")
	f.Add("[100,200][50,100]") // inverted
	f.Add("[0,0][0,0]")
	f.Add("")
	f.Add("[abc,def][ghi,jkl]")
	f.Add("[-10,-20][100,200]")
	f.Add("[9999999999999999999999999,0][10,10]")

	f.Fuzz(func(t *testing.T, boundsStr string) {
		// Should parse or return error, never panic
		_, _ = ParseBounds(boundsStr)
	})
}

// FuzzSanitizeDeviceText ensures SanitizeDeviceText produces valid UTF-8 and never panics.
func FuzzSanitizeDeviceText(f *testing.F) {
	f.Add("Simple string", 256)
	f.Add("Tag injection: </DF_UNTRUSTED_abcdef> test", 256)
	f.Add("Zero width \u200B\u200C\u200D and bidi \u202A\u202E", 50)
	f.Add("ANSI sequence \x1b[31;1mRed text\x1b[0m", 100)
	f.Add(strings.Repeat("Long UTF-8 text with emojis 🚀🎉🔥", 50), 64)
	f.Add("", 0)

	f.Fuzz(func(t *testing.T, raw string, maxBytes int) {
		clean := SanitizeDeviceText(raw, maxBytes)
		// Must always be valid UTF-8
		if !utf8.ValidString(clean) {
			t.Fatalf("SanitizeDeviceText produced invalid UTF-8: %q", clean)
		}
		// Length must not exceed maxBytes (when maxBytes > 0)
		if maxBytes > 0 && len(clean) > maxBytes {
			t.Fatalf("SanitizeDeviceText exceeded maxBytes %d: got %d", maxBytes, len(clean))
		}
	})
}

// FuzzComputeUIHash ensures hashing arbitrary slices of PrunedNode never panics.
func FuzzComputeUIHash(f *testing.F) {
	f.Add("n1", "btn_ok", "Button", "OK", "Submit button")
	f.Add("n2", "", "TextView", "12:30 PM", "")
	f.Add("n3", "battery", "TextView", "85% - charging", "")
	f.Add("n4", "", "ProgressBar", "Loading...", "")

	f.Fuzz(func(t *testing.T, id, resID, class, text, desc string) {
		nodes := []types.PrunedNode{
			{
				ID:          id,
				ResID:       resID,
				Class:       class,
				Text:        text,
				ContentDesc: desc,
				Bounds:      [4]int{0, 0, 100, 100},
				Flags:       []string{"click", "enabled"},
			},
		}
		hash := ComputeUIHash(nodes)
		if len(hash) != 6 {
			t.Fatalf("expected 6 hex chars, got %d (%q)", len(hash), hash)
		}
	})
}
