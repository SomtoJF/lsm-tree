package main

import (
	"errors"
	"testing"

	"github.com/SomtoJF/lsm-tree/database"
)

type fakeDatabase struct {
	closed bool
	set    func(int, string) (string, error)
}

func (db *fakeDatabase) Set(key int, value string) (string, error) {
	if db.set != nil {
		return db.set(key, value)
	}
	return "", nil
}

func (db *fakeDatabase) Get(int) (string, error) {
	return "", errors.New("not implemented")
}

func (db *fakeDatabase) Close() {
	db.closed = true
}

var _ database.Database = (*fakeDatabase)(nil)

type fakeReader struct {
	inputs []string
	index  int
	closed bool
}

func (reader *fakeReader) Readline() (string, error) {
	if reader.index == len(reader.inputs) {
		return "", errors.New("input exhausted")
	}
	input := reader.inputs[reader.index]
	reader.index++
	return input, nil
}

func (reader *fakeReader) Close() error {
	reader.closed = true
	return nil
}

func TestRunClosesDatabaseWhenExitCommandIsEntered(t *testing.T) {
	db := &fakeDatabase{}
	reader := &fakeReader{inputs: []string{"exit"}}

	run(db, reader)

	if !db.closed {
		t.Fatal("expected database to be closed")
	}
	if !reader.closed {
		t.Fatal("expected line reader to be closed")
	}
}

func TestRunClosesDatabaseWhenDatabasePanics(t *testing.T) {
	db := &fakeDatabase{
		set: func(int, string) (string, error) {
			panic("database failure")
		},
	}
	reader := &fakeReader{inputs: []string{"set 1 value"}}

	defer func() {
		if recover() == nil {
			t.Fatal("expected database panic to propagate")
		}
		if !db.closed {
			t.Fatal("expected database to be closed while unwinding a panic")
		}
	}()

	run(db, reader)
}
