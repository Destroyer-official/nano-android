package uitree

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"regexp"
	"sort"
	"strings"

	"github.com/anthropic/droidforge-mcp/internal/types"
)

var (
	// Regex matching clock times: HH:MM, HH:MM:SS, 12:34 PM, 9:41, timezones (EST, UTC, etc.)
	clockRegex = regexp.MustCompile(`(?i)\b(?:0?[0-9]|1[0-9]|2[0-3]):[0-5][0-9](?::[0-5][0-9])?(?:\s*(?:am|pm))?(?:\s*(?:est|edt|pst|pdt|cst|cdt|utc|gmt))?\b`)

	// Regex matching relative timestamps: "just now", "2m ago", "1 hour ago", "yesterday at 4pm", etc.
	relTimeRegex = regexp.MustCompile(`(?i)\b(?:just now|\d+\s*(?:s|sec|secs|second|seconds|m|min|mins|minute|minutes|h|hr|hrs|hour|hours|d|day|days|wk|wks|week|weeks|mo|mos|month|months|y|yr|yrs|year|years)\s*ago|\d+[smhdwy]\s*ago|yesterday(?:\s+at\s+[^,\n]+)?|today(?:\s+at\s+[^,\n]+)?|tomorrow)\b`)

	// Regex matching battery percentages and charging strings: 100%, 98%, "Charging (85%)"
	batteryRegex = regexp.MustCompile(`(?i)(?:charging\s*\(\d{1,3}%\)|\b\d{1,3}%\s*(?:-\s*charging|\(charging\))?|\b\d{1,3}%)`)

	// Regex to strip existing untrusted data nonce tags if present
	nonceTagRegex = regexp.MustCompile(`</?DF_UNTRUSTED_[a-fA-F0-9]+>`)
)

// CanonicalHashNode represents a pruned node with all volatile dynamic noise masked out.
type CanonicalHashNode struct {
	ID     string   `json:"id"`
	Res    string   `json:"res,omitempty"`
	Cls    string   `json:"cls"`
	Text   string   `json:"text,omitempty"`
	Desc   string   `json:"desc,omitempty"`
	Bounds [4]int   `json:"bounds"`
	Flags  []string `json:"flags,omitempty"`
}

// MaskNoise strips or replaces volatile dynamic noise (clocks, battery, relative timestamps, progress).
func MaskNoise(text string) string {
	if text == "" {
		return ""
	}

	// 1. Strip any existing nonce fences first so they don't alter the structural hash
	s := nonceTagRegex.ReplaceAllString(text, "")

	// 2. Mask clock times
	s = clockRegex.ReplaceAllString(s, "[TIME]")

	// 3. Mask relative timestamps
	s = relTimeRegex.ReplaceAllString(s, "[REL_TIME]")

	// 4. Mask battery percentages
	s = batteryRegex.ReplaceAllString(s, "[BATTERY]")

	return strings.TrimSpace(s)
}

// isProgressWidget returns true if the node class represents a progress or seek bar.
func isProgressWidget(className string) bool {
	cls := strings.ToLower(className)
	return strings.Contains(cls, "progressbar") ||
		strings.Contains(cls, "seekbar") ||
		strings.Contains(cls, "ratingbar") ||
		strings.Contains(cls, "slider") ||
		strings.Contains(cls, "indicator")
}

// ComputeUIHash calculates the SHA-256 noise-masked structural hash of a pruned tree,
// returning the first 6 hexadecimal characters.
func ComputeUIHash(nodes []types.PrunedNode) string {
	if len(nodes) == 0 {
		return "000000"
	}

	canonicalNodes := make([]CanonicalHashNode, len(nodes))
	for i, n := range nodes {
		text := n.Text
		desc := n.ContentDesc

		// Mask progress bars and sliders to eliminate indeterminate animation noise
		if isProgressWidget(n.Class) {
			text = "[PROGRESS]"
			desc = "[PROGRESS]"
		} else {
			text = MaskNoise(text)
			desc = MaskNoise(desc)
		}

		// Ensure flags are sorted for deterministic JSON serialization
		flags := make([]string, len(n.Flags))
		copy(flags, n.Flags)
		sort.Strings(flags)

		canonicalNodes[i] = CanonicalHashNode{
			ID:     n.ID,
			Res:    n.ResID,
			Cls:    n.Class,
			Text:   text,
			Desc:   desc,
			Bounds: n.Bounds,
			Flags:  flags,
		}
	}

	data, err := json.Marshal(canonicalNodes)
	if err != nil {
		// Fallback to empty hash if marshaling fails
		return "000000"
	}

	sum := sha256.Sum256(data)
	hexHash := hex.EncodeToString(sum[:])
	if len(hexHash) > 6 {
		return hexHash[:6]
	}
	return hexHash
}
