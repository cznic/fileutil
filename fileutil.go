// Copyright (c) 2011 CZ.NIC z.s.p.o. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// blame: jnml, labs.nic.cz

// Package fileutil collects some file utility functions.
package fileutil

import (
	"fmt"
	"io"
	"os"
	"runtime"
	"sync"
	"syscall"
)

// GoMFile is a concurrent access safe version of MFile.
type GoMFile struct {
	mfile *MFile
	mutex sync.Mutex
}

// NewGoMFile return a newly created GoMFile.
func NewGoMFile(fname string, flag int, perm os.FileMode, delta_ns int64) (m *GoMFile, err error) {
	m = &GoMFile{}
	if m.mfile, err = NewMFile(fname, flag, perm, delta_ns); err != nil {
		m = nil
	}
	return
}

func (m *GoMFile) File() (file *os.File, err error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	return m.mfile.File()
}

func (m *GoMFile) SetChanged() {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.mfile.SetChanged()
}

func (m *GoMFile) SetHandler(h MFileHandler) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.mfile.SetHandler(h)
}

// MFileHandler resolves modifications of File.
// Possible File context is expected to be a part of the handler's closure.
type MFileHandler func(*os.File) error

// MFile represents an os.File with a guard/handler on change/modification.
// Example use case is an app with a configuration file which can be modified at any time
// and have to be reloaded in such event prior to performing something configurable by that
// file. The checks are made only on access to the MFile file by
// File() and a time threshold/hysteresis value can be chosen on creating a new MFile.
type MFile struct {
	file    *os.File
	handler MFileHandler
	t0      int64
	delta   int64
	ctime   int64
}

// NewMFile returns a newly created MFile or Error if any.
// The fname, flag and perm parameters have the same meaning as in os.Open.
// For meaning of the delta_ns parameter please see the (m *MFile) File() docs.
func NewMFile(fname string, flag int, perm os.FileMode, delta_ns int64) (m *MFile, err error) {
	m = &MFile{}
	var secs, nsecs int64
	if secs, nsecs, err = os.Time(); err != nil {
		return
	}

	m.t0 = secs*1e9 + nsecs
	if m.file, err = os.OpenFile(fname, flag, perm); err != nil {
		return
	}

	var fi os.FileInfo
	if fi, err = m.file.Stat(); err != nil {
		return
	}

	m.ctime = fi.ModTime().UnixNano()
	m.delta = delta_ns
	runtime.SetFinalizer(m, func(m *MFile) {
		m.file.Close()
	})
	return
}

// SetChanged forces next File() to unconditionally handle modification of the wrapped os.File.
func (m *MFile) SetChanged() {
	m.ctime = -1
}

// SetHandler sets a function to be invoked when modification of MFile is to be processed.
func (m *MFile) SetHandler(h MFileHandler) {
	m.handler = h
}

// File returns an os.File from MFile. If time elapsed between the last invocation of this function
// and now is at least delta_ns ns (a parameter of NewMFile) then the file is checked for
// change/modification. For delta_ns == 0 the modification is checked w/o getting os.Time().
// If a change is detected a handler is invoked on the MFile file.
// Any of these steps can produce an Error. If that happens the function returns nil, Error.
func (m *MFile) File() (file *os.File, err error) {
	var now int64

	mustCheck := m.delta == 0
	if !mustCheck {
		var secs, nsecs int64
		if secs, nsecs, err = os.Time(); err != nil {
			return
		}

		now = secs*1e9 + nsecs
		mustCheck = now-m.t0 > m.delta
	}

	if mustCheck { // check interval reached
		var fi os.FileInfo
		if fi, err = m.file.Stat(); err != nil {
			return
		}

		if fi.ModTime().UnixNano() != m.ctime { // modification detected
			if m.handler == nil {
				return nil, fmt.Errorf("no handler set for modified file %q", m.file.Name())
			}
			if err = m.handler(m.file); err != nil {
				return
			}

			m.ctime = fi.ModTime().UnixNano()
		}
		m.t0 = now
	}

	return m.file, nil
}

// Read reads buf from r. It will either fill the full buf or fail.
// It wraps the functionality of an io.Reader which may return less bytes than requested,
// but may block if not all data are ready for the io.Reader.
func Read(r io.Reader, buf []byte) (err error) {
	have := 0
	remain := len(buf)
	got := 0
	for remain > 0 {
		if got, err = r.Read(buf[have:]); err != nil {
			return
		}

		remain -= got
		have += got
	}
	return
}

// "os" and/or "syscall" extensions

// FadviseAdvice is used by Fadvise.
type FadviseAdvice int

// FAdviseAdvice values.
const (
	// $ grep FADV /usr/include/bits/fcntl.h
	POSIX_FADV_NORMAL     FadviseAdvice = iota // No further special treatment.  
	POSIX_FADV_RANDOM                          // Expect random page references.  
	POSIX_FADV_SEQUENTIAL                      // Expect sequential page references.	 
	POSIX_FADV_WILLNEED                        // Will need these pages.  
	POSIX_FADV_DONTNEED                        // Don't need these pages.  
	POSIX_FADV_NOREUSE                         // Data will be accessed once.  
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
