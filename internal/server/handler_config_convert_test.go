package server

import (
	"encoding/json"
	"testing"
)

func TestAsFloat64_ReturnsFloat64FromFloat64(t *testing.T) {
	v, ok := asFloat64(float64(3.14))
	if !ok {
		t.Fatal("expected ok=true for float64 input")
	}
	if v != 3.14 {
		t.Errorf("expected 3.14, got %f", v)
	}
}

func TestAsFloat64_ReturnsFloat64FromFloat32(t *testing.T) {
	v, ok := asFloat64(float32(2.5))
	if !ok {
		t.Fatal("expected ok=true for float32 input")
	}
	if v < 2.49 || v > 2.51 {
		t.Errorf("expected ~2.5, got %f", v)
	}
}

func TestAsFloat64_ReturnsFloat64FromInt(t *testing.T) {
	v, ok := asFloat64(int(42))
	if !ok {
		t.Fatal("expected ok=true for int input")
	}
	if v != 42.0 {
		t.Errorf("expected 42.0, got %f", v)
	}
}

func TestAsFloat64_ReturnsFloat64FromInt64(t *testing.T) {
	v, ok := asFloat64(int64(100))
	if !ok {
		t.Fatal("expected ok=true for int64 input")
	}
	if v != 100.0 {
		t.Errorf("expected 100.0, got %f", v)
	}
}

func TestAsFloat64_ReturnsFloat64FromJSONNumber(t *testing.T) {
	v, ok := asFloat64(json.Number("1.618"))
	if !ok {
		t.Fatal("expected ok=true for json.Number input")
	}
	if v < 1.617 || v > 1.619 {
		t.Errorf("expected ~1.618, got %f", v)
	}
}

func TestAsFloat64_ReturnsFalseForString(t *testing.T) {
	_, ok := asFloat64("not a number")
	if ok {
		t.Error("expected ok=false for string input")
	}
}

func TestAsFloat64_ReturnsFalseForNil(t *testing.T) {
	_, ok := asFloat64(nil)
	if ok {
		t.Error("expected ok=false for nil input")
	}
}

func TestAsInt_ReturnsIntFromFloat64(t *testing.T) {
	v, ok := asInt(float64(42.9))
	if !ok {
		t.Fatal("expected ok=true for float64 input")
	}
	if v != 42 {
		t.Errorf("expected 42, got %d", v)
	}
}

func TestAsInt_ReturnsIntFromFloat32(t *testing.T) {
	v, ok := asInt(float32(7.0))
	if !ok {
		t.Fatal("expected ok=true for float32 input")
	}
	if v != 7 {
		t.Errorf("expected 7, got %d", v)
	}
}

func TestAsInt_ReturnsIntFromInt(t *testing.T) {
	v, ok := asInt(int(99))
	if !ok {
		t.Fatal("expected ok=true for int input")
	}
	if v != 99 {
		t.Errorf("expected 99, got %d", v)
	}
}

func TestAsInt_ReturnsIntFromInt64(t *testing.T) {
	v, ok := asInt(int64(256))
	if !ok {
		t.Fatal("expected ok=true for int64 input")
	}
	if v != 256 {
		t.Errorf("expected 256, got %d", v)
	}
}

func TestAsInt_ReturnsIntFromJSONNumber(t *testing.T) {
	v, ok := asInt(json.Number("1024"))
	if !ok {
		t.Fatal("expected ok=true for json.Number input")
	}
	if v != 1024 {
		t.Errorf("expected 1024, got %d", v)
	}
}

func TestAsInt_ReturnsFalseForString(t *testing.T) {
	_, ok := asInt("not a number")
	if ok {
		t.Error("expected ok=false for string input")
	}
}

func TestAsInt_ReturnsFalseForNil(t *testing.T) {
	_, ok := asInt(nil)
	if ok {
		t.Error("expected ok=false for nil input")
	}
}
