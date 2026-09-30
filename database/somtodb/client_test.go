package somtodb

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

func TestSegmentHintStoresUint64Keys(t *testing.T) {
	dbDir := t.TempDir()
	db, err := Init(dbDir, "data.txt")
	if err != nil {
		t.Fatalf("failed to initialize database: %v", err)
	}
	defer db.Close()

	records := make(map[uint64]string)
	for key := 0; key < 3; key++ {
		record, err := db.Set(key, "012345678901234567890123456789")
		if err != nil {
			t.Fatalf("failed to set key %d: %v", key, err)
		}
		records[uint64(key)] = record
	}

	hintPath := filepath.Join(dbDir, "segments", "1.hint")
	hint, err := os.ReadFile(hintPath)
	if err != nil {
		t.Fatalf("failed to read hint file: %v", err)
	}

	if len(hint) == 0 || len(hint)%SEGMENT_HEADER_SIZE != 0 {
		t.Fatalf("invalid hint file size: %d", len(hint))
	}
	if entries := len(hint) / SEGMENT_HEADER_SIZE; entries != 2 {
		t.Fatalf("expected 2 hint entries, got %d", entries)
	}

	dataPath := filepath.Join(dbDir, "segments", "1.txt")
	segmentData, err := os.ReadFile(dataPath)
	if err != nil {
		t.Fatalf("failed to read segment data: %v", err)
	}

	seen := make(map[uint64]bool)
	for offset := 0; offset < len(hint); offset += SEGMENT_HEADER_SIZE {
		entry := hint[offset : offset+SEGMENT_HEADER_SIZE]
		key := binary.LittleEndian.Uint64(entry[8:16])
		length := binary.LittleEndian.Uint32(entry[16:20])
		dataOffset := binary.LittleEndian.Uint64(entry[20:28])

		record, ok := records[key]
		if !ok {
			t.Fatalf("unexpected key in hint: %d", key)
		}
		if seen[key] {
			t.Fatalf("duplicate key in hint: %d", key)
		}
		seen[key] = true

		if uint32(len(record)) != length {
			t.Errorf("key %d: expected length %d, got %d", key, len(record), length)
		}
		if dataOffset+uint64(length) > uint64(len(segmentData)) {
			t.Fatalf("key %d: data range exceeds segment file", key)
		}

		got := segmentData[dataOffset : dataOffset+uint64(length)]
		if string(got) != record {
			t.Errorf("key %d: segment record differs", key)
		}
	}

	if len(seen) != 2 {
		t.Fatalf("expected 2 distinct keys in hint, got %d", len(seen))
	}
}
