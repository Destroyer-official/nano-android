package pathcontainment

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var windowsReservedNames = map[string]bool{
	"CON": true, "PRN": true, "AUX": true, "NUL": true,
	"COM1": true, "COM2": true, "COM3": true, "COM4": true,
	"COM5": true, "COM6": true, "COM7": true, "COM8": true, "COM9": true,
	"LPT1": true, "LPT2": true, "LPT3": true, "LPT4": true,
	"LPT5": true, "LPT6": true, "LPT7": true, "LPT8": true, "LPT9": true,
}

// CheckPathSyntax rejects null bytes, UNC paths, traversal segments, NTFS alternate
// data streams, and Windows reserved device filenames.
func CheckPathSyntax(candidate string) error {
	trimmed := strings.TrimSpace(candidate)
	if trimmed == "" {
		return fmt.Errorf("empty path")
	}
	if strings.ContainsRune(trimmed, '\x00') {
		return fmt.Errorf("path contains null byte")
	}
	if strings.HasPrefix(trimmed, `\\`) || strings.HasPrefix(trimmed, "//") {
		return fmt.Errorf("UNC/network paths are forbidden")
	}
	// Reject NTFS alternate data streams (colon outside of leading Windows drive prefix C:\).
	colonCheck := trimmed
	if len(colonCheck) >= 2 && ((colonCheck[0] >= 'a' && colonCheck[0] <= 'z') || (colonCheck[0] >= 'A' && colonCheck[0] <= 'Z')) && colonCheck[1] == ':' {
		colonCheck = colonCheck[2:]
	}
	if strings.Contains(colonCheck, ":") {
		return fmt.Errorf("alternate data streams or extra colons are forbidden")
	}
	normalized := strings.ReplaceAll(trimmed, `\`, `/`)
	for _, seg := range strings.Split(normalized, "/") {
		if seg == ".." {
			return fmt.Errorf("path traversal segment '..' is forbidden")
		}
		upperBase := strings.ToUpper(strings.TrimSuffix(seg, filepath.Ext(seg)))
		if windowsReservedNames[upperBase] {
			return fmt.Errorf("reserved device name %q is forbidden", seg)
		}
	}
	return nil
}

// ResolveExistingContained resolves candidate and verifies it resides strictly inside
// at least one of allowedRoots after symlink resolution.
func ResolveExistingContained(candidate string, allowedRoots []string, allowedExts []string) (string, error) {
	if err := CheckPathSyntax(candidate); err != nil {
		return "", err
	}
	if len(allowedRoots) == 0 {
		return "", fmt.Errorf("no allowed roots configured")
	}

	var absCand string
	if filepath.IsAbs(candidate) {
		absCand = filepath.Clean(candidate)
	} else {
		var err error
		absCand, err = filepath.Abs(candidate)
		if err != nil {
			return "", fmt.Errorf("resolve absolute path: %w", err)
		}
	}

	realCand, err := filepath.EvalSymlinks(absCand)
	if err != nil {
		return "", fmt.Errorf("path cannot be resolved safely: %w", err)
	}
	st, err := os.Stat(realCand)
	if err != nil || st.IsDir() {
		return "", fmt.Errorf("target file does not exist or is a directory")
	}

	if len(allowedExts) > 0 && !hasAllowedExt(realCand, allowedExts) {
		return "", fmt.Errorf("file extension %q is not permitted", filepath.Ext(realCand))
	}

	if !isContainedInAnyRoot(realCand, allowedRoots) {
		return "", fmt.Errorf("access denied: %q escapes allowed workspace directories", filepath.Base(realCand))
	}
	return realCand, nil
}

// ResolveExistingDirContained resolves candidate directory and verifies it resides strictly inside allowedRoots.
func ResolveExistingDirContained(candidate string, allowedRoots []string) (string, error) {
	if err := CheckPathSyntax(candidate); err != nil {
		return "", err
	}
	if len(allowedRoots) == 0 {
		return "", fmt.Errorf("no allowed roots configured")
	}
	absCand, err := filepath.Abs(candidate)
	if err != nil {
		return "", fmt.Errorf("resolve absolute dir: %w", err)
	}
	realCand, err := filepath.EvalSymlinks(absCand)
	if err != nil {
		return "", fmt.Errorf("dir cannot be resolved safely: %w", err)
	}
	st, err := os.Stat(realCand)
	if err != nil || !st.IsDir() {
		return "", fmt.Errorf("target path does not exist or is not a directory")
	}
	if !isContainedInAnyRoot(realCand, allowedRoots) {
		return "", fmt.Errorf("access denied: %q escapes allowed workspace directories", filepath.Base(realCand))
	}
	return realCand, nil
}

// ResolveWriteContained resolves a destination path inside rootDir, verifying the parent
// directory chain has no symlinks escaping rootDir and the filename is safe.
func ResolveWriteContained(rootDir, filename string, allowedExts []string) (string, error) {
	if err := CheckPathSyntax(filename); err != nil {
		return "", err
	}
	cleanBase := filepath.Base(strings.ReplaceAll(strings.TrimSpace(filename), `\`, `/`))
	if cleanBase == "" || cleanBase == "." || cleanBase == ".." {
		return "", fmt.Errorf("invalid destination filename")
	}
	if cleanBase != strings.ReplaceAll(strings.TrimSpace(filename), `\`, `/`) && filepath.IsAbs(filename) {
		return "", fmt.Errorf("absolute destination paths are forbidden")
	}
	if len(allowedExts) > 0 && !hasAllowedExt(cleanBase, allowedExts) {
		return "", fmt.Errorf("file extension %q is not permitted", filepath.Ext(cleanBase))
	}

	absRoot, err := filepath.Abs(rootDir)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(absRoot, 0750); err != nil {
		return "", err
	}
	realRoot, err := filepath.EvalSymlinks(absRoot)
	if err != nil {
		return "", err
	}

	target := filepath.Join(realRoot, cleanBase)
	// If target exists and is a symlink, reject symlink overwrite attacks.
	if info, lErr := os.Lstat(target); lErr == nil && (info.Mode()&os.ModeSymlink) != 0 {
		return "", fmt.Errorf("refusing to write through symlink %q", cleanBase)
	}
	if !isWithinRoot(realRoot, target) {
		return "", fmt.Errorf("destination escapes root directory")
	}
	return target, nil
}

// SanitizeFilename strips unsafe characters from a user-supplied basename while preserving extension.
func SanitizeFilename(raw string) (string, error) {
	base := filepath.Base(strings.ReplaceAll(strings.TrimSpace(raw), `\`, `/`))
	if err := CheckPathSyntax(base); err != nil {
		return "", err
	}
	safe := strings.Map(func(c rune) rune {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '.' || c == '-' || c == '_' {
			return c
		}
		return '_'
	}, base)
	if safe == "" || safe == "." || safe == ".." {
		return "", fmt.Errorf("invalid filename")
	}
	upperBase := strings.ToUpper(strings.TrimSuffix(safe, filepath.Ext(safe)))
	if windowsReservedNames[upperBase] {
		return "", fmt.Errorf("reserved filename %q", safe)
	}
	return safe, nil
}

func hasAllowedExt(path string, exts []string) bool {
	low := strings.ToLower(filepath.Ext(path))
	for _, ext := range exts {
		if low == strings.ToLower(ext) {
			return true
		}
	}
	return false
}

func isContainedInAnyRoot(realTarget string, roots []string) bool {
	for _, r := range roots {
		if strings.TrimSpace(r) == "" {
			continue
		}
		absRoot, err := filepath.Abs(r)
		if err != nil {
			continue
		}
		realRoot, err := filepath.EvalSymlinks(absRoot)
		if err != nil {
			continue
		}
		if isWithinRoot(realRoot, realTarget) {
			return true
		}
	}
	return false
}

func isWithinRoot(realRoot, realTarget string) bool {
	rel, err := filepath.Rel(realRoot, realTarget)
	if err != nil {
		return false
	}
	if rel == "." {
		return true
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return false
	}
	return true
}
