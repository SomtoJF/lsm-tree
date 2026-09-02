package somtodb

import (
	"fmt"
	"os"
)

type SomtoDB struct {
	filePath string
	// maps keys to their corresponding value offsets in the file
	indexes map[int]int
	// number of bytes written to the file
	fileSize int
}

func Init(filePath string) *SomtoDB {
	db := &SomtoDB{}
	db.filePath = filePath
	db.fileSize = 0
	db.indexes = make(map[int]int)
	return db
}

func (db *SomtoDB) write(key int, data []byte) error {
	f, err := os.OpenFile(db.filePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = f.Write(data)
	if err != nil {
		return err
	}

	// store the offset of the value in the file
	db.indexes[key] = db.fileSize
	// increment the file size
	db.fileSize += len(data)
	return nil
}

func (db *SomtoDB) read(offset int) ([]byte, error) {
	f, err := os.Open(db.filePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	_, err = f.Seek(int64(offset), 0)
	if err != nil {
		return nil, err
	}

	data := make([]byte, 100)
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

func (db SomtoDB) Get(key int) (string, error) {
	// TODO: implement
	offset, ok := db.indexes[key]
	if !ok {
		return "", fmt.Errorf("key not found")
	}

	readData, err := db.read(offset)
	if err != nil {
		return "", err
	}
	return string(readData), nil
}
