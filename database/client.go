package database

import "github.com/SomtoJF/lsm-tree/database/somtodb"

type Database interface {
	Set(key int, value string) string
	Get(key int) string
}

func NewDatabase(filePath string) Database {
	return somtodb.Init(filePath)
}
