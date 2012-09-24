// Copyright (c) 2011 CZ.NIC z.s.p.o. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// blame: jnml, labs.nic.cz

package fileutil

import (
	"fmt"
	"os"
)

// Fadvise predeclares an access pattern for file data.
// See also 'man 2 posix_fadvise'. Not available in OSX.
func Fadvise(f *os.File, off, len int64, advice FadviseAdvice) (err error) {
	return os.NewSyscallError("SYS_FADVISE64", fmt.Errorf("not implemented"))
}
