package harness

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/anthropic/droidforge-mcp/internal/types"
)

// FixtureEntry holds metadata for one cached fixture in fixtures.lock.json.
type FixtureEntry struct {
	LocalPath          string  `json:"local_path"`
	SHA256             string  `json:"sha256"`
	GoldenOutputSHA256 string  `json:"golden_output_sha256,omitempty"`
	GoldenOutputPath   string  `json:"golden_output_path,omitempty"`
	MinVMAF            float64 `json:"min_vmaf,omitempty"`
	MinSSIM            float64 `json:"min_ssim,omitempty"`
}

// FixturesManifest maps fixture IDs to their local fixture descriptors.
type FixturesManifest struct {
	Fixtures map[string]FixtureEntry `json:"fixtures"`
}

// MediaPushVerifyRequest encapsulates all parameters for fixture push & output verification.
type MediaPushVerifyRequest struct {
	Serial           string  `json:"serial"`
	FixtureID        string  `json:"fixture_id,omitempty"`
	LocalFixturePath string  `json:"local_fixture_path,omitempty"`
	DeviceDestPath   string  `json:"device_dest_path,omitempty"`
	OutputDevicePath string  `json:"output_device_path,omitempty"`
	ManifestPath     string  `json:"manifest_path,omitempty"`
	Oracle           string  `json:"oracle,omitempty"` // "sha256" | "quality_bounded" | "vmaf_ssim"
	GoldenSHA256     string  `json:"golden_sha256,omitempty"`
	MinVMAF          float64 `json:"min_vmaf,omitempty"`
	MinSSIM          float64 `json:"min_ssim,omitempty"`
}

// LoadFixturesManifest loads a fixtures.lock.json file.
func LoadFixturesManifest(path string) (*FixturesManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read fixture manifest: %w", err)
	}
	var manifest FixturesManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("failed to parse fixture manifest JSON: %w", err)
	}
	return &manifest, nil
}

// ComputeLocalSHA256 computes the SHA256 hex digest of a local file.
func ComputeLocalSHA256(filePath string) (string, error) {
	f, err := os.Open(filePath)
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

// ParseDeviceSHA256 parses the SHA-256 hash from the output of `adb shell sha256sum <path>`.
func ParseDeviceSHA256(output string) (string, error) {
	fields := strings.Fields(output)
	if len(fields) == 0 {
		return "", fmt.Errorf("empty sha256sum output")
	}
	candidate := strings.ToLower(fields[0])
	// Valid SHA256 is 64 hex characters
	if len(candidate) == 64 {
		return candidate, nil
	}
	return "", fmt.Errorf("invalid sha256 output from device: %q", output)
}

// PushAndVerifyMedia pushes a fixture (with sha256 caching) and verifies output integrity/quality.
func PushAndVerifyMedia(
	ctx context.Context,
	dm types.DeviceManager,
	sidecar SidecarRunner,
	req MediaPushVerifyRequest,
) (*types.MediaVerifyResult, error) {
	if dm == nil {
		return nil, fmt.Errorf("device manager is required")
	}
	if req.Serial == "" {
		return nil, fmt.Errorf("device serial is required")
	}

	localPath := req.LocalFixturePath
	expectedLocalHash := ""
	var fixtureEntry *FixtureEntry

	// 1. Resolve fixture from manifest if fixture_id is provided
	if req.FixtureID != "" {
		manifestPath := req.ManifestPath
		if manifestPath == "" {
			candidates := []string{
				".droidforge/fixtures.lock.json",
				"fixtures.lock.json",
				filepath.Join("droidforge-mcp", "fixtures.lock.json"),
			}
			for _, c := range candidates {
				if _, err := os.Stat(c); err == nil {
					manifestPath = c
					break
				}
			}
		}

		if manifestPath != "" {
			manifest, err := LoadFixturesManifest(manifestPath)
			if err == nil {
				if entry, ok := manifest.Fixtures[req.FixtureID]; ok {
					fixtureEntry = &entry
					if localPath == "" {
						localPath = entry.LocalPath
					}
					expectedLocalHash = entry.SHA256
					if req.GoldenSHA256 == "" {
						req.GoldenSHA256 = entry.GoldenOutputSHA256
					}
					if req.MinVMAF <= 0 && entry.MinVMAF > 0 {
						req.MinVMAF = entry.MinVMAF
					}
					if req.MinSSIM <= 0 && entry.MinSSIM > 0 {
						req.MinSSIM = entry.MinSSIM
					}
				}
			}
		}
	}

	pushedCached := false

	// 2. Push Fixture if localPath provided
	if localPath != "" {
		if expectedLocalHash == "" {
			var err error
			expectedLocalHash, err = ComputeLocalSHA256(localPath)
			if err != nil {
				return nil, fmt.Errorf("failed to compute local fixture hash: %w", err)
			}
		}

		destPath := req.DeviceDestPath
		if destPath == "" {
			destPath = "/sdcard/Download/" + filepath.Base(localPath)
		}

		// Check on-device hash first
		checkOut, err := dm.ExecADB(req.Serial, "shell", "sha256sum", destPath)
		if err == nil {
			onDeviceHash, parseErr := ParseDeviceSHA256(checkOut)
			if parseErr == nil && strings.EqualFold(onDeviceHash, expectedLocalHash) {
				pushedCached = true
			}
		}

		// Push only if not cached
		if !pushedCached {
			_, err := dm.ExecADB(req.Serial, "push", localPath, destPath)
			if err != nil {
				return nil, fmt.Errorf("failed to push fixture to %s: %w", destPath, err)
			}
		}
	}

	result := &types.MediaVerifyResult{
		PushedCached: pushedCached,
	}

	// 3. Verify Output if output_device_path is provided
	if req.OutputDevicePath != "" {
		oracle := strings.ToLower(strings.TrimSpace(req.Oracle))
		if oracle == "" {
			oracle = "sha256"
		}

		// Bit-exact mode (sha256 on device)
		if oracle == "sha256" || oracle == "bit_exact" {
			out, err := dm.ExecADB(req.Serial, "shell", "sha256sum", req.OutputDevicePath)
			if err != nil {
				return nil, fmt.Errorf("failed to compute output sha256 on device: %w (%s)", err, out)
			}
			deviceHash, err := ParseDeviceSHA256(out)
			if err != nil {
				return nil, fmt.Errorf("failed to parse output sha256: %w", err)
			}
			result.OutputSHA256 = deviceHash

			if req.GoldenSHA256 != "" {
				result.HashMatch = strings.EqualFold(deviceHash, req.GoldenSHA256)
			} else if expectedLocalHash != "" {
				result.HashMatch = strings.EqualFold(deviceHash, expectedLocalHash)
			} else {
				result.HashMatch = true
			}
			return result, nil
		}

		// Quality-bounded mode (VMAF / SSIM via sidecar)
		if oracle == "quality_bounded" || oracle == "vmaf_ssim" {
			if sidecar == nil {
				sidecar = NewPythonSidecarRunner("")
			}

			// Pull output file from device to temp directory
			tmpFile, err := os.CreateTemp("", "df_pulled_output_*"+filepath.Ext(req.OutputDevicePath))
			if err != nil {
				return nil, fmt.Errorf("failed to create temp file for pulled output: %w", err)
			}
			tmpLocalOutput := tmpFile.Name()
			tmpFile.Close()
			defer os.Remove(tmpLocalOutput)

			_, err = dm.ExecADB(req.Serial, "pull", req.OutputDevicePath, tmpLocalOutput)
			if err != nil {
				return nil, fmt.Errorf("failed to pull output from device %s: %w", req.OutputDevicePath, err)
			}

			// Reference path for comparison
			refPath := localPath
			if fixtureEntry != nil && fixtureEntry.GoldenOutputPath != "" {
				refPath = fixtureEntry.GoldenOutputPath
			}

			minVMAF := req.MinVMAF
			if minVMAF <= 0 {
				minVMAF = 90.0
			}
			minSSIM := req.MinSSIM
			if minSSIM <= 0 {
				minSSIM = 0.95
			}

			sidecarRes, err := sidecar.VerifyMedia(ctx, refPath, tmpLocalOutput, "quality_bounded", req.GoldenSHA256, "", minVMAF, minSSIM)
			if err != nil {
				return nil, fmt.Errorf("sidecar media verification failed: %w", err)
			}

			result.OutputSHA256 = sidecarRes.OutputSHA256
			result.HashMatch = sidecarRes.HashMatch
			result.FFProbeMeta = sidecarRes.FFProbeMeta
			result.VMAF = sidecarRes.VMAF
			result.SSIM = sidecarRes.SSIM
			result.DecodeErrors = sidecarRes.DecodeErrors
			return result, nil
		}

		return nil, fmt.Errorf("unsupported oracle mode %q (must be sha256 or quality_bounded)", oracle)
	}

	return result, nil
}
