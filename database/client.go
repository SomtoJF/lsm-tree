package database

import "github.com/SomtoJF/lsm-tree/database/somtodb"

type Database interface {
	Set(key int, value string) (string, error)
	Get(key int) (string, error)
}

func NewDatabase(filePath string) Database {
	return somtodb.Init(filePath)
}
