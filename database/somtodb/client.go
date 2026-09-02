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
	// number of bytes written to the file
	fileSize int
	// mutex to protect concurrent access to the database
	mutex sync.Mutex
}

type indexEntry struct {
	offset int
	length int
}

func Init(filePath string) *SomtoDB {
	db := &SomtoDB{}
	db.filePath = filePath
	db.fileSize = 0
	db.indexes = make(map[int]indexEntry)
	return db
}

func (db *SomtoDB) write(key int, data []byte) error {
	db.mutex.Lock()
	defer db.mutex.Unlock()
	f, err := os.OpenFile(db.filePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = f.Write(data)
	if err != nil {
		return err
	}

	entry := indexEntry{
		offset: db.fileSize,
		length: len(data),
	}

	// store the offset of the value in the file
	db.indexes[key] = entry
	// increment the file size
	db.fileSize += len(data)
	return nil
}

func (db *SomtoDB) read(indexData indexEntry) ([]byte, error) {
	f, err := os.Open(db.filePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	_, err = f.Seek(int64(indexData.offset), 0)
	if err != nil {
		return nil, err
	}

	data := make([]byte, indexData.length)
	_, err = f.Read(data)
	if err != nil {
		return nil, err
	}

	return data, nil
}

func (db *SomtoDB) Set(key int, value string) (string, error) {
	// TODO: implement
	text := fmt.Sprintf("key: %d, value: %s\n", key, value)
	textbytes := []byte(text)
	err := db.write(key, textbytes)
	if err != nil {
		return "", err
	}
	return text, nil
}

func (db *SomtoDB) Get(key int) (string, error) {
	db.mutex.Lock()
	defer db.mutex.Unlock()

	indexData, ok := db.indexes[key]
	if !ok {
		return "", fmt.Errorf("key not found")
	}

	readData, err := db.read(indexData)
	if err != nil {
		return "", err
	}

	value := strings.TrimPrefix(string(readData), fmt.Sprintf("key: %d, value: ", key))
	value = strings.TrimSuffix(value, "\n")
	return value, nil
}
