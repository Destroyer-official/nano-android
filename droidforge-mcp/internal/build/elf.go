package build

import (
	"bufio"
	"debug/elf"
	"fmt"
	"io"
	"strconv"
	"strings"
)

const (
	// Target16KBAlignment is 16384 bytes (16 KB page size required for Android 15+ / 2027+).
	Target16KBAlignment uint64 = 16384
)

// ELFAlignmentReport contains the result of checking an ELF binary's page alignment.
type ELFAlignmentReport struct {
	Aligned         bool     `json:"aligned"`
	MinAlign        uint64   `json:"min_align"`
	SegmentsChecked int      `json:"segments_checked"`
	Violations      []string `json:"violations,omitempty"`
}

// CheckELF16KBAlignment inspects an ELF binary (typically an ARM64 .so) for 16 KB page alignment.
// According to Android 15+ requirements:
// 1. Every PT_LOAD segment's p_align must be >= 16384 (0x4000).
// 2. (p_vaddr % 16384) == (p_offset % 16384) for each PT_LOAD segment.
func CheckELF16KBAlignment(r io.ReaderAt) (*ELFAlignmentReport, error) {
	file, err := elf.NewFile(r)
	if err != nil {
		return nil, fmt.Errorf("failed to parse ELF: %w", err)
	}
	defer file.Close()

	report := &ELFAlignmentReport{
		Aligned:  true,
		MinAlign: ^uint64(0), // max uint64
	}

	loadCount := 0
	for i, prog := range file.Progs {
		if prog.Type != elf.PT_LOAD {
			continue
		}
		loadCount++

		align := prog.Align
		if align < report.MinAlign {
			report.MinAlign = align
		}

		if align < Target16KBAlignment {
			report.Aligned = false
			report.Violations = append(report.Violations,
				fmt.Sprintf("PT_LOAD segment %d alignment %d (0x%x) < 16384 (16 KB)", i, align, align))
		}

		// Virtual address and file offset congruence check
		if (prog.Vaddr % Target16KBAlignment) != (prog.Off % Target16KBAlignment) {
			report.Aligned = false
			report.Violations = append(report.Violations,
				fmt.Sprintf("PT_LOAD segment %d p_vaddr (0x%x) %% 16384 != p_offset (0x%x) %% 16384",
					i, prog.Vaddr, prog.Off))
		}
	}

	report.SegmentsChecked = loadCount
	if loadCount == 0 {
		report.MinAlign = 0
	}

	return report, nil
}

// ParseReadelfOutput parses standard `readelf -l` output to verify PT_LOAD segment alignment.
func ParseReadelfOutput(output string) (*ELFAlignmentReport, error) {
	report := &ELFAlignmentReport{
		Aligned:  true,
		MinAlign: ^uint64(0),
	}

	scanner := bufio.NewScanner(strings.NewReader(output))
	inHeaders := false
	loadCount := 0

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "Program Headers:") {
			inHeaders = true
			continue
		}
		if inHeaders && (strings.HasPrefix(line, "Section to Segment") || strings.HasPrefix(line, "Dynamic section")) {
			break
		}

		if inHeaders && strings.HasPrefix(line, "LOAD") {
			loadCount++
			fields := strings.Fields(line)
			// Typical readelf -l row:
			// LOAD 0x000000 0x000000 0x000000 0x001000 0x001000 R E 0x4000
			// Or split across 2 lines in 64-bit output:
			// LOAD 0x0000000000000000 0x0000000000000000 0x0000000000000000
			//      0x0000000000001000 0x0000000000001000  R E    0x4000
			var alignHex string
			if len(fields) >= 8 {
				alignHex = fields[len(fields)-1]
			} else if scanner.Scan() {
				// Continuation line
				contLine := strings.TrimSpace(scanner.Text())
				contFields := strings.Fields(contLine)
				if len(contFields) > 0 {
					alignHex = contFields[len(contFields)-1]
				}
			}

			if alignHex != "" {
				alignVal, err := strconv.ParseUint(strings.TrimPrefix(alignHex, "0x"), 16, 64)
				if err == nil {
					if alignVal < report.MinAlign {
						report.MinAlign = alignVal
					}
					if alignVal < Target16KBAlignment {
						report.Aligned = false
						report.Violations = append(report.Violations,
							fmt.Sprintf("PT_LOAD segment %d alignment %d (0x%x) < 16384 (16 KB)", loadCount, alignVal, alignVal))
					}
				}
			}
		}
	}

	report.SegmentsChecked = loadCount
	if loadCount == 0 {
		report.MinAlign = 0
	}

	return report, nil
}

// ParseZipalignOutput parses stdout from `zipalign -c -P 16 <apk>`.
func ParseZipalignOutput(output string, exitCode int) (bool, error) {
	if exitCode == 0 && (strings.Contains(output, "Verification succesful") || strings.Contains(output, "Verification successful") || strings.TrimSpace(output) == "") {
		return true, nil
	}
	return false, nil
}
