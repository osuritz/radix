//go:build windows

package server

import "syscall"

// errAddrInUse is the errno a failed bind reports when the address is already
// taken. On Windows, winsock returns WSAEADDRINUSE (10048); syscall.EADDRINUSE
// is a placeholder POSIX value there that real socket errors never carry, so
// matching against it silently fails and users get the generic bind message
// instead of the friendly "already in use" one.
const errAddrInUse = syscall.Errno(10048) // WSAEADDRINUSE
