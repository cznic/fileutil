// Copyright (c) 2011 CZ.NIC z.s.p.o. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// blame: jnml, labs.nic.cz


package falloc

import (
	"fmt"
	"os"
)

// EBadRequest is an error produced for invalid operation, e.g. for data of more than maximum allowed.
type EBadRequest struct {
	Name string
	Size int
}

func (e *EBadRequest) String() string {
	return fmt.Sprintf("%s: size %d", e.Name, e.Size)
}

// EClose is a file/store close error.
type EClose struct {
	Name string
	Err  os.Error
}

func (e *EClose) String() string {
	return fmt.Sprintf("%sx: %s", e.Name, e.Err)
}

// ECorrupted is a file/store format error.
type ECorrupted struct {
	Name string
	Ofs  int64
}

func (e *ECorrupted) String() string {
	return fmt.Sprintf("%s: corrupted data @%#x", e.Name, e.Ofs)
}

// ECreate is a file/store create error.
type ECreate struct {
	Name string
	Err  os.Error
}

func (e *ECreate) String() string {
	return fmt.Sprintf("%s: %s", e.Name, e.Err)
}

// EFreeList is a file/store format error.
type EFreeList struct {
	Name  string
	Size  int64
	Block int64
}

func (e *EFreeList) String() string {
	return fmt.Sprintf("%s: invalid free list item, size %#x, block %#x", e.Name, e.Size, e.Block)
}

// EHandle is an error type reported for invalid Handles.
type EHandle struct {
	Name   string
	Handle Handle
}

func (e EHandle) String() string {
	return fmt.Sprintf("%s: invalid handle %#x", e.Name, e.Handle)
}

// EHeader is a file/store format error.
type EHeader struct {
	Name     string
	Header   []byte
	Expected []byte
}

func (e *EHeader) String() string {
	return fmt.Sprintf("%s: invalid header, got [% x], expected [% x]", e.Name, e.Header, e.Expected)
}

// ENullHandle is a file/store access error via a null handle.
type ENullHandle string

func (e ENullHandle) String() string {
	return fmt.Sprintf("%s: access via null handle", e)
}

// EOpen is a file/store open error.
type EOpen struct {
	Name string
	Err  os.Error
}

func (e *EOpen) String() string {
	return fmt.Sprintf("%s: %s", e.Name, e.Err)
}

// ERead is a file/store read error.
type ERead struct {
	Name string
	Ofs  int64
	Err  os.Error
}

func (e *ERead) String() string {
	return fmt.Sprintf("%s, %#x: %s", e.Name, e.Ofs, e.Err)
}

// ESize is a file/store size error.
type ESize struct {
	Name string
	Size int64
}

func (e *ESize) String() string {
	return fmt.Sprintf("%s: invalid size %#x(%d), size %%16 != 0", e.Name, e.Size, e.Size)
}

// EWrite is a file/store write error.
type EWrite struct {
	Name string
	Ofs  int64
	Err  os.Error
}

func (e *EWrite) String() string {
	return fmt.Sprintf("%s, %#x: %s", e.Name, e.Ofs, e.Err)
}
