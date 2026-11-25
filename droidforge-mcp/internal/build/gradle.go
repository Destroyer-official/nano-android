package build

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/anthropic/droidforge-mcp/internal/types"
)

// Regex patterns for Gradle output and compiler errors
var (
	reBuildSuccessful = regexp.MustCompile(`(?i)BUILD SUCCESSFUL in (?:(\d+)m\s*)?(\d+(?:\.\d+)?)(s|ms)`)
	reBuildFailed     = regexp.MustCompile(`(?i)BUILD FAILED`)
	reAPKOutput       = regexp.MustCompile(`(?i)(?:APK:\s*|Built APK:\s*|^|> Task [^\n]*\n)?([^\s\r\n]+\.apk)`)

	// Kotlin compiler error patterns:
	// e: /path/to/File.kt: (15, 23): Unresolved reference: foo
	// e: /path/to/File.kt:15:23: Unresolved reference: foo
	// e: file:///path/to/File.kt:15:23 Unresolved reference: foo
	reKotlinError = regexp.MustCompile(`(?m)^e:\s*(?:file:///)?([^:\r\n]+):(?:\s*\(|:)?(\d+)(?:[,\:]\s*\d+\)?)?:?\s*(.*)$`)

	// Java compiler error patterns:
	// /path/to/File.java:15: error: cannot find symbol
	reJavaError = regexp.MustCompile(`(?m)^([^:\r\n]+):(\d+):\s*error:\s*(.*)$`)

	// Roborazzi diff image pattern in test logs or failure output
	reRoborazziDiff = regexp.MustCompile(`(?i)([^\s"']+_compare\.png|[^\s"']*roborazzi[^\s"']*\.png)`)
)

// Capitalize returns the string with the first letter capitalized.
func capitalize(s string) string {
	if s == "" {
		return ""
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// resolveGradleWrapper finds the gradle wrapper executable for the host platform.
func resolveGradleWrapper(gradleRoot string) string {
	if runtime.GOOS == "windows" {
		bat := filepath.Join(gradleRoot, "gradlew.bat")
		if _, err := os.Stat(bat); err == nil {
			return bat
		}
		cmd := filepath.Join(gradleRoot, "gradlew.cmd")
		if _, err := os.Stat(cmd); err == nil {
			return cmd
		}
		// Fallback to gradlew.bat
		return "gradlew.bat"
	}

	sh := filepath.Join(gradleRoot, "gradlew")
	if _, err := os.Stat(sh); err == nil {
		return sh
	}
	return "./gradlew"
}

// ParseGradleBuildOutput parses Gradle build output for duration, status, APK path, and compiler errors.
func ParseGradleBuildOutput(output string, exitCode int, defaultDurationMS int64) *types.BuildResult {
	res := &types.BuildResult{
		Status:     "success",
		DurationMS: defaultDurationMS,
		Errors:     []types.CompileError{},
	}

	if exitCode != 0 || reBuildFailed.MatchString(output) {
		res.Status = "failure"
	}

	// Parse duration if available in output
	if matches := reBuildSuccessful.FindStringSubmatch(output); len(matches) > 2 {
		var totalMS int64
		if matches[1] != "" {
			if mins, err := strconv.ParseInt(matches[1], 10, 64); err == nil {
				totalMS += mins * 60 * 1000
			}
		}
		unit := matches[3]
		val, _ := strconv.ParseFloat(matches[2], 64)
		if unit == "ms" {
			totalMS += int64(val)
		} else {
			totalMS += int64(val * 1000)
		}
		if totalMS > 0 {
			res.DurationMS = totalMS
		}
	}

	// Extract compiler errors
	seenErrors := make(map[string]bool)

	// Kotlin errors
	for _, m := range reKotlinError.FindAllStringSubmatch(output, -1) {
		file := strings.TrimSpace(m[1])
		line, _ := strconv.Atoi(m[2])
		msg := strings.TrimSpace(m[3])
		key := fmt.Sprintf("%s:%d:%s", file, line, msg)
		if !seenErrors[key] {
			seenErrors[key] = true
			res.Errors = append(res.Errors, types.CompileError{
				File:    file,
				Line:    line,
				Message: msg,
			})
		}
	}

	// Java errors
	for _, m := range reJavaError.FindAllStringSubmatch(output, -1) {
		file := strings.TrimSpace(m[1])
		line, _ := strconv.Atoi(m[2])
		msg := strings.TrimSpace(m[3])
		key := fmt.Sprintf("%s:%d:%s", file, line, msg)
		if !seenErrors[key] {
			seenErrors[key] = true
			res.Errors = append(res.Errors, types.CompileError{
				File:    file,
				Line:    line,
				Message: msg,
			})
		}
	}

	// Extract APK path from output if printed
	lines := strings.Split(output, "\n")
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if strings.HasSuffix(strings.ToLower(l), ".apk") {
			matches := reAPKOutput.FindStringSubmatch(l)
			if len(matches) > 1 {
				res.APKPath = matches[1]
			}
		}
	}

	return res
}

// computeSHA256 returns the hex-encoded SHA-256 of the given file.
func computeSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// findGeneratedAPK searches standard Gradle build output directories for an APK.
func findGeneratedAPK(gradleRoot, module, variant string) (string, error) {
	variantLower := strings.ToLower(variant)
	candidates := []string{
		filepath.Join(gradleRoot, module, "build", "outputs", "apk", variantLower),
		filepath.Join(gradleRoot, module, "build", "outputs", "apk"),
		filepath.Join(gradleRoot, "build", "outputs", "apk", variantLower),
		filepath.Join(gradleRoot, "build", "outputs", "apk"),
	}

	for _, dir := range candidates {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			continue
		}

		var newestAPK string
		var newestTime time.Time

		_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			if strings.HasSuffix(strings.ToLower(d.Name()), ".apk") {
				info, err := d.Info()
				if err == nil {
					if info.ModTime().After(newestTime) {
						newestTime = info.ModTime()
						newestAPK = p
					}
				}
			}
			return nil
		})

		if newestAPK != "" {
			return newestAPK, nil
		}
	}

	return "", fmt.Errorf("no APK found for module %q variant %q", module, variant)
}

// JUnit XML structures
type jUnitTestSuites struct {
	XMLName  xml.Name         `xml:"testsuites"`
	Suites   []jUnitTestSuite `xml:"testsuite"`
	Tests    int              `xml:"tests,attr"`
	Failures int              `xml:"failures,attr"`
	Errors   int              `xml:"errors,attr"`
	Skipped  int              `xml:"skipped,attr"`
	Time     float64          `xml:"time,attr"`
}

type jUnitTestSuite struct {
	XMLName   xml.Name        `xml:"testsuite"`
	Name      string          `xml:"name,attr"`
	Tests     int             `xml:"tests,attr"`
	Failures  int             `xml:"failures,attr"`
	Errors    int             `xml:"errors,attr"`
	Skipped   int             `xml:"skipped,attr"`
	Time      float64         `xml:"time,attr"`
	TestCases []jUnitTestCase `xml:"testcase"`
}

type jUnitTestCase struct {
	XMLName   xml.Name      `xml:"testcase"`
	Name      string        `xml:"name,attr"`
	ClassName string        `xml:"classname,attr"`
	Time      float64       `xml:"time,attr"`
	Failure   *jUnitProblem `xml:"failure"`
	Error     *jUnitProblem `xml:"error"`
	Skipped   *struct{}     `xml:"skipped"`
}

type jUnitProblem struct {
	Message string `xml:"message,attr"`
	Type    string `xml:"type,attr"`
	Value   string `xml:",chardata"`
}

// ParseJUnitXML parses all JUnit XML test result files in the specified directory or files.
func ParseJUnitXML(searchDir string) (*types.TestResult, error) {
	result := &types.TestResult{
		Failures: []types.TestFailure{},
	}

	if _, err := os.Stat(searchDir); os.IsNotExist(err) {
		return result, nil
	}

	var totalTimeSec float64

	err := filepath.WalkDir(searchDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() || !strings.HasSuffix(strings.ToLower(d.Name()), ".xml") {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		// Try unmarshaling as testsuites or testsuite
		var suites jUnitTestSuites
		if err := xml.Unmarshal(data, &suites); err == nil && (len(suites.Suites) > 0 || suites.Tests > 0) {
			for _, s := range suites.Suites {
				processSuite(&s, result, &totalTimeSec, searchDir)
			}
			return nil
		}

		var singleSuite jUnitTestSuite
		if err := xml.Unmarshal(data, &singleSuite); err == nil {
			processSuite(&singleSuite, result, &totalTimeSec, searchDir)
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("error walking test results dir: %w", err)
	}

	result.DurationMS = int64(totalTimeSec * 1000)
	return result, nil
}

func processSuite(suite *jUnitTestSuite, result *types.TestResult, totalTimeSec *float64, searchDir string) {
	*totalTimeSec += suite.Time

	for _, tc := range suite.TestCases {
		if tc.Skipped != nil {
			result.Skipped++
			continue
		}

		prob := tc.Failure
		if prob == nil {
			prob = tc.Error
		}

		if prob != nil {
			result.Failed++
			stack := strings.TrimSpace(prob.Value)
			if stack == "" && prob.Message != "" {
				stack = prob.Message
			}

			// Extract Roborazzi diff image if mentioned
			var diffImg string
			if match := reRoborazziDiff.FindString(stack); match != "" {
				diffImg = match
			} else {
				// Check if Roborazzi diff output exists for this test
				diffImg = findRoborazziDiffImage(searchDir, tc.ClassName, tc.Name)
			}

			result.Failures = append(result.Failures, types.TestFailure{
				Class:       tc.ClassName,
				Method:      tc.Name,
				StackTrace:  stack,
				DiffImgPath: diffImg,
			})
		} else {
			result.Passed++
		}
	}
}

// findRoborazziDiffImage looks for Roborazzi comparison diff images in build/outputs/roborazzi.
func findRoborazziDiffImage(searchDir, className, methodName string) string {
	roborazziDir := filepath.Join(searchDir, "..", "..", "outputs", "roborazzi")
	if _, err := os.Stat(roborazziDir); os.IsNotExist(err) {
		return ""
	}

	sanitizedClass := strings.ReplaceAll(className, ".", "_")
	var found string
	_ = filepath.WalkDir(roborazziDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		name := d.Name()
		if strings.HasSuffix(name, "_compare.png") {
			if strings.Contains(name, sanitizedClass) || (methodName != "" && strings.Contains(name, methodName)) {
				found = p
				return filepath.SkipDir
			}
		}
		return nil
	})
	return found
}

// executeBuild performs the Gradle build invocation.
func (b *GradleEngine) executeBuild(ctx context.Context, variant, module string, clean bool) (*types.BuildResult, error) {
	if module == "" {
		module = "app"
	}
	if variant == "" {
		variant = "debug"
	}

	args := []string{}
	if clean {
		args = append(args, "clean")
	}
	args = append(args, fmt.Sprintf(":%s:assemble%s", module, capitalize(variant)))
	args = append(args, "--configuration-cache", "--build-cache")

	gradleCmd := resolveGradleWrapper(b.gradleRoot)
	env := append(os.Environ(), fmt.Sprintf("JAVA_OPTS=-Xmx%dm", b.maxHeapMB))

	startTime := time.Now()
	outputBytes, cmdErr := b.executor.Run(ctx, b.gradleRoot, env, gradleCmd, args...)
	elapsedMS := time.Since(startTime).Milliseconds()

	outputStr := string(outputBytes)
	exitCode := 0
	if cmdErr != nil {
		exitCode = 1
	}

	buildResult := ParseGradleBuildOutput(outputStr, exitCode, elapsedMS)

	// If build succeeded, locate the APK and compute SHA-256
	if buildResult.Status == "success" {
		apkPath := buildResult.APKPath
		if apkPath == "" || !filepath.IsAbs(apkPath) {
			if diskAPK, err := findGeneratedAPK(b.gradleRoot, module, variant); err == nil {
				apkPath = diskAPK
			}
		}

		if apkPath != "" {
			buildResult.APKPath = apkPath
			if hash, err := computeSHA256(apkPath); err == nil {
				buildResult.APKSHA256 = hash
			}
		}
	}

	return buildResult, nil
}

// executeTestJVM runs Robolectric / JVM unit tests and Roborazzi golden compares.
func (b *GradleEngine) executeTestJVM(ctx context.Context, filter string, roborazzi, recordGoldens bool) (*types.TestResult, error) {
	args := []string{":app:testDebugUnitTest"}
	if filter != "" {
		args = append(args, "--tests", filter)
	}

	if recordGoldens {
		args = append(args, ":app:recordRoborazziDebug")
	} else if roborazzi {
		args = append(args, ":app:compareRoborazziDebug")
	}

	gradleCmd := resolveGradleWrapper(b.gradleRoot)
	env := append(os.Environ(), fmt.Sprintf("JAVA_OPTS=-Xmx%dm", b.maxHeapMB))

	startTime := time.Now()
	_, _ = b.executor.Run(ctx, b.gradleRoot, env, gradleCmd, args...)
	elapsedMS := time.Since(startTime).Milliseconds()

	// Locate test results directory
	resultsDir := filepath.Join(b.gradleRoot, "app", "build", "test-results")
	if _, err := os.Stat(resultsDir); os.IsNotExist(err) {
		resultsDir = filepath.Join(b.gradleRoot, "build", "test-results")
	}

	testResult, err := ParseJUnitXML(resultsDir)
	if err != nil {
		return nil, fmt.Errorf("failed to parse JUnit XML results: %w", err)
	}

	if testResult.DurationMS == 0 {
		testResult.DurationMS = elapsedMS
	}

	return testResult, nil
}
