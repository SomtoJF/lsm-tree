package somtodb_test

import (
	"path/filepath"
	"strconv"
	"testing"

	"github.com/SomtoJF/lsm-tree/database/somtodb"
)

const benchmarkRecordCount = 1000

func benchmarkDatabase(b *testing.B) *somtodb.SomtoDB {
	b.Helper()

	db, err := somtodb.Init(filepath.Join(b.TempDir(), "benchmark.db"))
	if err != nil {
		b.Fatalf("failed to initialize database: %v", err)
	}
	b.Cleanup(func() {
		db.Close()
	})

	return db
}

func populateBenchmarkDatabase(b *testing.B, db *somtodb.SomtoDB) {
	b.Helper()

	for i := 0; i < benchmarkRecordCount; i++ {
		if _, err := db.Set(i, strconv.Itoa(i)); err != nil {
			b.Fatalf("failed to populate database: %v", err)
		}
	}
}

func BenchmarkDatabaseSet(b *testing.B) {
	db := benchmarkDatabase(b)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if _, err := db.Set(i, strconv.Itoa(i)); err != nil {
			b.Fatalf("failed to set value: %v", err)
		}
	}
}

func BenchmarkDatabaseGet(b *testing.B) {
	db := benchmarkDatabase(b)
	populateBenchmarkDatabase(b, db)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if _, err := db.Get(i % benchmarkRecordCount); err != nil {
			b.Fatalf("failed to get value: %v", err)
		}
	}
}

func BenchmarkDatabaseParallelGet(b *testing.B) {
	db := benchmarkDatabase(b)
	populateBenchmarkDatabase(b, db)
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			if _, err := db.Get(i % benchmarkRecordCount); err != nil {
				b.Errorf("failed to get value: %v", err)
			}
			i++
		}
	})
}
