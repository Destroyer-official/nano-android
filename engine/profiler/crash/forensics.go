package crash

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"nano_android/engine/security/audit"
)

// R8MethodRule represents one obfuscated -> original method line-range mapping from a ProGuard/R8 mapping.txt.
type R8MethodRule struct {
	ObfStart   int    `json:"obf_start"`
	ObfEnd     int    `json:"obf_end"`
	OrigClass  string `json:"orig_class"`
	OrigMethod string `json:"orig_method"`
	OrigArgs   string `json:"orig_args,omitempty"`
	OrigStart  int    `json:"orig_start"`
	OrigEnd    int    `json:"orig_end"`
}

// R8Mapping holds parsed class and method rules for ProGuard/R8 stack trace retrace.
type R8Mapping struct {
	ObfToOrigClass map[string]string         `json:"obf_to_orig_class"`
	MethodRules    map[string][]R8MethodRule `json:"method_rules"` // key: obfClass + "#" + obfMethod
}

var (
	reClassHeader = regexp.MustCompile(`^([a-zA-Z0-9_.$]+)\s+->\s+([a-zA-Z0-9_.$]+):\s*$`)
	reMethodLine  = regexp.MustCompile(`^\s+(?:(\d+):(\d+):)?(?:[a-zA-Z0-9_.$\[\]]+\s+)?([a-zA-Z0-9_.$<>]+)\(([^)]*)\)(?::(\d+)(?::(\d+))?)?\s+->\s+([a-zA-Z0-9_.$<>]+)\s*$`)
	reStackFrame  = regexp.MustCompile(`^(\s*at\s+)([a-zA-Z0-9_.$]+)\.([a-zA-Z0-9_.$<>]+)\(([^():]*)(?::(\d+))?\)(.*)$`)
	reHexOrDigits = regexp.MustCompile(`0x[0-9a-fA-F]+|@[0-9a-fA-F]+|\b\d{4,}\b`)
	reCausedBy    = regexp.MustCompile(`^Caused by:\s*([^:\r\n]+)(?::\s*(.*))?$`)
	reExHeader    = regexp.MustCompile(`^(?:FATAL EXCEPTION:\s*\S+|([a-zA-Z0-9_.$]*(?:Exception|Error|Throwable))(?::\s*(.*))?)$`)
)

// ParseR8Mapping parses a standard ProGuard/R8 mapping.txt document into deterministic lookup tables.
func ParseR8Mapping(mappingText string) *R8Mapping {
	m := &R8Mapping{
		ObfToOrigClass: make(map[string]string),
		MethodRules:    make(map[string][]R8MethodRule),
	}
	var currentOrigClass, currentObfClass string
	for _, raw := range strings.Split(mappingText, "\n") {
		line := strings.TrimRight(raw, "\r")
		if strings.TrimSpace(line) == "" || strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		if cm := reClassHeader.FindStringSubmatch(line); len(cm) == 3 {
			currentOrigClass = cm[1]
			currentObfClass = cm[2]
			m.ObfToOrigClass[currentObfClass] = currentOrigClass
			continue
		}
		if currentObfClass == "" {
			continue
		}
		if mm := reMethodLine.FindStringSubmatch(line); len(mm) == 8 {
			obfStart, _ := strconv.Atoi(mm[1])
			obfEnd, _ := strconv.Atoi(mm[2])
			origMethod := mm[3]
			origArgs := mm[4]
			origStart, _ := strconv.Atoi(mm[5])
			origEnd, _ := strconv.Atoi(mm[6])
			obfMethod := mm[7]
			if origEnd == 0 {
				origEnd = origStart
			}

			origClass := currentOrigClass
			// Handle R8 cross-class inlined frames like com.foo.Bar.baz
			if idx := strings.LastIndex(origMethod, "."); idx > 0 {
				origClass = origMethod[:idx]
				origMethod = origMethod[idx+1:]
			}

			key := currentObfClass + "#" + obfMethod
			m.MethodRules[key] = append(m.MethodRules[key], R8MethodRule{
				ObfStart:   obfStart,
				ObfEnd:     obfEnd,
				OrigClass:  origClass,
				OrigMethod: origMethod,
				OrigArgs:   origArgs,
				OrigStart:  origStart,
				OrigEnd:    origEnd,
			})
		}
	}
	return m
}

// RetraceStackTrace de-obfuscates a Java/Kotlin stack trace using the parsed ProGuard/R8 mapping.
func RetraceStackTrace(mapping *R8Mapping, trace string) string {
	if mapping == nil || strings.TrimSpace(trace) == "" {
		return trace
	}
	lines := strings.Split(trace, "\n")
	out := make([]string, 0, len(lines))
	for _, raw := range lines {
		line := strings.TrimRight(raw, "\r")
		if fm := reStackFrame.FindStringSubmatch(line); len(fm) == 7 {
			prefix, cls, meth, srcFile, lineStr, suffix := fm[1], fm[2], fm[3], fm[4], fm[5], fm[6]
			lineNum, _ := strconv.Atoi(lineStr)

			origCls := cls
			if mappedCls, ok := mapping.ObfToOrigClass[cls]; ok {
				origCls = mappedCls
			}
			origMeth := meth
			origLine := lineNum

			if rules := mapping.MethodRules[cls+"#"+meth]; len(rules) > 0 {
				var matched []R8MethodRule
				for _, rule := range rules {
					if rule.ObfStart == 0 || (lineNum >= rule.ObfStart && lineNum <= rule.ObfEnd) {
						matched = append(matched, rule)
					}
				}
				if len(matched) == 0 {
					matched = append(matched, rules[0])
				}
				for idx, rule := range matched {
					if idx > 0 {
						out = append(out, prefix+fmt.Sprintf("%s.%s(%s:%d)%s", rule.OrigClass, rule.OrigMethod, inferSourceName(rule.OrigClass, srcFile), rule.OrigStart, suffix))
						continue
					}
					origCls = rule.OrigClass
					origMeth = rule.OrigMethod
					if rule.OrigStart > 0 && rule.ObfStart > 0 && lineNum >= rule.ObfStart {
						offset := lineNum - rule.ObfStart
						origLine = rule.OrigStart + offset
						if rule.OrigEnd > 0 && origLine > rule.OrigEnd {
							origLine = rule.OrigEnd
						}
					} else if rule.OrigStart > 0 {
						origLine = rule.OrigStart
					}
				}
			}

			resolvedSrc := inferSourceName(origCls, srcFile)
			if origLine > 0 {
				out = append(out, fmt.Sprintf("%s%s.%s(%s:%d)%s", prefix, origCls, origMeth, resolvedSrc, origLine, suffix))
			} else {
				out = append(out, fmt.Sprintf("%s%s.%s(%s)%s", prefix, origCls, origMeth, resolvedSrc, suffix))
			}
			continue
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

func inferSourceName(origClass, fallback string) string {
	if fallback != "" && !strings.EqualFold(fallback, "SourceFile") && !strings.EqualFold(fallback, "Unknown Source") {
		return fallback
	}
	simple := origClass
	if idx := strings.LastIndex(simple, "."); idx >= 0 {
		simple = simple[idx+1:]
	}
	if idx := strings.Index(simple, "$"); idx > 0 {
		simple = simple[:idx]
	}
	if simple == "" {
		return fallback
	}
	return simple + ".java"
}

// CrashReport represents a parsed, secret-redacted, optionally retraced crash or ANR event.
type CrashReport struct {
	Signature       string   `json:"signature"`
	Kind            string   `json:"kind"` // EXCEPTION, ANR, MEMORY_LIMITER
	ExceptionClass  string   `json:"exception_class"`
	Message         string   `json:"message"`
	RootCauseClass  string   `json:"root_cause_class"`
	TopAppFrame     string   `json:"top_app_frame,omitempty"`
	CauseChain      []string `json:"cause_chain,omitempty"`
	Retraced        bool     `json:"retraced"`
	RawTrace        string   `json:"raw_trace"`
	RetracedTrace   string   `json:"retraced_trace"`
	BuildTag        string   `json:"build_tag,omitempty"`
	EnvironmentTier string   `json:"environment_tier,omitempty"`
}

// CrashCluster aggregates identical crashes across runs or workers by signature.
type CrashCluster struct {
	Signature       string   `json:"signature"`
	Kind            string   `json:"kind"`
	ExceptionClass  string   `json:"exception_class"`
	RootCauseClass  string   `json:"root_cause_class"`
	TopAppFrame     string   `json:"top_app_frame,omitempty"`
	Count           int      `json:"count"`
	SampleMessage   string   `json:"sample_message"`
	SampleTrace     string   `json:"sample_trace"`
	BuildsAffected  []string `json:"builds_affected,omitempty"`
}

// CrashDiff summarizes regressions and resolutions between baseline and candidate crash clusters.
type CrashDiff struct {
	NewSignatures      []string `json:"new_signatures"`
	ResolvedSignatures []string `json:"resolved_signatures"`
	Persistent         []string `json:"persistent_signatures"`
	BaselineTotal      int      `json:"baseline_total"`
	CandidateTotal     int      `json:"candidate_total"`
}

// CrashReproBundle packages redacted forensics cleanly for archival and CI export.
type CrashReproBundle struct {
	Artifact        string       `json:"artifact"`
	EnvironmentTier string       `json:"environment_tier"`
	Report          CrashReport  `json:"report"`
	DeviceProfile   string       `json:"device_profile,omitempty"`
	ReproSteps      []string     `json:"repro_steps,omitempty"`
}

// AnalyzeTrace parses raw exception/ANR trace text, redacts secrets, optionally applies R8 mapping, and computes its cluster signature.
func AnalyzeTrace(rawTrace, buildTag, envTier string, mapping *R8Mapping) CrashReport {
	redacted := audit.RedactString(rawTrace)
	retraced := redacted
	didRetrace := false
	if mapping != nil {
		cand := RetraceStackTrace(mapping, redacted)
		if cand != redacted {
			didRetrace = true
			retraced = cand
		}
	}

	rep := CrashReport{
		Kind:            "EXCEPTION",
		RawTrace:        redacted,
		RetracedTrace:   retraced,
		Retraced:        didRetrace,
		BuildTag:        buildTag,
		EnvironmentTier: envTier,
	}

	if strings.Contains(retraced, "MemoryLimiter:AnonSwap") {
		rep.Kind = "MEMORY_LIMITER"
	} else if strings.Contains(retraced, "ANR in ") || strings.Contains(retraced, "Application Not Responding") {
		rep.Kind = "ANR"
	}

	var topFrame string
	for _, rawLine := range strings.Split(retraced, "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" {
			continue
		}
		if rep.ExceptionClass == "" {
			if m := reExHeader.FindStringSubmatch(line); len(m) > 0 && m[1] != "" {
				rep.ExceptionClass = m[1]
				rep.Message = strings.TrimSpace(m[2])
			} else if idx := strings.Index(line, "Exception"); idx >= 0 && rep.ExceptionClass == "" {
				parts := strings.SplitN(line, ":", 2)
				rep.ExceptionClass = strings.TrimSpace(parts[0])
				if len(parts) > 1 {
					rep.Message = strings.TrimSpace(parts[1])
				}
			}
		}
		if cm := reCausedBy.FindStringSubmatch(line); len(cm) >= 2 {
			rep.CauseChain = append(rep.CauseChain, cm[1])
			rep.RootCauseClass = strings.TrimSpace(cm[1])
		}
		if strings.HasPrefix(line, "at ") {
			frame := strings.TrimPrefix(line, "at ")
			if topFrame == "" {
				topFrame = frame
			}
			// Prefer non-JDK / non-framework frame if present
			if rep.TopAppFrame == "" && !strings.HasPrefix(frame, "java.") && !strings.HasPrefix(frame, "android.") && !strings.HasPrefix(frame, "nanoandroid.") {
				rep.TopAppFrame = frame
			}
		}
	}

	if rep.ExceptionClass == "" {
		rep.ExceptionClass = rep.Kind
	}
	if rep.RootCauseClass == "" {
		rep.RootCauseClass = rep.ExceptionClass
	}
	if rep.TopAppFrame == "" {
		rep.TopAppFrame = topFrame
	}

	normalizedFrame := reHexOrDigits.ReplaceAllString(rep.TopAppFrame, "<N>")
	sigInput := fmt.Sprintf("%s|%s|%s|%s", rep.Kind, rep.ExceptionClass, rep.RootCauseClass, normalizedFrame)
	h := sha256.Sum256([]byte(sigInput))
	rep.Signature = hex.EncodeToString(h[:8])
	return rep
}

// ClusterCrashes groups CrashReports by deterministic normalized signature.
func ClusterCrashes(reports []CrashReport) []CrashCluster {
	bySig := make(map[string]*CrashCluster)
	buildsSeen := make(map[string]map[string]bool)
	for _, r := range reports {
		cl, ok := bySig[r.Signature]
		if !ok {
			cl = &CrashCluster{
				Signature:      r.Signature,
				Kind:           r.Kind,
				ExceptionClass: r.ExceptionClass,
				RootCauseClass: r.RootCauseClass,
				TopAppFrame:    r.TopAppFrame,
				SampleMessage:  r.Message,
				SampleTrace:    r.RetracedTrace,
			}
			bySig[r.Signature] = cl
			buildsSeen[r.Signature] = make(map[string]bool)
		}
		cl.Count++
		if r.BuildTag != "" && !buildsSeen[r.Signature][r.BuildTag] {
			buildsSeen[r.Signature][r.BuildTag] = true
			cl.BuildsAffected = append(cl.BuildsAffected, r.BuildTag)
		}
	}
	out := make([]CrashCluster, 0, len(bySig))
	for _, cl := range bySig {
		sort.Strings(cl.BuildsAffected)
		out = append(out, *cl)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Signature < out[j].Signature
	})
	return out
}

// CompareClusters identifies new crash regressions vs resolved crashes between two builds.
func CompareClusters(baseline, candidate []CrashCluster) CrashDiff {
	baseSet := map[string]bool{}
	candSet := map[string]bool{}
	for _, b := range baseline {
		baseSet[b.Signature] = true
	}
	for _, c := range candidate {
		candSet[c.Signature] = true
	}
	diff := CrashDiff{
		BaselineTotal:  len(baseline),
		CandidateTotal: len(candidate),
	}
	for sig := range candSet {
		if baseSet[sig] {
			diff.Persistent = append(diff.Persistent, sig)
		} else {
			diff.NewSignatures = append(diff.NewSignatures, sig)
		}
	}
	for sig := range baseSet {
		if !candSet[sig] {
			diff.ResolvedSignatures = append(diff.ResolvedSignatures, sig)
		}
	}
	sort.Strings(diff.NewSignatures)
	sort.Strings(diff.ResolvedSignatures)
	sort.Strings(diff.Persistent)
	return diff
}

// BuildReproBundle wraps a CrashReport into an isolated reproduction package.
func BuildReproBundle(artifact, envTier, deviceProfile string, rep CrashReport, steps []string) CrashReproBundle {
	if envTier == "" {
		envTier = rep.EnvironmentTier
	}
	return CrashReproBundle{
		Artifact:        artifact,
		EnvironmentTier: envTier,
		Report:          rep,
		DeviceProfile:   deviceProfile,
		ReproSteps:      append([]string(nil), steps...),
	}
}
