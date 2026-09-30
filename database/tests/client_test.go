package somtodb_test

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/SomtoJF/lsm-tree/database"
	"github.com/google/uuid"
)

func getFilePaths(t *testing.T) (dbDir string, dbFileName string) {
	t.Helper()
	fileName := "data.txt"
	return t.TempDir(), fileName
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
	dbDir, fileName := getFilePaths(t)
	db, err := database.NewDatabase(dbDir, fileName)
	if err != nil {
		t.Fatalf("failed to initialize database: %v", err)
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
	dbDir, fileName := getFilePaths(t)
	db, err := database.NewDatabase(dbDir, fileName)
	if err != nil {
		t.Fatalf("failed to initialize database: %v", err)
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
	dbDir, fileName := getFilePaths(t)
	db, err := database.NewDatabase(dbDir, fileName)
	if err != nil {
		t.Fatalf("failed to initialize database: %v", err)
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

func TestDatabaseRestoresActiveIndexesOnReopen(t *testing.T) {
	dbDir, fileName := getFilePaths(t)
	db, err := database.NewDatabase(dbDir, fileName)
	if err != nil {
		t.Fatalf("failed to initialize database: %v", err)
	}

	if _, err := db.Set(1, "initial"); err != nil {
		t.Fatalf("failed to set initial value: %v", err)
	}
	if _, err := db.Set(1, "updated"); err != nil {
		t.Fatalf("failed to update value: %v", err)
	}
	if _, err := db.Set(2, "another"); err != nil {
		t.Fatalf("failed to set second key: %v", err)
	}
	db.Close()

	db, err = database.NewDatabase(dbDir, fileName)
	if err != nil {
		t.Fatalf("failed to reopen database: %v", err)
	}
	defer db.Close()

	for key, expected := range map[int]string{1: "updated", 2: "another"} {
		got, err := db.Get(key)
		if err != nil {
			t.Fatalf("failed to get key %d after reopen: %v", key, err)
		}
		if got != expected {
			t.Errorf("key %d: expected %q, got %q", key, expected, got)
		}
	}
}

func TestDatabaseRestoresSegmentIndexesOnReopen(t *testing.T) {
	dbDir, fileName := getFilePaths(t)
	db, err := database.NewDatabase(dbDir, fileName)
	if err != nil {
		t.Fatalf("failed to initialize database: %v", err)
	}

	oldValue := strings.Repeat("a", 400)
	middleValue := strings.Repeat("b", 400)
	newValue := strings.Repeat("c", 400)
	activeValue := strings.Repeat("d", 400)

	for key, value := range map[int]string{1: oldValue, 2: oldValue, 3: middleValue} {
		if _, err := db.Set(key, value); err != nil {
			t.Fatalf("failed to set key %d: %v", key, err)
		}
	}
	if _, err := db.Set(1, newValue); err != nil {
		t.Fatalf("failed to overwrite segmented key: %v", err)
	}
	if _, err := db.Set(4, activeValue); err != nil {
		t.Fatalf("failed to set active key: %v", err)
	}
	db.Close()

	db, err = database.NewDatabase(dbDir, fileName)
	if err != nil {
		t.Fatalf("failed to reopen database: %v", err)
	}
	defer db.Close()

	expectedValues := map[int]string{
		1: newValue,
		2: oldValue,
		3: middleValue,
		4: activeValue,
	}
	for key, expected := range expectedValues {
		got, err := db.Get(key)
		if err != nil {
			t.Fatalf("failed to get key %d after reopen: %v", key, err)
		}
		if got != expected {
			t.Errorf("key %d: recovered value does not match", key)
		}
	}
}

func TestDatabaseRejectsTruncatedHintOnReopen(t *testing.T) {
	dbDir, fileName := getFilePaths(t)
	db, err := database.NewDatabase(dbDir, fileName)
	if err != nil {
		t.Fatalf("failed to initialize database: %v", err)
	}
	value := strings.Repeat("x", 400)
	for key := 1; key <= 3; key++ {
		if _, err := db.Set(key, value); err != nil {
			t.Fatalf("failed to set key %d: %v", key, err)
		}
	}
	db.Close()

	hintPath := filepath.Join(dbDir, "segments", "1.hint")
	if err := os.WriteFile(hintPath, []byte{1}, 0644); err != nil {
		t.Fatalf("failed to corrupt hint for test: %v", err)
	}
	if reopened, err := database.NewDatabase(dbDir, fileName); err == nil {
		reopened.Close()
		t.Fatal("expected initialization to reject truncated hint")
	}
}
