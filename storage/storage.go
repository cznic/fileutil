// Copyright (c) 2011 CZ.NIC z.s.p.o. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// blame: jnml, labs.nic.cz

// WIP: Package storage defines and implements storage providers and store accessors.
package storage

import (
	"log"
	"os"
	"time"
)

const asserts = false

func init() {
	log.SetFlags(log.Flags() | log.Lshortfile)
	if asserts {
		log.Print("assertions enabled")
	}
}

// FileInfo is a type implementing os.FileInfo which has setable fields, like
// the older os.FileInfo used to have. It is needed when e.g. the Size is to be
// faked (encapsulated/memory only file, file cache, etc.).
type FileInfo struct {
	FName    string      // base name of the file
	FSize    int64       // length in bytes
	FMode    os.FileMode // file mode bits
	FModTime time.Time   // modification time
	FIsDir   bool        // abbreviation for Mode().IsDir()
}

// NewFileInfo creates FileInfo from os.FileInfo fi.
func NewFileInfo(fi os.FileInfo) *FileInfo {
	return &FileInfo{fi.Name(), fi.Size(), fi.Mode(), fi.ModTime(), fi.IsDir()}
}

// Implementation of os.FileInfo
func (fi *FileInfo) Name() string {
	return fi.FName
}

// Implementation of os.FileInfo
func (fi *FileInfo) Size() int64 {
	return fi.FSize
}

// Implementation of os.FileInfo
func (fi *FileInfo) Mode() os.FileMode {
	return fi.FMode
}

// Implementation of os.FileInfo
func (fi *FileInfo) ModTime() time.Time {
	return fi.FModTime
}

// Implementation of os.FileInfo
func (fi *FileInfo) IsDir() bool {
	return fi.FIsDir
}

// Accessor provides I/O methods to access a store.
type Accessor interface {
	// Close closes the store, rendering it unusable for I/O. It returns an error, if any.
	Close() error
	// Name returns the name of the file as presented to Open.
	Name() string
	// ReadAt reads len(b) bytes from the store starting at byte offset off.
	// It returns the number of bytes read and the error, if any.
	// EOF is signaled by a zero count with err set to os.EOF.
	// ReadAt always returns a non-nil Error when n != len(b).
	ReadAt(b []byte, off int64) (n int, err error)
	// Stat returns the FileInfo structure describing the store. It returns the os.FileInfo and an error, if any.
	Stat() (fi os.FileInfo, err error)
	// Sync commits the current contents of the store to stable storage.
	// Typically, this means flushing the file system's in-memory copy of recently written data to disk.
	Sync() (err error)
	// Truncate changes the size of the store. It does not change the I/O offset.
	Truncate(size int64) error
	// WriteAt writes len(b) bytes to the store starting at byte offset off.
	// It returns the number of bytes written and an error, if any.
	// WriteAt returns a non-nil Error when n != len(b).
	WriteAt(b []byte, off int64) (n int, err error)
}

// WIP
//
// Acid provides an abstract, simplified ACID (atomicity, consistency,
// isolation, durability) model over a store (Accessor). This level is intended
// primairly for structural changes, not for DB data transactions. There is no
// explicit "commit" and/or "rollback" - a structural "transaction" is assumed
// to be always finished by an implicit "commit". By implicity here it is meant
// reaching a zero level of (possibly nested) BeginUpdates and EndUpdates.
//
// Before every [structural] change of a store the BeginUpdate is to be called
// and paired with EndUpdate after the change makes the store's state
// consistent again. Invokations of BeginUpdate may nest. On invoking the last
// unnested EndUpdate an implicit "commit" should be performed by the ACID
// store/provider. The concrete mechanism is unspecified. It could be for
// example a WAL (write-ahead logging).
//
// Tracking of the nesting level is left to the concrete type implementing the
// Acid interface. Note - such type probably must coordinate the Begin/End
// Update calls with all methods defined by the Accessor interface.
//
// Store providers may return an Accessor which implements Acid. Then the store
// user may choose to use the additional Acid methods.
type Acid interface {
	Accessor
	BeginUpdate() error
	EndUpdate() error
}
