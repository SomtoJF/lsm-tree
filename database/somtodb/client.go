package somtodb

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"maps"
	"os"
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

func Init(dbDir string, dbFileName string) (*SomtoDB, error) {
	db := &SomtoDB{}
	db.dbFileName = dbFileName
	db.fileSize = 0
	db.currIndexes = make(map[int]indexEntry)

	err := os.MkdirAll(dbDir, 0755)
	if err != nil {
		return nil, err
	}

	f, err := os.OpenFile(db.filePath, os.O_CREATE|os.O_APPEND|os.O_RDWR, 0644)
	if err != nil {
		return nil, err
	}
	db.file = f

	segmentCount, segmentDir, err := db.initSegments()
	if err != nil {
		return nil, err
	}
	db.segmentCount = segmentCount
	db.segmentDir = segmentDir

	currIndexes, segmentedIndexes, err := db.readIndexes(segmentDir, segmentCount, db.filePath)
	if err != nil {
		return nil, err
	}
	db.currIndexes = currIndexes
	db.segementedIndexes = segmentedIndexes

	// Good impl here would be a couple of mbs but for testing purposes, we can set it to a small value
	db.maxSegmentSize = 100
	return db, nil
}

func (db *SomtoDB) readIndexes(segmentDir string, segmentCount int, dbfilePath string) (currIndexes map[int]indexEntry, segmentedIndexes map[int]indexEntry, err error) {
	currIndexes = make(map[int]indexEntry)
	segmentedIndexes = make(map[int]indexEntry)
	// TODO: Read all the indexes from the segments and add to segmented indexes
	// TODO: read the db file and build current data indexes
	return currIndexes, segmentedIndexes, nil
}

func (db *SomtoDB) initSegments() (segmentCount int, segmentDir string, err error) {
	segmentsDir := db.dbDir + "/segments"

	err = os.MkdirAll(segmentsDir, 0755)
	if err != nil {
		return 0, "", err
	}

	// count .hint files in the segments directory
	entries, err := os.ReadDir(segmentsDir)
	if err != nil {
		return 0, segmentsDir, err
	}

	count := 0
	for _, entry := range entries {
		// Only count files, skip sub-directories
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".hint") {
			count++
		}
	}
	return count, segmentsDir, nil
}

func (db *SomtoDB) Close() {
	db.file.Close()
}

func (db *SomtoDB) Set(key int, value string) (string, error) {
	text := fmt.Sprintf("key: %d, value: %s\n", key, value)
	textbytes := []byte(text)
	// TODO: if adding this to the filesize would exceed the max segment size, create a new segment
	// For each segment, (.hint) file, there needs to be a corresponding data file. i.e segments/1.hint and segments/1.txt. hint file contains offsets, sizes of the records in the corresponding data file
	/**
	STEPS:
	1. Check if adding this to the filesize would exceed the max segment size
	2. If yes, Build new segment from current indexes
	3. Clear the db file and exec the new write in a new go routine while we run compaction in a new go routine
	4. In a separate go routine, write the new segment to disk with a filename <segmentCount + 1>.hint
	*/

	if len(textbytes)+db.fileSize > db.maxSegmentSize {
		_, err := db.runCompaction()
		if err != nil {
			return "", err
		}

		// Clear the curr db file
		db.clearCurrentDBFile()
	}

	// Write to current indexes
	err := db.write(key, textbytes)
	if err != nil {
		return "", err
	}
	return text, nil
}

type compactionResult struct {
	segmentFileName     string
	segmentDataFileName string
}

func (db *SomtoDB) runCompaction() (compactionResult, error) {
	segmentHintFileName := fmt.Sprintf("%s/%d.hint", db.segmentDir, db.segmentCount+1)
	segmentHintFile, err := os.Create(segmentHintFileName)
	if err != nil {
		return compactionResult{}, err
	}

	segmentDataFileName := fmt.Sprintf("%s/%d.txt", db.segmentDir, db.segmentCount+1)
	segmentDataFile, err := os.Create(segmentDataFileName)
	if err != nil {
		return compactionResult{}, err
	}

	db.segmentCount++

	defer segmentHintFile.Close()
	defer segmentDataFile.Close()

	writer := bufio.NewWriter(segmentHintFile)

	// Compact the current db file into a new segments/<id>.txt file
	segmentSize := 0
	segmentIndexes := make(map[int]indexEntry)
	for key, indexData := range db.currIndexes {
		newEntry, err := db.appendToSegmentDataFile(segmentDataFile, key, indexData, segmentSize)
		if err != nil {
			return compactionResult{}, err
		}

		newEntry.segmentName = &segmentDataFileName

		hintEntry := hintEntry{
			Timestamp: uint64(time.Now().Unix()),
			Key:       uint64(key),
			Length:    uint32(newEntry.length),
			Offset:    uint64(newEntry.offset),
		}

		// Build a new segments/<id>.hint file with the new segment data
		headerBuf := make([]byte, SEGMENT_HEADER_SIZE)
		if err := db.appendToSegmentHintFile(writer, hintEntry, headerBuf); err != nil {
			return compactionResult{}, err
		}

		segmentIndexes[key] = newEntry
		segmentSize += indexData.length
	}

	maps.Copy(db.segementedIndexes, segmentIndexes)
	return compactionResult{}, nil
}

func (db *SomtoDB) clearCurrentDBFile() error {
	db.fileSize = 0
	db.currIndexes = make(map[int]indexEntry)
	return os.Truncate(db.filePath, 0)
}

type hintEntry struct {
	Timestamp uint64
	Key       uint64
	Length    uint32
	Offset    uint64
}

func (db *SomtoDB) appendToSegmentHintFile(w io.Writer, entry hintEntry, headerBuf []byte) error {

	// Serialize fixed-width fields into the 22-byte buffer
	binary.LittleEndian.PutUint64(headerBuf[0:8], entry.Timestamp)
	binary.LittleEndian.PutUint64(headerBuf[8:16], entry.Key)
	binary.LittleEndian.PutUint32(headerBuf[16:20], entry.Length)
	binary.LittleEndian.PutUint64(headerBuf[20:28], entry.Offset)

	// 1. Write the header
	if _, err := w.Write(headerBuf[:SEGMENT_HEADER_SIZE]); err != nil {
		return err
	}

	// 2. Write the variable-length key
	// Create an 8-byte buffer
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, entry.Key)
	if _, err := w.Write(buf); err != nil {
		return err
	}

	return nil
}

func (db *SomtoDB) appendToSegmentDataFile(segmentDataFile *os.File, key int, value indexEntry, oldSegmentSize int) (newEntry indexEntry, err error) {
	currFile := db.file
	data := make([]byte, value.length)
	_, err = currFile.ReadAt(data, int64(value.offset))
	if err != nil {
		return indexEntry{}, err
	}

	_, err = segmentDataFile.Write(data)
	if err != nil {
		return indexEntry{}, err
	}
	return indexEntry{
		offset: oldSegmentSize,
		length: value.length,
	}, nil
}

func (db *SomtoDB) write(key int, data []byte) error {
	db.mutex.Lock()
	defer db.mutex.Unlock()

	_, err := db.file.Write(data)
	if err != nil {
		return err
	}

	dataLength := len(data)

	entry := indexEntry{
		offset: db.fileSize,
		length: dataLength,
	}

	// store the offset of the value in the file
	db.currIndexes[key] = entry
	// increment the file size
	db.fileSize += dataLength
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
	file := db.file

	if indexData.segmentName != nil {
		segmentFile, err := db.getSegmentFile(*indexData.segmentName)
		if err != nil {
			return nil, err
		}
		file = segmentFile
		defer segmentFile.Close()
	}
	data := make([]byte, indexData.length)
	_, err := file.ReadAt(data, int64(indexData.offset))
	if err != nil {
		return nil, err
	}

	return data, nil
}

func (db *SomtoDB) getSegmentFile(segmentName string) (*os.File, error) {
	f, err := os.OpenFile(segmentName, os.O_CREATE|os.O_APPEND|os.O_RDWR, 0644)
	if err != nil {
		return nil, err
	}
	return f, nil
}

func (db *SomtoDB) readIndex(index int) (indexEntry, error) {
	indexData, ok := db.currIndexes[index]
	if !ok {
		indexData, ok = db.segementedIndexes[index]
		if !ok {
			return indexEntry{}, fmt.Errorf("key not found")
		}
	}
	return indexData, nil
}
