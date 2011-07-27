// Copyright (c) 2011 CZ.NIC z.s.p.o. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// blame: jnml, labs.nic.cz

// WIP: Package storage defines and implements storage providers and store accessors.
package storage

import (
	"log"
	"os"
)

const asserts = true

func init() {
	log.SetFlags(log.Flags() | log.Lshortfile)
	if asserts {
		log.Print("assertions enabled")
	}
}

// Accessor provides I/O methods to access a store.
type Accessor interface {
	// Close closes the store, rendering it unusable for I/O. It returns an os.Error, if any.
	Close() os.Error
	// Name returns the name of the file as presented to Open.
	Name() string
	// ReadAt reads len(b) bytes from the store starting at byte offset off.
	// It returns the number of bytes read and the os.Error, if any.
	// EOF is signaled by a zero count with err set to os.EOF.
	// ReadAt always returns a non-nil Error when n != len(b).
	ReadAt(b []byte, off int64) (n int, err os.Error)
	// Stat returns the FileInfo structure describing the store. It returns the os.FileInfo and an os.Error, if any.
	Stat() (fi *os.FileInfo, err os.Error)
	// Sync commits the current contents of the store to stable storage.
	// Typically, this means flushing the file system's in-memory copy of recently written data to disk.
	Sync() (err os.Error)
	// Truncate changes the size of the store. It does not change the I/O offset.
	Truncate(size int64) os.Error
	// WriteAt writes len(b) bytes to the store starting at byte offset off.
	// It returns the number of bytes written and an os.Error, if any.
	// WriteAt returns a non-nil Error when n != len(b).
	WriteAt(b []byte, off int64) (n int, err os.Error)
}
