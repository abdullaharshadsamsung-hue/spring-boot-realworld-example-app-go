package store

import (
	"database/sql"
	"database/sql/driver"
	"errors"
	"testing"
	"time"
)

func TestNewUTCTime(t *testing.T) {
	loc := time.FixedZone("X", 5*3600)
	in := time.Date(2020, 1, 2, 8, 4, 5, 123456789, loc)
	u := NewUTCTime(in)
	if !u.Valid {
		t.Fatal("expected valid")
	}
	if u.Time.Location() != time.UTC {
		t.Errorf("expected UTC, got %v", u.Time.Location())
	}
	want := time.Date(2020, 1, 2, 3, 4, 5, 123000000, time.UTC)
	if !u.Time.Equal(want) {
		t.Errorf("got %v want %v", u.Time, want)
	}
}

func TestUTCTimeValue(t *testing.T) {
	inst := time.Date(2021, 6, 1, 12, 0, 0, 987654321, time.UTC)
	ny := time.FixedZone("NY", -4*3600)
	tokyo := time.FixedZone("TK", 9*3600)
	want := time.Date(2021, 6, 1, 12, 0, 0, 987000000, time.UTC)

	tests := []struct {
		name    string
		in      UTCTime
		wantNil bool
	}{
		{"null binds nil", UTCTime{}, true},
		{"null with time set binds nil", UTCTime{Time: inst, Valid: false}, true},
		{"utc", UTCTime{Time: inst, Valid: true}, false},
		{"ny same instant", UTCTime{Time: inst.In(ny), Valid: true}, false},
		{"tokyo same instant", UTCTime{Time: inst.In(tokyo), Valid: true}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			v, err := tc.in.Value()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.wantNil {
				if v != nil {
					t.Fatalf("expected nil, got %v", v)
				}
				return
			}
			got, ok := v.(time.Time)
			if !ok {
				t.Fatalf("expected time.Time, got %T", v)
			}
			if got.Location() != time.UTC {
				t.Errorf("not UTC: %v", got.Location())
			}
			if !got.Equal(want) || got.UnixMilli() != inst.UnixMilli() {
				t.Errorf("got %v want %v", got, want)
			}
		})
	}
}

func TestUTCTimeScan(t *testing.T) {
	inst := time.Date(2022, 3, 4, 5, 6, 7, 8000000, time.UTC)
	tests := []struct {
		name      string
		src       any
		wantValid bool
		want      time.Time
		wantErr   bool
	}{
		{"null", nil, false, time.Time{}, false},
		{"time", inst.In(time.FixedZone("Z", 3600)), true, inst, false},
		{"int64 millis", inst.UnixMilli(), true, inst, false},
		{"bytes", []byte("2022-03-04 05:06:07.008"), true, inst, false},
		{"string", "2022-03-04T05:06:07.008Z", true, inst, false},
		{"unsupported", true, false, time.Time{}, true},
		{"bad string", "nope", false, time.Time{}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			u := UTCTime{Time: time.Now(), Valid: true}
			err := u.Scan(tc.src)
			if tc.wantErr {
				if !errors.Is(err, ErrUnsupportedTimeValue) {
					t.Fatalf("expected ErrUnsupportedTimeValue, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if u.Valid != tc.wantValid {
				t.Fatalf("valid=%v want %v", u.Valid, tc.wantValid)
			}
			if tc.wantValid {
				if !u.Time.Equal(tc.want) || u.Time.Location() != time.UTC {
					t.Errorf("got %v want %v", u.Time, tc.want)
				}
			} else if !u.Time.IsZero() {
				t.Errorf("expected zero time for NULL")
			}
		})
	}
}

func TestRoundTrip(t *testing.T) {
	zones := []*time.Location{time.UTC, time.FixedZone("A", -7*3600), time.FixedZone("B", 10*3600+1800)}
	base := time.Date(2019, 12, 31, 23, 59, 59, 999000000, time.UTC)
	for _, z := range zones {
		t.Run(z.String(), func(t *testing.T) {
			v, err := NewUTCTime(base.In(z)).Value()
			if err != nil {
				t.Fatal(err)
			}
			var u UTCTime
			if err := u.Scan(v); err != nil {
				t.Fatal(err)
			}
			if !u.Valid || u.Time.UnixMilli() != base.UnixMilli() {
				t.Errorf("round trip mismatch: %v", u.Time)
			}
		})
	}
	// NULL round trip
	v, err := UTCTime{}.Value()
	if err != nil || v != nil {
		t.Fatalf("null value: %v %v", v, err)
	}
	var u UTCTime
	if err := u.Scan(v); err != nil || u.Valid {
		t.Fatalf("null scan: %v %v", u, err)
	}
}

func TestPtr(t *testing.T) {
	if (UTCTime{}).Ptr() != nil {
		t.Error("expected nil")
	}
	inst := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	u := NewUTCTime(inst)
	p := u.Ptr()
	if p == nil || !p.Equal(inst) {
		t.Fatalf("got %v", p)
	}
	*p = p.Add(time.Hour)
	if !u.Time.Equal(inst) {
		t.Error("Ptr must return a copy")
	}
}

func TestToDBAndMillis(t *testing.T) {
	in := time.Date(2020, 5, 5, 5, 5, 5, 555555555, time.FixedZone("Q", -3600))
	v := ToDB(in)
	got, ok := v.(time.Time)
	if !ok {
		t.Fatalf("got %T", v)
	}
	if got.Location() != time.UTC || got.Nanosecond() != 555000000 || got.UnixMilli() != in.UnixMilli() {
		t.Errorf("ToDB got %v", got)
	}
	tests := []int64{0, 1, -1, 1700000000123}
	for _, ms := range tests {
		ft := FromMillis(ms)
		if ft.Location() != time.UTC {
			t.Errorf("not UTC")
		}
		if ToMillis(ft) != ms {
			t.Errorf("millis %d -> %d", ms, ToMillis(ft))
		}
	}
	if ToMillis(in) != in.UnixMilli() {
		t.Error("ToMillis mismatch")
	}
}

func TestFromDB(t *testing.T) {
	inst := time.Date(2023, 7, 8, 9, 10, 11, 120000000, time.UTC)
	day := time.Date(2023, 7, 8, 0, 0, 0, 0, time.UTC)
	ms := inst.UnixMilli()
	tests := []struct {
		name    string
		in      any
		want    time.Time
		wantErr bool
	}{
		{"time", inst, inst, false},
		{"int64", ms, inst, false},
		{"int", int(ms), inst, false},
		{"int32", int32(1000), FromMillis(1000), false},
		{"float64", float64(ms), inst, false},
		{"string millis", "  1688807411120 ", inst, false},
		{"rfc3339", "2023-07-08T11:10:11.12+02:00", inst, false},
		{"pg tz", "2023-07-08 09:10:11.12+00", inst, false},
		{"pg tz full", "2023-07-08 09:10:11.12+00:00", inst, false},
		{"no zone space", "2023-07-08 09:10:11.12", inst, false},
		{"no zone T", "2023-07-08T09:10:11.12", inst, false},
		{"date", []byte("2023-07-08"), day, false},
		{"nil", nil, time.Time{}, true},
		{"empty", "   ", time.Time{}, true},
		{"garbage", []byte("xyz"), time.Time{}, true},
		{"unsupported type", struct{}{}, time.Time{}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := FromDB(tc.in)
			if tc.wantErr {
				if !errors.Is(err, ErrUnsupportedTimeValue) {
					t.Fatalf("expected ErrUnsupportedTimeValue, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !got.Equal(tc.want) || got.Location() != time.UTC {
				t.Errorf("got %v want %v", got, tc.want)
			}
		})
	}
}

// fakeRow mimics a DB driver delivering a column value to a Scanner,
// standing in for ResultSet / CallableStatement lookups.
type rowSource interface {
	ScanCol(dest sql.Scanner) error
}

type fakeRow struct {
	val any
	err error
}

func (f fakeRow) ScanCol(dest sql.Scanner) error {
	if f.err != nil {
		return f.err
	}
	return dest.Scan(f.val)
}

func TestScanViaSourcePropagatesErrors(t *testing.T) {
	driverErr := errors.New("unknown column")
	tests := []struct {
		name    string
		src     rowSource
		wantErr error
		valid   bool
	}{
		{"value", fakeRow{val: time.Unix(10, 0)}, nil, true},
		{"null", fakeRow{val: nil}, nil, false},
		{"driver error", fakeRow{err: driverErr}, driverErr, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var u UTCTime
			err := tc.src.ScanCol(&u)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if u.Valid != tc.valid {
				t.Errorf("valid=%v", u.Valid)
			}
		})
	}
	var _ driver.Valuer = UTCTime{}
}