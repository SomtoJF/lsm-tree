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
	compacting     bool
	compactionWG   sync.WaitGroup

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
	pendingIndexes, err := db.readPendingCompactionIndexes()
	if err != nil {
		return err
	}
	for key, entry := range pendingIndexes {
		segmentedIndexes[key] = entry
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

func (db *SomtoDB) readPendingCompactionIndexes() (map[int]indexEntry, error) {
	result := make(map[int]indexEntry)
	paths, err := filepath.Glob(db.filePath + ".compacting.*")
	if err != nil {
		return nil, err
	}
	for _, path := range paths {
		file, err := os.OpenFile(path, os.O_RDWR, 0644)
		if err != nil {
			return nil, err
		}
		indexes, recoveredSize, err := readFileIndexes(file, path)
		if err != nil {
			_ = file.Close()
			return nil, err
		}
		if err := file.Truncate(recoveredSize); err != nil {
			_ = file.Close()
			return nil, err
		}
		_ = file.Close()
		for key, entry := range indexes {
			filePath := path
			entry.segmentName = &filePath
			result[key] = entry
		}
	}
	return result, nil
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
			_, value, nextOffset, err := decodeRecordAt(dataFile, frameOffset, dataInfo.Size())
			if err != nil || int(nextOffset-frameOffset) != recordHeaderSize+length+recordChecksumSize || len(value) != length {
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

func readFileIndexes(file *os.File, filePath string) (map[int]indexEntry, int64, error) {
	info, err := file.Stat()
	if err != nil {
		return nil, 0, err
	}
	indexes := make(map[int]indexEntry)
	offset := int64(0)
	for offset < info.Size() {
		key, value, nextOffset, err := decodeRecordAt(file, offset, info.Size())
		if err != nil {
			if err == io.ErrUnexpectedEOF {
				return indexes, offset, nil
			}
			if errors.Is(err, errRecordChecksum) && nextOffset == info.Size() {
				return indexes, offset, nil
			}
			return nil, 0, fmt.Errorf("database file %q is corrupt at offset %d: %w", filePath, offset, err)
		}
		indexes[key] = indexEntry{offset: int(offset + recordHeaderSize), length: len(value)}
		offset = nextOffset
	}
	return indexes, offset, nil
}

func (db *SomtoDB) readActiveIndexes() (map[int]indexEntry, int64, error) {
	return readFileIndexes(db.file, db.filePath)
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
	db.compactionWG.Wait()
	db.mutex.Lock()
	defer db.mutex.Unlock()
	if db.file != nil {
		_ = db.file.Close()
		db.file = nil
	}
}

func (db *SomtoDB) waitForCompaction() {
	db.compactionWG.Wait()
}

func (db *SomtoDB) Set(key int, value string) (string, error) {
	text := fmt.Sprintf("key: %d, value: %s\n", key, value)
	request := &writeRequest{key: key, payload: []byte(value), response: make(chan error, 1)}
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
	dirty := false
	for _, request := range batch {
		frame, err := encodeRecord(request.key, request.payload)
		if err != nil {
			db.mutex.Unlock()
			return err
		}
		if err := db.writeLocked(request.key, request.payload, frame); err != nil {
			db.mutex.Unlock()
			return err
		}
		dirty = true
	}
	if dirty {
		if err := db.file.Sync(); err != nil {
			db.mutex.Unlock()
			return err
		}
	}
	shouldCompact := db.fileSize > db.maxSegmentSize && !db.compacting
	db.mutex.Unlock()
	if shouldCompact {
		db.startCompaction()
	}
	return nil
}

func (db *SomtoDB) startCompaction() {
	db.mutex.Lock()
	if db.compacting || len(db.currIndexes) == 0 || db.fileSize <= db.maxSegmentSize {
		db.mutex.Unlock()
		return
	}
	if err := db.file.Sync(); err != nil {
		db.mutex.Unlock()
		return
	}
	snapshotPath := fmt.Sprintf("%s.compacting.%d", db.filePath, time.Now().UnixNano())
	if err := db.file.Close(); err != nil {
		db.mutex.Unlock()
		return
	}
	if err := os.Rename(db.filePath, snapshotPath); err != nil {
		db.mutex.Unlock()
		return
	}
	newFile, err := os.OpenFile(db.filePath, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0644)
	if err != nil {
		db.mutex.Unlock()
		return
	}
	snapshotIndexes := make(map[int]indexEntry, len(db.currIndexes))
	for key, entry := range db.currIndexes {
		entry.segmentName = &snapshotPath
		snapshotIndexes[key] = entry
		db.segementedIndexes[key] = entry
	}
	db.file = newFile
	db.fileSize = 0
	db.currIndexes = make(map[int]indexEntry)
	db.compacting = true
	db.compactionWG.Add(1)
	db.mutex.Unlock()

	go func() {
		defer db.compactionWG.Done()
		db.compactSnapshot(snapshotPath, snapshotIndexes)
	}()
}

func (db *SomtoDB) compactSnapshot(snapshotPath string, indexes map[int]indexEntry) {
	segmentID := db.nextSegmentID()
	dataName := fmt.Sprintf("%d.db", segmentID)
	hintName := fmt.Sprintf("%d.hint", segmentID)
	tempDataPath := filepath.Join(db.segmentDir, dataName+".tmp")
	tempHintPath := filepath.Join(db.segmentDir, hintName+".tmp")
	dataPath := filepath.Join(db.segmentDir, dataName)
	hintPath := filepath.Join(db.segmentDir, hintName)
	sourceFile, err := os.Open(snapshotPath)
	if err != nil {
		db.finishCompaction(snapshotPath, nil, indexes)
		return
	}
	defer sourceFile.Close()
	dataFile, err := os.OpenFile(tempDataPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		db.finishCompaction(snapshotPath, nil, indexes)
		return
	}
	hintFile, err := os.OpenFile(tempHintPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		_ = dataFile.Close()
		db.finishCompaction(snapshotPath, nil, indexes)
		return
	}
	writer := bufio.NewWriter(hintFile)
	hash := crc32.NewIEEE()
	segmentOffset := int64(0)
	newIndexes := make(map[int]indexEntry)
	header := make([]byte, SEGMENT_HEADER_SIZE)
	for key, indexData := range indexes {
		payload := make([]byte, indexData.length)
		if _, err := sourceFile.ReadAt(payload, int64(indexData.offset)); err != nil {
			_ = dataFile.Close()
			_ = hintFile.Close()
			db.finishCompaction(snapshotPath, nil, indexes)
			return
		}
		frame, err := encodeRecord(key, payload)
		if err != nil {
			_ = dataFile.Close()
			_ = hintFile.Close()
			db.finishCompaction(snapshotPath, nil, indexes)
			return
		}
		if _, err := dataFile.Write(frame); err != nil {
			_ = dataFile.Close()
			_ = hintFile.Close()
			db.finishCompaction(snapshotPath, nil, indexes)
			return
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
			db.finishCompaction(snapshotPath, nil, indexes)
			return
		}
		segmentOffset += int64(len(frame))
	}
	if err := writer.Flush(); err != nil {
		_ = dataFile.Close()
		_ = hintFile.Close()
		db.finishCompaction(snapshotPath, nil, indexes)
		return
	}
	if err := dataFile.Sync(); err != nil {
		_ = dataFile.Close()
		_ = hintFile.Close()
		db.finishCompaction(snapshotPath, nil, indexes)
		return
	}
	if err := hintFile.Sync(); err != nil {
		_ = dataFile.Close()
		_ = hintFile.Close()
		db.finishCompaction(snapshotPath, nil, indexes)
		return
	}
	if err := dataFile.Close(); err != nil {
		_ = hintFile.Close()
		db.finishCompaction(snapshotPath, nil, indexes)
		return
	}
	if err := hintFile.Close(); err != nil {
		db.finishCompaction(snapshotPath, nil, indexes)
		return
	}
	if err := os.Rename(tempDataPath, dataPath); err != nil {
		db.finishCompaction(snapshotPath, nil, indexes)
		return
	}
	if err := os.Rename(tempHintPath, hintPath); err != nil {
		db.finishCompaction(snapshotPath, nil, indexes)
		return
	}
	db.mutex.Lock()
	newSegments := append(append([]manifestSegment(nil), db.segments...), manifestSegment{ID: segmentID, Data: dataName, Hint: hintName, DataSize: segmentOffset, DataCRC: hash.Sum32()})
	db.mutex.Unlock()
	if err := publishManifest(db.segmentDir, manifest{Version: manifestVersion, Segments: newSegments}); err != nil {
		db.finishCompaction(snapshotPath, nil, nil)
		return
	}
	db.finishCompaction(snapshotPath, newSegments, newIndexes)
}

func (db *SomtoDB) nextSegmentID() int {
	db.mutex.Lock()
	defer db.mutex.Unlock()
	db.segmentCount++
	return db.segmentCount
}

func (db *SomtoDB) finishCompaction(snapshotPath string, segments []manifestSegment, indexes map[int]indexEntry) {
	db.mutex.Lock()
	if segments != nil {
		db.segments = segments
		for key, entry := range indexes {
			db.segementedIndexes[key] = entry
		}
	} else {
		for key, entry := range indexes {
			db.segementedIndexes[key] = entry
		}
	}
	if segments != nil && db.file != nil {
		_ = os.Remove(snapshotPath)
		_ = syncDirectory(db.dbDir)
	}
	db.compacting = false
	db.mutex.Unlock()
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
	return string(readData), nil
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
