package sst

import (
	"bytes"
	"encoding/binary"
	"github.com/spirit-labs/tektite/common"
	"github.com/spirit-labs/tektite/encoding"
	"github.com/spirit-labs/tektite/errors"
	"github.com/spirit-labs/tektite/iteration"
	"math"
	"time"
)

type SSTableID []byte

type SSTable struct {
	format       common.DataFormat
	maxKeyLength uint32
	numEntries   uint32
	numDeletes   uint32
	indexOffset  uint32
	creationTime uint64
	data         []byte
}

func BuildSSTable(format common.DataFormat, buffSizeEstimate int, entriesEstimate int,
	iter iteration.Iterator) (*SSTable, []byte, []byte, uint64, uint64, error) {

	type indexEntry struct {
		key    []byte
		offset uint32
	}

	var smallestKey, largestKey []byte

	indexEntries := make([]indexEntry, 0, entriesEstimate)
	buff := make([]byte, 0, buffSizeEstimate)

	// First byte is the format, then 4 bytes (uint32) which is an offset to the metadata section that we will fill in
	// later
	buff = append(buff, byte(format), 0, 0, 0, 0)

	var maxVersion uint64
	var minVersion uint64 = math.MaxUint64
	maxKeyLength := 0
	numEntries := 0
	numDeletes := 0
	first := true
	var prevKey []byte
	for {
		v, err := iter.IsValid()
		if err != nil {
			return nil, nil, nil, 0, 0, err
		}
		if !v {
			break
		}
		kv := iter.Current()
		// Sanity checks - can maybe remove them or activate them only with a flag for performance
		if prevKey != nil && bytes.Compare(prevKey, kv.Key) >= 0 {
			panic("keys not in order / contains duplicates")
		}
		prevKey = kv.Key
		if first {
			smallestKey = kv.Key
			first = false
		}
		offset := uint32(len(buff))
		lk := len(kv.Key)
		if lk > maxKeyLength {
			maxKeyLength = lk
		}
		buff = appendBytesWithLengthPrefix(buff, kv.Key)
		buff = appendBytesWithLengthPrefix(buff, kv.Value)
		indexEntries = append(indexEntries, indexEntry{
			key:    kv.Key,
			offset: offset,
		})
		numEntries++
		if len(kv.Value) == 0 {
			numDeletes++
		}
		largestKey = kv.Key
		version := math.MaxUint64 - binary.BigEndian.Uint64(kv.Key[len(kv.Key)-8:]) // last 8 bytes is version
		if version > maxVersion {
			maxVersion = version
		}
		if version < minVersion {
			minVersion = version
		}

		if err := iter.Next(); err != nil {
			return nil, nil, nil, 0, 0, err
		}
	}

	indexOffset := len(buff)

	for _, entry := range indexEntries {
		buff = append(buff, entry.key...)
		paddingBytes := maxKeyLength - len(entry.key)
		if paddingBytes > 0 {
			if len(buff)+paddingBytes <= cap(buff) {
				// Extend the buffer by slicing - more efficient than allocating a new buffer
				buff = buff[:len(buff)+paddingBytes]
			} else {
				buff = append(buff, make([]byte, paddingBytes)...)
			}
		}
		buff = encoding.AppendUint32ToBufferLE(buff, entry.offset)
	}

	// Now fill in metadata offset
	metadataOffset := len(buff)
	if metadataOffset > math.MaxUint32 {
		return nil, nil, nil, 0, 0, errors.New("SSTable too big")
	}
	buff[1] = byte(metadataOffset)
	buff[2] = byte(metadataOffset >> 8)
	buff[3] = byte(metadataOffset >> 16)
	buff[4] = byte(metadataOffset >> 24)

	return &SSTable{
		format:       format,
		maxKeyLength: uint32(maxKeyLength),
		numEntries:   uint32(numEntries),
		numDeletes:   uint32(numDeletes),
		indexOffset:  uint32(indexOffset),
		creationTime: uint64(time.Now().UTC().UnixMilli()),
		data:         buff,
	}, smallestKey, largestKey, minVersion, maxVersion, nil
}

func (s *SSTable) Serialize() []byte {
	// To avoid copying the data buffer, we put all the meta-data at the end
	buff := encoding.AppendUint32ToBufferLE(s.data, s.maxKeyLength)
	buff = encoding.AppendUint32ToBufferLE(buff, s.numEntries)
	buff = encoding.AppendUint32ToBufferLE(buff, s.numDeletes)
	buff = encoding.AppendUint32ToBufferLE(buff, s.indexOffset)
	buff = encoding.AppendUint64ToBufferLE(buff, s.creationTime)
	return buff
}

func (s *SSTable) Deserialize(buff []byte, offset int) int {
	s.format = common.DataFormat(buff[offset])
	offset++
	var metadataOffset uint32
	metadataOffset, _ = encoding.ReadUint32FromBufferLE(buff, offset)
	offset = int(metadataOffset)
	s.maxKeyLength, offset = encoding.ReadUint32FromBufferLE(buff, offset)
	s.numEntries, offset = encoding.ReadUint32FromBufferLE(buff, offset)
	s.numDeletes, offset = encoding.ReadUint32FromBufferLE(buff, offset)
	s.indexOffset, offset = encoding.ReadUint32FromBufferLE(buff, offset)
	s.creationTime, offset = encoding.ReadUint64FromBufferLE(buff, offset)
	s.data = buff[:len(buff)-24]
	return offset
}

func (s *SSTable) SizeBytes() int {
	return len(s.data) + 24
}

func (s *SSTable) NumEntries() int {
	return int(s.numEntries)
}

func (s *SSTable) NumDeletes() int {
	return int(s.numDeletes)
}

func (s *SSTable) DeleteRatio() float64 {
	return float64(s.numDeletes) / float64(s.numEntries)
}

func (s *SSTable) CreationTime() uint64 {
	return s.creationTime
}

func appendBytesWithLengthPrefix(buff []byte, bytes []byte) []byte {
	buff = encoding.AppendUint32ToBufferLE(buff, uint32(len(bytes)))
	buff = append(buff, bytes...)
	return buff
}

func (s *SSTable) findOffset(key []byte) int {
	indexRecordLen := int(s.maxKeyLength) + 4
	numEntries := int(s.numEntries)
	indexOffset := int(s.indexOffset)
	maxKeyLength := int(s.maxKeyLength)

	// We do a binary search in the index
	low := 0
	outerHighBound := numEntries - 1
	high := outerHighBound
	for low < high {
		middle := low + (high-low)/2
		recordStart := middle*indexRecordLen + indexOffset
		midKey := s.data[recordStart : recordStart+maxKeyLength]
		if bytes.Compare(midKey, key) < 0 {
			low = middle + 1
		} else {
			high = middle
		}
	}
	if high == outerHighBound {
		recordStart := high*indexRecordLen + indexOffset
		highKey := s.data[recordStart : recordStart+maxKeyLength]
		if bytes.Compare(highKey, key) < 0 {
			// Didn't find key
			return -1
		}
	}
	recordStart := high*indexRecordLen + indexOffset
	valueStart := recordStart + maxKeyLength
	off, _ := encoding.ReadUint32FromBufferLE(s.data, valueStart)
	return int(off)
}
