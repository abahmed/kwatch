package inventory

import (
	"strconv"
	"time"
)

// Value is a small typed scalar. Observations carry values rather than objects
// so the model stays bounded and never retains full Kubernetes objects.
type Value struct {
	kind valueKind
	text string
	num  float64
}

type valueKind uint8

const (
	valueNone valueKind = iota
	valueText
	valueNumber
	valueBool
	valueTime
)

// Text builds a string value.
func Text(value string) Value { return Value{kind: valueText, text: value} }

// Number builds a numeric value.
func Number(value float64) Value { return Value{kind: valueNumber, num: value} }

// Bool builds a boolean value.
func Bool(value bool) Value {
	if value {
		return Value{kind: valueBool, num: 1}
	}
	return Value{kind: valueBool}
}

// Time builds a timestamp value with second precision.
func Time(value time.Time) Value {
	return Value{kind: valueTime, num: float64(value.Unix())}
}

// IsZero reports whether the value is unset.
func (v Value) IsZero() bool { return v.kind == valueNone }

// AsText returns the value rendered as text.
func (v Value) AsText() string {
	switch v.kind {
	case valueText:
		return v.text
	case valueNumber:
		return strconv.FormatFloat(v.num, 'f', -1, 64)
	case valueBool:
		return strconv.FormatBool(v.num != 0)
	case valueTime:
		return v.AsTime().UTC().Format(time.RFC3339)
	default:
		return ""
	}
}

// AsNumber returns the numeric value and whether the value is numeric.
func (v Value) AsNumber() (float64, bool) {
	return v.num, v.kind == valueNumber
}

// AsBool returns the boolean value and whether the value is a boolean.
func (v Value) AsBool() (bool, bool) {
	return v.num != 0, v.kind == valueBool
}

// AsTime returns the timestamp value, or zero when it is not a time.
func (v Value) AsTime() time.Time {
	if v.kind != valueTime {
		return time.Time{}
	}
	return time.Unix(int64(v.num), 0)
}

// Equal compares two values by kind and content.
func (v Value) Equal(other Value) bool {
	return v.kind == other.kind && v.text == other.text && v.num == other.num
}
