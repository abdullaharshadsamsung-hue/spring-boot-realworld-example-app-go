// Package store contains the PostgreSQL-backed persistence layer (the
// replacement for the source's MyBatis mappers and type handlers).
package store

import (
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// ErrUnsupportedTimeValue is returned when a database value cannot be
// interpreted as a timestamp.
var ErrUnsupportedTimeValue = errors.New("store: unsupported timestamp value")

// UTCTime is a nullable timestamp that is always written to and read from the
// database as a UTC instant with millisecond precision.
//
// It replaces the source's MyBatis DateTimeHandler (a TypeHandler<DateTime>
// that bound/read JDBC Timestamps through a UTC Calendar). In Go the same
// "implicit conversion at the DB boundary" is expressed by implementing
// driver.Valuer (setParameter) and sql.Scanner (all three getResult
// overloads — by column name, by column index and from a callable
// statement collapse into a single Scan, because database/sql resolves the
// column before calling the scanner).
//
// Valid=false represents SQL NULL, the equivalent of a null Joda DateTime.
//
// MIGRATION_NOTE: MyBatis applied this handler transparently to every
// DateTime property. database/sql has no global type registry, so repository
// code must pass UTCTime (or the result of ToDB) as query arguments and scan
// into *UTCTime explicitly.
type UTCTime struct {
	Time  time.Time
	Valid bool
}

// NewUTCTime wraps t as a valid (non-NULL) UTCTime normalised to UTC and
// truncated to millisecond precision, matching Joda DateTime's resolution.
func NewUTCTime(t time.Time) UTCTime {
	return UTCTime{Time: normalize(t), Valid: true}
}

// Value implements driver.Valuer. A NULL UTCTime binds SQL NULL; otherwise the
// instant is bound as a UTC time.Time so that a PostgreSQL TIMESTAMP (without
// time zone) column stores the UTC wall-clock, exactly as the source's
// ps.setTimestamp(i, ts, UTC_CALENDAR) did.
func (u UTCTime) Value() (driver.Value, error) {
	if !u.Valid {
		return nil, nil
	}
	return normalize(u.Time), nil
}

// Scan implements sql.Scanner. SQL NULL yields Valid=false; any other
// supported value is converted with FromDB.
func (u *UTCTime) Scan(src any) error {
	if src == nil {
		*u = UTCTime{}
		return nil
	}
	t, err := FromDB(src)
	if err != nil {
		return err
	}
	*u = UTCTime{Time: t, Valid: true}
	return nil
}

// Ptr returns a pointer to the time, or nil when the value is NULL. It is a
// convenience for DTOs that model a nullable timestamp as *time.Time.
func (u UTCTime) Ptr() *time.Time {
	if !u.Valid {
		return nil
	}
	t := u.Time
	return &t
}

// ToDB converts t to a value suitable as a query argument for a timestamp
// column: the UTC instant at millisecond precision.
func ToDB(t time.Time) driver.Value {
	return normalize(t)
}

// ToMillis returns t as milliseconds since the Unix epoch (UTC), the same
// value Joda's DateTime.getMillis() produced.
func ToMillis(t time.Time) int64 {
	return t.UnixMilli()
}

// FromMillis converts epoch milliseconds into a UTC time.Time.
func FromMillis(ms int64) time.Time {
	return time.UnixMilli(ms).UTC()
}

// textLayouts are the textual timestamp formats accepted defensively by
// FromDB (PostgreSQL text output, RFC 3339 and common SQL formats).
var textLayouts = []string{
	time.RFC3339Nano,
	"2006-01-02 15:04:05.999999999Z07:00",
	"2006-01-02 15:04:05.999999999Z07",
	"2006-01-02 15:04:05.999999999",
	"2006-01-02T15:04:05.999999999",
	"2006-01-02",
}

// FromDB converts a non-NULL database value into a UTC time.Time.
//
// Supported inputs are time.Time (the normal PostgreSQL driver result),
// integer epoch milliseconds (the representation sqlite-jdbc wrote for the
// source project), and textual timestamps ([]byte or string), which are parsed
// as UTC when they carry no zone — mirroring the UTC Calendar the source used
// when reading. A nil input is an error; use UTCTime to handle NULL.
func FromDB(v any) (time.Time, error) {
	switch x := v.(type) {
	case time.Time:
		return normalize(x), nil
	case int64:
		return FromMillis(x), nil
	case int:
		return FromMillis(int64(x)), nil
	case int32:
		return FromMillis(int64(x)), nil
	case float64:
		return FromMillis(int64(x)), nil
	case []byte:
		return parseText(string(x))
	case string:
		return parseText(x)
	case nil:
		return time.Time{}, fmt.Errorf("%w: NULL", ErrUnsupportedTimeValue)
	default:
		return time.Time{}, fmt.Errorf("%w: %T", ErrUnsupportedTimeValue, v)
	}
}

func parseText(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, fmt.Errorf("%w: empty string", ErrUnsupportedTimeValue)
	}
	if ms, err := strconv.ParseInt(s, 10, 64); err == nil {
		return FromMillis(ms), nil
	}
	for _, layout := range textLayouts {
		if t, err := time.ParseInLocation(layout, s, time.UTC); err == nil {
			return normalize(t), nil
		}
	}
	return time.Time{}, fmt.Errorf("%w: %q", ErrUnsupportedTimeValue, s)
}

// normalize converts t to UTC and truncates it to millisecond precision.
func normalize(t time.Time) time.Time {
	return t.UTC().Truncate(time.Millisecond)
}

// Compile-time interface checks.
var (
	_ driver.Valuer = UTCTime{}
	_ sql.Scanner   = (*UTCTime)(nil)
)
