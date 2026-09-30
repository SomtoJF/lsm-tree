package database

import "github.com/SomtoJF/lsm-tree/database/somtodb"

type Database interface {
	Set(key int, value string) (string, error)
	Get(key int) (string, error)
	Close()
}

func NewDatabase(dbDirectory string, fileName string) (Database, error) {
	return somtodb.Init(dbDirectory, fileName)
}
