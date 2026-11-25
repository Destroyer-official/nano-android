package profiler_media

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// MediaVerificationRequest contains parameters for perceptual video & codec auditing.
type MediaVerificationRequest struct {
	ReferencePath    string  `json:"reference_path"`
	CompressedPath   string  `json:"compressed_path"`
	TargetCodec      string  `json:"target_codec"` // h264, h265, av1
	TargetBitrateBps int     `json:"target_bitrate_bps"`
	TolerancePercent float64 `json:"tolerance_percent"` // default 15.0%
	MinVMAF          float64 `json:"min_vmaf"`          // default 80.0
	MinSSIM          float64 `json:"min_ssim"`          // default 0.92
	MinPSNR          float64 `json:"min_psnr_db"`       // default 35.0 dB
	PythonPath       string  `json:"python_path,omitempty"`
}

// MediaVerificationResult contains perceptual scores and codec conformance metrics.
type MediaVerificationResult struct {
	ReferencePath           string  `json:"reference_path"`
	CompressedPath          string  `json:"compressed_path"`
	TargetCodec             string  `json:"target_codec"`
	DetectedCodec           string  `json:"detected_codec"`
	CodecValid              bool    `json:"codec_valid"`
	TargetBitrateBps        int     `json:"target_bitrate_bps"`
	ActualBitrateBps        int     `json:"actual_bitrate_bps"`
	BitrateDeltaPercent     float64 `json:"bitrate_delta_percent"`
	BitrateReductionPercent float64 `json:"bitrate_reduction_percent"`
	BitrateValid            bool    `json:"bitrate_valid"`
	VMAF                    float64 `json:"vmaf"`
	SSIM                    float64 `json:"ssim"`
	MSSSMI                  float64 `json:"ms_ssim,omitempty"`
	MinSSIM                 float64 `json:"min_ssim,omitempty"`
	PSNR                    float64 `json:"psnr_db"`
	MinPSNR                 float64 `json:"min_psnr_db,omitempty"`
	FrameCount              int     `json:"frame_count"`
	DurationSeconds         float64 `json:"duration_seconds"`
	Resolution              string  `json:"resolution"`
	PerceptualPass          bool    `json:"perceptual_pass"`
	OverallPass             bool    `json:"overall_pass"`
	Summary                 string  `json:"summary"`
}

// MediaBaseline defines regression thresholds from `.droidforge/baselines/`.
type MediaBaseline struct {
	MinVMAF                   float64 `json:"min_vmaf"`
	MinSSIM                   float64 `json:"min_ssim"`
	MinPSNR                   float64 `json:"min_psnr_db"`
	MinBitrateReductionPercent float64 `json:"min_bitrate_reduction_percent"`
	MaxBitrateVariancePercent float64 `json:"max_bitrate_variance_percent"`
}

// VerifyMedia invokes the Python media verification sidecar to benchmark video quality.
func VerifyMedia(ctx context.Context, req MediaVerificationRequest) (*MediaVerificationResult, error) {
	if req.ReferencePath == "" || req.CompressedPath == "" {
		return nil, fmt.Errorf("reference and compressed paths are mandatory")
	}

	if req.TolerancePercent <= 0 {
		req.TolerancePercent = 15.0
	}
	if req.MinVMAF <= 0 {
		req.MinVMAF = 80.0
	}
	if req.MinSSIM <= 0 {
		req.MinSSIM = 0.92
	}
	if req.MinPSNR <= 0 {
		req.MinPSNR = 35.0
	}
	if req.TargetCodec == "" {
		req.TargetCodec = "h264"
	}

	pyExe := req.PythonPath
	if pyExe == "" {
		pyExe = "python"
	}

	// Locate media_verifier.py sidecar script
	scriptPath := filepath.Join(filepath.Dir(req.ReferencePath), "media_verifier.py")
	if _, err := os.Stat(scriptPath); os.IsNotExist(err) {
		// Try local directory
		scriptPath = "media_verifier.py"
		if _, err := os.Stat(scriptPath); os.IsNotExist(err) {
			// Try directory of caller / module
			cwd, _ := os.Getwd()
			scriptPath = filepath.Join(cwd, "media_verifier.py")
		}
	}

	args := []string{
		scriptPath,
		"--ref", req.ReferencePath,
		"--comp", req.CompressedPath,
		"--codec", req.TargetCodec,
		"--target-bitrate", fmt.Sprintf("%d", req.TargetBitrateBps),
		"--tolerance", fmt.Sprintf("%.2f", req.TolerancePercent),
		"--min-vmaf", fmt.Sprintf("%.2f", req.MinVMAF),
		"--min-ssim", fmt.Sprintf("%.4f", req.MinSSIM),
		"--min-psnr", fmt.Sprintf("%.2f", req.MinPSNR),
		"--json",
	}

	cmd := exec.CommandContext(ctx, pyExe, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	// Parse JSON even if exit code is 1 (which indicates verification failure)
	outBytes := bytes.TrimSpace(stdout.Bytes())
	if len(outBytes) > 0 {
		var res MediaVerificationResult
		if jerr := json.Unmarshal(outBytes, &res); jerr == nil {
			return &res, nil
		}
	}

	if err != nil {
		return nil, fmt.Errorf("media_verifier.py failed: %w (stderr: %s)", err, stderr.String())
	}

	return nil, fmt.Errorf("no output received from media_verifier.py")
}

// EvaluatePerceptualMetricsNative evaluates VMAF approximation, SSIM, and PSNR in pure Go.
func EvaluatePerceptualMetricsNative(refY, compY []byte, width, height int) (psnr float64, ssim float64, vmaf float64) {
	if len(refY) != len(compY) || len(refY) == 0 || width <= 0 || height <= 0 {
		return 0, 0, 0
	}

	n := float64(len(refY))
	var sumSqDiff float64
	var sumRef, sumComp float64

	for i := 0; i < len(refY); i++ {
		r := float64(refY[i])
		c := float64(compY[i])
		diff := r - c
		sumSqDiff += diff * diff
		sumRef += r
		sumComp += c
	}

	mse := sumSqDiff / n
	if mse < 1e-10 {
		psnr = 100.0
	} else {
		psnr = 10.0 * math.Log10((255.0*255.0)/mse)
	}

	meanRef := sumRef / n
	meanComp := sumComp / n

	var varRef, varComp, covar float64
	for i := 0; i < len(refY); i++ {
		r := float64(refY[i])
		c := float64(compY[i])
		varRef += (r - meanRef) * (r - meanRef)
		varComp += (c - meanComp) * (c - meanComp)
		covar += (r - meanRef) * (c - meanComp)
	}
	varRef /= n
	varComp /= n
	covar /= n

	c1 := 6.5025   // (0.01 * 255)^2
	c2 := 58.5225  // (0.03 * 255)^2

	num := (2.0*meanRef*meanComp + c1) * (2.0*covar + c2)
	den := (meanRef*meanRef + meanComp*meanComp + c1) * (varRef + varComp + c2)
	if den > 1e-12 {
		ssim = num / den
	} else {
		ssim = 1.0
	}

	if ssim > 1.0 {
		ssim = 1.0
	} else if ssim < -1.0 {
		ssim = -1.0
	}

	// Calibrated perceptual VMAF approximation from SSIM and PSNR
	vmafRaw := (ssim * 70.0) + math.Min(30.0, (psnr/50.0)*30.0)
	vmaf = math.Max(0.0, math.Min(100.0, vmafRaw))

	return math.Round(psnr*100) / 100, math.Round(ssim*10000) / 10000, math.Round(vmaf*100) / 100
}

// CheckBaselineCompliance compares media verification results against `.droidforge/baselines/`.
func CheckBaselineCompliance(res *MediaVerificationResult, baselinePath string) (bool, []string, error) {
	data, err := os.ReadFile(baselinePath)
	if err != nil {
		return false, nil, fmt.Errorf("failed to read baseline file: %w", err)
	}

	var base MediaBaseline
	if err := json.Unmarshal(data, &base); err != nil {
		return false, nil, fmt.Errorf("failed to parse baseline JSON: %w", err)
	}

	var violations []string

	if res.VMAF < base.MinVMAF {
		violations = append(violations, fmt.Sprintf(
			"VMAF regression: got %.2f, required minimum %.2f",
			res.VMAF, base.MinVMAF,
		))
	}

	if res.SSIM < base.MinSSIM {
		violations = append(violations, fmt.Sprintf(
			"SSIM regression: got %.4f, required minimum %.4f",
			res.SSIM, base.MinSSIM,
		))
	}

	if res.PSNR < base.MinPSNR {
		violations = append(violations, fmt.Sprintf(
			"PSNR regression: got %.2f dB, required minimum %.2f dB",
			res.PSNR, base.MinPSNR,
		))
	}

	if res.BitrateReductionPercent < base.MinBitrateReductionPercent {
		violations = append(violations, fmt.Sprintf(
			"Bitrate reduction regression: achieved %.2f%%, required minimum %.2f%%",
			res.BitrateReductionPercent, base.MinBitrateReductionPercent,
		))
	}

	return len(violations) == 0, violations, nil
}

// ValidateCodecParameters checks codec conformance and bitrate reduction criteria.
func ValidateCodecParameters(
	targetCodec, detectedCodec string,
	targetBitrateBps, actualBitrateBps, refBitrateBps int,
	tolerancePercent float64,
) (codecValid bool, bitrateValid bool, reductionPercent float64) {
	normTarget := strings.ToLower(strings.TrimSpace(targetCodec))
	normDetected := strings.ToLower(strings.TrimSpace(detectedCodec))

	switch normTarget {
	case "h264", "avc", "avc1":
		codecValid = (normDetected == "h264" || normDetected == "avc1" || normDetected == "avc")
	case "h265", "hevc", "hvc1", "hev1":
		codecValid = (normDetected == "h265" || normDetected == "hevc" || normDetected == "hvc1" || normDetected == "hev1")
	case "av1", "av01":
		codecValid = (normDetected == "av1" || normDetected == "av01")
	default:
		codecValid = (normTarget == normDetected)
	}

	if targetBitrateBps > 0 && actualBitrateBps > 0 {
		delta := math.Abs(float64(actualBitrateBps-targetBitrateBps)) / float64(targetBitrateBps) * 100.0
		bitrateValid = delta <= tolerancePercent
	} else {
		bitrateValid = true
	}

	if refBitrateBps > 0 && actualBitrateBps > 0 {
		reductionPercent = (1.0 - (float64(actualBitrateBps) / float64(refBitrateBps))) * 100.0
	}

	return codecValid, bitrateValid, math.Round(reductionPercent*100) / 100
}
