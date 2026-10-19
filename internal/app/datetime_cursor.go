package app

import (
	"errors"
	"fmt"
	"strconv"
	"time"
)

// ErrInvalidCursor is returned (wrapped) by ParseDateTimeCursor when the
// cursor string is not a base-10 int64 count of epoch milliseconds. Match it
// with errors.Is(err, app.ErrInvalidCursor).
//
// MIGRATION_NOTE: the source let Long.parseLong throw an unchecked
// NumberFormatException; Go returns this sentinel instead.
var ErrInvalidCursor = errors.New("invalid cursor")

// DateTimeCursor is a keyset-pagination cursor whose value is a timestamp.
// It serializes as the decimal epoch milliseconds of that timestamp
// (see String) and round-trips through ParseDateTimeCursor.
//
// MIGRATION_NOTE: the source extends the generic PageCursor<DateTime>; Go has
// no class inheritance, so the cursor is a small concrete value type exposing
// the same Data accessor and String serialization.
type DateTimeCursor struct {
	data time.Time
}

// NewDateTimeCursor returns a cursor wrapping t as its data.
func NewDateTimeCursor(t time.Time) DateTimeCursor {
	return DateTimeCursor{data: t}
}

// Data returns the wrapped timestamp.
func (c DateTimeCursor) Data() time.Time { return c.data }

// String returns the wrapped timestamp as decimal epoch milliseconds,
// e.g. "1700000000123". Sub-millisecond precision is truncated, as with
// Joda's DateTime.getMillis.
func (c DateTimeCursor) String() string {
	return strconv.FormatInt(c.data.UnixMilli(), 10)
}

// ParseDateTimeCursor converts a cursor string of decimal epoch milliseconds
// into a UTC time. An empty cursor means "no cursor" and yields (nil, nil),
// mirroring the source's null-in/null-out behavior. Any non-numeric or
// out-of-range input yields an error wrapping ErrInvalidCursor.
//
// MIGRATION_NOTE: Go strings cannot be null, so the empty string stands in
// for Java's null. (In the source, "" would have thrown NumberFormatException;
// here it is treated as absent.)
func ParseDateTimeCursor(cursor string) (*time.Time, error) {
	if cursor == "" {
		return nil, nil
	}
	ms, err := strconv.ParseInt(cursor, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("parse date-time cursor %q: %w", cursor, errors.Join(ErrInvalidCursor, err))
	}
	t := time.UnixMilli(ms).UTC()
	return &t, nil
}
