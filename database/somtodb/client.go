package somtodb

import (
	"fmt"
	"os"
	"strings"
	"sync"
)

type SomtoDB struct {
	filePath string
	// maps keys to their corresponding value offsets in the file
	indexes map[int]indexEntry
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

func Init(filePath string) (*SomtoDB, error) {
	db := &SomtoDB{}
	db.filePath = filePath
	db.fileSize = 0
	db.indexes = make(map[int]indexEntry)

	// Create the directory if it doesn't exist
	dbDirectory := strings.Join(strings.Split(db.filePath, "/")[:len(strings.Split(db.filePath, "/"))-1], "/")

	err := os.MkdirAll(dbDirectory, 0755)
	if err != nil {
		return nil, err
	}

	f, err := os.OpenFile(db.filePath, os.O_CREATE|os.O_APPEND|os.O_RDWR, 0644)
	if err != nil {
		return nil, err
	}
	db.file = f

	segmentCount, segmentDir, err := db.initSegments(dbDirectory)
	if err != nil {
		return nil, err
	}
	db.segmentCount = segmentCount
	db.segmentDir = segmentDir

	currIndexes, segmentedIndexes, err := db.readIndexes(segmentDir, segmentCount, db.filePath)
	if err != nil {
		return nil, err
	}
	db.indexes = currIndexes
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

func (db *SomtoDB) initSegments(dbDir string) (segmentCount int, segmentDir string, err error) {
	segmentsDir := dbDir + "/segments"

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
	db.indexes[key] = entry
	// increment the file size
	db.fileSize += dataLength
	return nil
}

func (db *SomtoDB) read(indexData indexEntry) ([]byte, error) {
	file := db.file

	if indexData.segmentName != nil {
		segmentFile, err := db.getSegmentFile(*indexData.segmentName)
		if err != nil {
			return nil, err
		}
		file = segmentFile
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

	runCompaction := false
	if len(textbytes)+db.fileSize > db.maxSegmentSize {
		// TODO: Build new segment (.hint) from current indexes
		// TODO: Clear the curr db file and return it's contents
		runCompaction = true
	}
	if runCompaction {
		// TODO: remove all duplicate keys from the current db file contents and write it to it's corresponding segment data file
	}

	// Write to current indexes
	err := db.write(key, textbytes)
	if err != nil {
		return "", err
	}
	return text, nil
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

func (db *SomtoDB) readIndex(index int) (indexEntry, error) {
	indexData, ok := db.indexes[index]
	if !ok {
		indexData, ok = db.segementedIndexes[index]
		if !ok {
			return indexEntry{}, fmt.Errorf("key not found")
		}
	}
	return indexData, nil
}
