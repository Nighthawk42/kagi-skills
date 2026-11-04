// Package cli holds the argument parsing and process plumbing shared by the
// kagi-* commands.
package cli

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
)

// Version is overridden at build time via
// -ldflags "-X github.com/Nighthawk42/kagi-skills/internal/cli.Version=v1.2.3".
var Version = "dev"

// ErrUsage signals that usage was requested explicitly, so the caller should
// exit successfully without doing any work.
var ErrUsage = errors.New("usage requested")

// SignalContext returns a context cancelled on SIGINT or SIGTERM, so that an
// in-flight API request is abandoned promptly on Ctrl-C.
func SignalContext() (context.Context, context.CancelFunc) {
	return signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
}

// Args walks a command line, pulling out flags and collecting positionals.
//
// This replaces the four copies of a hand-rolled loop that the v0 tools each
// carried. Notably it parses integers with strconv rather than fmt.Sscanf:
// Sscanf accepted "--timeout 12abc" as 12 with no error.
type Args struct {
	args []string
	i    int

	// curFlag is the current flag's name with any "=value" suffix removed, so
	// that error messages name the flag rather than the whole token.
	curFlag string
	// inlineValue holds the value from a --flag=value token, if any.
	inlineValue string
	hasInline   bool

	// Positionals collects non-flag arguments, including everything after a
	// bare "--" terminator.
	Positionals []string
}

// NewArgs begins parsing args, which should exclude the program name.
func NewArgs(args []string) *Args {
	return &Args{args: args, i: -1, Positionals: make([]string, 0, len(args))}
}

// Next advances to the next argument, returning false when they are exhausted.
// A bare "--" consumes the remainder as positionals.
func (a *Args) Next() bool {
	a.i++
	for a.i < len(a.args) {
		arg := a.args[a.i]
		switch {
		case arg == "--":
			a.Positionals = append(a.Positionals, a.args[a.i+1:]...)
			a.i = len(a.args)
			return false
		case strings.HasPrefix(arg, "-") && arg != "-":
			a.curFlag, a.inlineValue, a.hasInline = arg, "", false
			if eq := strings.IndexByte(arg, '='); eq > 0 {
				a.curFlag, a.inlineValue, a.hasInline = arg[:eq], arg[eq+1:], true
			}
			return true
		default:
			a.Positionals = append(a.Positionals, arg)
			a.i++
		}
	}
	return false
}

// Flag returns the current flag's name, e.g. "--limit", with any "=value"
// suffix stripped.
func (a *Args) Flag() string { return a.curFlag }

// Is reports whether the current flag matches any of the given names.
func (a *Args) Is(names ...string) bool {
	for _, n := range names {
		if a.curFlag == n {
			return true
		}
	}
	return false
}

// value consumes and returns the flag's argument, accepting both
// "--flag value" and "--flag=value".
func (a *Args) value() (string, error) {
	if a.hasInline {
		return a.inlineValue, nil
	}
	if a.i+1 >= len(a.args) {
		return "", fmt.Errorf("missing value for %s", a.curFlag)
	}
	a.i++
	return a.args[a.i], nil
}

// String consumes the flag's argument as a string.
func (a *Args) String() (string, error) { return a.value() }

// Int consumes the flag's argument as an integer.
func (a *Args) Int() (int, error) {
	raw, err := a.value()
	if err != nil {
		return 0, err
	}
	n, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return 0, fmt.Errorf("invalid value for %s: %q is not an integer", a.Flag(), raw)
	}
	return n, nil
}

// IntInRange consumes an integer and rejects it unless lo <= n <= hi.
func (a *Args) IntInRange(lo, hi int) (int, error) {
	n, err := a.Int()
	if err != nil {
		return 0, err
	}
	if n < lo || n > hi {
		return 0, fmt.Errorf("invalid value for %s: %d is outside %d-%d", a.Flag(), n, lo, hi)
	}
	return n, nil
}

// Float consumes the flag's argument as a float.
func (a *Args) Float() (float64, error) {
	raw, err := a.value()
	if err != nil {
		return 0, err
	}
	f, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil {
		return 0, fmt.Errorf("invalid value for %s: %q is not a number", a.Flag(), raw)
	}
	return f, nil
}

// FloatInRange consumes a float and rejects it unless lo <= f <= hi.
func (a *Args) FloatInRange(lo, hi float64) (float64, error) {
	f, err := a.Float()
	if err != nil {
		return 0, err
	}
	// NaN compares false against everything, so it would otherwise pass both
	// bounds checks and reach the API as a nonsense value.
	if math.IsNaN(f) {
		return 0, fmt.Errorf("invalid value for %s: not a number", a.Flag())
	}
	if f < lo || f > hi {
		return 0, fmt.Errorf("invalid value for %s: %g is outside %g-%g", a.Flag(), f, lo, hi)
	}
	return f, nil
}

// Enum consumes the flag's argument and rejects it unless it is one of allowed.
// Matching is case-insensitive and the canonical lowercase form is returned.
func (a *Args) Enum(allowed ...string) (string, error) {
	raw, err := a.value()
	if err != nil {
		return "", err
	}
	got := strings.ToLower(strings.TrimSpace(raw))
	for _, want := range allowed {
		if got == want {
			return got, nil
		}
	}
	return "", fmt.Errorf("invalid value for %s: %q (valid: %s)",
		a.Flag(), raw, strings.Join(allowed, ", "))
}

// StringList consumes a comma-separated list, appending to dst. Repeating the
// flag accumulates rather than replacing.
func (a *Args) StringList(dst []string) ([]string, error) {
	raw, err := a.value()
	if err != nil {
		return dst, err
	}
	for _, part := range strings.Split(raw, ",") {
		if p := strings.TrimSpace(part); p != "" {
			dst = append(dst, p)
		}
	}
	return dst, nil
}

// Unknown builds the error for an unrecognised flag.
func (a *Args) Unknown() error {
	return fmt.Errorf("unknown option: %s", a.args[a.i])
}

// Query joins the collected positionals into a single query string.
func (a *Args) Query() string {
	return strings.TrimSpace(strings.Join(a.Positionals, " "))
}
