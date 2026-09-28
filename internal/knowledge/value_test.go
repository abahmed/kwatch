package knowledge

import (
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestTextConstructor(t *testing.T) {
	v := Text("hello")
	assert.Equal(t, "hello", v.AsText())
	assert.True(t, !v.IsZero())

	empty := Text("")
	assert.Equal(t, "", empty.AsText())
	assert.True(t, !empty.IsZero())
}

func TestNumberConstructor(t *testing.T) {
	v := Number(42.5)
	num, ok := v.AsNumber()
	assert.True(t, ok)
	assert.Equal(t, 42.5, num)
	assert.True(t, !v.IsZero())
}

func TestBoolConstructor(t *testing.T) {
	vTrue := Bool(true)
	bval, ok := vTrue.AsBool()
	assert.True(t, ok)
	assert.True(t, bval)
	assert.True(t, !vTrue.IsZero())

	vFalse := Bool(false)
	bval, ok = vFalse.AsBool()
	assert.True(t, ok)
	assert.False(t, bval)
	assert.True(t, !vFalse.IsZero())
}

func TestTimeConstructor(t *testing.T) {
	ts := time.Date(2025, 6, 15, 10, 30, 45, 0, time.UTC)
	v := Time(ts)
	assert.Equal(t, ts.Unix(), v.AsTime().Unix())
	assert.True(t, !v.IsZero())
}

func TestAsTextRendering(t *testing.T) {
	tests := []struct {
		name     string
		value    Value
		expected string
	}{
		{"text value", Text("world"), "world"},
		{"number value", Number(123.456), "123.456"},
		{"number zero", Number(0), "0"},
		{"number negative", Number(-42.5), "-42.5"},
		{"bool true", Bool(true), "true"},
		{"bool false", Bool(false), "false"},
		{
			"time value",
			Time(time.Date(2025, 6, 15, 10, 30, 45, 0, time.UTC)),
			"2025-06-15T10:30:45Z",
		},
		{"zero value", Value{}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.value.AsText())
		})
	}
}

func TestAsNumberKindCheck(t *testing.T) {
	v := Number(99.5)
	num, ok := v.AsNumber()
	assert.True(t, ok)
	assert.Equal(t, 99.5, num)

	nonNum := Text("not a number")
	_, ok = nonNum.AsNumber()
	assert.False(t, ok)

	zero := Value{}
	_, ok = zero.AsNumber()
	assert.False(t, ok)
}

func TestAsBoolKindCheck(t *testing.T) {
	vTrue := Bool(true)
	b, ok := vTrue.AsBool()
	assert.True(t, ok)
	assert.True(t, b)

	vFalse := Bool(false)
	b, ok = vFalse.AsBool()
	assert.True(t, ok)
	assert.False(t, b)

	notBool := Text("yes")
	_, ok = notBool.AsBool()
	assert.False(t, ok)

	zero := Value{}
	_, ok = zero.AsBool()
	assert.False(t, ok)
}

func TestAsTimeKindCheck(t *testing.T) {
	ts := time.Date(2025, 6, 15, 10, 30, 45, 0, time.UTC)
	v := Time(ts)
	result := v.AsTime()
	assert.Equal(t, ts.Unix(), result.Unix())

	notTime := Text("2025-06-15")
	result = notTime.AsTime()
	assert.True(t, result.IsZero())

	zero := Value{}
	result = zero.AsTime()
	assert.True(t, result.IsZero())
}

func TestEqualComparison(t *testing.T) {
	tests := []struct {
		name     string
		v1       Value
		v2       Value
		expected bool
	}{
		{"same text", Text("hello"), Text("hello"), true},
		{
			"different text",
			Text("hello"),
			Text("world"),
			false,
		},
		{"same number", Number(42.5), Number(42.5), true},
		{
			"different number",
			Number(42.5),
			Number(42.6),
			false,
		},
		{"same bool true", Bool(true), Bool(true), true},
		{"same bool false", Bool(false), Bool(false), true},
		{"different bool", Bool(true), Bool(false), false},
		{
			"same time",
			Time(time.Date(2025, 6, 15, 10, 30, 45, 0, time.UTC)),
			Time(time.Date(2025, 6, 15, 10, 30, 45, 0, time.UTC)),
			true,
		},
		{
			"different time",
			Time(time.Date(2025, 6, 15, 10, 30, 45, 0, time.UTC)),
			Time(time.Date(2025, 6, 15, 10, 30, 46, 0, time.UTC)),
			false,
		},
		{
			"different kinds (text vs number)",
			Text("42"),
			Number(42),
			false,
		},
		{"both zero", Value{}, Value{}, true},
		{"zero vs non-zero", Value{}, Text("x"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.v1.Equal(tt.v2))
		})
	}
}

func TestIsZero(t *testing.T) {
	tests := []struct {
		name     string
		value    Value
		expected bool
	}{
		{"zero value", Value{}, true},
		{"text value", Text("hello"), false},
		{"number value", Number(0), false},
		{"number zero is not IsZero", Number(0.0), false},
		{"bool false", Bool(false), false},
		{"bool true", Bool(true), false},
		{"time value", Time(time.Time{}), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.value.IsZero())
		})
	}
}

func TestNumberRenderingEdgeCases(t *testing.T) {
	// Test that Number rendering doesn't add unnecessary zeros
	v := Number(1)
	assert.Equal(t, "1", v.AsText())

	v = Number(1.0)
	assert.Equal(t, "1", v.AsText())

	v = Number(1.5)
	assert.Equal(t, "1.5", v.AsText())

	v = Number(0.1)
	assert.Equal(t, "0.1", v.AsText())

	// Very large number
	v = Number(1e20)
	str := v.AsText()
	parsed, err := strconv.ParseFloat(str, 64)
	assert.NoError(t, err)
	assert.Equal(t, 1e20, parsed)
}

func TestTimeRoundsToSeconds(t *testing.T) {
	// Time.Time() uses Unix which loses nanosecond precision
	ts := time.Date(2025, 6, 15, 10, 30, 45, 123456789, time.UTC)
	v := Time(ts)
	result := v.AsTime()

	// Unix() loses nanoseconds
	assert.Equal(t, ts.Unix(), result.Unix())
	assert.Equal(t, 0, result.Nanosecond())
}
