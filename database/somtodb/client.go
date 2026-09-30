package somtodb

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const SEGMENT_HEADER_SIZE = 28

type SomtoDB struct {
	dbFileName string
	dbDir      string
	filePath   string
	// maps keys to their corresponding value offsets in the file
	currIndexes map[int]indexEntry
	// holds indexes for segmented keys i.e previously written keys to previous segments
	segementedIndexes map[int]indexEntry
	// number of bytes written to the file
	fileSize int
	// mutex to protect concurrent access to the database
	mutex sync.Mutex
	// max segment size in bytes
	maxSegmentSize int
	// db file
	file *os.File
	// Number of segments in the database
	segmentCount int
	// Directory where segments are stored
	segmentDir string
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

func Init(dbDir string, dbFileName string) (*SomtoDB, error) {
	if dbDir == "" || dbFileName == "" {
		return nil, fmt.Errorf("database directory and file name must be provided")
	}

	if err := os.MkdirAll(dbDir, 0755); err != nil {
		return nil, err
	}

	filePath := filepath.Join(dbDir, dbFileName)
	f, err := os.OpenFile(filePath, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0644)
	if err != nil {
		return nil, err
	}

	db := &SomtoDB{
		dbFileName:        dbFileName,
		dbDir:             dbDir,
		filePath:          filePath,
		currIndexes:       make(map[int]indexEntry),
		segementedIndexes: make(map[int]indexEntry),
		file:              f,
		maxSegmentSize:    100,
		segmentDir:        filepath.Join(dbDir, "segments"),
	}

	if err := os.MkdirAll(db.segmentDir, 0755); err != nil {
		_ = f.Close()
		return nil, err
	}

	fileInfo, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	if fileInfo.Size() > int64(^uint(0)>>1) {
		_ = f.Close()
		return nil, fmt.Errorf("active database file is too large for this platform")
	}

	currIndexes, segmentedIndexes, segmentCount, err := db.readIndexes(db.segmentDir, db.filePath)
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	db.currIndexes = currIndexes
	db.segementedIndexes = segmentedIndexes
	db.fileSize = int(fileInfo.Size())
	db.segmentCount = segmentCount
	return db, nil
}

func (db *SomtoDB) readIndexes(segmentDir string, dbFilePath string) (map[int]indexEntry, map[int]indexEntry, int, error) {
	entries, err := os.ReadDir(segmentDir)
	if err != nil {
		return nil, nil, 0, err
	}

	segmentIDs := make([]int, 0)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".hint") {
			continue
		}
		idText := strings.TrimSuffix(entry.Name(), ".hint")
		id, err := strconv.Atoi(idText)
		if err != nil || id < 1 {
			return nil, nil, 0, fmt.Errorf("invalid segment hint filename %q", entry.Name())
		}
		segmentIDs = append(segmentIDs, id)
	}
	sort.Ints(segmentIDs)

	segmentedIndexes := make(map[int]indexEntry)
	for _, id := range segmentIDs {
		hintPath := filepath.Join(segmentDir, fmt.Sprintf("%d.hint", id))
		hintData, err := os.ReadFile(hintPath)
		if err != nil {
			return nil, nil, 0, fmt.Errorf("read segment hint %q: %w", hintPath, err)
		}
		if len(hintData)%SEGMENT_HEADER_SIZE != 0 {
			return nil, nil, 0, fmt.Errorf("segment hint %q has a truncated entry", hintPath)
		}

		dataPath := filepath.Join(segmentDir, fmt.Sprintf("%d.txt", id))
		dataInfo, err := os.Stat(dataPath)
		if err != nil {
			return nil, nil, 0, fmt.Errorf("stat segment data %q: %w", dataPath, err)
		}
		dataSize := uint64(dataInfo.Size())

		for offset := 0; offset < len(hintData); offset += SEGMENT_HEADER_SIZE {
			entry := hintData[offset : offset+SEGMENT_HEADER_SIZE]
			rawKey := binary.LittleEndian.Uint64(entry[8:16])
			key64 := int64(rawKey)
			key := int(key64)
			if int64(key) != key64 {
				return nil, nil, 0, fmt.Errorf("key %d in %q cannot be represented as int", key64, hintPath)
			}

			length := binary.LittleEndian.Uint32(entry[16:20])
			dataOffset := binary.LittleEndian.Uint64(entry[20:28])
			if dataOffset > dataSize || uint64(length) > dataSize-dataOffset {
				return nil, nil, 0, fmt.Errorf("segment hint %q contains an out-of-range data entry for key %d", hintPath, key)
			}
			if dataOffset > uint64(^uint(0)>>1) || uint64(length) > uint64(^uint(0)>>1) {
				return nil, nil, 0, fmt.Errorf("segment hint %q contains an entry too large for this platform", hintPath)
			}

			dataFileName := dataPath
			segmentedIndexes[key] = indexEntry{
				offset:      int(dataOffset),
				length:      int(length),
				segmentName: &dataFileName,
			}
		}
	}

	activeData, err := os.ReadFile(dbFilePath)
	if err != nil {
		return nil, nil, 0, fmt.Errorf("read active database file %q: %w", dbFilePath, err)
	}
	currIndexes := make(map[int]indexEntry)
	for recordOffset := 0; recordOffset < len(activeData); {
		recordEnd := bytes.IndexByte(activeData[recordOffset:], '\n')
		if recordEnd < 0 {
			return nil, nil, 0, fmt.Errorf("active database file %q ends with a truncated record", dbFilePath)
		}
		recordEnd += recordOffset + 1
		record := string(activeData[recordOffset:recordEnd])
		keyText, _, ok := strings.Cut(strings.TrimSuffix(record, "\n"), ", value: ")
		if !ok || !strings.HasPrefix(keyText, "key: ") {
			return nil, nil, 0, fmt.Errorf("active database file %q contains a malformed record at offset %d", dbFilePath, recordOffset)
		}
		key, err := strconv.Atoi(strings.TrimPrefix(keyText, "key: "))
		if err != nil {
			return nil, nil, 0, fmt.Errorf("active database file %q contains an invalid key at offset %d: %w", dbFilePath, recordOffset, err)
		}
		currIndexes[key] = indexEntry{
			offset: recordOffset,
			length: recordEnd - recordOffset,
		}
		recordOffset = recordEnd
	}

	segmentCount := 0
	if len(segmentIDs) > 0 {
		segmentCount = segmentIDs[len(segmentIDs)-1]
	}
	return currIndexes, segmentedIndexes, segmentCount, nil
}

func (db *SomtoDB) Close() {
	db.mutex.Lock()
	defer db.mutex.Unlock()
	if db.file != nil {
		_ = db.file.Close()
		db.file = nil
	}
}

func (db *SomtoDB) Set(key int, value string) (string, error) {
	text := fmt.Sprintf("key: %d, value: %s\n", key, value)
	textbytes := []byte(text)

	db.mutex.Lock()
	defer db.mutex.Unlock()

	if len(textbytes)+db.fileSize > db.maxSegmentSize {
		if _, err := db.runCompaction(); err != nil {
			return "", err
		}
		if err := db.clearCurrentDBFile(); err != nil {
			return "", err
		}
	}

	if err := db.writeLocked(key, textbytes); err != nil {
		return "", err
	}
	return text, nil
}

func (db *SomtoDB) runCompaction() (compactionResult, error) {
	if len(db.currIndexes) == 0 {
		return compactionResult{}, nil
	}

	db.segmentCount++
	segmentID := db.segmentCount
	segmentHintFileName := filepath.Join(db.segmentDir, fmt.Sprintf("%d.hint", segmentID))
	segmentDataFileName := filepath.Join(db.segmentDir, fmt.Sprintf("%d.txt", segmentID))

	segmentHintFile, err := os.Create(segmentHintFileName)
	if err != nil {
		return compactionResult{}, err
	}
	defer segmentHintFile.Close()

	segmentDataFile, err := os.Create(segmentDataFileName)
	if err != nil {
		return compactionResult{}, err
	}
	defer segmentDataFile.Close()

	writer := bufio.NewWriter(segmentHintFile)
	headerBuf := make([]byte, SEGMENT_HEADER_SIZE)
	segmentOffset := 0
	for key, indexData := range db.currIndexes {
		payload := make([]byte, indexData.length)
		if _, err := db.file.ReadAt(payload, int64(indexData.offset)); err != nil {
			return compactionResult{}, err
		}
		if _, err := segmentDataFile.Write(payload); err != nil {
			return compactionResult{}, err
		}

		newEntry := indexEntry{
			offset:      segmentOffset,
			length:      indexData.length,
			segmentName: &segmentDataFileName,
		}
		db.segementedIndexes[key] = newEntry

		hintEntry := hintEntry{
			Timestamp: uint64(time.Now().UnixNano()),
			Key:       uint64(key),
			Length:    uint32(newEntry.length),
			Offset:    uint64(newEntry.offset),
		}
		binary.LittleEndian.PutUint64(headerBuf[0:8], hintEntry.Timestamp)
		binary.LittleEndian.PutUint64(headerBuf[8:16], hintEntry.Key)
		binary.LittleEndian.PutUint32(headerBuf[16:20], hintEntry.Length)
		binary.LittleEndian.PutUint64(headerBuf[20:28], hintEntry.Offset)
		if _, err := writer.Write(headerBuf[:SEGMENT_HEADER_SIZE]); err != nil {
			return compactionResult{}, err
		}

		segmentOffset += indexData.length
	}
	if err := writer.Flush(); err != nil {
		return compactionResult{}, err
	}

	return compactionResult{
		segmentFileName:     segmentHintFileName,
		segmentDataFileName: segmentDataFileName,
	}, nil
}

func (db *SomtoDB) clearCurrentDBFile() error {
	if db.file == nil {
		return fmt.Errorf("database file is closed")
	}
	if _, err := db.file.Seek(0, 0); err != nil {
		return err
	}
	if err := db.file.Truncate(0); err != nil {
		return err
	}
	db.fileSize = 0
	db.currIndexes = make(map[int]indexEntry)
	return nil
}

func (db *SomtoDB) writeLocked(key int, data []byte) error {
	if _, err := db.file.Seek(0, io.SeekEnd); err != nil {
		return err
	}
	if _, err := db.file.Write(data); err != nil {
		return err
	}

	entry := indexEntry{
		offset: db.fileSize,
		length: len(data),
	}
	db.currIndexes[key] = entry
	db.fileSize += len(data)
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
