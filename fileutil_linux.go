// Copyright (c) 2011 CZ.NIC z.s.p.o. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// blame: jnml, labs.nic.cz
package fileutil

import (
	"os"
	"syscall"
)
// Fadvise predeclares an access pattern for file data.
// See also 'man 2 posix_fadvise'.
func Fadvise(f *os.File, off, len int64, advice FadviseAdvice) (err error) {
	_, _, errno := syscall.Syscall6(
		syscall.SYS_FADVISE64,
		uintptr(f.Fd()),
		uintptr(off),
		uintptr(len),
		uintptr(advice),
		0, 0)
	return os.NewSyscallError("SYS_FADVISE64", errno)
}
