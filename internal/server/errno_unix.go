//go:build !windows

package server

import "syscall"

// errAddrInUse is the errno a failed bind reports when the address is already
// taken. See errno_windows.go for why this needs a per-platform value.
const errAddrInUse = syscall.EADDRINUSE
