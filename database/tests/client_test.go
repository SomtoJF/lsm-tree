package somtodb_test

import (
	"log"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/SomtoJF/lsm-tree/database/somtodb"
	"github.com/google/uuid"
)

func getFilePath() string {
	fileName := "test_data.txt"
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
	if err := clearDBFile(filePath); err != nil {
		// ignore missing file during the first run; the database init will recreate it
		_ = err
	}
	db, err := somtodb.Init(filePath)
	if err != nil {
		t.Errorf("failed to initialize database: %v", err)
	}

	defer db.Close()

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
func TestDatabaseHandlesConcurrentReads(t *testing.T) {
	// setup
	filePath := getFilePath()
	if err := clearDBFile(filePath); err != nil {
		// ignore missing file during the first run; the database init will recreate it
		_ = err
	}
	db, err := somtodb.Init(filePath)
	if err != nil {
		t.Errorf("failed to initialize database: %v", err)
	}

	defer db.Close()

	testCases := generateRandomTestCases(20)
	for _, testCase := range testCases {
		_, err := db.Set(testCase.key, testCase.value)
		if err != nil {
			t.Errorf("failed to set key-value pair: %v", err)
		}
	}

	wg := sync.WaitGroup{}
	wg.Add(len(testCases))

	for _, tc := range testCases {
		go func(tc testCase) {
			defer wg.Done()
			value, err := db.Get(tc.key)
			if err != nil {
				t.Errorf("failed to get value for key: %v", err)
			}
			if value != tc.value {
				t.Errorf("expected value: %s, got: %s", tc.value, value)
			}
		}(tc)

	}

	wg.Wait()
}

func TestDatabaseHandlesConcurrentWrites(t *testing.T) {
	// setup
	filePath := getFilePath()
	if err := clearDBFile(filePath); err != nil {
		// ignore missing file during the first run; the database init will recreate it
		_ = err
	}
	db, err := somtodb.Init(filePath)
	if err != nil {
		t.Errorf("failed to initialize database: %v", err)
	}

	defer db.Close()

	wg := sync.WaitGroup{}
	testCases := generateRandomTestCases(20)
	wg.Add(len(testCases))
	for _, tc := range testCases {
		go func(tc testCase) {
			defer wg.Done()
			_, err := db.Set(tc.key, tc.value)
			if err != nil {
				t.Errorf("failed to set key-value pair: %v", err)
			}
		}(tc)
	}

	wg.Wait()

	for _, tc := range testCases {

		value, err := db.Get(tc.key)
		if err != nil {
			t.Errorf("failed to get value for key: %v", err)
		}
		if value != tc.value {
			t.Errorf("expected value: %s, got: %s", tc.value, value)
		}

	}
}
