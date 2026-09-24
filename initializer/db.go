package initializer

import (
	"log"
	"os"
	"path/filepath"

	"github.com/SomtoJF/lsm-tree/database"
)

func InitDB() database.Database {
	fileName := "data.txt"
	dir, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}
	path1 := filepath.Join(dir, fileName)
	database, err := database.NewDatabase(path1)
	if err != nil {
		log.Fatal(err)
	}

	return database
}
