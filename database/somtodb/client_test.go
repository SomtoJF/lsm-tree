package somtodb

import (
	"log"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
)

func getFilePath() string {
	fileName := "data.txt"
	dir, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}
	path1 := filepath.Join(dir, fileName)
	return path1
}

func clearDBFile(filePath string) error {
	f, err := os.OpenFile(filePath, os.O_TRUNC|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}

	// write an empty string to the file to clear its contents
	if err := os.Truncate(filePath, 0); err != nil {
		return err
	}
	defer f.Close()
	return nil
}

type testCase struct {
	key   int
	value string
}

func generateRandomTestCases(n int) []testCase {
	testCases := make([]testCase, n)
	for i := 0; i < n; i++ {
		testCases[i] = testCase{
			key:   i,
			value: uuid.New().String(),
		}
	}
	return testCases
}

// TestHelloName calls greetings.Hello with a name, checking
// for a valid return value.
func TestDatabaseSetsValues(t *testing.T) {
	// setup
	filePath := getFilePath()
	db := Init("testdb.txt")
	defer clearDBFile(filePath)

	testCases := generateRandomTestCases(10)
	for _, testCase := range testCases {
		_, err := db.Set(testCase.key, testCase.value)
		if err != nil {
			t.Errorf("failed to set key-value pair: %v", err)
		}
	}

	for _, testCase := range testCases {
		value, err := db.Get(testCase.key)
		if err != nil {
			t.Errorf("failed to get value for key: %v", err)
		}
		if value != testCase.value {
			t.Errorf("expected value: %s, got: %s", testCase.value, value)
		}
	}

}

// TestHelloEmpty calls greetings.Hello with an empty string,
// checking for an error.
func TestDatabaseHandlesConcurrency(t *testing.T) {

}
