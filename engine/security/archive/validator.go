package archive

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"

	"nano_android/engine/security/pathcontainment"
)

// Limits defines strict upper bounds for APK/JAR/AAB/APKS archive ingestion.
type Limits struct {
	MaxArchiveBytes           int64
	MaxEntries                int
	MaxEntryUncompressedBytes int64
	MaxTotalUncompressedBytes int64
	MaxCompressionRatio       float64
	AllowSymlinks             bool
	RejectJanusPolyglot       bool
}

// DefaultLimits returns P0 hardened archive limits.
func DefaultLimits() Limits {
	return Limits{
		MaxArchiveBytes:           512 << 20, // 512 MiB
		MaxEntries:                50000,
		MaxEntryUncompressedBytes: 64 << 20,  // 64 MiB
		MaxTotalUncompressedBytes: 1 << 30,   // 1 GiB
		MaxCompressionRatio:       100.0,
		AllowSymlinks:             false,
		RejectJanusPolyglot:       true,
	}
}

// AuditSummary describes verified metadata of an ingested archive.
type AuditSummary struct {
	ArchiveSizeBytes     int64  `json:"archive_size_bytes"`
	EntryCount           int    `json:"entry_count"`
	TotalUncompressed    int64  `json:"total_uncompressed_bytes"`
	DexCount             int    `json:"dex_count"`
	HasManifest          bool   `json:"has_manifest"`
	HasResourcesArsc     bool   `json:"has_resources_arsc"`
	NativeLibCount       int    `json:"native_lib_count"`
}

// ValidateArchive verifies archive headers, entry names, symlinks, duplicate entries,
// actual streaming decompression sizes (defeating forged ZIP header bombs), and polyglots.
func ValidateArchive(path string, limits Limits) (*AuditSummary, error) {
	if limits.MaxArchiveBytes <= 0 {
		limits = DefaultLimits()
	}

	st, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("stat archive: %w", err)
	}
	if st.IsDir() {
		return nil, fmt.Errorf("archive path is a directory")
	}
	if st.Size() == 0 {
		return nil, fmt.Errorf("archive is empty")
	}
	if st.Size() > limits.MaxArchiveBytes {
		return nil, fmt.Errorf("archive size %d exceeds max limit %d", st.Size(), limits.MaxArchiveBytes)
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open archive: %w", err)
	}
	defer f.Close()

	// Reject Janus APK+DEX polyglots and malformed headers by verifying ZIP magic at offset 0.
	if limits.RejectJanusPolyglot {
		var magic [4]byte
		if _, err := io.ReadFull(f, magic[:]); err != nil {
			return nil, fmt.Errorf("read archive header: %w", err)
		}
		if !bytes.Equal(magic[:], []byte{'P', 'K', 0x03, 0x04}) && !bytes.Equal(magic[:], []byte{'P', 'K', 0x05, 0x06}) {
			return nil, fmt.Errorf("invalid ZIP/APK magic header (possible malformed or Janus polyglot file)")
		}
	}

	zr, err := zip.NewReader(f, st.Size())
	if err != nil {
		return nil, fmt.Errorf("corrupt ZIP central directory: %w", err)
	}
	if len(zr.File) > limits.MaxEntries {
		return nil, fmt.Errorf("entry count %d exceeds max %d", len(zr.File), limits.MaxEntries)
	}

	summary := &AuditSummary{
		ArchiveSizeBytes: st.Size(),
		EntryCount:       len(zr.File),
	}

	seenExact := make(map[string]bool, len(zr.File))
	seenLower := make(map[string]bool, len(zr.File))
	var cumulative int64

	for _, entry := range zr.File {
		name := entry.Name
		if err := validateEntryName(name); err != nil {
			return nil, fmt.Errorf("unsafe archive entry %q: %w", name, err)
		}

		if seenExact[name] {
			return nil, fmt.Errorf("duplicate archive entry %q", name)
		}
		low := strings.ToLower(name)
		if seenLower[low] && !strings.HasSuffix(name, "/") {
			return nil, fmt.Errorf("case-colliding duplicate archive entry %q", name)
		}
		seenExact[name] = true
		seenLower[low] = true

		if !limits.AllowSymlinks && isSymlinkEntry(entry) {
			return nil, fmt.Errorf("symlink entry %q is forbidden in archive", name)
		}

		if entry.UncompressedSize64 > uint64(limits.MaxEntryUncompressedBytes) {
			return nil, fmt.Errorf("entry %q declared size %d exceeds max %d", name, entry.UncompressedSize64, limits.MaxEntryUncompressedBytes)
		}

		// Actually stream-decompress with a strict cap so forged headers cannot mask a deflate bomb.
		rc, err := entry.Open()
		if err != nil {
			return nil, fmt.Errorf("open entry %q: %w", name, err)
		}
		remainingBudget := limits.MaxTotalUncompressedBytes - cumulative
		if remainingBudget < 0 {
			remainingBudget = 0
		}
		entryCap := limits.MaxEntryUncompressedBytes
		if remainingBudget < entryCap {
			entryCap = remainingBudget
		}
		n, copyErr := io.Copy(io.Discard, io.LimitReader(rc, entryCap+1))
		_ = rc.Close()
		if copyErr != nil {
			return nil, fmt.Errorf("corrupt compressed stream in %q: %w", name, copyErr)
		}
		if n > limits.MaxEntryUncompressedBytes {
			return nil, fmt.Errorf("entry %q actual decompressed size exceeds limit %d (ZIP bomb blocked)", name, limits.MaxEntryUncompressedBytes)
		}
		cumulative += n
		if cumulative > limits.MaxTotalUncompressedBytes {
			return nil, fmt.Errorf("total decompressed size exceeds limit %d (ZIP bomb blocked)", limits.MaxTotalUncompressedBytes)
		}

		if n > (1<<20) && entry.CompressedSize64 > 0 {
			ratio := float64(n) / float64(entry.CompressedSize64)
			if ratio > limits.MaxCompressionRatio {
				return nil, fmt.Errorf("entry %q compression ratio %.1f:1 exceeds limit %.1f:1", name, ratio, limits.MaxCompressionRatio)
			}
		}

		if name == "AndroidManifest.xml" {
			summary.HasManifest = true
		} else if name == "resources.arsc" {
			summary.HasResourcesArsc = true
		} else if strings.HasPrefix(name, "classes") && strings.HasSuffix(name, ".dex") {
			summary.DexCount++
		} else if strings.HasSuffix(low, ".so") {
			summary.NativeLibCount++
		}
	}

	summary.TotalUncompressed = cumulative
	return summary, nil
}

func validateEntryName(name string) error {
	if name == "" {
		return fmt.Errorf("empty entry name")
	}
	if strings.HasPrefix(name, "/") || strings.HasPrefix(name, `\`) {
		return fmt.Errorf("absolute path entry")
	}
	if strings.Contains(name, `\`) {
		return fmt.Errorf("backslash in ZIP entry")
	}
	return pathcontainment.CheckPathSyntax(name)
}

func isSymlinkEntry(entry *zip.File) bool {
	if (entry.Mode() & os.ModeSymlink) != 0 {
		return true
	}
	// Check POSIX symlink bits (0120000) in upper 16 bits of ExternalAttrs (creator OS Unix = 3).
	creatorOS := entry.CreatorVersion >> 8
	if creatorOS == 3 {
		unixMode := entry.ExternalAttrs >> 16
		if (unixMode & 0170000) == 0120000 {
			return true
		}
	}
	return false
}
