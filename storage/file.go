// Copyright (c) 2011 CZ.NIC z.s.p.o. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// blame: jnml, labs.nic.cz


package storage

import (
	"os"
)

// NewFile returns an Accessor backed by an os.File named name, 
// It opens the named file with specified flag (os.O_RDWR etc.) and perm, (0666 etc.) if applicable.
// If successful, methods on the returned Accessor can be used for I/O.
// It returns the Accessor and an Error, if any.
func NewFile(name string, flag int, perm uint32) (store Accessor, err os.Error) {
	return os.OpenFile(name, flag, perm)
}

// OpenFile returns an Accessor backed by an existing os.File named name, 
// It opens the named file with specified flag (os.O_RDWR etc.) and perm, (0666 etc.) if applicable.
// If successful, methods on the returned Accessor can be used for I/O.
// It returns the Accessor and an Error, if any.
func OpenFile(name string, flag int, perm uint32) (store Accessor, err os.Error) {
	return os.OpenFile(name, flag, perm)
}
