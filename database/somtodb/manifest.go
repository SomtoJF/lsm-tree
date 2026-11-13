package somtodb

import (
	"encoding/json"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"path/filepath"
)

const (
	manifestName         = "MANIFEST"
	previousManifestName = "MANIFEST.prev"
	manifestVersion      = 1
)

type manifest struct {
	Version  int               `json:"version"`
	Segments []manifestSegment `json:"segments"`
}

type manifestSegment struct {
	ID       int    `json:"id"`
	Data     string `json:"data"`
	Hint     string `json:"hint"`
	DataSize int64  `json:"data_size"`
	DataCRC  uint32 `json:"data_crc"`
}

func loadManifest(segmentDir string) (manifest, error) {
	found := false
	var lastErr error
	for _, name := range []string{manifestName, previousManifestName} {
		path := filepath.Join(segmentDir, name)
		data, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			continue
		}
		found = true
		if err != nil {
			lastErr = fmt.Errorf("read manifest %q: %w", path, err)
			continue
		}
		var result manifest
		if err := json.Unmarshal(data, &result); err != nil {
			lastErr = fmt.Errorf("parse manifest %q: %w", path, err)
			continue
		}
		if result.Version != manifestVersion {
			lastErr = fmt.Errorf("unsupported manifest version in %q", path)
			continue
		}
		if err := validateManifest(segmentDir, result); err != nil {
			lastErr = err
			continue
		}
		return result, nil
	}
	if found {
		return manifest{}, fmt.Errorf("no valid segment manifest: %w", lastErr)
	}
	return manifest{Version: manifestVersion}, nil
}

func validateManifest(segmentDir string, value manifest) error {
	for _, segment := range value.Segments {
		dataPath := filepath.Join(segmentDir, segment.Data)
		hintPath := filepath.Join(segmentDir, segment.Hint)
		info, err := os.Stat(dataPath)
		if err != nil {
			return fmt.Errorf("stat manifest segment %q: %w", dataPath, err)
		}
		if info.Size() != segment.DataSize {
			return fmt.Errorf("manifest segment %q size mismatch", dataPath)
		}
		checksum, err := fileCRC(dataPath)
		if err != nil {
			return err
		}
		if checksum != segment.DataCRC {
			return fmt.Errorf("manifest segment %q checksum mismatch", dataPath)
		}
		if _, err := os.Stat(hintPath); err != nil {
			return fmt.Errorf("stat manifest hint %q: %w", hintPath, err)
		}
	}
	return nil
}

func publishManifest(segmentDir string, value manifest) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	tmpPath := filepath.Join(segmentDir, manifestName+".tmp")
	manifestPath := filepath.Join(segmentDir, manifestName)
	previousPath := filepath.Join(segmentDir, previousManifestName)
	file, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if _, err := os.Stat(manifestPath); err == nil {
		if err := os.Rename(manifestPath, previousPath); err != nil {
			return err
		}
	}
	if err := os.Rename(tmpPath, manifestPath); err != nil {
		return err
	}
	return syncDirectory(segmentDir)
}

func fileCRC(path string) (uint32, error) {
	file, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer file.Close()
	hash := crc32.NewIEEE()
	if _, err := io.Copy(hash, file); err != nil {
		return 0, err
	}
	return hash.Sum32(), nil
}

func syncDirectory(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}
