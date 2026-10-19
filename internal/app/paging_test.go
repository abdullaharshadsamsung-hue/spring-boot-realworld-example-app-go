package app

import (
	"math"
	"strings"
	"testing"
)

func TestDefaultPage(t *testing.T) {
	p := DefaultPage()
	if p.Offset() != 0 {
		t.Errorf("Offset() = %d, want 0", p.Offset())
	}
	if p.Limit() != 20 {
		t.Errorf("Limit() = %d, want 20", p.Limit())
	}
	if DefaultOffset != 0 || DefaultLimit != 20 || MaxLimit != 100 {
		t.Errorf("unexpected constants: %d %d %d", DefaultOffset, DefaultLimit, MaxLimit)
	}
}

func TestNewPage(t *testing.T) {
	tests := []struct {
		name       string
		offset     int
		limit      int
		wantOffset int
		wantLimit  int
	}{
		{"normal values", 10, 50, 10, 50},
		{"zeros use defaults", 0, 0, 0, 20},
		{"negative offset", -5, 10, 0, 10},
		{"negative limit", 3, -1, 3, 20},
		{"limit at max", 0, 100, 0, 100},
		{"limit just above max", 0, 101, 0, 100},
		{"limit max int32", 0, math.MaxInt32, 0, 100},
		{"limit max int", 0, math.MaxInt, 0, 100},
		{"limit one", 0, 1, 0, 1},
		{"offset one", 1, 20, 1, 20},
		{"large offset no cap", math.MaxInt32, 20, math.MaxInt32, 20},
		{"max int offset", math.MaxInt, 20, math.MaxInt, 20},
		{"min int values", math.MinInt, math.MinInt, 0, 20},
		{"offset five", 5, 0, 5, 20},
		{"limit 200 capped", 0, 200, 0, 100},
		{"both invalid", -1, 500, 0, 100},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewPage(tt.offset, tt.limit)
			if got := p.Offset(); got != tt.wantOffset {
				t.Errorf("Offset() = %d, want %d", got, tt.wantOffset)
			}
			if got := p.Limit(); got != tt.wantLimit {
				t.Errorf("Limit() = %d, want %d", got, tt.wantLimit)
			}
			if p.Offset() < 0 {
				t.Errorf("invariant violated: offset %d < 0", p.Offset())
			}
			if p.Limit() < 1 || p.Limit() > 100 {
				t.Errorf("invariant violated: limit %d not in [1,100]", p.Limit())
			}
		})
	}
}

func TestPageEquality(t *testing.T) {
	tests := []struct {
		name string
		a, b Page
		want bool
	}{
		{"zeros vs default", NewPage(0, 0), DefaultPage(), true},
		{"sanitized equal", NewPage(-1, 500), NewPage(0, 100), true},
		{"same values", NewPage(10, 50), NewPage(10, 50), true},
		{"different offset", NewPage(1, 20), NewPage(2, 20), false},
		{"different limit", NewPage(0, 10), NewPage(0, 11), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.a.Equal(tt.b); got != tt.want {
				t.Errorf("Equal() = %v, want %v", got, tt.want)
			}
			if got := tt.b.Equal(tt.a); got != tt.want {
				t.Errorf("symmetric Equal() = %v, want %v", got, tt.want)
			}
			if got := tt.a == tt.b; got != tt.want {
				t.Errorf("== = %v, want %v", got, tt.want)
			}
			// map key behavior (hashCode analogue)
			m := map[Page]int{tt.a: 1}
			_, ok := m[tt.b]
			if ok != tt.want {
				t.Errorf("map lookup = %v, want %v", ok, tt.want)
			}
		})
	}
}

func TestPageString(t *testing.T) {
	tests := []struct {
		name string
		p    Page
		want string
	}{
		{"default", DefaultPage(), "Page(offset=0, limit=20)"},
		{"custom", NewPage(10, 50), "Page(offset=10, limit=50)"},
		{"capped", NewPage(-3, 1000), "Page(offset=0, limit=100)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := tt.p.String()
			if s != tt.want {
				t.Errorf("String() = %q, want %q", s, tt.want)
			}
			if !strings.Contains(s, "offset=") || !strings.Contains(s, "limit=") {
				t.Errorf("String() %q missing fields", s)
			}
		})
	}
}