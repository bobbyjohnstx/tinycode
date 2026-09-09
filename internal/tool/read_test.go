package tool

import (
	"testing"
)

func TestIsBinaryData(t *testing.T) {
	tests := []struct {
		name   string
		data   []byte
		wantBi bool
	}{
		{"empty", []byte{}, false},
		{"plain text", []byte("hello world\nthis is text"), false},
		{"go source", []byte("package main\n\nfunc main() {\n\tfmt.Println(\"hello\")\n}"), false},
		{"null bytes", []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0}, true},
		{"mixed binary", makeBinaryData(100, 0.5), true},
		{"mostly text with few control chars", makeBinaryData(100, 0.05), false},
		{"utf8 text", []byte("日本語テスト hello world"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isBinaryData(tt.data)
			if got != tt.wantBi {
				t.Errorf("isBinaryData() = %v, want %v", got, tt.wantBi)
			}
		})
	}
}

func makeBinaryData(size int, binaryRatio float64) []byte {
	data := make([]byte, size)
	binaryCount := int(float64(size) * binaryRatio)
	for i := 0; i < size; i++ {
		if i < binaryCount {
			data[i] = 0x01 // non-text control char
		} else {
			data[i] = 'a'
		}
	}
	return data
}
