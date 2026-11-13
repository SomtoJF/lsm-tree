package somtodb

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

func TestSegmentHintStoresUint64Keys(t *testing.T) {
	dbDir := t.TempDir()
	db, err := Init(dbDir, "data.db")
	if err != nil {
		t.Fatalf("failed to initialize database: %v", err)
	}
	defer db.Close()
	db.maxSegmentSize = 160

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

	dataPath := filepath.Join(dbDir, "segments", "1.db")
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

func TestRecoversTruncatedActiveTail(t *testing.T) {
	dbDir := t.TempDir()
	db, err := Init(dbDir, "data.db")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Set(1, "ok"); err != nil {
		t.Fatal(err)
	}
	db.Close()
	file, err := os.OpenFile(filepath.Join(dbDir, "data.db"), os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = file.Write([]byte("SMDB"))
	_ = file.Close()

	db, err = Init(dbDir, "data.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	value, err := db.Get(1)
	if err != nil || value != "ok" {
		t.Fatalf("recovered value = %q, err = %v", value, err)
	}
}

func TestRecoversChecksumInvalidActiveTail(t *testing.T) {
	dbDir := t.TempDir()
	db, err := Init(dbDir, "data.db")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Set(1, "ok"); err != nil {
		t.Fatal(err)
	}
	db.Close()
	frame := encodeRecord([]byte("key: 2, value: bad\n"))
	frame[len(frame)-1]++
	file, err := os.OpenFile(filepath.Join(dbDir, "data.db"), os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = file.Write(frame)
	_ = file.Close()

	db, err = Init(dbDir, "data.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Get(2); err == nil {
		t.Fatal("expected invalid tail record to be discarded")
	}
}

func TestRejectsCorruptActiveMiddle(t *testing.T) {
	dbDir := t.TempDir()
	db, err := Init(dbDir, "data.db")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = db.Set(1, "one")
	_, _ = db.Set(2, "two")
	_, _ = db.Set(3, "three")
	db.Close()
	path := filepath.Join(dbDir, "data.db")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	data[recordHeaderSize]++
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
	if db, err := Init(dbDir, "data.db"); err == nil {
		db.Close()
		t.Fatal("expected middle corruption error")
	}
}

func TestRepairCopiesValidRecords(t *testing.T) {
	sourceDir := t.TempDir()
	db, err := Init(sourceDir, "data.db")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = db.Set(1, "one")
	_, _ = db.Set(2, "two")
	db.Close()
	path := filepath.Join(sourceDir, "data.db")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	data[len(encodeRecord([]byte("key: 1, value: one\n")))+recordHeaderSize]++
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}

	targetDir := filepath.Join(t.TempDir(), "repaired")
	if err := Repair(sourceDir, "data.db", targetDir, "data.db"); err != nil {
		t.Fatal(err)
	}
	repaired, err := Init(targetDir, "data.db")
	if err != nil {
		t.Fatal(err)
	}
	defer repaired.Close()
	value, err := repaired.Get(1)
	if err != nil || value != "one" {
		t.Fatalf("repaired value = %q, err = %v", value, err)
	}
}
