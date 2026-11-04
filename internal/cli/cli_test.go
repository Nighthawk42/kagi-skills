package cli

import (
	"reflect"
	"strings"
	"testing"
)

func TestPositionalsInterleavedWithFlags(t *testing.T) {
	a := NewArgs([]string{"rust", "--limit", "5", "async", "--format=json", "runtime"})
	limit := 0
	format := ""
	for a.Next() {
		switch {
		case a.Is("--limit"):
			n, err := a.Int()
			if err != nil {
				t.Fatalf("Int: %v", err)
			}
			limit = n
		case a.Is("--format"):
			s, err := a.String()
			if err != nil {
				t.Fatalf("String: %v", err)
			}
			format = s
		default:
			t.Fatalf("unexpected flag %q", a.Flag())
		}
	}
	if limit != 5 || format != "json" {
		t.Errorf("limit=%d format=%q, want 5 and json", limit, format)
	}
	if got := a.Query(); got != "rust async runtime" {
		t.Errorf("Query() = %q, want %q", got, "rust async runtime")
	}
}

func TestDoubleDashTerminatorTakesDashedPositionals(t *testing.T) {
	a := NewArgs([]string{"--limit=3", "--", "--not-a-flag", "-x", "end"})
	for a.Next() {
		if !a.Is("--limit") {
			t.Fatalf("unexpected flag %q", a.Flag())
		}
		if _, err := a.Int(); err != nil {
			t.Fatalf("Int: %v", err)
		}
	}
	want := []string{"--not-a-flag", "-x", "end"}
	if !reflect.DeepEqual(a.Positionals, want) {
		t.Errorf("Positionals = %q, want %q", a.Positionals, want)
	}
	if got := a.Query(); got != "--not-a-flag -x end" {
		t.Errorf("Query() = %q", got)
	}
}

func TestBareDashIsPositional(t *testing.T) {
	a := NewArgs([]string{"-"})
	if a.Next() {
		t.Errorf("Next() treated bare - as a flag (%q)", a.Flag())
	}
	if !reflect.DeepEqual(a.Positionals, []string{"-"}) {
		t.Errorf("Positionals = %q, want [-]", a.Positionals)
	}
}

func TestIntRejectsTrailingGarbage(t *testing.T) {
	for _, raw := range []string{"12abc", "abc", "1.5", ""} {
		t.Run(raw, func(t *testing.T) {
			a := NewArgs([]string{"--timeout", raw})
			if !a.Next() {
				t.Fatal("Next() = false")
			}
			if _, err := a.Int(); err == nil {
				t.Errorf("Int() on %q returned nil error", raw)
			}
		})
	}
}

func TestIntAcceptsValid(t *testing.T) {
	a := NewArgs([]string{"--limit", " 42 "})
	a.Next()
	n, err := a.Int()
	if err != nil || n != 42 {
		t.Errorf("Int() = %d, %v; want 42, nil", n, err)
	}
}

func TestIntInRangeRejectsOutOfRange(t *testing.T) {
	cases := []struct {
		raw     string
		wantErr bool
	}{
		{"0", true},
		{"1", false},
		{"1024", false},
		{"1025", true},
	}
	for _, tc := range cases {
		a := NewArgs([]string{"--limit", tc.raw})
		a.Next()
		_, err := a.IntInRange(1, 1024)
		if tc.wantErr && err == nil {
			t.Errorf("IntInRange(1,1024) on %s: nil error", tc.raw)
		}
		if !tc.wantErr && err != nil {
			t.Errorf("IntInRange(1,1024) on %s: %v", tc.raw, err)
		}
	}

	a := NewArgs([]string{"--limit", "2000"})
	a.Next()
	_, err := a.IntInRange(1, 1024)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "1-1024") {
		t.Errorf("error %q does not report bounds 1-1024", err)
	}
}

func TestFloatInRangeRejectsOutOfRange(t *testing.T) {
	for _, raw := range []string{"0.1", "4.5", "-1"} {
		a := NewArgs([]string{"--timeout", raw})
		a.Next()
		_, err := a.FloatInRange(0.5, 4)
		if err == nil {
			t.Errorf("FloatInRange(0.5,4) on %s: nil error", raw)
		}
	}

	a := NewArgs([]string{"--timeout", "5"})
	a.Next()
	_, err := a.FloatInRange(0.5, 4)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "0.5-4") {
		t.Errorf("error %q does not report bounds 0.5-4", err)
	}
}

func TestFloatInRangeAcceptsBoundsAndValues(t *testing.T) {
	for _, raw := range []string{"0.5", "2.25", "4"} {
		a := NewArgs([]string{"--timeout", raw})
		a.Next()
		if _, err := a.FloatInRange(0.5, 4); err != nil {
			t.Errorf("FloatInRange(0.5,4) on %s: %v", raw, err)
		}
	}
}

// TestFloatInRangeRejectsNaN is a regression test: strconv.ParseFloat accepts
// "NaN", and every comparison against NaN is false, so a naive range check
// passes it straight through to the API.
func TestFloatInRangeRejectsNaN(t *testing.T) {
	for _, raw := range []string{"NaN", "nan", "-nan"} {
		a := NewArgs([]string{"--timeout", raw})
		a.Next()
		if _, err := a.FloatInRange(0.5, 4); err == nil {
			t.Errorf("FloatInRange(0.5,4) accepted %q", raw)
		}
	}
}

func TestEnumIsCaseInsensitiveAndCanonical(t *testing.T) {
	for _, raw := range []string{"JSON", "Json", "json", " json "} {
		a := NewArgs([]string{"--format", raw})
		a.Next()
		got, err := a.Enum("text", "json", "markdown")
		if err != nil {
			t.Errorf("Enum on %q: %v", raw, err)
			continue
		}
		if got != "json" {
			t.Errorf("Enum on %q = %q, want canonical json", raw, got)
		}
	}
}

func TestEnumRejectsAndListsValidValues(t *testing.T) {
	a := NewArgs([]string{"--format", "yaml"})
	a.Next()
	_, err := a.Enum("text", "json", "markdown")
	if err == nil {
		t.Fatal("Enum accepted invalid value")
	}
	for _, want := range []string{"text", "json", "markdown", "--format"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q missing %q", err, want)
		}
	}
}

func TestStringListSplitsTrimsAndSkipsEmpty(t *testing.T) {
	a := NewArgs([]string{"--site", " a.com , ,b.com,,  c.com ,"})
	a.Next()
	got, err := a.StringList(nil)
	if err != nil {
		t.Fatalf("StringList: %v", err)
	}
	want := []string{"a.com", "b.com", "c.com"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("StringList = %q, want %q", got, want)
	}
}

func TestStringListAccumulatesAcrossRepeatedFlags(t *testing.T) {
	a := NewArgs([]string{"--site", "a.com", "--site=b.com,c.com", "--site", "d.com"})
	var sites []string
	for a.Next() {
		var err error
		sites, err = a.StringList(sites)
		if err != nil {
			t.Fatalf("StringList: %v", err)
		}
	}
	want := []string{"a.com", "b.com", "c.com", "d.com"}
	if !reflect.DeepEqual(sites, want) {
		t.Errorf("accumulated = %q, want %q", sites, want)
	}
}

func TestSpaceAndEqualsFormsAgree(t *testing.T) {
	spaced := NewArgs([]string{"--limit", "7", "--format", "JSON", "--timeout", "2.5"})
	equals := NewArgs([]string{"--limit=7", "--format=JSON", "--timeout=2.5"})

	parse := func(a *Args) (int, string, float64) {
		var limit int
		var format string
		var timeout float64
		for a.Next() {
			var err error
			switch {
			case a.Is("--limit"):
				limit, err = a.Int()
			case a.Is("--format"):
				format, err = a.Enum("text", "json")
			case a.Is("--timeout"):
				timeout, err = a.Float()
			default:
				t.Fatalf("unexpected flag %q", a.Flag())
			}
			if err != nil {
				t.Fatalf("parsing %s: %v", a.Flag(), err)
			}
		}
		return limit, format, timeout
	}

	l1, f1, t1 := parse(spaced)
	l2, f2, t2 := parse(equals)
	if l1 != 7 || f1 != "json" || t1 != 2.5 {
		t.Errorf("spaced form = %d %q %g", l1, f1, t1)
	}
	if l1 != l2 || f1 != f2 || t1 != t2 {
		t.Errorf("forms disagree: spaced=(%d %q %g) equals=(%d %q %g)", l1, f1, t1, l2, f2, t2)
	}
}

func TestEqualsFormDoesNotMutateCallerSlice(t *testing.T) {
	backing := make([]string, 3, 8)
	backing[0] = "--limit=5"
	backing[1] = "--site=a.com"
	backing[2] = "query"
	snapshot := append([]string(nil), backing[:cap(backing)]...)

	argv := backing[:3]
	a := NewArgs(argv)
	for a.Next() {
		switch {
		case a.Is("--limit"):
			if _, err := a.Int(); err != nil {
				t.Fatalf("Int: %v", err)
			}
		case a.Is("--site"):
			if _, err := a.StringList(nil); err != nil {
				t.Fatalf("StringList: %v", err)
			}
		}
	}

	if !reflect.DeepEqual(backing[:cap(backing)], snapshot) {
		t.Errorf("caller's backing array was mutated: got %q, want %q", backing[:cap(backing)], snapshot)
	}
	if !reflect.DeepEqual(argv, []string{"--limit=5", "--site=a.com", "query"}) {
		t.Errorf("caller's argv changed: %q", argv)
	}
}

func TestMissingValueNamesFlag(t *testing.T) {
	a := NewArgs([]string{"--query"})
	a.Next()
	_, err := a.String()
	if err == nil {
		t.Fatal("String() with no value returned nil error")
	}
	if !strings.Contains(err.Error(), "missing value for --query") {
		t.Errorf("error = %q, want 'missing value for --query'", err)
	}

	a = NewArgs([]string{"--limit"})
	a.Next()
	if _, err := a.Int(); err == nil || !strings.Contains(err.Error(), "--limit") {
		t.Errorf("Int() missing value error = %v, want it to name --limit", err)
	}
}

func TestFlagReturnsNameWithoutValue(t *testing.T) {
	a := NewArgs([]string{"--limit=5", "--format", "json"})
	a.Next()
	if got := a.Flag(); got != "--limit" {
		t.Errorf("Flag() = %q, want --limit", got)
	}
	a.Next()
	if got := a.Flag(); got != "--format" {
		t.Errorf("Flag() = %q, want --format", got)
	}
}

func TestUnknownNamesOffendingFlag(t *testing.T) {
	a := NewArgs([]string{"--bogus"})
	a.Next()
	err := a.Unknown()
	if err == nil || !strings.Contains(err.Error(), "--bogus") {
		t.Errorf("Unknown() = %v, want it to name --bogus", err)
	}
}

func TestQueryTrimsAndJoins(t *testing.T) {
	a := NewArgs([]string{"  hello ", "world  "})
	for a.Next() {
	}
	// Outer whitespace is trimmed from the joined string; whitespace inside
	// each positional is kept as given.
	if got := a.Query(); got != "hello  world" {
		t.Errorf("Query() = %q, want %q", got, "hello  world")
	}
}
