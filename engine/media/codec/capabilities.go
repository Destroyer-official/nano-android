package codec

import (
	"regexp"
	"sort"
	"strings"
)

// CodecCapability describes one MediaCodec encoder/decoder parsed from real device `dumpsys media.codec`.
type CodecCapability struct {
	Name                string   `json:"name"`
	MimeType            string   `json:"mime_type"`
	CanonicalCodec      string   `json:"canonical_codec"`
	IsEncoder           bool     `json:"is_encoder"`
	HardwareAccelerated bool     `json:"hardware_accelerated"`
	MaxResolution       string   `json:"max_resolution,omitempty"`
	Profiles            []string `json:"profiles,omitempty"`
}

var (
	reCodecHeader = regexp.MustCompile(`^(?:Decoder|Encoder)\s+"([^"]+)"\s+supports\s+(?:\[([^\]]+)\]|([a-z0-9/.\-]+))`)
	reMediaLine   = regexp.MustCompile(`([a-zA-Z0-9_.\-]+)\s+\((video|audio)/([a-zA-Z0-9.\-]+)\)`)
)

// NormalizeMimeToCodec maps Android MIME strings / codec names to canonical families.
func NormalizeMimeToCodec(raw string) string {
	l := strings.ToLower(strings.TrimSpace(raw))
	switch {
	case strings.Contains(l, "avc") || strings.Contains(l, "h264"):
		return "h264"
	case strings.Contains(l, "hevc") || strings.Contains(l, "h265") || strings.Contains(l, "hev1"):
		return "h265"
	case strings.Contains(l, "av01") || strings.Contains(l, "av1"):
		return "av1"
	case strings.Contains(l, "vp9"):
		return "vp9"
	case strings.Contains(l, "mp4a") || strings.Contains(l, "aac"):
		return "aac"
	case strings.Contains(l, "opus"):
		return "opus"
	default:
		return l
	}
}

// IsSoftwareCodecName checks Android OMX/C2 software codec naming prefixes.
func IsSoftwareCodecName(name string) bool {
	l := strings.ToLower(name)
	return strings.HasPrefix(l, "omx.google.") ||
		strings.HasPrefix(l, "c2.android.") ||
		strings.Contains(l, ".sw.")
}

// ParseMediaCodecDumpsys parses `adb shell dumpsys media.codec` output into structured CodecCapability entries.
func ParseMediaCodecDumpsys(output string) []CodecCapability {
	seen := make(map[string]CodecCapability)
	for _, rawLine := range strings.Split(output, "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" {
			continue
		}
		if m := reCodecHeader.FindStringSubmatch(line); len(m) >= 2 {
			name := m[1]
			mime := m[2]
			if mime == "" {
				mime = m[3]
			}
			mime = strings.TrimSpace(strings.Split(mime, ",")[0])
			isEnc := strings.HasPrefix(line, "Encoder") || strings.Contains(strings.ToLower(name), ".encoder")
			capObj := CodecCapability{
				Name:                name,
				MimeType:            mime,
				CanonicalCodec:      NormalizeMimeToCodec(mime),
				IsEncoder:           isEnc,
				HardwareAccelerated: !IsSoftwareCodecName(name),
			}
			if idx := strings.Index(line, "max="); idx > 0 {
				rest := strings.Fields(line[idx+4:])
				if len(rest) > 0 {
					capObj.MaxResolution = strings.Trim(rest[0], ",;")
				}
			}
			seen[name+"#"+mime] = capObj
			continue
		}
		if m := reMediaLine.FindStringSubmatch(line); len(m) == 4 {
			name := m[1]
			mime := m[2] + "/" + m[3]
			capObj := CodecCapability{
				Name:                name,
				MimeType:            mime,
				CanonicalCodec:      NormalizeMimeToCodec(mime),
				IsEncoder:           strings.Contains(strings.ToLower(line), "encoder") || strings.Contains(strings.ToLower(name), "enc"),
				HardwareAccelerated: !IsSoftwareCodecName(name),
			}
			seen[name+"#"+mime] = capObj
		}
	}
	out := make([]CodecCapability, 0, len(seen))
	for _, v := range seen {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})
	return out
}
