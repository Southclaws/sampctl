package analytics

import (
	"reflect"
	"testing"
)

func TestParseJsonTagEmpty(t *testing.T) {
	name, omitempty := parseJsonTag("", "default")

	if name != "default" {
		t.Error("invalid field name found in empty tag:", name)
	}

	if omitempty {
		t.Error("unexpected 'omitempty' state found in empty tag")
	}
}

func TestParseJsonTagName(t *testing.T) {
	name, omitempty := parseJsonTag("name", "default")

	if name != "name" {
		t.Error("invalid field name found in json tag:", name)
	}

	if omitempty {
		t.Error("unexpected 'omitempty' state found in json tag")
	}
}

func TestParseJsonTagOmitempty(t *testing.T) {
	name, omitempty := parseJsonTag(",omitempty", "default")

	if name != "default" {
		t.Error("invalid field name found in omitempty tag:", name)
	}

	if !omitempty {
		t.Error("expected 'omitempty' state not found in json tag")
	}
}

func TestParseJsonTagNameOmitempty(t *testing.T) {
	name, omitempty := parseJsonTag("name,omitempty", "default")

	if name != "name" {
		t.Error("invalid field name found in json tag:", name)
	}

	if !omitempty {
		t.Error("expected 'omitempty' state not found in json tag")
	}
}

func TestStructToMap(t *testing.T) {
	type X struct {
		A bool
		B int    `json:"b"`
		C string `json:"c,omitempty"`
	}

	if m := structToMap(reflect.ValueOf(X{}), nil); !reflect.DeepEqual(m, map[string]interface{}{
		"A": false,
		"b": 0,
	}) {
		t.Error("invalid JSON object representation of a struct:", m)
	}
}

func TestIsZeroValueStringTrue(t *testing.T) {
	if !isZeroValue(reflect.ValueOf("")) {
		t.Error("empty string should be a zero-value")
	}
}

func TestIsZeroValueStringFalse(t *testing.T) {
	if isZeroValue(reflect.ValueOf("A")) {
		t.Error("non-empty string should not be a zero-value")
	}
}

func TestIsZeroValueSliceTrue(t *testing.T) {
	if !isZeroValue(reflect.ValueOf([]int{})) {
		t.Error("empty slice should be a zero-value")
	}
}

func TestIsZeroValueSliceFalse(t *testing.T) {
	if isZeroValue(reflect.ValueOf([]int{0})) {
		t.Error("non-empty slice should not be a zero-value")
	}
}

func TestIsZeroValueMapTrue(t *testing.T) {
	if !isZeroValue(reflect.ValueOf(map[string]string{})) {
		t.Error("empty map should be a zero-value")
	}
}

func TestIsZeroValueMapFalse(t *testing.T) {
	if isZeroValue(reflect.ValueOf(map[string]string{
		"A": "a",
	})) {
		t.Error("non-empty map should not be a zero-value")
	}
}

func TestIsZeroValueBoolTrue(t *testing.T) {
	if !isZeroValue(reflect.ValueOf(false)) {
		t.Error("`false` should be a zero-value")
	}
}

func TestIsZeroValueBoolFalse(t *testing.T) {
	if isZeroValue(reflect.ValueOf(true)) {
		t.Error("`true` should not be a zero-value")
	}
}

func TestIsZeroValueIntTrue(t *testing.T) {
	if !isZeroValue(reflect.ValueOf(0)) {
		t.Error("`0` should be a zero-value")
	}
}

func TestIsZeroValueIntFalse(t *testing.T) {
	if isZeroValue(reflect.ValueOf(1)) {
		t.Error("`1` should not be a zero-value")
	}
}

func TestIsZeroValueUintTrue(t *testing.T) {
	if !isZeroValue(reflect.ValueOf(uint(0))) {
		t.Error("`0` should be a zero-value")
	}
}

func TestIsZeroValueUintFalse(t *testing.T) {
	if isZeroValue(reflect.ValueOf(uint(1))) {
		t.Error("`1` should not be a zero-value")
	}
}

func TestIsZeroValueFloatTrue(t *testing.T) {
	if !isZeroValue(reflect.ValueOf(0.0)) {
		t.Error("`0.0` should be a zero-value")
	}
}

func TestIsZeroValueFloatFalse(t *testing.T) {
	if isZeroValue(reflect.ValueOf(1.0)) {
		t.Error("`1.0` should not be a zero-value")
	}
}

func TestIsZeroValuePointerTrue(t *testing.T) {
	if !isZeroValue(reflect.ValueOf((*int)(nil))) {
		t.Error("nil pointer should be a zero-value")
	}
}

func TestIsZeroValuePointerFalse(t *testing.T) {
	if isZeroValue(reflect.ValueOf(t)) {
		t.Error("non-nil pointer should not be a zero-value")
	}
}

func TestIsZeroValueStructTrue(t *testing.T) {
	type T struct {
		A bool
		B int
		C string
	}

	if !isZeroValue(reflect.ValueOf(T{})) {
		t.Error("empty struct should be a zero-value")
	}
}

func TestIsZeroValueStructFalse(t *testing.T) {
	type T struct {
		A bool
		B int
		C string
	}

	if isZeroValue(reflect.ValueOf(T{A: true})) {
		t.Error("non-empty struct should not be a zero-value")
	}
}

func TestIsZeroValueNil(t *testing.T) {
	if !isZeroValue(reflect.ValueOf(nil)) {
		t.Error("nil should be a zero-value")
	}
}

func TestIsZeroValueFunc(t *testing.T) {
	if isZeroValue(reflect.ValueOf(func() {})) {
		t.Error("functions should be be zero-value")
	}
}
