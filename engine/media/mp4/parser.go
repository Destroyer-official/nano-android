package mp4

import (
	"encoding/binary"
	"fmt"
	"os"
	"strings"
)

const (
	maxParseDepth = 12
	maxBoxCount   = 10000
)

// AtomSummary records one ISO BMFF box discovered inside an MP4/MOV stream.
type AtomSummary struct {
	Type   string `json:"type"`
	Offset int64  `json:"offset"`
	Size   int64  `json:"size"`
}

// TrackInfo describes one parsed audio/video track inside `moov -> trak`.
type TrackInfo struct {
	HandlerType  string  `json:"handler_type"` // "vide", "soun", "hint", "meta"
	CodecFourCC  string  `json:"codec_fourcc,omitempty"`
	Width        int     `json:"width,omitempty"`
	Height       int     `json:"height,omitempty"`
	Channels     int     `json:"channels,omitempty"`
	SampleRateHz int     `json:"sample_rate_hz,omitempty"`
	SampleCount  int     `json:"sample_count,omitempty"`
	Timescale    uint32  `json:"timescale,omitempty"`
	Duration     uint64  `json:"duration,omitempty"`
	FPS          float64 `json:"fps,omitempty"`
}

// InspectionReport summarizes ISO BMFF container geometry, codec tracks, duration, bitrate, and corruption checks.
type InspectionReport struct {
	Valid             bool          `json:"valid"`
	MajorBrand        string        `json:"major_brand,omitempty"`
	CompatibleBrands  []string      `json:"compatible_brands,omitempty"`
	Timescale         uint32        `json:"timescale,omitempty"`
	DurationUnits     uint64        `json:"duration_units,omitempty"`
	DurationSeconds   float64       `json:"duration_seconds"`
	TotalBytes        int64         `json:"total_bytes"`
	MediaDataBytes    int64         `json:"media_data_bytes"`
	EstimatedBitrate  int64         `json:"estimated_bitrate_bps"`
	Tracks            []TrackInfo   `json:"tracks,omitempty"`
	TopLevelAtoms     []AtomSummary `json:"top_level_atoms,omitempty"`
	Errors            []string      `json:"errors,omitempty"`
}

// ParseFile inspects an MP4/ISO BMFF file on disk.
func ParseFile(path string) (*InspectionReport, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read mp4 file: %w", err)
	}
	return ParseBytes(data), nil
}

// ParseBytes parses raw ISO BMFF container bytes without external dependencies.
func ParseBytes(data []byte) *InspectionReport {
	rep := &InspectionReport{
		TotalBytes: int64(len(data)),
	}
	if len(data) < 8 {
		rep.Errors = append(rep.Errors, "file smaller than minimum 8-byte ISO BMFF box header")
		return rep
	}

	boxCount := 0
	var hasFtyp, hasMoov bool
	offset := int64(0)
	n := int64(len(data))

	for offset+8 <= n {
		boxCount++
		if boxCount > maxBoxCount {
			rep.Errors = append(rep.Errors, "exceeded max ISO BMFF box count")
			break
		}
		boxType, headerLen, boxSize, err := readBoxHeader(data[offset:])
		if err != nil {
			rep.Errors = append(rep.Errors, fmt.Sprintf("offset %d: %v", offset, err))
			break
		}
		if boxSize == 0 {
			boxSize = n - offset
		}
		if boxSize < headerLen || offset+boxSize > n {
			rep.Errors = append(rep.Errors, fmt.Sprintf("truncated or malformed box %q at offset %d (size=%d, remaining=%d)", boxType, offset, boxSize, n-offset))
			break
		}

		rep.TopLevelAtoms = append(rep.TopLevelAtoms, AtomSummary{
			Type:   boxType,
			Offset: offset,
			Size:   boxSize,
		})

		payload := data[offset+headerLen : offset+boxSize]
		switch boxType {
		case "ftyp":
			hasFtyp = true
			parseFtyp(payload, rep)
		case "moov":
			hasMoov = true
			parseContainer(payload, rep, nil, 1)
		case "mdat":
			rep.MediaDataBytes += int64(len(payload))
		}

		offset += boxSize
	}

	if !hasFtyp {
		rep.Errors = append(rep.Errors, "missing mandatory 'ftyp' box")
	}
	if !hasMoov {
		rep.Errors = append(rep.Errors, "missing mandatory 'moov' box")
	}
	if rep.DurationSeconds > 0 {
		basisBytes := rep.MediaDataBytes
		if basisBytes == 0 {
			basisBytes = rep.TotalBytes
		}
		rep.EstimatedBitrate = int64((float64(basisBytes) * 8.0) / rep.DurationSeconds)
	}
	rep.Valid = hasFtyp && hasMoov && len(rep.Errors) == 0
	return rep
}

func readBoxHeader(b []byte) (boxType string, headerLen, boxSize int64, err error) {
	if len(b) < 8 {
		return "", 0, 0, fmt.Errorf("short header (%d bytes)", len(b))
	}
	sz32 := binary.BigEndian.Uint32(b[0:4])
	boxType = string(b[4:8])
	if sz32 == 1 {
		if len(b) < 16 {
			return boxType, 0, 0, fmt.Errorf("short extended 64-bit box header")
		}
		sz64 := binary.BigEndian.Uint64(b[8:16])
		if sz64 > uint64(len(b)) {
			return boxType, 16, int64(sz64), nil
		}
		return boxType, 16, int64(sz64), nil
	}
	return boxType, 8, int64(sz32), nil
}

func parseFtyp(payload []byte, rep *InspectionReport) {
	if len(payload) < 8 {
		rep.Errors = append(rep.Errors, "ftyp box shorter than 8 bytes")
		return
	}
	rep.MajorBrand = strings.TrimSpace(string(payload[0:4]))
	for i := 8; i+4 <= len(payload); i += 4 {
		brand := strings.TrimSpace(string(payload[i : i+4]))
		if brand != "" {
			rep.CompatibleBrands = append(rep.CompatibleBrands, brand)
		}
	}
}

func parseContainer(payload []byte, rep *InspectionReport, curTrack *TrackInfo, depth int) {
	if depth > maxParseDepth {
		return
	}
	offset := int64(0)
	n := int64(len(payload))
	for offset+8 <= n {
		boxType, headerLen, boxSize, err := readBoxHeader(payload[offset:])
		if err != nil {
			rep.Errors = append(rep.Errors, fmt.Sprintf("nested box error: %v", err))
			return
		}
		if boxSize == 0 {
			boxSize = n - offset
		}
		if boxSize < headerLen || offset+boxSize > n {
			rep.Errors = append(rep.Errors, fmt.Sprintf("truncated nested box %q", boxType))
			return
		}
		sub := payload[offset+headerLen : offset+boxSize]
		switch boxType {
		case "mvhd":
			parseMvhd(sub, rep)
		case "trak":
			var tr TrackInfo
			parseContainer(sub, rep, &tr, depth+1)
			if tr.Timescale > 0 && tr.Duration > 0 && tr.SampleCount > 0 {
				dur := float64(tr.Duration) / float64(tr.Timescale)
				if dur > 0 {
					tr.FPS = float64(int((float64(tr.SampleCount)/dur)*100+0.5)) / 100
				}
			}
			rep.Tracks = append(rep.Tracks, tr)
		case "mdia", "minf", "stbl":
			parseContainer(sub, rep, curTrack, depth+1)
		case "mdhd":
			if curTrack != nil {
				parseMdhd(sub, curTrack)
			}
		case "hdlr":
			if curTrack != nil && len(sub) >= 12 {
				curTrack.HandlerType = string(sub[8:12])
			}
		case "stsd":
			if curTrack != nil {
				parseStsd(sub, curTrack)
			}
		case "stsz":
			if curTrack != nil && len(sub) >= 12 {
				curTrack.SampleCount = int(binary.BigEndian.Uint32(sub[8:12]))
			}
		}
		offset += boxSize
	}
}

func parseMvhd(sub []byte, rep *InspectionReport) {
	if len(sub) < 20 {
		return
	}
	version := sub[0]
	if version == 0 && len(sub) >= 20 {
		ts := binary.BigEndian.Uint32(sub[12:16])
		dur := uint64(binary.BigEndian.Uint32(sub[16:20]))
		rep.Timescale = ts
		rep.DurationUnits = dur
		if ts > 0 {
			rep.DurationSeconds = float64(dur) / float64(ts)
		}
	} else if version == 1 && len(sub) >= 32 {
		ts := binary.BigEndian.Uint32(sub[20:24])
		dur := binary.BigEndian.Uint64(sub[24:32])
		rep.Timescale = ts
		rep.DurationUnits = dur
		if ts > 0 {
			rep.DurationSeconds = float64(dur) / float64(ts)
		}
	}
}

func parseMdhd(sub []byte, tr *TrackInfo) {
	if len(sub) < 20 {
		return
	}
	version := sub[0]
	if version == 0 && len(sub) >= 20 {
		tr.Timescale = binary.BigEndian.Uint32(sub[12:16])
		tr.Duration = uint64(binary.BigEndian.Uint32(sub[16:20]))
	} else if version == 1 && len(sub) >= 32 {
		tr.Timescale = binary.BigEndian.Uint32(sub[20:24])
		tr.Duration = binary.BigEndian.Uint64(sub[24:32])
	}
}

func parseStsd(sub []byte, tr *TrackInfo) {
	// FullBox (4 bytes) + entry_count (4 bytes) + SampleEntry(size[4] + codec[4] + reserved[6] + dref_idx[2] + ...)
	if len(sub) < 16 {
		return
	}
	entryCount := binary.BigEndian.Uint32(sub[4:8])
	if entryCount == 0 || len(sub) < 16 {
		return
	}
	tr.CodecFourCC = string(sub[12:16])
	// VisualSampleEntry stores width @ offset 24..26 and height @ 26..28 of SampleEntry (sub[8:])
	if tr.HandlerType == "vide" && len(sub) >= 8+36 {
		tr.Width = int(binary.BigEndian.Uint16(sub[8+24 : 8+26]))
		tr.Height = int(binary.BigEndian.Uint16(sub[8+26 : 8+28]))
	}
	// AudioSampleEntry stores channelcount @ offset 16..18 and samplerate (16.16 fixed-point) @ offset 24..28
	if tr.HandlerType == "soun" && len(sub) >= 8+28 {
		tr.Channels = int(binary.BigEndian.Uint16(sub[8+16 : 8+18]))
		tr.SampleRateHz = int(binary.BigEndian.Uint16(sub[8+24 : 8+26]))
	}
}
