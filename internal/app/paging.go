package app

import (
	"fmt"
	"time"
)

// Direction is the paging direction requested by a cursor query.
//
// It replaces the source's nested CursorPager.Direction enum.
type Direction int

const (
	// DirectionPrev requests the page that comes before the supplied cursor.
	DirectionPrev Direction = iota
	// DirectionNext requests the page that comes after the supplied cursor.
	// It is also the direction used when no cursor is supplied.
	DirectionNext
)

// String returns the source enum's constant name ("PREV" or "NEXT").
func (d Direction) String() string {
	switch d {
	case DirectionPrev:
		return "PREV"
	case DirectionNext:
		return "NEXT"
	default:
		return "UNKNOWN"
	}
}

// Node is an item that can appear in a cursor-paginated result. Each item
// must provide an opaque cursor that identifies its position in the
// ordering.
//
// MIGRATION_NOTE: the source's Node.getCursor() returns a PageCursor object
// that callers only ever serialize with toString(). Here Cursor returns that
// serialized form directly, so StartCursor and EndCursor can report
// (string, bool) as the agreed plan specifies.
type Node interface {
	// Cursor returns the opaque, serialized cursor for this item.
	Cursor() string
}

// CursorPager is one page of Node items in a Relay-style cursor
// pagination. It records whether a following or preceding page exists.
//
// It is a read-only value object. Build it with NewCursorPager.
type CursorPager[T Node] struct {
	data     []T
	next     bool
	previous bool
}

// NewCursorPager builds a page from data.
//
// hasExtra reports whether the caller's query (usually limit+1) found at
// least one more item past this page in the requested direction.
//   - For DirectionNext, HasNext is hasExtra and HasPrevious is false.
//   - For any other direction (DirectionPrev), HasPrevious is hasExtra and
//     HasNext is false.
//
// This matches the source exactly: any value that is not NEXT is treated
// as PREV.
func NewCursorPager[T Node](data []T, direction Direction, hasExtra bool) *CursorPager[T] {
	p := &CursorPager[T]{data: data}
	if direction == DirectionNext {
		p.previous = false
		p.next = hasExtra
	} else {
		p.next = false
		p.previous = hasExtra
	}
	return p
}

// Data returns the items supplied when the page was built.
//
// The returned slice is the same one passed to NewCursorPager. It is not
// copied, which matches the source's getter.
func (p *CursorPager[T]) Data() []T {
	if p == nil {
		return nil
	}
	return p.data
}

// HasNext reports whether a following page exists.
func (p *CursorPager[T]) HasNext() bool {
	return p != nil && p.next
}

// HasPrevious reports whether a preceding page exists.
func (p *CursorPager[T]) HasPrevious() bool {
	return p != nil && p.previous
}

// StartCursor returns the cursor of the first item on the page.
//
// ok is false when the page is empty. That replaces the source's null
// return value.
func (p *CursorPager[T]) StartCursor() (cursor string, ok bool) {
	if p == nil || len(p.data) == 0 {
		return "", false
	}
	return cursorOf(p.data[0])
}

// EndCursor returns the cursor of the last item on the page.
//
// ok is false when the page is empty. That replaces the source's null
// return value.
func (p *CursorPager[T]) EndCursor() (cursor string, ok bool) {
	if p == nil || len(p.data) == 0 {
		return "", false
	}
	return cursorOf(p.data[len(p.data)-1])
}

// cursorOf returns the cursor of item. ok is false when item is a nil
// interface value.
//
// MIGRATION_NOTE: Java would throw a NullPointerException for a null
// element. Here a nil element is reported as an absent cursor, so it cannot
// panic.
func cursorOf[T Node](item T) (string, bool) {
	if any(item) == nil {
		return "", false
	}
	return item.Cursor(), true
}

const (
	// DefaultCursorPageLimit is the page size used when no positive limit is
	// supplied.
	DefaultCursorPageLimit = 20
	// MaxCursorPageLimit is the largest page size a caller may request;
	// larger values are clamped to it.
	MaxCursorPageLimit = 1000
)

// CursorPageParameter is the cursor (keyset) pagination input passed from
// the API layers to the read queries. It replaces the source's generic
// CursorPageParameter<T>; every source caller used a DateTime cursor, so the
// cursor is a *time.Time here (nil means "start from the beginning").
//
// limit and cursor are unexported and can only be set through
// NewCursorPageParameter, mirroring the source's private setters. Direction
// may be changed with SetDirection.
//
// The zero value is the source's no-args instance: limit 20, nil cursor and
// no NEXT direction (IsNext reports false).
type CursorPageParameter struct {
	limit     int // 0 means DefaultCursorPageLimit, so the zero value is usable
	cursor    *time.Time
	direction Direction
}

// NewDefaultCursorPageParameter returns a parameter with default values:
// limit 20, nil cursor and the zero direction.
func NewDefaultCursorPageParameter() CursorPageParameter {
	return CursorPageParameter{limit: DefaultCursorPageLimit}
}

// NewCursorPageParameter builds a pagination parameter. A limit above
// MaxCursorPageLimit is capped at MaxCursorPageLimit; a non-positive limit is
// ignored and the default of 20 is kept.
func NewCursorPageParameter(cursor *time.Time, limit int, direction Direction) CursorPageParameter {
	p := NewDefaultCursorPageParameter()
	p.setLimit(limit)
	p.cursor = cursor
	p.direction = direction
	return p
}

func (p *CursorPageParameter) setLimit(limit int) {
	switch {
	case limit > MaxCursorPageLimit:
		p.limit = MaxCursorPageLimit
	case limit > 0:
		p.limit = limit
	}
}

// Limit returns the page size, always within 1..MaxCursorPageLimit.
func (p CursorPageParameter) Limit() int {
	if p.limit <= 0 {
		return DefaultCursorPageLimit
	}
	return p.limit
}

// Cursor returns the cursor position, or nil when paging from the start.
func (p CursorPageParameter) Cursor() *time.Time {
	return p.cursor
}

// Direction returns the requested paging direction.
func (p CursorPageParameter) Direction() Direction {
	return p.direction
}

// SetDirection changes the requested paging direction.
func (p *CursorPageParameter) SetDirection(direction Direction) {
	p.direction = direction
}

// IsNext reports whether the direction is DirectionNext.
func (p CursorPageParameter) IsNext() bool {
	return p.direction == DirectionNext
}

// QueryLimit returns Limit()+1. Queries fetch one extra row so the caller
// can tell whether another page exists.
func (p CursorPageParameter) QueryLimit() int {
	return p.Limit() + 1
}

// Equal reports whether p and other have the same limit, cursor instant and
// direction (the source's Lombok-generated equals). Cursors are compared by
// instant, not by pointer identity.
func (p CursorPageParameter) Equal(other CursorPageParameter) bool {
	if p.Limit() != other.Limit() || p.direction != other.direction {
		return false
	}
	switch {
	case p.cursor == nil && other.cursor == nil:
		return true
	case p.cursor == nil || other.cursor == nil:
		return false
	default:
		return p.cursor.Equal(*other.cursor)
	}
}

// String returns a debug representation like the source's Lombok toString.
func (p CursorPageParameter) String() string {
	cursor := "null"
	if p.cursor != nil {
		cursor = p.cursor.String()
	}
	return fmt.Sprintf("CursorPageParameter(limit=%d, cursor=%s, direction=%s)",
		p.Limit(), cursor, p.direction)
}
