package database

import "fmt"

type Database interface {
	Set(key string, value string)
	Get(key string) string
}

type SomtoDB struct {
	filePath string
}

func New(filePath string) *SomtoDB {
	return &SomtoDB{
		filePath: filePath,
	}
}

func (db *SomtoDB) Set(key int, value string) string {
	// TODO: implement
	return fmt.Sprintf("Key: %d, Value: %s", key, value)
}

func (db SomtoDB) Get(key int) string {
	// TODO: implement

	return "value"
}
