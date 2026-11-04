//go:build !windows

package cli

import "syscall"

// On Unix a closed pipe is EPIPE, which IsBrokenPipe already matches.
func isBrokenPipeErrno(syscall.Errno) bool { return false }
