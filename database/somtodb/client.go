package somtodb

import (
	"fmt"
	"os"
)

type SomtoDB struct {
	filePath string
}

func Init(filePath string) *SomtoDB {
	db := &SomtoDB{}
	db.filePath = filePath
	return db
}

func (db *SomtoDB) write(data []byte) error {
	f, err := os.OpenFile(db.filePath, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = f.Write(data)
	if err != nil {
		return err
	}
	return nil
}

func (db *SomtoDB) Set(key int, value string) string {
	// TODO: implement
	text := fmt.Sprintf("key: %d, value: %s", key, value)
	textbytes := []byte(text)
	err := db.write(textbytes)
	if err != nil {
		return "Error: failed to write to file"
	}
	return text
}

func (db SomtoDB) Get(key int) string {
	// TODO: implement

	return "value"
}
