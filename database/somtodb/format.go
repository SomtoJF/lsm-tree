package somtodb

import (
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"io"
	"os"
)

var errRecordChecksum = fmt.Errorf("record checksum mismatch")

const (
	recordHeaderSize   = 20
	recordChecksumSize = 4
	recordVersion      = 2
	recordTypeSet      = 1
	maxRecordPayload   = 64 << 20
)

var recordMagic = [4]byte{'S', 'M', 'D', 'B'}

func encodeRecord(key int, value []byte) ([]byte, error) {
	key64 := int64(key)
	if int(key64) != key {
		return nil, fmt.Errorf("key %d cannot be represented as int64", key)
	}
	frame := make([]byte, recordHeaderSize+len(value)+recordChecksumSize)
	copy(frame[:4], recordMagic[:])
	frame[4] = recordVersion
	frame[5] = recordTypeSet
	binary.LittleEndian.PutUint64(frame[8:16], uint64(key64))
	binary.LittleEndian.PutUint32(frame[16:20], uint32(len(value)))
	copy(frame[recordHeaderSize:], value)
	checksum := crc32.ChecksumIEEE(frame[4 : recordHeaderSize+len(value)])
	binary.LittleEndian.PutUint32(frame[recordHeaderSize+len(value):], checksum)
	return frame, nil
}

func decodeRecordAt(file *os.File, offset int64, fileSize int64) (key int, value []byte, nextOffset int64, err error) {
	header := make([]byte, recordHeaderSize)
	n, readErr := file.ReadAt(header, offset)
	if readErr != nil {
		if readErr == io.EOF && n == 0 {
			return 0, nil, offset, io.EOF
		}
		return 0, nil, offset, io.ErrUnexpectedEOF
	}
	if string(header[:4]) != string(recordMagic[:]) || header[4] != recordVersion || header[5] != recordTypeSet {
		return 0, nil, offset, fmt.Errorf("invalid record header at offset %d", offset)
	}
	key64 := int64(binary.LittleEndian.Uint64(header[8:16]))
	key = int(key64)
	if int64(key) != key64 {
		return 0, nil, offset, fmt.Errorf("key %d cannot be represented as int", key64)
	}
	valueLength := int64(binary.LittleEndian.Uint32(header[16:20]))
	if valueLength > maxRecordPayload {
		return 0, nil, offset, fmt.Errorf("record at offset %d exceeds maximum payload size", offset)
	}
	end := offset + recordHeaderSize + valueLength + recordChecksumSize
	if end > fileSize {
		return 0, nil, offset, io.ErrUnexpectedEOF
	}
	value = make([]byte, valueLength)
	if _, err := file.ReadAt(value, offset+recordHeaderSize); err != nil {
		return 0, nil, offset, err
	}
	checksumBytes := make([]byte, recordChecksumSize)
	if _, err := file.ReadAt(checksumBytes, offset+recordHeaderSize+valueLength); err != nil {
		return 0, nil, offset, err
	}
	checksum := binary.LittleEndian.Uint32(checksumBytes)
	checksumInput := make([]byte, recordHeaderSize-4+len(value))
	copy(checksumInput, header[4:])
	copy(checksumInput[recordHeaderSize-4:], value)
	if crc32.ChecksumIEEE(checksumInput) != checksum {
		return 0, nil, end, fmt.Errorf("%w at offset %d", errRecordChecksum, offset)
	}
	return key, value, end, nil
}
