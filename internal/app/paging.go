// Package app contains the application-layer (query/command service) types of
// the RealWorld application.
package app

import "fmt"

// Pagination defaults and bounds.
const (
	// DefaultOffset is the offset used when none (or a non-positive one) is given.
	DefaultOffset = 0
	// DefaultLimit is the page size used when none (or a non-positive one) is given.
	DefaultLimit = 20
	// MaxLimit is the largest page size a caller may request; larger values are capped.
	MaxLimit = 100
)

// Page holds sanitized offset/limit pagination parameters.
//
// Fields are unexported so a Page can only hold sanitized values: build one
// with NewPage or DefaultPage. The zero value (Page{}) is NOT equivalent to
// the source's no-arg constructor because its limit is 0; use DefaultPage.
//
// Page is a comparable value type: == (or Equal) compares offset and limit,
// matching the equals/hashCode Lombok's @Data generated, and Page can be used
// directly as a map key.
type Page struct {
	offset int
	limit  int
}

// DefaultPage returns a Page with the default offset (0) and limit (20).
func DefaultPage() Page {
	return Page{offset: DefaultOffset, limit: DefaultLimit}
}

// NewPage returns a Page built from caller-supplied values, sanitized rather
// than rejected:
//   - offset: values <= 0 are ignored and the default 0 is kept.
//   - limit: values <= 0 are ignored and the default 20 is kept; values above
//     MaxLimit (100) are capped at 100.
func NewPage(offset, limit int) Page {
	p := DefaultPage()
	if offset > 0 {
		p.offset = offset
	}
	switch {
	case limit > MaxLimit:
		p.limit = MaxLimit
	case limit > 0:
		p.limit = limit
	}
	return p
}

// Offset returns the sanitized number of rows to skip.
func (p Page) Offset() int { return p.offset }

// Limit returns the sanitized maximum number of rows to return.
func (p Page) Limit() int { return p.limit }

// Equal reports whether p and other have the same offset and limit.
func (p Page) Equal(other Page) bool { return p == other }

// String returns a representation like Lombok's toString: Page(offset=0, limit=20).
func (p Page) String() string {
	return fmt.Sprintf("Page(offset=%d, limit=%d)", p.offset, p.limit)
}
