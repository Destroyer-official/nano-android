package oracle

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"

	"github.com/anthropic/droidforge-mcp/engine/profiler_media"
)

const (
	OracleModeBitExactSHA256 = "BIT_EXACT_SHA256"
	OracleModeLossyPerceptual = "PERCEPTUAL_LOSSY"
)

// LosslessResult verifies strict SHA-256 bit-exact equivalence for passthrough/ContentResolver media streams.
type LosslessResult struct {
	OracleMode     string `json:"oracle_mode"`
	Passed         bool   `json:"passed"`
	ExpectedSHA256 string `json:"expected_sha256"`
	ActualSHA256   string `json:"actual_sha256"`
	BytesChecked   int64  `json:"bytes_checked"`
	Reason         string `json:"reason"`
}

// FFprobeSummary captures real ffprobe container/stream metadata when ffprobe is installed on the host.
type FFprobeSummary struct {
	Available       bool    `json:"available"`
	FormatName      string  `json:"format_name,omitempty"`
	DurationSeconds float64 `json:"duration_seconds,omitempty"`
	BitrateBps      int64   `json:"bitrate_bps,omitempty"`
	VideoCodec      string  `json:"video_codec,omitempty"`
	AudioCodec      string  `json:"audio_codec,omitempty"`
	Width           int     `json:"width,omitempty"`
	Height          int     `json:"height,omitempty"`
}

// VerifyFilesBitExact checks strict SHA-256 equality between two files (zero tolerance for bit changes).
func VerifyFilesBitExact(refPath, actualPath string) (*LosslessResult, error) {
	refHash, refBytes, err := fileSHA256(refPath)
	if err != nil {
		return nil, err
	}
	actHash, actBytes, err := fileSHA256(actualPath)
	if err != nil {
		return nil, err
	}
	ok := refHash == actHash && refBytes == actBytes
	reason := "bit-exact SHA-256 match"
	if !ok {
		reason = fmt.Sprintf("bit-exact mismatch: ref=%s (%d B) != actual=%s (%d B)", refHash, refBytes, actHash, actBytes)
	}
	return &LosslessResult{
		OracleMode:     OracleModeBitExactSHA256,
		Passed:         ok,
		ExpectedSHA256: refHash,
		ActualSHA256:   actHash,
		BytesChecked:   actBytes,
		Reason:         reason,
	}, nil
}

// VerifyStreamBitExact checks an in-memory stream buffer against an expected lowercase hex SHA-256 digest.
func VerifyStreamBitExact(data []byte, expectedSHA256 string) LosslessResult {
	sum := sha256.Sum256(data)
	actual := hex.EncodeToString(sum[:])
	ok := actual == expectedSHA256
	reason := "bit-exact stream SHA-256 match"
	if !ok {
		reason = fmt.Sprintf("stream SHA-256 mismatch: expected=%s actual=%s", expectedSHA256, actual)
	}
	return LosslessResult{
		OracleMode:     OracleModeBitExactSHA256,
		Passed:         ok,
		ExpectedSHA256: expectedSHA256,
		ActualSHA256:   actual,
		BytesChecked:   int64(len(data)),
		Reason:         reason,
	}
}

const maxContentStreamVerifyBytes int64 = 512 * 1024 * 1024

// VerifyContentStreamIntegrity hashes a bounded ContentResolver/URI stream and checks bit-exact SHA-256 equality.
func VerifyContentStreamIntegrity(r io.Reader, expectedSHA256 string) (*LosslessResult, error) {
	if r == nil {
		return nil, fmt.Errorf("stream reader is required")
	}
	h := sha256.New()
	limited := io.LimitReader(r, maxContentStreamVerifyBytes+1)
	n, err := io.Copy(h, limited)
	if err != nil {
		return nil, fmt.Errorf("read content stream: %w", err)
	}
	if n > maxContentStreamVerifyBytes {
		return nil, fmt.Errorf("content stream exceeded max verification cap (%d bytes)", maxContentStreamVerifyBytes)
	}
	actual := hex.EncodeToString(h.Sum(nil))
	ok := actual == expectedSHA256
	reason := "ContentResolver/URI stream bit-exact match"
	if !ok {
		reason = fmt.Sprintf("ContentResolver stream mismatch: expected=%s actual=%s", expectedSHA256, actual)
	}
	return &LosslessResult{
		OracleMode:     OracleModeBitExactSHA256,
		Passed:         ok,
		ExpectedSHA256: expectedSHA256,
		ActualSHA256:   actual,
		BytesChecked:   n,
		Reason:         reason,
	}, nil
}

// EvaluateLossyLumaFrames delegates strictly to perceptual PSNR/SSIM/VMAF thresholds for lossy encodes.
func EvaluateLossyLumaFrames(refY, compY []byte, width, height int, minPSNR, minSSIM, minVMAF float64) (passed bool, psnr, ssim, vmaf float64, details string) {
	psnr, ssim, vmaf = profiler_media.EvaluatePerceptualMetricsNative(refY, compY, width, height)
	ok := psnr >= minPSNR && ssim >= minSSIM && vmaf >= minVMAF
	return ok, psnr, ssim, vmaf, fmt.Sprintf("oracle=%s psnr=%.2fdB(>=%.2f) ssim=%.4f(>=%.4f) vmaf=%.2f(>=%.2f)",
		OracleModeLossyPerceptual, psnr, minPSNR, ssim, minSSIM, vmaf, minVMAF)
}

// ProbeWithFFprobe invokes `ffprobe` directly when available on PATH; never fakes metadata if absent.
func ProbeWithFFprobe(ctx context.Context, mediaPath string) (*FFprobeSummary, error) {
	exe, err := exec.LookPath("ffprobe")
	if err != nil {
		return &FFprobeSummary{Available: false}, nil
	}
	cmd := exec.CommandContext(ctx, exe, "-v", "quiet", "-print_format", "json", "-show_format", "-show_streams", mediaPath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return &FFprobeSummary{Available: true}, fmt.Errorf("ffprobe failed: %w", err)
	}
	var raw struct {
		Format struct {
			FormatName string `json:"format_name"`
			Duration   string `json:"duration"`
			BitRate    string `json:"bit_rate"`
		} `json:"format"`
		Streams []struct {
			CodecType string `json:"codec_type"`
			CodecName string `json:"codec_name"`
			Width     int    `json:"width"`
			Height    int    `json:"height"`
		} `json:"streams"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, err
	}
	sum := &FFprobeSummary{
		Available:  true,
		FormatName: raw.Format.FormatName,
	}
	sum.DurationSeconds, _ = strconv.ParseFloat(raw.Format.Duration, 64)
	sum.BitrateBps, _ = strconv.ParseInt(raw.Format.BitRate, 10, 64)
	for _, s := range raw.Streams {
		if s.CodecType == "video" && sum.VideoCodec == "" {
			sum.VideoCodec = s.CodecName
			sum.Width = s.Width
			sum.Height = s.Height
		} else if s.CodecType == "audio" && sum.AudioCodec == "" {
			sum.AudioCodec = s.CodecName
		}
	}
	return sum, nil
}

func fileSHA256(path string) (string, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(h.Sum(nil)), n, nil
}
