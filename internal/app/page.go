package app

const (
	// DefaultPageLimit is the page size used when no positive limit is given.
	DefaultPageLimit = 20
	// MaxPageLimit is the largest page size a caller may request.
	MaxPageLimit = 100
)

// Page is the offset/limit pagination input for the offset-based list
// queries (the source's Page class). The zero value means offset 0 and the
// default limit of 20.
type Page struct {
	offset int
	limit  int // 0 means DefaultPageLimit
}

// NewPage builds a Page. A negative offset is ignored (0 is kept); a limit
// above MaxPageLimit is capped and a non-positive limit keeps the default.
func NewPage(offset, limit int) Page {
	p := Page{limit: DefaultPageLimit}
	if offset > 0 {
		p.offset = offset
	}
	switch {
	case limit > MaxPageLimit:
		p.limit = MaxPageLimit
	case limit > 0:
		p.limit = limit
	}
	return p
}

// Offset returns the number of rows to skip.
func (p Page) Offset() int { return p.offset }

// Limit returns the page size, always within 1..MaxPageLimit.
func (p Page) Limit() int {
	if p.limit <= 0 {
		return DefaultPageLimit
	}
	return p.limit
}
