package app

import (
	"testing"
	"time"
)

func TestNewDateTimeCursor_Data(t *testing.T) {
	loc := time.FixedZone("X", 5*3600)
	tests := []struct {
		name string
		in   time.Time
	}{
		{"utc", time.UnixMilli(1500000000000).UTC()},
		{"zoned", time.UnixMilli(1500000000000).In(loc)},
		{"zero value", time.Time{}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := NewDateTimeCursor(tc.in)
			got := c.Data()
			if !got.Equal(tc.in) || got.Location() != tc.in.Location() {
				t.Fatalf("Data() = %v, want %v", got, tc.in)
			}
		})
	}
}

func TestDateTimeCursor_String(t *testing.T) {
	tokyo := time.FixedZone("JST", 9*3600)
	tests := []struct {
		name string
		in   time.Time
		want string
	}{
		{"positive", time.UnixMilli(1500000000000), "1500000000000"},
		{"epoch", time.UnixMilli(0).UTC(), "0"},
		{"before 1970", time.UnixMilli(-1000), "-1000"},
		{"other zone same instant", time.UnixMilli(1500000000000).In(tokyo), "1500000000000"},
		{"sub-ms truncated", time.Unix(1700000000, 123456789), "1700000000123"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := NewDateTimeCursor(tc.in).String(); got != tc.want {
				t.Fatalf("String() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestParseDateTimeCursor_Valid(t *testing.T) {
	tests := []struct {
		in   string
		ms   int64
		want time.Time
	}{
		{"1500000000000", 1500000000000, time.Date(2017, 7, 14, 2, 40, 0, 0, time.UTC)},
		{"0", 0, time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)},
		{"-1000", -1000, time.Date(1969, 12, 31, 23, 59, 59, 0, time.UTC)},
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			got, err := ParseDateTimeCursor(tc.in)
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if got == nil {
				t.Fatal("nil result")
			}
			if got.UnixMilli() != tc.ms || !got.Equal(tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
			if got.Location() != time.UTC {
				t.Fatalf("location = %v, want UTC", got.Location())
			}
		})
	}
}