package storage

import (
	"crypto/sha256"
	"encoding/binary"
)

type (
	Record []byte

	OffsetRecord struct {
		Record
		Offset int64
	}

	recordHeader struct {
		Length   uint32
		Checksum int64
	}
)

func newHeader(data Record) recordHeader {
	h := sha256.Sum256(data)
	return recordHeader{
		Length:   uint32(len(data)),
		Checksum: int64(binary.LittleEndian.Uint64(h[:])),
	}
}
