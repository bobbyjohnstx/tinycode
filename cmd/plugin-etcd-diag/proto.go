package main

import (
	"encoding/binary"
	"errors"
	"fmt"
)

// k8s protobuf magic prefix: "k8s\x00"
var k8sMagic = []byte{0x6b, 0x38, 0x73, 0x00}

// typeMeta holds apiVersion and kind extracted from a k8s protobuf-encoded value.
type typeMeta struct {
	APIVersion string
	Kind       string
}

// decodeTypeMeta extracts apiVersion and kind from a k8s protobuf-encoded value.
// Returns zero typeMeta and error if the value is not k8s protobuf format.
//
// Wire format: 4-byte magic "k8s\x00", then a protobuf message containing
// field 1 (TypeMeta submessage) with field 1 = apiVersion (string) and
// field 2 = kind (string).
func decodeTypeMeta(value []byte) (typeMeta, error) {
	if len(value) < 4 {
		return typeMeta{}, errors.New("value too short for k8s protobuf")
	}
	for i := 0; i < 4; i++ {
		if value[i] != k8sMagic[i] {
			return typeMeta{}, errors.New("not k8s protobuf format")
		}
	}

	data := value[4:]

	// Read outer message: expect field 1, wire type 2 (length-delimited) = tag byte 0x0a
	typeMetaBytes, _, err := readField(data, 1)
	if err != nil {
		return typeMeta{}, fmt.Errorf("reading TypeMeta field: %w", err)
	}

	// Parse TypeMeta submessage: field 1 = apiVersion, field 2 = kind
	var tm typeMeta
	pos := 0
	for pos < len(typeMetaBytes) {
		fieldNum, wireType, n := readTag(typeMetaBytes[pos:])
		if n == 0 {
			break
		}
		pos += n

		if wireType != 2 { // only handle length-delimited (strings)
			// skip unknown wire types
			break
		}
		strLen, n := binary.Uvarint(typeMetaBytes[pos:])
		if n <= 0 {
			break
		}
		pos += n
		if pos+int(strLen) > len(typeMetaBytes) {
			break
		}
		s := string(typeMetaBytes[pos : pos+int(strLen)])
		pos += int(strLen)

		switch fieldNum {
		case 1:
			tm.APIVersion = s
		case 2:
			tm.Kind = s
		}
	}

	return tm, nil
}

// readTag reads a protobuf tag (field number + wire type) from data.
// Returns fieldNum, wireType, bytes consumed.
func readTag(data []byte) (fieldNum uint64, wireType int, n int) {
	tag, bytesRead := binary.Uvarint(data)
	if bytesRead <= 0 {
		return 0, 0, 0
	}
	return tag >> 3, int(tag & 0x07), bytesRead
}

// readField finds and returns the value of a length-delimited field with the given
// field number in data. Returns the field bytes, remaining data position, and error.
func readField(data []byte, targetField uint64) ([]byte, int, error) {
	pos := 0
	for pos < len(data) {
		fieldNum, wireType, n := readTag(data[pos:])
		if n == 0 {
			break
		}
		pos += n

		if wireType == 2 { // length-delimited
			length, n := binary.Uvarint(data[pos:])
			if n <= 0 {
				return nil, pos, errors.New("invalid varint for field length")
			}
			pos += n
			if pos+int(length) > len(data) {
				return nil, pos, errors.New("field length exceeds data")
			}
			if fieldNum == targetField {
				return data[pos : pos+int(length)], pos + int(length), nil
			}
			pos += int(length)
		} else if wireType == 0 { // varint
			_, n := binary.Uvarint(data[pos:])
			if n <= 0 {
				break
			}
			pos += n
		} else {
			// skip unknown wire types
			break
		}
	}
	return nil, pos, fmt.Errorf("field %d not found", targetField)
}
