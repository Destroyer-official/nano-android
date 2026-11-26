package render

import (
	"container/list"
	"encoding/binary"
	"hash/crc64"
	"image"
	"math/bits"
	"sync"
)

// HashAlgorithm specifies the hashing engine to use for visual delta detection.
type HashAlgorithm string

const (
	// HashXXH64 uses 64-bit xxHash for ultra-high throughput (>10 GB/s).
	HashXXH64 HashAlgorithm = "XXH64"
	// HashCRC64 uses CRC64-ECMA for fast hardware-assisted frame hashing.
	HashCRC64 HashAlgorithm = "CRC64"
)

var (
	crc64ECMATable = crc64.MakeTable(crc64.ECMA)
)

// HasFrameChanged compares two 64-bit visual hashes and returns true if pixels have changed.
// This allows callers to skip redundant WebP compression and IPC transmission when the UI is static.
func HasFrameChanged(prevHash uint64, currHash uint64) bool {
	return prevHash != currHash
}

// VisualHasher computes 64-bit frame hashes over raw RGBA image buffers.
type VisualHasher struct {
	algo HashAlgorithm
}

// NewVisualHasher constructs a hasher with the specified algorithm.
func NewVisualHasher(algo HashAlgorithm) *VisualHasher {
	if algo != HashCRC64 {
		algo = HashXXH64
	}
	return &VisualHasher{algo: algo}
}

// HashRGBA hashes the pixel buffer of an *image.RGBA image.
func (h *VisualHasher) HashRGBA(img *image.RGBA) uint64 {
	if img == nil || len(img.Pix) == 0 {
		return 0
	}
	return h.HashBytes(img.Pix)
}

// HashBytes computes the 64-bit visual hash over any raw byte slice.
func (h *VisualHasher) HashBytes(b []byte) uint64 {
	if len(b) == 0 {
		return 0
	}
	switch h.algo {
	case HashCRC64:
		return crc64.Checksum(b, crc64ECMATable)
	case HashXXH64:
		fallthrough
	default:
		return XXH64(b, 0)
	}
}

// FrameDiffResult details visual differences between two rendered frames.
type FrameDiffResult struct {
	Changed        bool      `json:"changed"`
	ChangedPixels  int       `json:"changedPixels"`
	TotalPixels    int       `json:"totalPixels"`
	DiffPercentage float64   `json:"diffPercentage"`
	BoundingBox    Rect      `json:"boundingBox"`
	DiffMask       *image.RGBA `json:"-"`
}

// CompareFrames compares two RGBA frames pixel by pixel and returns metrics and changed bounding box.
func CompareFrames(imgA, imgB *image.RGBA, threshold uint8, generateMask bool) FrameDiffResult {
	if imgA == nil || imgB == nil {
		return FrameDiffResult{Changed: imgA != imgB}
	}

	bA := imgA.Bounds()
	bB := imgB.Bounds()
	if bA != bB {
		return FrameDiffResult{
			Changed:        true,
			TotalPixels:    bA.Dx() * bA.Dy(),
			DiffPercentage: 100.0,
			BoundingBox:    Rect{Left: 0, Top: 0, Right: max(bA.Dx(), bB.Dx()), Bottom: max(bA.Dy(), bB.Dy())},
		}
	}

	w := bA.Dx()
	h := bA.Dy()
	totalPixels := w * h
	if totalPixels == 0 {
		return FrameDiffResult{Changed: false}
	}

	minX, minY := w, h
	maxX, maxY := -1, -1
	changedCount := 0

	var mask *image.RGBA
	if generateMask {
		mask = image.NewRGBA(image.Rect(0, 0, w, h))
	}

	strideA := imgA.Stride
	strideB := imgB.Stride
	pixA := imgA.Pix
	pixB := imgB.Pix

	for y := 0; y < h; y++ {
		offA := y * strideA
		offB := y * strideB
		for x := 0; x < w; x++ {
			iA := offA + x*4
			iB := offB + x*4

			dr := absDiff(pixA[iA], pixB[iB])
			dg := absDiff(pixA[iA+1], pixB[iB+1])
			db := absDiff(pixA[iA+2], pixB[iB+2])
			da := absDiff(pixA[iA+3], pixB[iB+3])

			if dr > threshold || dg > threshold || db > threshold || da > threshold {
				changedCount++
				if x < minX {
					minX = x
				}
				if x > maxX {
					maxX = x
				}
				if y < minY {
					minY = y
				}
				if y > maxY {
					maxY = y
				}

				if generateMask {
					mIdx := y*mask.Stride + x*4
					mask.Pix[mIdx] = 239   // Red highlight
					mask.Pix[mIdx+1] = 68
					mask.Pix[mIdx+2] = 68
					mask.Pix[mIdx+3] = 180
				}
			}
		}
	}

	if changedCount == 0 {
		return FrameDiffResult{
			Changed:     false,
			TotalPixels: totalPixels,
		}
	}

	diffPct := (float64(changedCount) / float64(totalPixels)) * 100.0
	bbox := Rect{
		Left:   minX,
		Top:    minY,
		Right:  maxX + 1,
		Bottom: maxY + 1,
	}

	return FrameDiffResult{
		Changed:        true,
		ChangedPixels:  changedCount,
		TotalPixels:    totalPixels,
		DiffPercentage: diffPct,
		BoundingBox:    bbox,
		DiffMask:       mask,
	}
}

func absDiff(a, b uint8) uint8 {
	if a > b {
		return a - b
	}
	return b - a
}

// FrameCacheEntry stores cached rendered outputs.
type FrameCacheEntry struct {
	Hash      uint64
	WebPData  []byte
	Base64URI string
}

// LRUFrameCache provides thread-safe LRU caching of rendered WebP frames.
type LRUFrameCache struct {
	mu       sync.RWMutex
	capacity int
	items    map[uint64]*list.Element
	lruList  *list.List
}

// NewLRUFrameCache creates a thread-safe LRU cache with the specified capacity.
func NewLRUFrameCache(capacity int) *LRUFrameCache {
	if capacity <= 0 {
		capacity = 32
	}
	return &LRUFrameCache{
		capacity: capacity,
		items:    make(map[uint64]*list.Element, capacity),
		lruList:  list.New(),
	}
}

// Get looks up a cached frame by its 64-bit visual hash and returns an immutable copy.
func (c *LRUFrameCache) Get(hash uint64) (*FrameCacheEntry, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if elem, ok := c.items[hash]; ok {
		c.lruList.MoveToFront(elem)
		entry := elem.Value.(*FrameCacheEntry)
		copied := *entry
		return &copied, true
	}
	return nil, false
}

// Put inserts or replaces an immutable entry in the LRU cache.
func (c *LRUFrameCache) Put(hash uint64, webp []byte, uri string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry := &FrameCacheEntry{
		Hash:      hash,
		WebPData:  webp,
		Base64URI: uri,
	}
	if elem, ok := c.items[hash]; ok {
		c.lruList.MoveToFront(elem)
		elem.Value = entry
		return
	}

	if c.lruList.Len() >= c.capacity {
		oldest := c.lruList.Back()
		if oldest != nil {
			c.lruList.Remove(oldest)
			oldEntry := oldest.Value.(*FrameCacheEntry)
			delete(c.items, oldEntry.Hash)
		}
	}

	elem := c.lruList.PushFront(entry)
	c.items[hash] = elem
}

// Len returns the current number of cached frames.
func (c *LRUFrameCache) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lruList.Len()
}

// Clear flushes all entries from the cache.
func (c *LRUFrameCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items = make(map[uint64]*list.Element, c.capacity)
	c.lruList.Init()
}

// XXH64 constants
const (
	prime64_1 uint64 = 0x9E3779B185EBCA87
	prime64_2 uint64 = 0xC2B2AE3D27D4EB4F
	prime64_3 uint64 = 0x165667B19E3779F9
	prime64_4 uint64 = 0x85EBCA77C2B2AE63
	prime64_5 uint64 = 0x27D4EB2F165667C5
)

func round64(acc, input uint64) uint64 {
	acc += input * prime64_2
	acc = bits.RotateLeft64(acc, 31)
	acc *= prime64_1
	return acc
}

func mergeRound64(acc, val uint64) uint64 {
	val = round64(0, val)
	acc ^= val
	acc = acc*prime64_1 + prime64_4
	return acc
}

// XXH64 calculates the 64-bit xxHash of input with a seed.
func XXH64(input []byte, seed uint64) uint64 {
	n := len(input)
	var h64 uint64

	if n >= 32 {
		v1 := seed + prime64_1 + prime64_2
		v2 := seed + prime64_2
		v3 := seed
		v4 := seed - prime64_1

		offset := 0
		limit := n - 32
		for offset <= limit {
			v1 = round64(v1, binary.LittleEndian.Uint64(input[offset:offset+8]))
			v2 = round64(v2, binary.LittleEndian.Uint64(input[offset+8:offset+16]))
			v3 = round64(v3, binary.LittleEndian.Uint64(input[offset+16:offset+24]))
			v4 = round64(v4, binary.LittleEndian.Uint64(input[offset+24:offset+32]))
			offset += 32
		}

		h64 = bits.RotateLeft64(v1, 1) +
			bits.RotateLeft64(v2, 7) +
			bits.RotateLeft64(v3, 12) +
			bits.RotateLeft64(v4, 18)

		h64 = mergeRound64(h64, v1)
		h64 = mergeRound64(h64, v2)
		h64 = mergeRound64(h64, v3)
		h64 = mergeRound64(h64, v4)

		input = input[offset:]
	} else {
		h64 = seed + prime64_5
	}

	h64 += uint64(n)

	for len(input) >= 8 {
		k1 := round64(0, binary.LittleEndian.Uint64(input[:8]))
		h64 ^= k1
		h64 = bits.RotateLeft64(h64, 27)*prime64_1 + prime64_4
		input = input[8:]
	}

	if len(input) >= 4 {
		h64 ^= uint64(binary.LittleEndian.Uint32(input[:4])) * prime64_1
		h64 = bits.RotateLeft64(h64, 23)*prime64_2 + prime64_3
		input = input[4:]
	}

	for len(input) > 0 {
		h64 ^= uint64(input[0]) * prime64_5
		h64 = bits.RotateLeft64(h64, 11) * prime64_1
		input = input[1:]
	}

	// Avalanche
	h64 ^= h64 >> 33
	h64 *= prime64_2
	h64 ^= h64 >> 29
	h64 *= prime64_3
	h64 ^= h64 >> 32

	return h64
}
