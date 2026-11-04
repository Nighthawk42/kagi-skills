//go:build windows

package cli

import "syscall"

// Windows reports a closed pipe through its own error codes rather than EPIPE.
const (
	errBrokenPipe syscall.Errno = 109 // ERROR_BROKEN_PIPE
	errNoData     syscall.Errno = 232 // ERROR_NO_DATA
)

func isBrokenPipeErrno(errno syscall.Errno) bool {
	return errno == errBrokenPipe || errno == errNoData
}
