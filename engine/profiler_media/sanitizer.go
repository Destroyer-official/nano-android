package profiler_media

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"
	"unicode"
)

// UntrustedSource identifies the origin of unverified Android data.
type UntrustedSource string

const (
	SourceScreen       UntrustedSource = "SCREEN"
	SourceSMS          UntrustedSource = "SMS"
	SourceNotification UntrustedSource = "NOTIFICATION"
	SourceLogcat       UntrustedSource = "LOGCAT"
	SourceClipboard    UntrustedSource = "CLIPBOARD"
	SourceGeneric      UntrustedSource = "GENERIC"
)

// ThreatCategory classifies detected prompt injection attacks.
type ThreatCategory string

const (
	ThreatInstructionOverride ThreatCategory = "INSTRUCTION_OVERRIDE"
	ThreatRoleplayJailbreak    ThreatCategory = "ROLEPLAY_JAILBREAK"
	ThreatDelimiterBreakout    ThreatCategory = "DELIMITER_BREAKOUT"
	ThreatCommandInjection     ThreatCategory = "COMMAND_INJECTION"
	ThreatDataExfiltration     ThreatCategory = "DATA_EXFILTRATION"
	ThreatUnicodeObfuscation   ThreatCategory = "UNICODE_OBFUSCATION"
)

// ThreatIndicator holds details of a detected attack signature.
type ThreatIndicator struct {
	Category    ThreatCategory `json:"category"`
	Pattern     string         `json:"pattern"`
	MatchedText string         `json:"matched_text"`
	Severity    string         `json:"severity"` // LOW, MEDIUM, HIGH, CRITICAL
}

// InjectionAnalysisResult contains the security audit of untrusted text.
type InjectionAnalysisResult struct {
	Detected         bool              `json:"detected"`
	RiskScore        float64           `json:"risk_score"` // 0.0 (safe) to 1.0 (malicious)
	Indicators       []ThreatIndicator `json:"indicators"`
	Categories       []ThreatCategory  `json:"categories"`
	HasBreakout      bool              `json:"has_breakout_attempt"`
	HasObfuscation   bool              `json:"has_obfuscation"`
	CleanedContent   string            `json:"cleaned_content"`
}

// QuarantineRecord holds a quarantined string along with its metadata and cryptographic hash.
type QuarantineRecord struct {
	Source           UntrustedSource         `json:"source"`
	RawContent       string                  `json:"raw_content"`
	SanitizedContent string                  `json:"sanitized_content"`
	QuarantinedText  string                  `json:"quarantined_text"`
	ChecksumHex      string                  `json:"checksum_hex"` // SHA-256 integrity hash
	Analysis         InjectionAnalysisResult `json:"analysis"`
	Timestamp        time.Time               `json:"timestamp"`
}

var (
	// Regex patterns for prompt injection detection.
	overridePatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)(?:ignore|disregard|forget|override)\s+(?:all\s+)?(?:previous|prior|above)\s+(?:instructions|prompts|directives|rules)`),
		regexp.MustCompile(`(?i)(?:new\s+instructions|system\s+override|system\s+prompt\s*:|assistant\s*:|developer\s+mode\s+enabled)`),
		regexp.MustCompile(`(?i)(?:bypass\s+all\s+(?:safety|content|guideline)\s+filters)`),
	}

	jailbreakPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)(?:you\s+are\s+now|act\s+as\s+an?\s+unfiltered|unrestricted\s+AI|DAN\s+mode|jailbreak)`),
		regexp.MustCompile(`(?i)(?:do\s+anything\s+now|always\s+comply|disable\s+safety)`),
	}

	breakoutPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)</?DF_UNTRUSTED_[A-Z_]+>`),
		regexp.MustCompile(`(?i)<\|im_start\|>|<\|im_end\|>|<\|endoftext\|>`),
		regexp.MustCompile(`(?i)\[INST\]|\[/INST\]|<<SYS>>|<</SYS>>`),
		regexp.MustCompile("(?i)```(?:system|admin|root|shell|bash)"),
	}

	commandPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)(?:rm\s+-rf|curl\s+-[sS]?[kK]?\s+https?://|wget\s+https?://|powershell(?:\.exe)?\s+-[eE]|cmd\.exe\s+/c|sh\s+-c|bash\s+-c)`),
		regexp.MustCompile(`(?i)(?:adb\s+shell\s+(?:su|rm|pm\s+uninstall|reboot)|nc\s+-[lvpne]+)`),
	}

	exfiltrationPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)(?:print|show|output|leak|reveal|send)\s+(?:your\s+)?(?:system\s+prompt|instructions|api[_\s]?key|credentials|secret)`),
		regexp.MustCompile(`(?i)(?:what\s+are\s+your\s+(?:exact\s+)?instructions\s+before\s+this)`),
	}

	// Bidirectional and zero-width characters used for obfuscation.
	zwcRegex = regexp.MustCompile("[\u200B-\u200D\uFEFF\u200E\u200F\u202A-\u202E\u2066-\u2069]")
)

// StripObfuscation removes zero-width and bidirectional control characters.
func StripObfuscation(input string) (string, bool) {
	hasObfuscation := zwcRegex.MatchString(input)
	cleaned := zwcRegex.ReplaceAllString(input, "")
	return cleaned, hasObfuscation
}

// NeutralizeBreakouts replaces fake quarantine tags with safe escaped markers.
func NeutralizeBreakouts(input string) (string, bool) {
	hasBreakout := false
	neutralized := breakoutPatterns[0].ReplaceAllStringFunc(input, func(m string) string {
		hasBreakout = true
		return strings.ReplaceAll(strings.ReplaceAll(m, "<", "&lt;"), ">", "&gt;")
	})
	return neutralized, hasBreakout
}

// AnalyzePromptInjection inspects text for jailbreak and injection signatures.
func AnalyzePromptInjection(text string) InjectionAnalysisResult {
	cleaned, hasObf := StripObfuscation(text)
	neutralized, hasBreak := NeutralizeBreakouts(cleaned)

	var indicators []ThreatIndicator
	categoryMap := make(map[ThreatCategory]bool)
	score := 0.0

	if hasObf {
		indicators = append(indicators, ThreatIndicator{
			Category:    ThreatUnicodeObfuscation,
			Pattern:     "ZWC/BIDI characters",
			MatchedText: "Hidden Unicode code points",
			Severity:    "MEDIUM",
		})
		categoryMap[ThreatUnicodeObfuscation] = true
		score += 0.35
	}

	if hasBreak {
		indicators = append(indicators, ThreatIndicator{
			Category:    ThreatDelimiterBreakout,
			Pattern:     "</?DF_UNTRUSTED_*>",
			MatchedText: "Quarantine tag breakout attempt",
			Severity:    "CRITICAL",
		})
		categoryMap[ThreatDelimiterBreakout] = true
		score += 0.60
	}

	for _, re := range overridePatterns {
		if loc := re.FindString(neutralized); loc != "" {
			indicators = append(indicators, ThreatIndicator{
				Category:    ThreatInstructionOverride,
				Pattern:     re.String(),
				MatchedText: loc,
				Severity:    "CRITICAL",
			})
			categoryMap[ThreatInstructionOverride] = true
			score += 0.50
		}
	}

	for _, re := range jailbreakPatterns {
		if loc := re.FindString(neutralized); loc != "" {
			indicators = append(indicators, ThreatIndicator{
				Category:    ThreatRoleplayJailbreak,
				Pattern:     re.String(),
				MatchedText: loc,
				Severity:    "CRITICAL",
			})
			categoryMap[ThreatRoleplayJailbreak] = true
			score += 0.50
		}
	}

	for _, re := range breakoutPatterns[1:] {
		if loc := re.FindString(neutralized); loc != "" {
			indicators = append(indicators, ThreatIndicator{
				Category:    ThreatDelimiterBreakout,
				Pattern:     re.String(),
				MatchedText: loc,
				Severity:    "HIGH",
			})
			categoryMap[ThreatDelimiterBreakout] = true
			score += 0.40
		}
	}

	for _, re := range commandPatterns {
		if loc := re.FindString(neutralized); loc != "" {
			indicators = append(indicators, ThreatIndicator{
				Category:    ThreatCommandInjection,
				Pattern:     re.String(),
				MatchedText: loc,
				Severity:    "HIGH",
			})
			categoryMap[ThreatCommandInjection] = true
			score += 0.45
		}
	}

	for _, re := range exfiltrationPatterns {
		if loc := re.FindString(neutralized); loc != "" {
			indicators = append(indicators, ThreatIndicator{
				Category:    ThreatDataExfiltration,
				Pattern:     re.String(),
				MatchedText: loc,
				Severity:    "HIGH",
			})
			categoryMap[ThreatDataExfiltration] = true
			score += 0.40
		}
	}

	if score > 1.0 {
		score = 1.0
	}

	categories := make([]ThreatCategory, 0, len(categoryMap))
	for cat := range categoryMap {
		categories = append(categories, cat)
	}

	return InjectionAnalysisResult{
		Detected:       len(indicators) > 0,
		RiskScore:      score,
		Indicators:     indicators,
		Categories:     categories,
		HasBreakout:    hasBreak,
		HasObfuscation: hasObf,
		CleanedContent: neutralized,
	}
}

// QuarantineText wraps untrusted content in <DF_UNTRUSTED_*> tags after neutralizing breakouts.
func QuarantineText(source UntrustedSource, rawContent string) QuarantineRecord {
	analysis := AnalyzePromptInjection(rawContent)
	var tagName string

	switch source {
	case SourceScreen:
		tagName = "DF_UNTRUSTED_SCREEN"
	case SourceSMS:
		tagName = "DF_UNTRUSTED_SMS"
	case SourceNotification:
		tagName = "DF_UNTRUSTED_NOTIFICATION"
	case SourceLogcat:
		tagName = "DF_UNTRUSTED_LOGCAT"
	case SourceClipboard:
		tagName = "DF_UNTRUSTED_CLIPBOARD"
	default:
		tagName = fmt.Sprintf("DF_UNTRUSTED_DATA source=\"%s\"", string(source))
	}

	var openTag, closeTag string
	if strings.Contains(tagName, " ") {
		openTag = fmt.Sprintf("<%s>", tagName)
		closeTag = "</DF_UNTRUSTED_DATA>"
	} else {
		openTag = fmt.Sprintf("<%s>", tagName)
		closeTag = fmt.Sprintf("</%s>", tagName)
	}

	quarantined := fmt.Sprintf("%s\n%s\n%s", openTag, analysis.CleanedContent, closeTag)
	sum := sha256.Sum256([]byte(analysis.CleanedContent))
	checksum := hex.EncodeToString(sum[:])

	return QuarantineRecord{
		Source:           source,
		RawContent:       rawContent,
		SanitizedContent: analysis.CleanedContent,
		QuarantinedText:  quarantined,
		ChecksumHex:      checksum,
		Analysis:         analysis,
		Timestamp:        time.Now().UTC(),
	}
}

// VerifyQuarantineIntegrity validates that the sanitized content matches its SHA-256 digest.
func VerifyQuarantineIntegrity(record QuarantineRecord) bool {
	sum := sha256.Sum256([]byte(record.SanitizedContent))
	return hex.EncodeToString(sum[:]) == record.ChecksumHex
}

// StreamQuarantineLogcat streams logcat lines from an io.Reader and yields quarantined chunks.
func StreamQuarantineLogcat(r io.Reader, maxLinesPerChunk int, onChunk func(QuarantineRecord) error) error {
	if maxLinesPerChunk <= 0 {
		maxLinesPerChunk = 50
	}
	scanner := bufio.NewScanner(r)
	var lines []string

	for scanner.Scan() {
		cleanLine := SanitizeLogcatLine(scanner.Text())
		lines = append(lines, cleanLine)
		if len(lines) >= maxLinesPerChunk {
			chunk := strings.Join(lines, "\n")
			record := QuarantineLogcat(chunk)
			if err := onChunk(record); err != nil {
				return err
			}
			lines = lines[:0]
		}
	}

	if len(lines) > 0 {
		chunk := strings.Join(lines, "\n")
		record := QuarantineLogcat(chunk)
		if err := onChunk(record); err != nil {
			return err
		}
	}

	return scanner.Err()
}

// QuarantineScreen wraps UI hierarchy and OCR text extracted from screens.
func QuarantineScreen(screenText string) QuarantineRecord {
	return QuarantineText(SourceScreen, screenText)
}

// QuarantineSMS wraps SMS sender and body.
func QuarantineSMS(sender, body string) QuarantineRecord {
	formatted := fmt.Sprintf("Sender: %s\nBody: %s", sender, body)
	return QuarantineText(SourceSMS, formatted)
}

// QuarantineNotification wraps Android notification title, package, and text.
func QuarantineNotification(pkg, title, text string) QuarantineRecord {
	formatted := fmt.Sprintf("Package: %s\nTitle: %s\nText: %s", pkg, title, text)
	return QuarantineText(SourceNotification, formatted)
}

// QuarantineLogcat wraps logcat dumps or stream slices.
func QuarantineLogcat(logLines string) QuarantineRecord {
	return QuarantineText(SourceLogcat, logLines)
}

// QuarantineClipboard wraps clipboard text.
func QuarantineClipboard(clipText string) QuarantineRecord {
	return QuarantineText(SourceClipboard, clipText)
}

// WrapUntrusted is a convenience wrapper returning only the quarantined string.
func WrapUntrusted(source UntrustedSource, raw string) string {
	return QuarantineText(source, raw).QuarantinedText
}

// IsStrictlyQuarantined verifies that a string begins with an opening <DF_UNTRUSTED_*> tag
// and closes properly with </DF_UNTRUSTED_*>, with no unneutralized internal breakout tags.
func IsStrictlyQuarantined(quarantinedText string) bool {
	trimmed := strings.TrimSpace(quarantinedText)
	if !strings.HasPrefix(trimmed, "<DF_UNTRUSTED_") {
		return false
	}
	if !strings.HasSuffix(trimmed, "</DF_UNTRUSTED_SCREEN>") &&
		!strings.HasSuffix(trimmed, "</DF_UNTRUSTED_SMS>") &&
		!strings.HasSuffix(trimmed, "</DF_UNTRUSTED_NOTIFICATION>") &&
		!strings.HasSuffix(trimmed, "</DF_UNTRUSTED_LOGCAT>") &&
		!strings.HasSuffix(trimmed, "</DF_UNTRUSTED_CLIPBOARD>") &&
		!strings.HasSuffix(trimmed, "</DF_UNTRUSTED_DATA>") {
		return false
	}

	// Verify inner body has no raw closing tag breakout
	firstClose := strings.Index(trimmed, ">")
	lastOpen := strings.LastIndex(trimmed, "</DF_UNTRUSTED_")
	if firstClose < 0 || lastOpen <= firstClose {
		return false
	}

	inner := trimmed[firstClose+1 : lastOpen]
	return !breakoutPatterns[0].MatchString(inner)
}

// SanitizeLogcatLine ensures individual logcat lines with control characters or ANSI escapes are stripped.
func SanitizeLogcatLine(line string) string {
	var sb strings.Builder
	for _, r := range line {
		if unicode.IsControl(r) && r != '\n' && r != '\t' && r != '\r' {
			continue
		}
		sb.WriteRune(r)
	}
	return sb.String()
}
