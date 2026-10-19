package core

import (
	"strings"
	"sync"
	"testing"
)

func TestIsEmpty(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  bool
	}{
		{name: "zero value (Go counterpart of Java null)", value: "", want: true},
		{name: "empty string literal", value: "", want: true},
		{name: "single space is not trimmed", value: " ", want: false},
		{name: "whitespace only tab newline spaces", value: "\t\n  ", want: false},
		{name: "carriage return only", value: "\r", want: false},
		{name: "non-empty abc", value: "abc", want: false},
		{name: "single null character has length 1", value: "\u0000", want: false},
		{name: "literal text null", value: "null", want: false},
		{name: "unicode characters", value: "héllo 世界", want: false},
		{name: "emoji characters", value: "😀🚀", want: false},
		{name: "non-breaking space", value: "\u00a0", want: false},
		{name: "long string", value: strings.Repeat("x", 10000), want: false},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("isEmpty(%q) panicked: %v", tt.value, r)
				}
			}()
			original := tt.value
			got := isEmpty(tt.value)
			if got != tt.want {
				t.Errorf("isEmpty(%q) = %v, want %v", tt.value, got, tt.want)
			}
			if tt.value != original {
				t.Errorf("isEmpty modified input: got %q, want %q", tt.value, original)
			}
			// Invariant: true iff length == 0.
			if got != (len(tt.value) == 0) {
				t.Errorf("isEmpty(%q) = %v, inconsistent with len %d", tt.value, got, len(tt.value))
			}
		})
	}
}

func TestIsEmptyDeterministic(t *testing.T) {
	inputs := []string{"", " ", "\t\n  ", "abc", "\u0000", "null", "😀"}
	for _, in := range inputs {
		first := isEmpty(in)
		for i := 0; i < 100; i++ {
			if got := isEmpty(in); got != first {
				t.Fatalf("isEmpty(%q) not deterministic: iteration %d got %v, first %v", in, i, got, first)
			}
		}
	}
}

func TestIsEmptyConcurrent(t *testing.T) {
	inputs := []struct {
		value string
		want  bool
	}{
		{"", true},
		{" ", false},
		{"abc", false},
		{"\u0000", false},
		{"null", false},
	}
	var wg sync.WaitGroup
	errs := make(chan string, 1000)
	for g := 0; g < 50; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				for _, in := range inputs {
					if got := isEmpty(in.value); got != in.want {
						select {
						case errs <- in.value:
						default:
						}
					}
				}
			}
		}()
	}
	wg.Wait()
	close(errs)
	for v := range errs {
		t.Errorf("concurrent isEmpty(%q) returned unexpected result", v)
	}
}