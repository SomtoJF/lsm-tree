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
	recordHeaderSize   = 12
	recordChecksumSize = 4
	recordVersion      = 1
	recordTypeSet      = 1
	maxRecordPayload   = 64 << 20
)

var recordMagic = [4]byte{'S', 'M', 'D', 'B'}

func encodeRecord(payload []byte) []byte {
	frame := make([]byte, recordHeaderSize+len(payload)+recordChecksumSize)
	copy(frame[:4], recordMagic[:])
	frame[4] = recordVersion
	frame[5] = recordTypeSet
	binary.LittleEndian.PutUint32(frame[8:12], uint32(len(payload)))
	copy(frame[recordHeaderSize:], payload)
	checksum := crc32.ChecksumIEEE(frame[4 : recordHeaderSize+len(payload)])
	binary.LittleEndian.PutUint32(frame[recordHeaderSize+len(payload):], checksum)
	return frame
}

func decodeRecordAt(file *os.File, offset int64, fileSize int64) (payload []byte, nextOffset int64, err error) {
	header := make([]byte, recordHeaderSize)
	n, readErr := file.ReadAt(header, offset)
	if readErr != nil {
		if readErr == io.EOF && n == 0 {
			return nil, offset, io.EOF
		}
		return nil, offset, io.ErrUnexpectedEOF
	}
	if string(header[:4]) != string(recordMagic[:]) || header[4] != recordVersion || header[5] != recordTypeSet {
		return nil, offset, fmt.Errorf("invalid record header at offset %d", offset)
	}
	payloadLength := int64(binary.LittleEndian.Uint32(header[8:12]))
	if payloadLength > maxRecordPayload {
		return nil, offset, fmt.Errorf("record at offset %d exceeds maximum payload size", offset)
	}
	end := offset + recordHeaderSize + payloadLength + recordChecksumSize
	if end > fileSize {
		return nil, offset, io.ErrUnexpectedEOF
	}
	payload = make([]byte, payloadLength)
	if _, err := file.ReadAt(payload, offset+recordHeaderSize); err != nil {
		return nil, offset, err
	}
	checksumBytes := make([]byte, recordChecksumSize)
	if _, err := file.ReadAt(checksumBytes, offset+recordHeaderSize+payloadLength); err != nil {
		return nil, offset, err
	}
	checksum := binary.LittleEndian.Uint32(checksumBytes)
	checksumInput := make([]byte, recordHeaderSize-4+len(payload))
	copy(checksumInput, header[4:])
	copy(checksumInput[recordHeaderSize-4:], payload)
	if crc32.ChecksumIEEE(checksumInput) != checksum {
		return nil, end, fmt.Errorf("%w at offset %d", errRecordChecksum, offset)
	}
	return payload, end, nil
}
