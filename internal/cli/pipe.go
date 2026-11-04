package cli

import (
	"errors"
	"io"
	"io/fs"
	"syscall"
)

// IsBrokenPipe reports whether err is the result of writing to a pipe whose
// reader has already gone away.
//
// This is normal in a shell pipeline — `kagi-search ... --json | head -5`
// closes the pipe as soon as head has what it wants — so it should exit
// quietly rather than reporting a failure.
func IsBrokenPipe(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, syscall.EPIPE) ||
		errors.Is(err, io.ErrClosedPipe) ||
		errors.Is(err, fs.ErrClosed) {
		return true
	}
	var errno syscall.Errno
	if errors.As(err, &errno) {
		return isBrokenPipeErrno(errno)
	}
	return false
}
