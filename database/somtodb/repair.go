package somtodb

import (
	"io"
	"os"
	"path/filepath"
)

// Repair copies the latest valid records from a damaged database into a new one.
// It preserves valid segment data and the valid prefix of the active file.
func Repair(sourceDir, sourceFileName, targetDir, targetFileName string) error {
	sourcePath := filepath.Join(sourceDir, sourceFileName)
	file, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer file.Close()

	source := &SomtoDB{file: file, filePath: sourcePath, segmentDir: filepath.Join(sourceDir, "segments")}
	loadedManifest, err := loadManifest(source.segmentDir)
	if err != nil {
		return err
	}
	segmentIndexes, err := source.readSegmentIndexes(loadedManifest)
	if err != nil {
		return err
	}
	records := make(map[int][]byte, len(segmentIndexes))
	for key, index := range segmentIndexes {
		payload, err := source.read(index)
		if err != nil {
			return err
		}
		records[key] = payload
	}

	info, err := file.Stat()
	if err != nil {
		return err
	}
	for offset := int64(0); offset < info.Size(); {
		key, value, nextOffset, err := decodeRecordAt(file, offset, info.Size())
		if err != nil || nextOffset == offset || err == io.EOF {
			break
		}
		records[key] = value
		offset = nextOffset
	}

	target, err := Init(targetDir, targetFileName)
	if err != nil {
		return err
	}
	defer target.Close()
	for key, payload := range records {
		if _, err := target.Set(key, string(payload)); err != nil {
			return err
		}
	}
	return nil
}
