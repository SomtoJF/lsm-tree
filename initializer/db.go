package initializer

import (
	"log"
	"os"
	"path/filepath"

	"github.com/SomtoJF/lsm-tree/database"
)

func InitDB() database.Database {
	fileName := "data.txt"
	dbDirectory := "database/data"
	dir, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}
	path1 := filepath.Join(dir, dbDirectory)
	database, err := database.NewDatabase(path1, fileName)
	if err != nil {
		log.Fatal(err)
	}

	return database
}
