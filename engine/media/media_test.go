package media_test

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	"nano_android/engine/media/codec"
	"nano_android/engine/media/mp4"
	"nano_android/engine/media/oracle"
)

func buildBox(typ string, payload []byte) []byte {
	out := make([]byte, 8+len(payload))
	binary.BigEndian.PutUint32(out[0:4], uint32(len(out)))
	copy(out[4:8], typ)
	copy(out[8:], payload)
	return out
}

func buildSyntheticMP4() []byte {
	// ftyp: isom + compatible isom, mp42
	ftypPayload := []byte("isom\x00\x00\x02\x00isommp42")
	ftyp := buildBox("ftyp", ftypPayload)

	// mvhd v0: timescale=1000, duration=2500 -> 2.5s
	mvhdPayload := make([]byte, 100)
	binary.BigEndian.PutUint32(mvhdPayload[12:16], 1000)
	binary.BigEndian.PutUint32(mvhdPayload[16:20], 2500)
	mvhd := buildBox("mvhd", mvhdPayload)

	// hdlr (vide)
	hdlrPayload := make([]byte, 24)
	copy(hdlrPayload[8:12], "vide")
	hdlr := buildBox("hdlr", hdlrPayload)

	// mdhd v0: timescale=30000, duration=75000 -> 2.5s
	mdhdPayload := make([]byte, 24)
	binary.BigEndian.PutUint32(mdhdPayload[12:16], 30000)
	binary.BigEndian.PutUint32(mdhdPayload[16:20], 75000)
	mdhd := buildBox("mdhd", mdhdPayload)

	// stsd: 1 entry avc1 1280x720
	stsdPayload := make([]byte, 4+4+40)
	binary.BigEndian.PutUint32(stsdPayload[4:8], 1)
	binary.BigEndian.PutUint32(stsdPayload[8:12], 40)
	copy(stsdPayload[12:16], "avc1")
	binary.BigEndian.PutUint16(stsdPayload[8+24:8+26], 1280)
	binary.BigEndian.PutUint16(stsdPayload[8+26:8+28], 720)
	stsd := buildBox("stsd", stsdPayload)

	// stsz: 75 samples -> 30.0 fps
	stszPayload := make([]byte, 12)
	binary.BigEndian.PutUint32(stszPayload[8:12], 75)
	stsz := buildBox("stsz", stszPayload)

	stbl := buildBox("stbl", bytes.Join([][]byte{stsd, stsz}, nil))
	minf := buildBox("minf", stbl)
	mdia := buildBox("mdia", bytes.Join([][]byte{mdhd, hdlr, minf}, nil))
	trak := buildBox("trak", mdia)

	// Audio trak (soun / mp4a / 2 channels / 48000 Hz)
	audHdlrPayload := make([]byte, 24)
	copy(audHdlrPayload[8:12], "soun")
	audHdlr := buildBox("hdlr", audHdlrPayload)
	audMdhdPayload := make([]byte, 24)
	binary.BigEndian.PutUint32(audMdhdPayload[12:16], 48000)
	binary.BigEndian.PutUint32(audMdhdPayload[16:20], 120000)
	audMdhd := buildBox("mdhd", audMdhdPayload)
	audStsdPayload := make([]byte, 4+4+32)
	binary.BigEndian.PutUint32(audStsdPayload[4:8], 1)
	binary.BigEndian.PutUint32(audStsdPayload[8:12], 32)
	copy(audStsdPayload[12:16], "mp4a")
	binary.BigEndian.PutUint16(audStsdPayload[8+16:8+18], 2)     // stereo
	binary.BigEndian.PutUint16(audStsdPayload[8+24:8+26], 48000) // 48 kHz
	audStsd := buildBox("stsd", audStsdPayload)
	audTrak := buildBox("trak", buildBox("mdia", bytes.Join([][]byte{audMdhd, audHdlr, buildBox("minf", buildBox("stbl", audStsd))}, nil)))

	moov := buildBox("moov", bytes.Join([][]byte{mvhd, trak, audTrak}, nil))
	mdat := buildBox("mdat", make([]byte, 1024))

	return bytes.Join([][]byte{ftyp, moov, mdat}, nil)
}

func TestMP4AtomParserAndCorruptionDetection(t *testing.T) {
	raw := buildSyntheticMP4()
	rep := mp4.ParseBytes(raw)
	if !rep.Valid {
		t.Fatalf("expected valid synthetic MP4, errors: %v", rep.Errors)
	}
	if rep.MajorBrand != "isom" || rep.DurationSeconds != 2.5 {
		t.Fatalf("unexpected header metadata: %+v", rep)
	}
	if len(rep.Tracks) != 2 || rep.Tracks[0].CodecFourCC != "avc1" || rep.Tracks[0].Width != 1280 || rep.Tracks[0].Height != 720 || rep.Tracks[0].FPS != 30 {
		t.Fatalf("unexpected video track parse: %+v", rep.Tracks)
	}
	if rep.Tracks[1].HandlerType != "soun" || rep.Tracks[1].CodecFourCC != "mp4a" || rep.Tracks[1].Channels != 2 || rep.Tracks[1].SampleRateHz != 48000 {
		t.Fatalf("unexpected audio track parse: %+v", rep.Tracks[1])
	}

	// Truncate container -> must report corruption
	corrupt := mp4.ParseBytes(raw[:len(raw)-200])
	if corrupt.Valid || len(corrupt.Errors) == 0 {
		t.Fatalf("expected truncated MP4 to be rejected, got %+v", corrupt)
	}
}

func TestStrictOracleSeparationLosslessVsLossy(t *testing.T) {
	tmp := t.TempDir()
	refFile := filepath.Join(tmp, "ref.bin")
	sameFile := filepath.Join(tmp, "same.bin")
	diffFile := filepath.Join(tmp, "diff.bin")

	payload := []byte("deterministic lossless passthrough bitstream bytes")
	diff := append([]byte(nil), payload...)
	diff[len(diff)-1] ^= 0x01 // 1-bit flip

	_ = os.WriteFile(refFile, payload, 0644)
	_ = os.WriteFile(sameFile, payload, 0644)
	_ = os.WriteFile(diffFile, diff, 0644)

	okRes, err := oracle.VerifyFilesBitExact(refFile, sameFile)
	if err != nil || !okRes.Passed || okRes.OracleMode != oracle.OracleModeBitExactSHA256 {
		t.Fatalf("expected bit-exact lossless match, err=%v res=%+v", err, okRes)
	}
	failRes, err := oracle.VerifyFilesBitExact(refFile, diffFile)
	if err != nil || failRes.Passed {
		t.Fatalf("expected 1-bit flip to fail bit-exact oracle, res=%+v", failRes)
	}
	streamRes, err := oracle.VerifyContentStreamIntegrity(bytes.NewReader(payload), okRes.ExpectedSHA256)
	if err != nil || !streamRes.Passed {
		t.Fatalf("expected VerifyContentStreamIntegrity to pass, err=%v res=%+v", err, streamRes)
	}

	refY := make([]byte, 64*64)
	compY := make([]byte, 64*64)
	for i := range refY {
		refY[i] = byte(i % 200)
		compY[i] = byte((i%200)+1)
	}
	pass, psnr, ssim, vmaf, _ := oracle.EvaluateLossyLumaFrames(refY, compY, 64, 64, 35.0, 0.95, 80.0)
	if !pass || psnr < 35.0 || ssim < 0.95 || vmaf < 80.0 {
		t.Fatalf("expected lossy perceptual metrics to pass, pass=%v psnr=%.2f ssim=%.4f vmaf=%.2f", pass, psnr, ssim, vmaf)
	}
}

func TestMediaCodecCapabilityParser(t *testing.T) {
	dumpsys := `
Decoder "c2.qti.avc.decoder" supports [video/avc] max=3840x2160
Encoder "c2.android.avc.encoder" supports [video/avc] max=1920x1080
Decoder "OMX.google.hevc.decoder" supports [video/hevc]
`
	caps := codec.ParseMediaCodecDumpsys(dumpsys)
	if len(caps) != 3 {
		t.Fatalf("expected 3 parsed codecs, got %d", len(caps))
	}
	hwCount := 0
	swCount := 0
	for _, c := range caps {
		if c.HardwareAccelerated {
			hwCount++
		} else {
			swCount++
		}
	}
	if hwCount != 1 || swCount != 2 {
		t.Fatalf("expected 1 HW and 2 SW codecs, got hw=%d sw=%d", hwCount, swCount)
	}
}
