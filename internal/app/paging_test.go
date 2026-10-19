package app

import (
	"math"
	"strings"
	"testing"
	"time"
)

type testNode struct{ c string }

func (n testNode) Cursor() string { return n.c }

func TestDirectionString(t *testing.T) {
	tests := []struct {
		d    Direction
		want string
	}{
		{DirectionPrev, "PREV"},
		{DirectionNext, "NEXT"},
		{Direction(99), "UNKNOWN"},
	}
	for _, tt := range tests {
		if got := tt.d.String(); got != tt.want {
			t.Errorf("Direction(%d).String() = %q, want %q", int(tt.d), got, tt.want)
		}
	}
}

func TestNewCursorPager(t *testing.T) {
	data := []testNode{{"a"}, {"b"}, {"c"}}
	tests := []struct {
		name     string
		data     []testNode
		dir      Direction
		extra    bool
		wantNext bool
		wantPrev bool
		start    string
		end      string
		ok       bool
	}{
		{"next extra", data, DirectionNext, true, true, false, "a", "c", true},
		{"next no extra", data, DirectionNext, false, false, false, "a", "c", true},
		{"prev extra", data, DirectionPrev, true, false, true, "a", "c", true},
		{"prev no extra", data, DirectionPrev, false, false, false, "a", "c", true},
		{"unknown as prev", data, Direction(7), true, false, true, "a", "c", true},
		{"empty", nil, DirectionNext, true, true, false, "", "", false},
		{"single", []testNode{{"x"}}, DirectionNext, false, false, false, "x", "x", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewCursorPager(tt.data, tt.dir, tt.extra)
			if p.HasNext() != tt.wantNext || p.HasPrevious() != tt.wantPrev {
				t.Errorf("next/prev = %v/%v, want %v/%v", p.HasNext(), p.HasPrevious(), tt.wantNext, tt.wantPrev)
			}
			if len(p.Data()) != len(tt.data) {
				t.Errorf("Data len = %d", len(p.Data()))
			}
			s, ok := p.StartCursor()
			if s != tt.start || ok != tt.ok {
				t.Errorf("StartCursor = %q,%v", s, ok)
			}
			e, ok := p.EndCursor()
			if e != tt.end || ok != tt.ok {
				t.Errorf("EndCursor = %q,%v", e, ok)
			}
		})
	}
}

func TestCursorPagerNilAndNilElements(t *testing.T) {
	var p *CursorPager[testNode]
	if p.Data() != nil || p.HasNext() || p.HasPrevious() {
		t.Error("nil pager should be empty")
	}
	if _, ok := p.StartCursor(); ok {
		t.Error("nil StartCursor ok")
	}
	if _, ok := p.EndCursor(); ok {
		t.Error("nil EndCursor ok")
	}
	np := NewCursorPager([]Node{nil}, DirectionNext, false)
	if _, ok := np.StartCursor(); ok {
		t.Error("nil element StartCursor ok")
	}
	if _, ok := np.EndCursor(); ok {
		t.Error("nil element EndCursor ok")
	}
}

func TestDefaultCursorPageParameter(t *testing.T) {
	for name, p := range map[string]CursorPageParameter{
		"constructor": NewDefaultCursorPageParameter(),
		"zero":        {},
	} {
		t.Run(name, func(t *testing.T) {
			if p.Limit() != 20 || p.Cursor() != nil || p.IsNext() || p.QueryLimit() != 21 {
				t.Errorf("unexpected default: %v", p)
			}
			if p.Direction() != DirectionPrev {
				t.Errorf("direction = %v", p.Direction())
			}
		})
	}
}

func TestNewCursorPageParameterLimit(t *testing.T) {
	tests := []struct {
		in, want int
	}{
		{50, 50}, {1, 1}, {1000, 1000}, {1001, 1000}, {math.MaxInt32, 1000}, {math.MaxInt, 1000},
		{0, 20}, {-5, 20}, {math.MinInt32, 20}, {math.MinInt, 20},
	}
	for _, tt := range tests {
		p := NewCursorPageParameter(nil, tt.in, DirectionNext)
		if p.Limit() != tt.want {
			t.Errorf("limit(%d) = %d, want %d", tt.in, p.Limit(), tt.want)
		}
		if p.QueryLimit() != p.Limit()+1 {
			t.Errorf("QueryLimit(%d) = %d", tt.in, p.QueryLimit())
		}
		if p.Limit() < 1 || p.Limit() > 1000 {
			t.Errorf("limit out of range: %d", p.Limit())
		}
	}
}

func TestQueryLimit(t *testing.T) {
	tests := []struct{ in, want int }{{0, 21}, {5000, 1001}, {1, 2}}
	for _, tt := range tests {
		if got := NewCursorPageParameter(nil, tt.in, DirectionNext).QueryLimit(); got != tt.want {
			t.Errorf("QueryLimit(%d) = %d, want %d", tt.in, got, tt.want)
		}
	}
}

func TestCursorAndDirection(t *testing.T) {
	ts := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
	tests := []struct {
		name   string
		cursor *time.Time
		dir    Direction
		isNext bool
	}{
		{"cursor next", &ts, DirectionNext, true},
		{"nil prev", nil, DirectionPrev, false},
		{"unknown dir", nil, Direction(42), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewCursorPageParameter(tt.cursor, 10, tt.dir)
			if p.Cursor() != tt.cursor {
				t.Errorf("cursor = %v", p.Cursor())
			}
			if p.Direction() != tt.dir || p.IsNext() != tt.isNext {
				t.Errorf("dir = %v isNext = %v", p.Direction(), p.IsNext())
			}
		})
	}
}

func TestSetDirection(t *testing.T) {
	p := NewCursorPageParameter(nil, 10, DirectionNext)
	p.SetDirection(DirectionPrev)
	if p.Direction() != DirectionPrev || p.IsNext() {
		t.Error("SetDirection(PREV) failed")
	}
	p.SetDirection(DirectionNext)
	if !p.IsNext() {
		t.Error("SetDirection(NEXT) failed")
	}
	if p.Limit() != 10 {
		t.Error("limit changed")
	}
}

func TestEqual(t *testing.T) {
	t1 := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	t1b := t1.In(time.FixedZone("X", 3600))
	t2 := t1.Add(time.Second)
	tests := []struct {
		name string
		a, b CursorPageParameter
		want bool
	}{
		{"same", NewCursorPageParameter(&t1, 10, DirectionNext), NewCursorPageParameter(&t1, 10, DirectionNext), true},
		{"same instant other ptr", NewCursorPageParameter(&t1, 10, DirectionNext), NewCursorPageParameter(&t1b, 10, DirectionNext), true},
		{"limit 0 vs 20", NewCursorPageParameter(&t1, 0, DirectionNext), NewCursorPageParameter(&t1, 20, DirectionNext), true},
		{"limit 1001 vs 1000", NewCursorPageParameter(&t1, 1001, DirectionNext), NewCursorPageParameter(&t1, 1000, DirectionNext), true},
		{"zero vs default", CursorPageParameter{}, NewDefaultCursorPageParameter(), true},
		{"both nil cursor", NewCursorPageParameter(nil, 5, DirectionPrev), NewCursorPageParameter(nil, 5, DirectionPrev), true},
		{"diff limit", NewCursorPageParameter(&t1, 10, DirectionNext), NewCursorPageParameter(&t1, 11, DirectionNext), false},
		{"diff dir", NewCursorPageParameter(&t1, 10, DirectionNext), NewCursorPageParameter(&t1, 10, DirectionPrev), false},
		{"diff cursor", NewCursorPageParameter(&t1, 10, DirectionNext), NewCursorPageParameter(&t2, 10, DirectionNext), false},
		{"nil vs cursor", NewCursorPageParameter(nil, 10, DirectionNext), NewCursorPageParameter(&t1, 10, DirectionNext), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.a.Equal(tt.b); got != tt.want {
				t.Errorf("a.Equal(b) = %v", got)
			}
			if got := tt.b.Equal(tt.a); got != tt.want {
				t.Errorf("b.Equal(a) = %v", got)
			}
			if tt.want && tt.a.Cursor() == nil && tt.a.String() != tt.b.String() {
				t.Errorf("String mismatch %q vs %q", tt.a.String(), tt.b.String())
			}
		})
	}
}

func TestString(t *testing.T) {
	ts := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	s := NewCursorPageParameter(nil, 0, DirectionNext).String()
	if s != "CursorPageParameter(limit=20, cursor=null, direction=NEXT)" {
		t.Errorf("String = %q", s)
	}
	s = NewCursorPageParameter(&ts, 5, DirectionPrev).String()
	if !strings.Contains(s, "limit=5") || !strings.Contains(s, ts.String()) || !strings.Contains(s, "direction=PREV") {
		t.Errorf("String = %q", s)
	}
}