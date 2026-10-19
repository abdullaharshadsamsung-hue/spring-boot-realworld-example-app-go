package app

import (
	"testing"
)

type testNode struct{ c string }

func (n testNode) Cursor() string { return n.c }

type ptrNode struct{ c string }

func (n *ptrNode) Cursor() string { return n.c }

func TestDirectionString(t *testing.T) {
	tests := []struct {
		d    Direction
		want string
	}{
		{DirectionPrev, "PREV"},
		{DirectionNext, "NEXT"},
		{Direction(99), "UNKNOWN"},
		{Direction(-1), "UNKNOWN"},
	}
	for _, tt := range tests {
		if got := tt.d.String(); got != tt.want {
			t.Errorf("Direction(%d).String() = %q, want %q", int(tt.d), got, tt.want)
		}
	}
}

func TestNewCursorPagerFlags(t *testing.T) {
	tests := []struct {
		name      string
		dir       Direction
		hasExtra  bool
		wantNext  bool
		wantPrev  bool
	}{
		{"next extra", DirectionNext, true, true, false},
		{"next no extra", DirectionNext, false, false, false},
		{"prev extra", DirectionPrev, true, false, true},
		{"prev no extra", DirectionPrev, false, false, false},
		{"unknown treated as prev extra", Direction(42), true, false, true},
		{"unknown treated as prev no extra", Direction(42), false, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewCursorPager([]testNode{{"a"}}, tt.dir, tt.hasExtra)
			if p.HasNext() != tt.wantNext {
				t.Errorf("HasNext = %v, want %v", p.HasNext(), tt.wantNext)
			}
			if p.HasPrevious() != tt.wantPrev {
				t.Errorf("HasPrevious = %v, want %v", p.HasPrevious(), tt.wantPrev)
			}
			if p.HasNext() && p.HasPrevious() {
				t.Error("next and previous both true")
			}
		})
	}
}

func TestDataSameInstanceAndOrder(t *testing.T) {
	data := []testNode{{"1"}, {"2"}, {"3"}}
	p := NewCursorPager(data, DirectionNext, false)
	got := p.Data()
	if len(got) != 3 || &got[0] != &data[0] {
		t.Fatal("Data did not return same slice")
	}
	for i := range data {
		if got[i].c != data[i].c {
			t.Errorf("order mismatch at %d", i)
		}
	}
}

func TestNilData(t *testing.T) {
	p := NewCursorPager[testNode](nil, DirectionNext, true)
	if p == nil {
		t.Fatal("expected non-nil pager")
	}
	if p.Data() != nil {
		t.Error("expected nil data")
	}
	if c, ok := p.StartCursor(); ok || c != "" {
		t.Errorf("StartCursor = %q,%v", c, ok)
	}
	if c, ok := p.EndCursor(); ok || c != "" {
		t.Errorf("EndCursor = %q,%v", c, ok)
	}
}

func TestNilPager(t *testing.T) {
	var p *CursorPager[testNode]
	if p.Data() != nil || p.HasNext() || p.HasPrevious() {
		t.Error("nil pager should report zero values")
	}
	if _, ok := p.StartCursor(); ok {
		t.Error("StartCursor ok on nil pager")
	}
	if _, ok := p.EndCursor(); ok {
		t.Error("EndCursor ok on nil pager")
	}
}

func TestCursors(t *testing.T) {
	tests := []struct {
		name      string
		data      []testNode
		wantStart string
		wantEnd   string
		wantOK    bool
	}{
		{"empty", []testNode{}, "", "", false},
		{"single", []testNode{{"x"}}, "x", "x", true},
		{"many", []testNode{{"a"}, {"b"}, {"c"}}, "a", "c", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewCursorPager(tt.data, DirectionPrev, true)
			s, ok := p.StartCursor()
			if s != tt.wantStart || ok != tt.wantOK {
				t.Errorf("StartCursor = %q,%v want %q,%v", s, ok, tt.wantStart, tt.wantOK)
			}
			e, ok := p.EndCursor()
			if e != tt.wantEnd || ok != tt.wantOK {
				t.Errorf("EndCursor = %q,%v want %q,%v", e, ok, tt.wantEnd, tt.wantOK)
			}
		})
	}
}

func TestCursorsComputedFromCurrentContents(t *testing.T) {
	data := []testNode{{"a"}, {"b"}}
	p := NewCursorPager(data, DirectionNext, false)
	data[0] = testNode{"z"}
	data[1] = testNode{"y"}
	if s, _ := p.StartCursor(); s != "z" {
		t.Errorf("StartCursor = %q, want z", s)
	}
	if e, _ := p.EndCursor(); e != "y" {
		t.Errorf("EndCursor = %q, want y", e)
	}
}

func TestNilElementCursor(t *testing.T) {
	var nilIface Node
	p := NewCursorPager([]Node{nilIface, testNode{"b"}, nilIface}, DirectionNext, false)
	if c, ok := p.StartCursor(); ok || c != "" {
		t.Errorf("StartCursor = %q,%v", c, ok)
	}
	if c, ok := p.EndCursor(); ok || c != "" {
		t.Errorf("EndCursor = %q,%v", c, ok)
	}
	p2 := NewCursorPager([]*ptrNode{{"p1"}, {"p2"}}, DirectionNext, false)
	if c, ok := p2.StartCursor(); !ok || c != "p1" {
		t.Errorf("StartCursor = %q,%v", c, ok)
	}
	if c, ok := p2.EndCursor(); !ok || c != "p2" {
		t.Errorf("EndCursor = %q,%v", c, ok)
	}
}