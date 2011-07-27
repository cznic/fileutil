// Copyright (c) 2011 CZ.NIC z.s.p.o. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// blame: jnml, labs.nic.cz


package storage

import (
	"testing"
	"time"
)

func TestDevTicks(t *testing.T) {
	if !*devFlag {
		return
	}

	ticker := time.NewTicker(1e9)
	for i := 1; i < 5; i++ {
		<-ticker.C
		t.Logf("%.3f", float64(time.Nanoseconds())/1e9)
		<-time.After(19e8)
	}
	ticker.Stop()
}
