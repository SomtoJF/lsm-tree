package somtodb

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	SEGMENT_HEADER_SIZE = 28
	maxSegmentSize      = 1024
	commitWindow        = 5 * time.Millisecond
)

type SomtoDB struct {
	dbFileName string
	dbDir      string
	filePath   string

	currIndexes       map[int]indexEntry
	segementedIndexes map[int]indexEntry
	segments          []manifestSegment
	fileSize          int

	mutex          sync.Mutex
	maxSegmentSize int
	file           *os.File
	segmentCount   int
	segmentDir     string

	writeQueue chan *writeRequest
	writerDone chan struct{}
	lifecycle  sync.RWMutex
	closed     bool
}

type indexEntry struct {
	offset      int
	length      int
	segmentName *string
}

type compactionResult struct {
	segmentFileName     string
	segmentDataFileName string
}

type hintEntry struct {
	Timestamp uint64
	Key       uint64
	Length    uint32
	Offset    uint64
}

type writeRequest struct {
	key      int
	payload  []byte
	response chan error
}

func Init(dbDir string, dbFileName string) (*SomtoDB, error) {
	if dbDir == "" || dbFileName == "" {
		return nil, fmt.Errorf("database directory and file name must be provided")
	}
	if err := os.MkdirAll(dbDir, 0755); err != nil {
		return nil, err
	}

	filePath := filepath.Join(dbDir, dbFileName)
	file, err := os.OpenFile(filePath, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0644)
	if err != nil {
		return nil, err
	}
	db := &SomtoDB{
		dbFileName:        dbFileName,
		dbDir:             dbDir,
		filePath:          filePath,
		currIndexes:       make(map[int]indexEntry),
		segementedIndexes: make(map[int]indexEntry),
		maxSegmentSize:    maxSegmentSize,
		file:              file,
		segmentDir:        filepath.Join(dbDir, "segments"),
		writeQueue:        make(chan *writeRequest, 256),
		writerDone:        make(chan struct{}),
	}
	if err := os.MkdirAll(db.segmentDir, 0755); err != nil {
		_ = file.Close()
		return nil, err
	}

	fileInfo, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	if fileInfo.Size() > int64(^uint(0)>>1) {
		_ = file.Close()
		return nil, fmt.Errorf("active database file is too large for this platform")
	}

	if err := db.loadIndexes(); err != nil {
		_ = file.Close()
		return nil, err
	}
	go db.runWriter()
	return db, nil
}

func (db *SomtoDB) loadIndexes() error {
	loadedManifest, err := loadManifest(db.segmentDir)
	if err != nil {
		return err
	}
	segmentedIndexes, err := db.readSegmentIndexes(loadedManifest)
	if err != nil {
		return err
	}
	currIndexes, recoveredSize, err := db.readActiveIndexes()
	if err != nil {
		return err
	}
	if recoveredSize != db.fileSizeOnDisk() {
		if err := db.file.Truncate(recoveredSize); err != nil {
			return fmt.Errorf("truncate recovered active file: %w", err)
		}
	}
	db.currIndexes = currIndexes
	db.segementedIndexes = segmentedIndexes
	db.segments = loadedManifest.Segments
	for _, segment := range loadedManifest.Segments {
		if segment.ID > db.segmentCount {
			db.segmentCount = segment.ID
		}
	}
	db.fileSize = int(recoveredSize)
	return nil
}

func (db *SomtoDB) fileSizeOnDisk() int64 {
	info, err := db.file.Stat()
	if err != nil {
		return -1
	}
	return info.Size()
}

func (db *SomtoDB) readSegmentIndexes(value manifest) (map[int]indexEntry, error) {
	result := make(map[int]indexEntry)
	for _, segment := range value.Segments {
		hintPath := filepath.Join(db.segmentDir, segment.Hint)
		dataPath := filepath.Join(db.segmentDir, segment.Data)
		hintData, err := os.ReadFile(hintPath)
		if err != nil {
			return nil, fmt.Errorf("read segment hint %q: %w", hintPath, err)
		}
		if len(hintData)%SEGMENT_HEADER_SIZE != 0 {
			return nil, fmt.Errorf("segment hint %q has a truncated entry", hintPath)
		}
		dataFile, err := os.Open(dataPath)
		if err != nil {
			return nil, err
		}
		dataInfo, err := dataFile.Stat()
		if err != nil {
			_ = dataFile.Close()
			return nil, err
		}
		for offset := 0; offset < len(hintData); offset += SEGMENT_HEADER_SIZE {
			entry := hintData[offset : offset+SEGMENT_HEADER_SIZE]
			rawKey := binary.LittleEndian.Uint64(entry[8:16])
			key64 := int64(rawKey)
			key := int(key64)
			if int64(key) != key64 {
				_ = dataFile.Close()
				return nil, fmt.Errorf("key %d in %q cannot be represented as int", key64, hintPath)
			}
			length := int(binary.LittleEndian.Uint32(entry[16:20]))
			dataOffset := int64(binary.LittleEndian.Uint64(entry[20:28]))
			frameOffset := dataOffset - recordHeaderSize
			if dataOffset < recordHeaderSize || dataOffset+int64(length) > dataInfo.Size() {
				_ = dataFile.Close()
				return nil, fmt.Errorf("segment hint %q contains an out-of-range data entry for key %d", hintPath, key)
			}
			payload, nextOffset, err := decodeRecordAt(dataFile, frameOffset, dataInfo.Size())
			if err != nil || int(nextOffset-frameOffset) != recordHeaderSize+length+recordChecksumSize || len(payload) != length {
				_ = dataFile.Close()
				return nil, fmt.Errorf("segment %q contains an invalid record for key %d: %w", dataPath, key, err)
			}
			dataFileName := dataPath
			result[key] = indexEntry{offset: int(dataOffset), length: length, segmentName: &dataFileName}
		}
		if err := dataFile.Close(); err != nil {
			return nil, err
		}
	}
	return result, nil
}

func (db *SomtoDB) readActiveIndexes() (map[int]indexEntry, int64, error) {
	info, err := db.file.Stat()
	if err != nil {
		return nil, 0, err
	}
	indexes := make(map[int]indexEntry)
	offset := int64(0)
	for offset < info.Size() {
		payload, nextOffset, err := decodeRecordAt(db.file, offset, info.Size())
		if err != nil {
			if err == io.ErrUnexpectedEOF {
				return indexes, offset, nil
			}
			if errors.Is(err, errRecordChecksum) && nextOffset == info.Size() {
				return indexes, offset, nil
			}
			return nil, 0, fmt.Errorf("active database file %q is corrupt at offset %d: %w", db.filePath, offset, err)
		}
		key, err := parseRecordKey(payload)
		if err != nil {
			return nil, 0, fmt.Errorf("active database file %q is corrupt at offset %d: %w", db.filePath, offset, err)
		}
		indexes[key] = indexEntry{offset: int(offset + recordHeaderSize), length: len(payload)}
		offset = nextOffset
	}
	return indexes, offset, nil
}

func parseRecordKey(record []byte) (int, error) {
	keyText, _, ok := strings.Cut(strings.TrimSuffix(string(record), "\n"), ", value: ")
	if !ok || !strings.HasPrefix(keyText, "key: ") {
		return 0, fmt.Errorf("malformed record")
	}
	key, err := strconv.Atoi(strings.TrimPrefix(keyText, "key: "))
	if err != nil {
		return 0, fmt.Errorf("invalid key: %w", err)
	}
	return key, nil
}

func (db *SomtoDB) Close() {
	db.lifecycle.Lock()
	if db.closed {
		db.lifecycle.Unlock()
		return
	}
	db.closed = true
	close(db.writeQueue)
	db.lifecycle.Unlock()
	<-db.writerDone
	db.mutex.Lock()
	defer db.mutex.Unlock()
	if db.file != nil {
		_ = db.file.Close()
		db.file = nil
	}
}

func (db *SomtoDB) Set(key int, value string) (string, error) {
	text := fmt.Sprintf("key: %d, value: %s\n", key, value)
	request := &writeRequest{key: key, payload: []byte(text), response: make(chan error, 1)}
	db.lifecycle.RLock()
	if db.closed {
		db.lifecycle.RUnlock()
		return "", fmt.Errorf("database is closed")
	}
	db.writeQueue <- request
	db.lifecycle.RUnlock()
	if err := <-request.response; err != nil {
		return "", err
	}
	return text, nil
}

func (db *SomtoDB) runWriter() {
	defer close(db.writerDone)
	for first := range db.writeQueue {
		batch := []*writeRequest{first}
		timer := time.NewTimer(commitWindow)
	collect:
		for {
			select {
			case request, ok := <-db.writeQueue:
				if !ok {
					break collect
				}
				batch = append(batch, request)
			case <-timer.C:
				break collect
			}
		}
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		err := db.commitBatch(batch)
		for _, request := range batch {
			request.response <- err
		}
	}
}

func (db *SomtoDB) commitBatch(batch []*writeRequest) error {
	db.mutex.Lock()
	defer db.mutex.Unlock()
	dirty := false
	for _, request := range batch {
		frame := encodeRecord(request.payload)
		if len(frame)+db.fileSize > db.maxSegmentSize {
			if dirty {
				if err := db.file.Sync(); err != nil {
					return err
				}
				dirty = false
			}
			// TODO: move compaction to a background process.
			if _, err := db.runCompaction(); err != nil {
				return err
			}
			if err := db.clearCurrentDBFile(); err != nil {
				return err
			}
		}
		if err := db.writeLocked(request.key, request.payload, frame); err != nil {
			return err
		}
		dirty = true
	}
	if dirty {
		if err := db.file.Sync(); err != nil {
			return err
		}
	}
	return nil
}

func (db *SomtoDB) runCompaction() (compactionResult, error) {
	if len(db.currIndexes) == 0 {
		return compactionResult{}, nil
	}
	db.segmentCount++
	segmentID := db.segmentCount
	dataName := fmt.Sprintf("%d.txt", segmentID)
	hintName := fmt.Sprintf("%d.hint", segmentID)
	tempDataPath := filepath.Join(db.segmentDir, dataName+".tmp")
	tempHintPath := filepath.Join(db.segmentDir, hintName+".tmp")
	dataPath := filepath.Join(db.segmentDir, dataName)
	hintPath := filepath.Join(db.segmentDir, hintName)
	dataFile, err := os.OpenFile(tempDataPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return compactionResult{}, err
	}
	hintFile, err := os.OpenFile(tempHintPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		_ = dataFile.Close()
		return compactionResult{}, err
	}
	writer := bufio.NewWriter(hintFile)
	hash := crc32.NewIEEE()
	segmentOffset := int64(0)
	newIndexes := make(map[int]indexEntry)
	header := make([]byte, SEGMENT_HEADER_SIZE)
	for key, indexData := range db.currIndexes {
		payload := make([]byte, indexData.length)
		if _, err := db.file.ReadAt(payload, int64(indexData.offset)); err != nil {
			_ = dataFile.Close()
			_ = hintFile.Close()
			return compactionResult{}, err
		}
		frame := encodeRecord(payload)
		if _, err := dataFile.Write(frame); err != nil {
			_ = dataFile.Close()
			_ = hintFile.Close()
			return compactionResult{}, err
		}
		_, _ = hash.Write(frame)
		payloadOffset := segmentOffset + recordHeaderSize
		dataFileName := dataPath
		newIndexes[key] = indexEntry{offset: int(payloadOffset), length: len(payload), segmentName: &dataFileName}
		entry := hintEntry{Timestamp: uint64(time.Now().UnixNano()), Key: uint64(key), Length: uint32(len(payload)), Offset: uint64(payloadOffset)}
		binary.LittleEndian.PutUint64(header[0:8], entry.Timestamp)
		binary.LittleEndian.PutUint64(header[8:16], entry.Key)
		binary.LittleEndian.PutUint32(header[16:20], entry.Length)
		binary.LittleEndian.PutUint64(header[20:28], entry.Offset)
		if _, err := writer.Write(header); err != nil {
			_ = dataFile.Close()
			_ = hintFile.Close()
			return compactionResult{}, err
		}
		segmentOffset += int64(len(frame))
	}
	if err := writer.Flush(); err != nil {
		_ = dataFile.Close()
		_ = hintFile.Close()
		return compactionResult{}, err
	}
	if err := dataFile.Sync(); err != nil {
		_ = dataFile.Close()
		_ = hintFile.Close()
		return compactionResult{}, err
	}
	if err := hintFile.Sync(); err != nil {
		_ = dataFile.Close()
		_ = hintFile.Close()
		return compactionResult{}, err
	}
	if err := dataFile.Close(); err != nil {
		_ = hintFile.Close()
		return compactionResult{}, err
	}
	if err := hintFile.Close(); err != nil {
		return compactionResult{}, err
	}
	if err := os.Rename(tempDataPath, dataPath); err != nil {
		return compactionResult{}, err
	}
	if err := os.Rename(tempHintPath, hintPath); err != nil {
		return compactionResult{}, err
	}
	newSegments := append(append([]manifestSegment(nil), db.segments...), manifestSegment{ID: segmentID, Data: dataName, Hint: hintName, DataSize: segmentOffset, DataCRC: hash.Sum32()})
	if err := publishManifest(db.segmentDir, manifest{Version: manifestVersion, Segments: newSegments}); err != nil {
		return compactionResult{}, err
	}
	db.segments = newSegments
	for key, entry := range newIndexes {
		db.segementedIndexes[key] = entry
	}
	return compactionResult{segmentFileName: hintPath, segmentDataFileName: dataPath}, nil
}

func (db *SomtoDB) clearCurrentDBFile() error {
	if db.file == nil {
		return fmt.Errorf("database file is closed")
	}
	if err := db.file.Truncate(0); err != nil {
		return err
	}
	db.fileSize = 0
	db.currIndexes = make(map[int]indexEntry)
	return nil
}

func (db *SomtoDB) writeLocked(key int, payload []byte, frame []byte) error {
	if _, err := db.file.Write(frame); err != nil {
		return err
	}
	db.currIndexes[key] = indexEntry{offset: db.fileSize + recordHeaderSize, length: len(payload)}
	db.fileSize += len(frame)
	return nil
}

func (db *SomtoDB) Get(key int) (string, error) {
	db.mutex.Lock()
	defer db.mutex.Unlock()
	indexData, err := db.readIndex(key)
	if err != nil {
		return "", err
	}
	readData, err := db.read(indexData)
	if err != nil {
		return "", err
	}
	value := strings.TrimPrefix(string(readData), fmt.Sprintf("key: %d, value: ", key))
	value = strings.TrimSuffix(value, "\n")
	return value, nil
}

func (db *SomtoDB) read(indexData indexEntry) ([]byte, error) {
	var file *os.File
	if indexData.segmentName != nil {
		segmentFile, err := os.Open(*indexData.segmentName)
		if err != nil {
			return nil, err
		}
		defer segmentFile.Close()
		file = segmentFile
	} else {
		file = db.file
	}
	data := make([]byte, indexData.length)
	if _, err := file.ReadAt(data, int64(indexData.offset)); err != nil {
		return nil, err
	}
	return data, nil
}

func (db *SomtoDB) readIndex(index int) (indexEntry, error) {
	if entry, ok := db.currIndexes[index]; ok {
		return entry, nil
	}
	if entry, ok := db.segementedIndexes[index]; ok {
		return entry, nil
	}
	return indexEntry{}, fmt.Errorf("key not found")
}
