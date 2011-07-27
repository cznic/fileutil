// Copyright (c) 2011 CZ.NIC z.s.p.o. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// blame: jnml, labs.nic.cz


package falloc

import (
	"bytes"
	"github.com/cznic/fileutil"
	"github.com/cznic/fileutil/storage"
	"github.com/cznic/mathutil"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"math"
	"os"
	"runtime"
	"testing"
	"time"
)

var (
	blockFlag      = flag.Uint("block", 256, "block size for some of the dev tests")
	cachedFlag     = flag.Bool("cache", false, "enable caching store")
	cacheTotalFlag = flag.Int64("cachemax", 1<<25, "cache total bytes")
	devFlag        = flag.Bool("dev", false, "enable dev tests")
	dropFlag       = flag.Bool("drop", false, "drop system file cache for some of the dev tests before measurement")
	fnFlag         = flag.String("f", "test.tmp", "test file name")
	fadviseFlag    = flag.Bool("fadvise", false, "hint kernel about random file access")
	nFlag          = flag.Int("n", 1, "parameter for some of the dev tests")
	probeFlag      = flag.Bool("probe", false, "report store probe statistics")
)

func init() {
	runtime.GOMAXPROCS(3)
	flag.Parse()
}

func fopen(fn string) (f *File, err os.Error) {
	var store storage.Accessor
	if store, err = storage.OpenFile(fn, os.O_RDWR, 0666); err != nil {
		return
	}

	var advise func(int64, int, bool)
	if *fadviseFlag {
		f := store.(*os.File)
		if err = fileutil.Fadvise(f, 0, 0, fileutil.POSIX_FADV_RANDOM); err != nil {
			return
		}
		advise = func(off int64, len int, write bool) {
			//if write {
			//if err = f.Sync(); err != nil {
			//log.Fatal("advisor sync err", err)
			//}
			//}
			if err = fileutil.Fadvise(f, off, off+int64(len), fileutil.POSIX_FADV_DONTNEED); err != nil {
				log.Fatal("advisor advise err", err)
			}
		}
	}

	var prob *storage.Probe
	if *probeFlag {
		prob = storage.NewProbe(store, nil)
		store = prob
	}
	if *cachedFlag {
		if store, err = storage.NewCache(store, *cacheTotalFlag, advise); err != nil {
			return
		}

		if *probeFlag {
			store = storage.NewProbe(store, prob)
		}
	}
	return Open(store)
}

func fcreate(fn string) (f *File, err os.Error) {
	var store storage.Accessor
	if store, err = storage.OpenFile(fn, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0666); err != nil {
		return
	}

	var advise func(int64, int, bool)
	if *fadviseFlag {
		f := store.(*os.File)
		if err = fileutil.Fadvise(f, 0, 0, fileutil.POSIX_FADV_RANDOM); err != nil {
			return
		}
		advise = func(off int64, len int, write bool) {
			//if write {
			//if err = f.Sync(); err != nil {
			//log.Fatal("advisor sync err", err)
			//}
			//}
			if err = fileutil.Fadvise(f, off, off+int64(len), fileutil.POSIX_FADV_DONTNEED); err != nil {
				log.Fatal("advisor advise err", err)
			}
		}
	}

	var prob *storage.Probe
	if *probeFlag {
		prob = storage.NewProbe(store, nil)
		store = prob
	}
	if *cachedFlag {
		if store, err = storage.NewCache(store, *cacheTotalFlag, advise); err != nil {
			return
		}

		if *probeFlag {
			store = storage.NewProbe(store, prob)
		}
	}
	return New(store)
}

func probed(t *testing.T, f *File) {
	if f == nil {
		return
	}

	dump := func(p *storage.Probe) {
		t.Logf("OpsRd %d OpsWr %d BytesRd %d(avg %.1f) BytesWr %d(avg %.1f) SectorsRd %d(%d, +%d, x%.2f) SectorsWr %d(%d, +%d, x%.2f)",
			p.OpsRd, p.OpsWr,
			p.BytesRd, float64(p.BytesRd)/float64(p.OpsRd),
			p.BytesWr, float64(p.BytesWr)/float64(p.OpsWr),
			p.SectorsRd,
			p.SectorsRd<<9,
			p.SectorsRd<<9-p.BytesRd,
			float64(p.SectorsRd<<9)/float64(p.BytesRd),
			p.SectorsWr,
			p.SectorsWr<<9,
			p.SectorsWr<<9-p.BytesWr,
			float64(p.SectorsWr<<9)/float64(p.BytesWr),
		)
	}

	if ph, ok := f.Accessor().(*storage.Probe); ok {
		dump(ph)
		if c, ok := ph.Accessor.(*storage.Cache); ok {
			if pl, ok := c.Accessor().(*storage.Probe); ok {
				dump(pl)
			}
		}
	}
}

func (f *File) audit() (usedblocks, totalblocks int64, err os.Error) {
	defer func() {
		if e := recover(); e != nil {
			err = e.(os.Error)
		}
	}()

	fi, err := f.f.Stat()
	if err != nil {
		panic(err)
	}

	freemap := map[int64]int64{}
	fp := int64(0)
	buf := make([]byte, 22)
	freeblocks := int64(0)

	// linear scan
	for fp < fi.Size {
		totalblocks++
		typ, size := f.getInfo(fp >> 4)
		f.read(buf[:1], fp+size<<4-1)
		last := buf[0]
		switch {
		default:
			panic("internal error")
		case typ == 0:
			if last != 0 {
				panic(fmt.Errorf("@%#x used empty, last @%#x: %#x != 0", fp, fp+size<<4-1, last))
			}
		case typ >= 0x1 && typ <= 0xed:
			if last >= 0xfe {
				panic(fmt.Errorf("@%#x used short, last @%#x: %#x > 0xfe", fp, fp+size<<4-1, last))
			}
		case typ >= 0xee && typ <= 0xfb:
			if last > 1 {
				panic(fmt.Errorf("@%#x used esc short, last @%#x: %#x > 1", fp, fp+size<<4-1, last))
			}
		case typ == 0xfc:
			f.read(buf[:2], fp+1)
			switch n := int(buf[0])<<8 + int(buf[1]); {
			default:
				panic(fmt.Errorf("@#x used long, illegal content length %#x < 0xee(238)", fp, n, n))
			case n >= 0xee && n <= 0xf0f0:
				if last >= 0xfe {
					panic(fmt.Errorf("@#x used long, last @%#x: %#x > 0xfe", fp, fp+size<<4-1, last))
				}
			case n >= 0xf0f1 && n <= 0xffff:
				if last > 1 {
					panic(fmt.Errorf("@%#x used esc long, last @%#x: %#x > 1", fp, fp+size<<4-1, last))
				}
			}
		case typ == 0xfd:
			if last != 0 {
				panic(fmt.Errorf("@%#x reloc, last @%#x: %#x != 0", fp, fp+size<<4-1, last))
			}

			var target int64
			f.read(buf[:7], fp+1)
			(*Handle)(&target).Get(buf)
			if target >= f.atoms {
				panic(fmt.Errorf("@%#x illegal reloc, target %#x > f.atoms(%#x)", fp, target, f.atoms))
			}

			ttyp, _ := f.getInfo(target)
			if ttyp >= 0xfe {
				panic(fmt.Errorf("@%#x reloc, points to unused @%#x", fp, fp+size<<4-1, target))
			}

			if ttyp == 0xfd {
				panic(fmt.Errorf("@%#x reloc, points to reloc @%#x", fp, fp+size<<4-1, target))
			}
		case typ == 0xfe:
			if size < 2 {
				panic(fmt.Errorf("@%#x illegal free block, atoms %d < 2", fp, size))
			}

			if fp>>4 < f.canfree {
				panic(fmt.Errorf("@%#x illegal free block @ < f.canfree", fp))
			}

			f.read(buf[:22], fp)
			var prev, next, sz int64
			(*Handle)(&prev).Get(buf[1:])
			(*Handle)(&next).Get(buf[8:])
			f.checkPrevNext(fp, prev, next)
			f.read(buf[:7], fp+size<<4-8)
			(*Handle)(&sz).Get(buf)
			if sz != size {
				panic(fmt.Errorf("@%#x mismatch size, %d != %d", fp, sz, size))
			}

			if last != 0xfe {
				panic(fmt.Errorf("@%#x free atom, last @%#x: %#x != 0xff", fp, fp+size<<4-1, last))
			}
			freemap[fp>>4] = size
			freeblocks++
		case typ == 0xff:
			f.read(buf[:14], fp+1)
			var prev, next int64
			(*Handle)(&prev).Get(buf)
			(*Handle)(&next).Get(buf[7:])
			f.checkPrevNext(fp, prev, next)
			if last != 0xff {
				panic(fmt.Errorf("@%#x free atom, last @%#x: %#x != 0xff", fp, fp+size<<4-1, last))
			}
			freemap[fp>>4] = size
			freeblocks++
		}
		fp += size << 4
	}
	usedblocks = totalblocks - freeblocks

	// check free table
	for size := len(f.freetab) - 1; size > 0; size-- {
		var prev, next, fprev int64
		this := f.freetab[size]
		for this != 0 {
			sz, ok := freemap[this]
			if !ok {
				panic(fmt.Errorf("bad freetab[%d] item @%#x", size, this))
			}

			freemap[this] = 0, false

			if sz < int64(size) {
				panic(fmt.Errorf("bad freetab[%d] item size @%#x %d", size, this, sz))
			}

			if sz == 1 {
				f.read(buf[:15], this<<4)
				(*Handle)(&fprev).Get(buf[1:])
				if fprev != prev {
					panic(fmt.Errorf("bad fprev %#x, exp %#x", fprev, prev))
				}

				(*Handle)(&next).Get(buf[8:])
			} else {
				f.read(buf, this<<4)
				(*Handle)(&fprev).Get(buf[1:])
				if fprev != prev {
					panic(fmt.Errorf("bad fprev %#x, exp %#x", fprev, prev))
				}
				var fsz int64
				(*Handle)(&fsz).Get(buf[15:])
				if fsz != sz {
					panic(fmt.Errorf("bad fsz %d @%#x, exp %#x", fsz, this<<4, sz))
				}

				(*Handle)(&next).Get(buf[8:])
			}

			prev, this = this, next
		}
	}

	if n := len(freemap); n != 0 {
		for h, s := range freemap {
			//fmt.Printf("\n%v\n", freemap)
			panic(fmt.Errorf("%d lost free blocks in freemap, e.g. %d free atoms @%#x", n, s, h))
		}
	}

	return

}

func (f *File) checkPrevNext(fp, prev, next int64) {
	if prev != 0 && prev < f.canfree {
		panic(fmt.Errorf("@%#x illegal free atom, prev %#x < f.canfree(%#x)", fp, prev, f.canfree))
	}

	if prev >= f.atoms {
		panic(fmt.Errorf("@%#x illegal free atom, prev %#x > f.atoms", fp, prev))
	}

	if next != 0 && next < f.canfree {
		panic(fmt.Errorf("@%#x illegal free atom, next %#x < f.canfree(%#x)", fp, next, f.canfree))
	}

	if next >= f.atoms {
		panic(fmt.Errorf("@%#x illegal free atom, next %#x > f.atoms", fp, next))
	}
}

//func (f *File) r(b []byte, off int64) {
//if n, err := f.f.ReadAt(b, off); n != len(b) {
//panic(err)
//}
//}

//func audit(fn string) (usedblocks, totalblocks int64, err os.Error) {
//f, e := fopen(fn)
//if e != nil {
//err = e
//return
//}

//defer func() {fclose(f); f = nil}()
//return f.audit()
//}


func reaudit(t *testing.T, f *File, fn string) (of *File) {
	var err os.Error
	if _, _, err := f.audit(); err != nil {
		t.Fatal(err)
	}

	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	f = nil
	runtime.GC()
	if of, err = fopen(fn); err != nil {
		t.Fatal(err)
	}

	if _, _, err := of.audit(); err != nil {
		println("reaudit err 286", err)
		t.Fatal(err)
	}

	return
}

func TestCreate(t *testing.T) {
	f, err := fcreate(*fnFlag)
	if err != nil {
		t.Fatal(err)
	}

	defer func() {
		err := os.Remove(*fnFlag)
		if err != nil {
			t.Fatal(10010, err)
		}
	}()

	f.Accessor().Sync()
	probed(t, f)
	if err = f.Close(); err != nil {
		t.Fatal(5, err)
	}

	b, err := ioutil.ReadFile(*fnFlag)
	if err != nil {
		t.Fatal(10, err)
	}

	x := b[:16]
	if !bytes.Equal(x, hdr) {
		t.Fatalf("20\n% x\n% x", x, hdr)
	}

	x = b[16:32]
	if !bytes.Equal(x, empty) {
		t.Fatalf("30\n% x\n% x", x, hdr)
	}
}

func TestOpen(t *testing.T) {
	f, err := fcreate(*fnFlag)
	if err != nil {
		t.Fatal(10, err)
	}

	defer func() {
		probed(t, f)
		ec := f.Close()
		er := os.Remove(*fnFlag)
		if ec != nil {
			t.Fatal(10000, ec)
		}

		if er != nil {
			t.Fatal(10010, er)
		}
	}()

	if err := f.Close(); err != nil {
		t.Fatal(20, err)
	}

	if f, err = fopen(*fnFlag); err != nil {
		t.Fatal(30, err)
	}

	for i, p := range f.freetab {
		if p != 0 {
			t.Fatal(40, i+1, p)
		}
	}
}

func alloc(f *File, b []byte) (y int64) {
	if h, err := f.Alloc(b); err != nil {
		panic(err)
	} else {
		y = int64(h)
	}
	return
}

func realloc(f *File, atom int64, b []byte, keepHandle bool) (y int64) {
	if h, err := f.Realloc(Handle(atom), b, keepHandle); err != nil {
		panic(err)
	} else {
		y = int64(h)
	}
	return
}

func testContentEncodingDecoding(t *testing.T, min, max int) {
	f, err := fcreate(*fnFlag)
	if err != nil {
		t.Fatal(err)
	}

	defer func() {
		ec := f.Close()
		er := os.Remove(*fnFlag)
		if ec != nil {
			t.Fatal(10000, ec)
		}

		if er != nil {
			t.Fatal(10010, er)
		}
	}()

	b := make([]byte, max)
	r, err := mathutil.NewFC32(math.MinInt32, math.MaxInt32, true)
	if err != nil {
		t.Fatal(err)
	}

	blocks := int64(3)
	a := make([]int64, 0, 4*(max-min+1))
	for cl := min; cl <= max; cl++ {
		src := b[:cl]
		for i := range src {
			b[i] = byte(r.Next())
		}
		a = append(a, alloc(f, src))
		blocks++
		if cl == 0 {
			continue
		}

		for i := range src {
			b[i] = byte(r.Next())
		}
		src[cl-1] = 0xfd
		a = append(a, alloc(f, src))
		blocks++
		for i := range src {
			b[i] = byte(r.Next())
		}
		src[cl-1] = 0xfe
		a = append(a, alloc(f, src))
		blocks++
		for i := range src {
			b[i] = byte(r.Next())
		}
		src[cl-1] = 0xff
		a = append(a, alloc(f, src))
		blocks++
	}

	f.Accessor().Sync()
	probed(t, f)
	if err := f.Close(); err != nil {
		t.Fatal(5, err)
	}

	f = nil
	runtime.GC()
	if f, err = fopen(*fnFlag); err != nil {
		t.Fatal(5, err)
	}

	r.Seek(0)
	ai := 0
	for cl := min; cl <= max; cl++ {
		h := a[ai]
		ai++
		src := b[:cl]
		for i := range src {
			b[i] = byte(r.Next())
		}
		got, _ := f.readUsed(h)
		if !bytes.Equal(src, got) {
			t.Fatalf("10 cl %d atom %#x\nexp % x\ngot % x", cl, h, src, got)
		}
		if cl == 0 {
			continue
		}

		for i := range src {
			b[i] = byte(r.Next())
		}
		src[cl-1] = 0xfd
		h = a[ai]
		ai++
		got, _ = f.readUsed(h)
		if !bytes.Equal(src, got) {
			t.Fatalf("20 cl %d atom %#x\nexp % x\ngot % x", cl, h, src, got)
		}

		for i := range src {
			b[i] = byte(r.Next())
		}
		src[cl-1] = 0xfe
		h = a[ai]
		ai++
		got, _ = f.readUsed(h)
		if !bytes.Equal(src, got) {
			t.Fatalf("30 cl %d atom %#x\nexp % x\ngot % x", cl, h, src, got)
		}

		for i := range src {
			b[i] = byte(r.Next())
		}
		src[cl-1] = 0xff
		h = a[ai]
		ai++
		got, _ = f.readUsed(h)
		if !bytes.Equal(src, got) {
			t.Fatalf("40 cl %d atom %#x\nexp % x\ngot % x", cl, h, src, got)
		}
	}

	auditblocks, _, err := f.audit()
	if err != nil {
		t.Fatal(45, err)
	}

	if auditblocks != blocks {
		t.Fatal(50, auditblocks, blocks)
	}

	if f = reaudit(t, f, *fnFlag); err != nil {
		t.Fatal(60, err)
	}
}

func TestContentEncodingDecoding(t *testing.T) {
	testContentEncodingDecoding(t, 0, 1024)
	testContentEncodingDecoding(t, 61680-17, 61680)
}

type freeItem struct {
	size int64
	head int64
}

func (f *File) reportFree() (report []freeItem) {
	for size, head := range f.freetab {
		if size != 0 && head != 0 {
			report = append(report, freeItem{int64(size), head})
		}
	}
	return
}

func free(f *File, h int64) {
	if err := f.Free(Handle(h)); err != nil {
		panic(err)
	}
}

func testFreeTail(t *testing.T, b []byte) {
	f, err := fcreate(*fnFlag)
	if err != nil {
		t.Fatal(err)
	}

	defer func() {
		ec := f.Close()
		er := os.Remove(*fnFlag)
		if ec != nil {
			t.Fatal(10000, ec)
		}

		if er != nil {
			t.Fatal(10010, er)
		}
	}()

	fs0 := f.atoms
	used0, total0, err := f.audit()
	if err != nil {
		panic(err)
	}

	if used0 != total0 {
		t.Fatal(10, used0, total0)
	}

	handle := alloc(f, b)
	free(f, handle)
	if fs1 := f.atoms; fs1 != fs0 {
		t.Fatal(20, fs1, fs0)
	}

	if rep := f.reportFree(); len(rep) != 0 {
		t.Fatal(30, rep)
	}

	if err := f.Close(); err != nil {
		t.Fatal(35, err)
	}

	f = nil
	runtime.GC()
	if f, err = fopen(*fnFlag); err != nil {
		t.Fatal(35, err)
	}

	used, total, err := f.audit()
	if err != nil {
		panic(err)
	}

	if used != used0 {
		t.Fatal(40, used, used0)
	}

	if total != total0 {
		t.Fatal(50, total, total0)
	}
}

func TestFreeTail(t *testing.T) {
	b := make([]byte, 61680)
	for n := 0; n <= 253+16; n++ {
		data := b[:n]
		testFreeTail(t, data)
		if n == 0 {
			continue
		}

		data[n-1] = 0xff
		testFreeTail(t, data)
		data[n-1] = 0
	}

	for n := 61680 - 16; n <= 61680; n++ {
		data := b[:n]
		testFreeTail(t, data)
		data[n-1] = 0xff
		testFreeTail(t, data)
		data[n-1] = 0
	}
}

func testFreeTail2(t *testing.T, b []byte) {
	f, err := fcreate(*fnFlag)
	if err != nil {
		t.Fatal(err)
	}

	defer func() {
		ec := f.Close()
		er := os.Remove(*fnFlag)
		if ec != nil {
			t.Fatal(10000, ec)
		}

		if er != nil {
			t.Fatal(10010, er)
		}
	}()

	fs0 := f.atoms
	used0, total0, err := f.audit()
	if err != nil {
		panic(err)
	}

	if used0 != total0 {
		t.Fatal(10, used0, total0)
	}

	handle := alloc(f, b)
	handle2 := alloc(f, b)
	free(f, handle)
	free(f, handle2)
	if fs1 := f.atoms; fs1 != fs0 {
		t.Fatal(20, fs1, fs0)
	}

	if rep := f.reportFree(); len(rep) != 0 {
		t.Fatal(30, rep)
	}

	if err := f.Close(); err != nil {
		t.Fatal(35, err)
	}

	f = nil
	runtime.GC()
	if f, err = fopen(*fnFlag); err != nil {
		t.Fatal(35, err)
	}

	used, total, err := f.audit()
	if err != nil {
		panic(err)
	}

	if used != used0 {
		t.Fatal(40, used, used0)
	}

	if total != total0 {
		t.Fatal(50, total, total0)
	}
}

func TestFreeTail2(t *testing.T) {
	b := make([]byte, 61680)
	for n := 0; n <= 253+16; n++ {
		data := b[:n]
		testFreeTail2(t, data)
		if n == 0 {
			continue
		}

		data[n-1] = 0xff
		testFreeTail2(t, data)
		data[n-1] = 0
	}

	for n := 61680 - 16; n <= 61680; n++ {
		data := b[:n]
		testFreeTail2(t, data)
		data[n-1] = 0xff
		testFreeTail2(t, data)
		data[n-1] = 0
	}
}

func testFreeIsolated(t *testing.T, b []byte) {
	f, err := fcreate(*fnFlag)
	if err != nil {
		t.Fatal(err)
	}

	defer func() {
		ec := f.Close()
		er := os.Remove(*fnFlag)
		if ec != nil {
			t.Fatal(10000, ec)
		}

		if er != nil {
			t.Fatal(10010, er)
		}
	}()

	rqAtoms := rq2Atoms(len(b))
	left := alloc(f, nil)
	handle := alloc(f, b)
	right := alloc(f, nil)

	fs0 := f.atoms
	used0, total0, err := f.audit()
	if err != nil {
		panic(err)
	}

	if used0 != total0 {
		t.Fatal(10, used0, total0)
	}

	free(f, handle)
	if fs1 := f.atoms; fs1 != fs0 {
		t.Fatal(20, fs1, fs0)
	}

	rep := f.reportFree()
	if len(rep) != 1 {
		t.Fatal(30, rep)
	}

	if x := rep[0]; x.size != rqAtoms || x.head != handle {
		t.Fatal(40, x)
	}

	used, total, err := f.audit()
	if err != nil {
		panic(err)
	}

	if n, free := f.getSize(left); n != 1 || free {
		t.Fatal(50, n, free)
	}

	if n, free := f.getSize(right); n != 1 || free {
		t.Fatal(60, n, free)
	}

	if used != used0-1 {
		t.Fatal(70, used, used0)
	}

	if total != total0 {
		t.Fatal(80, total, total0)
	}

	if free := total - used; free != 1 {
		t.Fatal(90, free)
	}

	// verify persisted file correct
	if err := f.Close(); err != nil {
		t.Fatal(95, err)
	}

	f = nil
	runtime.GC()
	if f, err = fopen(*fnFlag); err != nil {
		t.Fatal(err)
	}

	if fs1 := f.atoms; fs1 != fs0 {
		t.Fatal(120, fs1, fs0)
	}

	rep = f.reportFree()
	if len(rep) != 1 {
		t.Fatal(130, rep)
	}

	if x := rep[0]; x.size != rqAtoms || x.head != handle {
		t.Fatal(140, x)
	}

	used, total, err = f.audit()
	if err != nil {
		panic(err)
	}

	if n, free := f.getSize(left); n != 1 || free {
		t.Fatal(150, n, free)
	}

	if n, free := f.getSize(right); n != 1 || free {
		t.Fatal(160, n, free)
	}

	if used != used0-1 {
		t.Fatal(170, used, used0)
	}

	if total != total0 {
		t.Fatal(180, total, total0)
	}

	if free := total - used; free != 1 {
		t.Fatal(190, free)
	}

}

func TestFreeIsolated(t *testing.T) {
	b := make([]byte, 61680)
	for n := 0; n <= 253+16; n++ {
		data := b[:n]
		testFreeIsolated(t, data)
	}

	for n := 61680 - 16; n <= 61680; n++ {
		data := b[:n]
		testFreeIsolated(t, data)
	}
}

func testFreeBlockList(t *testing.T, a, b int) {
	var h [2]int64

	t.Log(a, b)
	f, err := fcreate(*fnFlag)
	if err != nil {
		t.Fatal(10, err)
	}

	defer func() {
		if f != nil {
			if err := f.Close(); err != nil {
				t.Fatal(15, err)
			}
		}

		f = nil
		runtime.GC()
		os.Remove(*fnFlag)
	}()

	used0, total0, err := f.audit()
	if err != nil {
		t.Fatal(20, err)
	}

	alloc(f, nil)
	h[0] = alloc(f, nil)
	alloc(f, nil)
	h[1] = alloc(f, nil)
	alloc(f, nil)

	if err := f.Close(); err != nil {
		t.Fatal(25, err)
	}

	f = nil
	runtime.GC()
	if f, err = fopen(*fnFlag); err != nil {
		t.Fatal(30, err)
	}

	used, total, err := f.audit()
	if err != nil {
		t.Fatal(40, err)
	}

	if used-used0 != 5 || total-total0 != 5 || used != total {
		t.Fatal(50, used0, total0, used, total)
	}

	free(f, h[a])
	free(f, h[b])

	used, total, err = f.audit()
	if err != nil {
		t.Fatal(60, err)
	}

	if used-used0 != 3 || total-total0 != 5 || total-used != 2 {
		t.Fatal(70, used0, total0, used, total)
	}

	if err := f.Close(); err != nil {
		t.Fatal(75, err)
	}

	f = nil
	runtime.GC()
	if f, err = fopen(*fnFlag); err != nil {
		t.Fatal(80, err)
	}

	used, total, err = f.audit()
	if err != nil {
		t.Fatal(90, err)
	}

	if used-used0 != 3 || total-total0 != 5 || total-used != 2 {
		t.Fatal(100, used0, total0, used, total)
	}
}

func TestFreeBlockList(t *testing.T) {
	testFreeBlockList(t, 0, 1)
	testFreeBlockList(t, 1, 0)
}

func testFreeBlockList2(t *testing.T, a, b, c int) {
	var h [3]int64

	f, err := fcreate(*fnFlag)
	if err != nil {
		t.Fatal(5, err)
	}

	defer func() {
		if f != nil {
			if err := f.Close(); err != nil {
				t.Fatal(6, err)
			}
		}

		f = nil
		runtime.GC()
		os.Remove(*fnFlag)
	}()

	used0, total0, err := f.audit()
	if err != nil {
		t.Fatal(err)
	}

	alloc(f, nil)
	h[0] = alloc(f, nil)
	alloc(f, nil)
	h[1] = alloc(f, nil)
	alloc(f, nil)
	h[2] = alloc(f, nil)
	alloc(f, nil)

	if err := f.Close(); err != nil {
		t.Fatal(7, err)
	}

	f = nil
	runtime.GC()
	if f, err = fopen(*fnFlag); err != nil {
		t.Fatal(err)
	}

	used, total, err := f.audit()
	if err != nil {
		t.Fatal(err)
	}

	if used-used0 != 7 || total-total0 != 7 || used != total {
		t.Fatal(used0, total0, used, total)
	}

	free(f, h[a])
	free(f, h[b])
	free(f, h[c])

	used, total, err = f.audit()
	if err != nil {
		t.Fatal(err)
	}

	if used-used0 != 4 || total-total0 != 7 || total-used != 3 {
		t.Fatal(used0, total0, used, total)
	}

	if err := f.Close(); err != nil {
		t.Fatal(8, err)
	}

	f = nil
	runtime.GC()
	if f, err = fopen(*fnFlag); err != nil {
		t.Fatal(err)
	}

	used, total, err = f.audit()
	if err != nil {
		t.Fatal(err)
	}

	if used-used0 != 4 || total-total0 != 7 || total-used != 3 {
		t.Fatal(used0, total0, used, total)
	}
}

func TestFreeBlockList2(t *testing.T) {
	testFreeBlockList2(t, 0, 1, 2)
	testFreeBlockList2(t, 0, 2, 1)
	testFreeBlockList2(t, 1, 0, 2)
	testFreeBlockList2(t, 1, 2, 0)
	testFreeBlockList2(t, 2, 0, 1)
	testFreeBlockList2(t, 2, 1, 0)
}

var crng *mathutil.FC32

func init() {
	var err os.Error
	if crng, err = mathutil.NewFC32(0, math.MaxInt32, true); err != nil {
		panic(err)
	}
}

func content(b []byte, h int64) (c []byte) {
	crng.Seed(h)
	crng.Seek(0)
	c = b[:crng.Next()%61681]
	for i := range c {
		c[i] = byte(crng.Next())
	}
	return
}

func testFreeBlockList3(t *testing.T, n, mod int) {
	rng, err := mathutil.NewFC32(0, n-1, true)
	if err != nil {
		t.Fatal(5, err)
	}

	f, err := fcreate(*fnFlag)
	if err != nil {
		t.Fatal(err)
	}

	defer func() {
		if f != nil {
			if err := f.Close(); err != nil {
				t.Fatal(6, err)
			}
		}

		f = nil
		runtime.GC()
		os.Remove(*fnFlag)
	}()

	ha := make([]int64, n)
	b := make([]byte, 61680)
	for i := range ha {
		h := f.atoms
		ha[i] = h
		c := content(b, h)
		if alloc(f, c) != h {
			t.Fatal(10, h)
		}
	}
	f = reaudit(t, f, *fnFlag)
	del := map[int64]bool{}
	for _ = range ha {
		i := rng.Next()
		if i%mod != 0 {
			h := ha[i]
			free(f, h)
			del[h] = true
		}
	}
	f = reaudit(t, f, *fnFlag)
	for _, h := range ha {
		if !del[h] {
			exp := content(b, h)
			got, _ := f.readUsed(h)
			if !bytes.Equal(exp, got) {
				t.Fatal(20, len(got), len(exp))
			}
		}
	}
}

func TestFreeBlockList3(t *testing.T) {
	testFreeBlockList3(t, 111, 1)
	testFreeBlockList3(t, 151, 2)
	testFreeBlockList3(t, 170, 3)
	testFreeBlockList3(t, 170, 4)
}

func TestRealloc1(t *testing.T) {
	f, err := fcreate(*fnFlag)
	if err != nil {
		t.Fatal(5, err)
	}

	defer func() {
		if f != nil {
			if err := f.Close(); err != nil {
				t.Fatal(6, err)
			}
		}

		f = nil
		runtime.GC()
		os.Remove(*fnFlag)
	}()

	b := make([]byte, 61680)
	c := content(b, 10)

	h10 := alloc(f, nil)
	h20 := alloc(f, nil)

	used0, total0, err := f.audit()
	if err != nil {
		t.Fatal(err)
	}

	exp := c[:15]
	if handle := realloc(f, h10, exp, false); handle != h10 {
		t.Fatal(10, handle, h10)
	}

	if got, _ := f.readUsed(h10); !bytes.Equal(got, exp) {
		t.Fatal(20, len(got), len(exp))
	}

	if got, _ := f.readUsed(h20); len(got) != 0 {
		t.Fatal(30, len(got), 0)
	}

	f = reaudit(t, f, *fnFlag)

	if got, _ := f.readUsed(h10); !bytes.Equal(got, exp) {
		t.Fatal(120, len(got), len(exp))
	}

	if got, _ := f.readUsed(h20); len(got) != 0 {
		t.Fatal(130, len(got), 0)
	}

	used, total, err := f.audit()
	if err != nil {
		t.Fatal(err)
	}

	if difused, diftotal, free := used-used0, total-total0, total-used; difused != 0 || diftotal != 0 || free != 0 {
		t.Fatal(140, difused, diftotal, free)
	}
}

func TestRealloc1Keep(t *testing.T) {
	f, err := fcreate(*fnFlag)
	if err != nil {
		t.Fatal(5, err)
	}

	defer func() {
		if f != nil {
			if err := f.Close(); err != nil {
				t.Fatal(6, err)
			}
		}

		f = nil
		runtime.GC()
		os.Remove(*fnFlag)
	}()

	b := make([]byte, 61680)
	c := content(b, 10)

	h10 := alloc(f, nil)
	h20 := alloc(f, nil)

	used0, total0, err := f.audit()
	if err != nil {
		t.Fatal(err)
	}

	exp := c[:15]
	if handle := realloc(f, h10, exp, true); handle != h10 {
		t.Fatal(10, handle, h10)
	}

	if got, _ := f.readUsed(h10); !bytes.Equal(got, exp) {
		t.Fatal(20, len(got), len(exp))
	}

	if got, _ := f.readUsed(h20); len(got) != 0 {
		t.Fatal(30, len(got), 0)
	}

	f = reaudit(t, f, *fnFlag)

	if got, _ := f.readUsed(h10); !bytes.Equal(got, exp) {
		t.Fatal(120, len(got), len(exp))
	}

	if got, _ := f.readUsed(h20); len(got) != 0 {
		t.Fatal(130, len(got), 0)
	}

	used, total, err := f.audit()
	if err != nil {
		t.Fatal(err)
	}

	if difused, diftotal, free := used-used0, total-total0, total-used; difused != 0 || diftotal != 0 || free != 0 {
		t.Fatal(140, difused, diftotal, free)
	}
}

func TestRealloc2(t *testing.T) {
	f, err := fcreate(*fnFlag)
	if err != nil {
		t.Fatal(5, err)
	}

	defer func() {
		if f != nil {
			if err := f.Close(); err != nil {
				t.Fatal(6, err)
			}
		}

		f = nil
		runtime.GC()
		os.Remove(*fnFlag)
	}()

	b := make([]byte, 61680)
	c := content(b, 10)

	h10 := alloc(f, c[:31])
	h20 := alloc(f, nil)

	used0, total0, err := f.audit()
	if err != nil {
		t.Fatal(err)
	}

	exp := c[:15]
	if handle := realloc(f, h10, exp, false); handle != h10 {
		t.Fatal(10, handle, h10)
	}

	if got, _ := f.readUsed(h10); !bytes.Equal(got, exp) {
		t.Fatal(20, len(got), len(exp))
	}

	if got, _ := f.readUsed(h20); len(got) != 0 {
		t.Fatal(30, len(got), 0)
	}

	f = reaudit(t, f, *fnFlag)

	if got, _ := f.readUsed(h10); !bytes.Equal(got, exp) {
		t.Fatal(120, len(got), len(exp))
	}

	if got, _ := f.readUsed(h20); len(got) != 0 {
		t.Fatal(130, len(got), 0)
	}

	used, total, err := f.audit()
	if err != nil {
		t.Fatal(err)
	}

	if difused, diftotal, free := used-used0, total-total0, total-used; difused != 0 || diftotal != 1 || free != 1 {
		t.Fatal(140, difused, diftotal, free)
	}
}

func TestRealloc2Keep(t *testing.T) {
	f, err := fcreate(*fnFlag)
	if err != nil {
		t.Fatal(5, err)
	}

	defer func() {
		if f != nil {
			if err := f.Close(); err != nil {
				t.Fatal(6, err)
			}
		}

		f = nil
		runtime.GC()
		os.Remove(*fnFlag)
	}()

	b := make([]byte, 61680)
	c := content(b, 10)

	h10 := alloc(f, c[:31])
	h20 := alloc(f, nil)

	used0, total0, err := f.audit()
	if err != nil {
		t.Fatal(err)
	}

	exp := c[:15]
	if handle := realloc(f, h10, exp, true); handle != h10 {
		t.Fatal(10, handle, h10)
	}

	if got, _ := f.readUsed(h10); !bytes.Equal(got, exp) {
		t.Fatal(20, len(got), len(exp))
	}

	if got, _ := f.readUsed(h20); len(got) != 0 {
		t.Fatal(30, len(got), 0)
	}

	f = reaudit(t, f, *fnFlag)

	if got, _ := f.readUsed(h10); !bytes.Equal(got, exp) {
		t.Fatal(120, len(got), len(exp))
	}

	if got, _ := f.readUsed(h20); len(got) != 0 {
		t.Fatal(130, len(got), 0)
	}

	used, total, err := f.audit()
	if err != nil {
		t.Fatal(err)
	}

	if difused, diftotal, free := used-used0, total-total0, total-used; difused != 0 || diftotal != 1 || free != 1 {
		t.Fatal(140, difused, diftotal, free)
	}
}

func TestRealloc3(t *testing.T) {
	f, err := fcreate(*fnFlag)
	if err != nil {
		t.Fatal(5, err)
	}

	defer func() {
		if f != nil {
			if err := f.Close(); err != nil {
				t.Fatal(6, err)
			}
		}

		f = nil
		runtime.GC()
		os.Remove(*fnFlag)
	}()

	b := make([]byte, 61680)
	c := content(b, 10)

	h10 := alloc(f, nil)
	h20 := alloc(f, nil)

	used0, total0, err := f.audit()
	if err != nil {
		t.Fatal(err)
	}

	exp := c[:31]
	var handle int64
	if handle = realloc(f, h10, exp, false); handle == h10 {
		t.Fatal(10, handle, h10)
	}

	if got, _ := f.readUsed(handle); !bytes.Equal(got, exp) {
		t.Fatal(20, len(got), len(exp))
	}

	if got, _ := f.readUsed(h20); len(got) != 0 {
		t.Fatal(30, len(got), 0)
	}

	f = reaudit(t, f, *fnFlag)

	if got, _ := f.readUsed(handle); !bytes.Equal(got, exp) {
		t.Fatal(120, len(got), len(exp))
	}

	if got, _ := f.readUsed(h20); len(got) != 0 {
		t.Fatal(130, len(got), 0)
	}

	used, total, err := f.audit()
	if err != nil {
		t.Fatal(err)
	}

	if difused, diftotal, free := used-used0, total-total0, total-used; difused != 0 || diftotal != 1 || free != 1 {
		t.Fatal(140, difused, diftotal, free)
	}
}

func TestRealloc3Keep(t *testing.T) {
	f, err := fcreate(*fnFlag)
	if err != nil {
		t.Fatal(5, err)
	}

	defer func() {
		if f != nil {
			if err := f.Close(); err != nil {
				t.Fatal(6, err)
			}
		}

		f = nil
		runtime.GC()
		os.Remove(*fnFlag)
	}()

	b := make([]byte, 61680)
	c := content(b, 10)

	h10 := alloc(f, nil)
	h20 := alloc(f, nil)

	used0, total0, err := f.audit()
	if err != nil {
		t.Fatal(err)
	}

	exp := c[:31]
	var handle int64
	if handle = realloc(f, h10, exp, true); handle != h10 {
		t.Fatal(10, handle, h10)
	}

	if got, _ := f.readUsed(handle); !bytes.Equal(got, exp) {
		t.Fatal(20, len(got), len(exp))
	}

	if got, _ := f.readUsed(h20); len(got) != 0 {
		t.Fatal(30, len(got), 0)
	}

	f = reaudit(t, f, *fnFlag)

	if got, _ := f.readUsed(handle); !bytes.Equal(got, exp) {
		t.Fatal(120, len(got), len(exp))
	}

	if got, _ := f.readUsed(h20); len(got) != 0 {
		t.Fatal(130, len(got), 0)
	}

	used, total, err := f.audit()
	if err != nil {
		t.Fatal(err)
	}

	if difused, diftotal, free := used-used0, total-total0, total-used; difused != 1 || diftotal != 1 || free != 0 {
		t.Fatal(140, difused, diftotal, free)
	}
}

func TestRealloc4Keep(t *testing.T) {
	f, err := fcreate(*fnFlag)
	if err != nil {
		t.Fatal(5, err)
	}

	defer func() {
		if f != nil {
			if err := f.Close(); err != nil {
				t.Fatal(6, err)
			}
		}

		f = nil
		runtime.GC()
		os.Remove(*fnFlag)
	}()

	b := make([]byte, 61680)
	c := content(b, 10)

	h10 := alloc(f, c[:31])
	h20 := alloc(f, nil)

	used0, total0, err := f.audit()
	if err != nil {
		t.Fatal(err)
	}

	exp := c[:47]
	var handle int64
	if handle = realloc(f, h10, exp, true); handle != h10 {
		t.Fatal(10, handle, h10)
	}

	if got, _ := f.readUsed(handle); !bytes.Equal(got, exp) {
		t.Fatal(20, len(got), len(exp))
	}

	if got, _ := f.readUsed(h20); len(got) != 0 {
		t.Fatal(30, len(got), 0)
	}

	f = reaudit(t, f, *fnFlag)

	if got, _ := f.readUsed(handle); !bytes.Equal(got, exp) {
		t.Fatal(120, len(got), len(exp))
	}

	if got, _ := f.readUsed(h20); len(got) != 0 {
		t.Fatal(130, len(got), 0)
	}

	used, total, err := f.audit()
	if err != nil {
		t.Fatal(err)
	}

	if difused, diftotal, free := used-used0, total-total0, total-used; difused != 1 || diftotal != 2 || free != 1 {
		t.Fatal(140, difused, diftotal, free)
	}
}

func TestRealloc5(t *testing.T) {
	f, err := fcreate(*fnFlag)
	if err != nil {
		t.Fatal(5, err)
	}

	defer func() {
		if f != nil {
			if err := f.Close(); err != nil {
				t.Fatal(6, err)
			}
		}

		f = nil
		runtime.GC()
		os.Remove(*fnFlag)
	}()

	b := make([]byte, 61680)
	c := content(b, 10)

	h10 := alloc(f, nil)
	h15 := alloc(f, nil)
	h20 := alloc(f, nil)

	used0, total0, err := f.audit()
	if err != nil {
		t.Fatal(err)
	}

	free(f, h15)
	exp := c[:31]
	var handle int64
	if handle = realloc(f, h10, exp, false); handle != h10 {
		t.Fatal(10, handle, h10)
	}

	if got, _ := f.readUsed(handle); !bytes.Equal(got, exp) {
		t.Fatal(20, len(got), len(exp))
	}

	if got, _ := f.readUsed(h20); len(got) != 0 {
		t.Fatal(30, len(got), 0)
	}

	f = reaudit(t, f, *fnFlag)

	if got, _ := f.readUsed(handle); !bytes.Equal(got, exp) {
		t.Fatal(120, len(got), len(exp))
	}

	if got, _ := f.readUsed(h20); len(got) != 0 {
		t.Fatal(130, len(got), 0)
	}

	used, total, err := f.audit()
	if err != nil {
		t.Fatal(err)
	}

	if difused, diftotal, free := used-used0, total-total0, total-used; difused != -1 || diftotal != -1 || free != 0 {
		t.Fatal(140, difused, diftotal, free)
	}
}

func TestRealloc5Keep(t *testing.T) {
	f, err := fcreate(*fnFlag)
	if err != nil {
		t.Fatal(5, err)
	}

	defer func() {
		if f != nil {
			if err := f.Close(); err != nil {
				t.Fatal(6, err)
			}
		}

		f = nil
		runtime.GC()
		os.Remove(*fnFlag)
	}()

	b := make([]byte, 61680)
	c := content(b, 10)

	h10 := alloc(f, nil)
	h15 := alloc(f, nil)
	h20 := alloc(f, nil)

	used0, total0, err := f.audit()
	if err != nil {
		t.Fatal(err)
	}

	free(f, h15)
	exp := c[:31]
	var handle int64
	if handle = realloc(f, h10, exp, true); handle != h10 {
		t.Fatal(10, handle, h10)
	}

	if got, _ := f.readUsed(handle); !bytes.Equal(got, exp) {
		t.Fatal(20, len(got), len(exp))
	}

	if got, _ := f.readUsed(h20); len(got) != 0 {
		t.Fatal(30, len(got), 0)
	}

	f = reaudit(t, f, *fnFlag)

	if got, _ := f.readUsed(handle); !bytes.Equal(got, exp) {
		t.Fatal(120, len(got), len(exp))
	}

	if got, _ := f.readUsed(h20); len(got) != 0 {
		t.Fatal(130, len(got), 0)
	}

	used, total, err := f.audit()
	if err != nil {
		t.Fatal(err)
	}

	if difused, diftotal, free := used-used0, total-total0, total-used; difused != -1 || diftotal != -1 || free != 0 {
		t.Fatal(140, difused, diftotal, free)
	}
}

func TestRealloc6(t *testing.T) {
	f, err := fcreate(*fnFlag)
	if err != nil {
		t.Fatal(5, err)
	}

	defer func() {
		if f != nil {
			if err := f.Close(); err != nil {
				t.Fatal(6, err)
			}
		}

		f = nil
		runtime.GC()
		os.Remove(*fnFlag)
	}()

	b := make([]byte, 61680)
	c := content(b, 10)

	h10 := alloc(f, nil)
	h15 := alloc(f, c[:31])
	h20 := alloc(f, nil)

	used0, total0, err := f.audit()
	if err != nil {
		t.Fatal(err)
	}

	free(f, h15)
	exp := c[:31]
	var handle int64
	if handle = realloc(f, h10, exp, false); handle != h10 {
		t.Fatal(10, handle, h10)
	}

	if got, _ := f.readUsed(handle); !bytes.Equal(got, exp) {
		t.Fatal(20, len(got), len(exp))
	}

	if got, _ := f.readUsed(h20); len(got) != 0 {
		t.Fatal(30, len(got), 0)
	}

	f = reaudit(t, f, *fnFlag)

	if got, _ := f.readUsed(handle); !bytes.Equal(got, exp) {
		t.Fatal(120, len(got), len(exp))
	}

	if got, _ := f.readUsed(h20); len(got) != 0 {
		t.Fatal(130, len(got), 0)
	}

	used, total, err := f.audit()
	if err != nil {
		t.Fatal(err)
	}

	if difused, diftotal, free := used-used0, total-total0, total-used; difused != -1 || diftotal != 0 || free != 1 {
		t.Fatal(140, difused, diftotal, free)
	}
}

func TestRealloc6Keep(t *testing.T) {
	f, err := fcreate(*fnFlag)
	if err != nil {
		t.Fatal(5, err)
	}

	defer func() {
		if f != nil {
			if err := f.Close(); err != nil {
				t.Fatal(6, err)
			}
		}

		f = nil
		runtime.GC()
		os.Remove(*fnFlag)
	}()

	b := make([]byte, 61680)
	c := content(b, 10)

	h10 := alloc(f, nil)
	h15 := alloc(f, c[:31])
	h20 := alloc(f, nil)

	used0, total0, err := f.audit()
	if err != nil {
		t.Fatal(err)
	}

	free(f, h15)
	exp := c[:31]
	var handle int64
	if handle = realloc(f, h10, exp, true); handle != h10 {
		t.Fatal(10, handle, h10)
	}

	if got, _ := f.readUsed(handle); !bytes.Equal(got, exp) {
		t.Fatal(20, len(got), len(exp))
	}

	if got, _ := f.readUsed(h20); len(got) != 0 {
		t.Fatal(30, len(got), 0)
	}

	f = reaudit(t, f, *fnFlag)

	if got, _ := f.readUsed(handle); !bytes.Equal(got, exp) {
		t.Fatal(120, len(got), len(exp))
	}

	if got, _ := f.readUsed(h20); len(got) != 0 {
		t.Fatal(130, len(got), 0)
	}

	used, total, err := f.audit()
	if err != nil {
		t.Fatal(err)
	}

	if difused, diftotal, free := used-used0, total-total0, total-used; difused != -1 || diftotal != 0 || free != 1 {
		t.Fatal(140, difused, diftotal, free)
	}
}

func TestRelocRealloc1(t *testing.T) {
	f, err := fcreate(*fnFlag)
	if err != nil {
		t.Fatal(5, err)
	}

	defer func() {
		if f != nil {
			if err := f.Close(); err != nil {
				t.Fatal(6, err)
			}
		}

		f = nil
		runtime.GC()
		os.Remove(*fnFlag)
	}()

	b := make([]byte, 61680)

	h10 := alloc(f, nil)
	h20 := alloc(f, nil)
	var handle int64
	if handle = realloc(f, h10, b[:31], true); handle != h10 {
		t.Fatal(5, handle, h10)
	}

	used0, total0, err := f.audit() // c+3, c+3
	if err != nil {
		t.Fatal(err)
	}

	c := content(b, 10)
	exp := c[:15]
	if handle = realloc(f, h10, exp, false); handle != h10 {
		t.Fatal(10, handle, h10)
	}

	if got, _ := f.readUsed(h10); !bytes.Equal(got, exp) {
		t.Fatal(20, len(got), len(exp))
	}

	if got, _ := f.readUsed(h20); len(got) != 0 {
		t.Fatal(30, len(got), 0)
	}

	f = reaudit(t, f, *fnFlag)

	if got, _ := f.readUsed(h10); !bytes.Equal(got, exp) {
		t.Fatal(120, len(got), len(exp))
	}

	if got, _ := f.readUsed(h20); len(got) != 0 {
		t.Fatal(130, len(got), 0)
	}

	used, total, err := f.audit() // c+2, c+2
	if err != nil {
		t.Fatal(err)
	}

	if difused, diftotal, free := used-used0, total-total0, total-used; difused != -1 || diftotal != -1 || free != 0 {
		t.Fatal(140, difused, diftotal, free)
	}
}

func TestRelocRealloc1Keep(t *testing.T) {
	f, err := fcreate(*fnFlag)
	if err != nil {
		t.Fatal(5, err)
	}

	defer func() {
		if f != nil {
			if err := f.Close(); err != nil {
				t.Fatal(6, err)
			}
		}

		f = nil
		runtime.GC()
		os.Remove(*fnFlag)
	}()

	b := make([]byte, 61680)

	h10 := alloc(f, nil)
	h20 := alloc(f, nil)
	var handle int64
	if handle = realloc(f, h10, b[:31], true); handle != h10 {
		t.Fatal(5, handle, h10)
	}

	used0, total0, err := f.audit() // c+3, c+3
	if err != nil {
		t.Fatal(err)
	}

	c := content(b, 10)
	exp := c[:15]
	if handle = realloc(f, h10, exp, true); handle != h10 {
		t.Fatal(10, handle, h10)
	}

	if got, _ := f.readUsed(h10); !bytes.Equal(got, exp) {
		t.Fatal(20, len(got), len(exp))
	}

	if got, _ := f.readUsed(h20); len(got) != 0 {
		t.Fatal(30, len(got), 0)
	}

	f = reaudit(t, f, *fnFlag)

	if got, _ := f.readUsed(h10); !bytes.Equal(got, exp) {
		t.Fatal(120, len(got), len(exp))
	}

	if got, _ := f.readUsed(h20); len(got) != 0 {
		t.Fatal(130, len(got), 0)
	}

	used, total, err := f.audit() // c+2, c+2
	if err != nil {
		t.Fatal(err)
	}

	if difused, diftotal, free := used-used0, total-total0, total-used; difused != -1 || diftotal != -1 || free != 0 {
		t.Fatal(140, difused, diftotal, free)
	}
}

func TestRelocRealloc2(t *testing.T) {
	f, err := fcreate(*fnFlag)
	if err != nil {
		t.Fatal(5, err)
	}

	defer func() {
		if f != nil {
			if err := f.Close(); err != nil {
				t.Fatal(6, err)
			}
		}

		f = nil
		runtime.GC()
		os.Remove(*fnFlag)
	}()

	b := make([]byte, 61680)

	h10 := alloc(f, nil)
	h20 := alloc(f, nil)
	var handle int64
	if handle = realloc(f, h10, b[:31], true); handle != h10 {
		t.Fatal(5, handle, h10)
	}

	free(f, h20)

	used0, total0, err := f.audit() // c+2, c+3
	if err != nil {
		t.Fatal(err)
	}

	c := content(b, 10)
	exp := c[:31]
	if handle = realloc(f, h10, exp, false); handle != h10 {
		t.Fatal(10, handle, h10)
	}

	if got, _ := f.readUsed(h10); !bytes.Equal(got, exp) {
		t.Fatal(20, len(got), len(exp))
	}

	f = reaudit(t, f, *fnFlag)

	if got, _ := f.readUsed(h10); !bytes.Equal(got, exp) {
		t.Fatal(120, len(got), len(exp))
	}

	used, total, err := f.audit() // c+1, c+1
	if err != nil {
		t.Fatal(err)
	}

	if difused, diftotal, free := used-used0, total-total0, total-used; difused != -1 || diftotal != -2 || free != 0 {
		t.Fatal(140, difused, diftotal, free)
	}
}

func TestRelocRealloc2Keep(t *testing.T) {
	f, err := fcreate(*fnFlag)
	if err != nil {
		t.Fatal(5, err)
	}

	defer func() {
		if f != nil {
			if err := f.Close(); err != nil {
				t.Fatal(6, err)
			}
		}

		f = nil
		runtime.GC()
		os.Remove(*fnFlag)
	}()

	b := make([]byte, 61680)

	h10 := alloc(f, nil)
	h20 := alloc(f, nil)
	var handle int64
	if handle = realloc(f, h10, b[:31], true); handle != h10 {
		t.Fatal(5, handle, h10)
	}

	free(f, h20)

	used0, total0, err := f.audit() // c+2, c+3
	if err != nil {
		t.Fatal(err)
	}

	c := content(b, 10)
	exp := c[:31]
	if handle = realloc(f, h10, exp, true); handle != h10 {
		t.Fatal(10, handle, h10)
	}

	if got, _ := f.readUsed(h10); !bytes.Equal(got, exp) {
		t.Fatal(20, len(got), len(exp))
	}

	f = reaudit(t, f, *fnFlag)

	if got, _ := f.readUsed(h10); !bytes.Equal(got, exp) {
		t.Fatal(120, len(got), len(exp))
	}

	used, total, err := f.audit() // c+1, c+1
	if err != nil {
		t.Fatal(err)
	}

	if difused, diftotal, free := used-used0, total-total0, total-used; difused != -1 || diftotal != -2 || free != 0 {
		t.Fatal(140, difused, diftotal, free)
	}
}

func TestRelocRealloc3(t *testing.T) {
	f, err := fcreate(*fnFlag)
	if err != nil {
		t.Fatal(5, err)
	}

	defer func() {
		if f != nil {
			if err := f.Close(); err != nil {
				t.Fatal(6, err)
			}
		}

		f = nil
		runtime.GC()
		os.Remove(*fnFlag)
	}()

	b := make([]byte, 61680)

	h10 := alloc(f, nil)
	h20 := alloc(f, b[:31])
	var handle int64
	if handle = realloc(f, h10, b[:31], true); handle != h10 {
		t.Fatal(5, handle, h10)
	}

	free(f, h20)

	used0, total0, err := f.audit() // c+2, c+3
	if err != nil {
		t.Fatal(err)
	}

	c := content(b, 10)
	exp := c[:31]
	if handle = realloc(f, h10, exp, false); handle != h10 {
		t.Fatal(10, handle, h10)
	}

	if got, _ := f.readUsed(h10); !bytes.Equal(got, exp) {
		t.Fatal(20, len(got), len(exp))
	}

	f = reaudit(t, f, *fnFlag)

	if got, _ := f.readUsed(h10); !bytes.Equal(got, exp) {
		t.Fatal(120, len(got), len(exp))
	}

	used, total, err := f.audit() // c+1, c+1
	if err != nil {
		t.Fatal(err)
	}

	if difused, diftotal, free := used-used0, total-total0, total-used; difused != -1 || diftotal != -2 || free != 0 {
		t.Fatal(140, difused, diftotal, free)
	}
}

func TestRelocRealloc3Keep(t *testing.T) {
	f, err := fcreate(*fnFlag)
	if err != nil {
		t.Fatal(5, err)
	}

	defer func() {
		if f != nil {
			if err := f.Close(); err != nil {
				t.Fatal(6, err)
			}
		}

		f = nil
		runtime.GC()
		os.Remove(*fnFlag)
	}()

	b := make([]byte, 61680)

	h10 := alloc(f, nil)
	h20 := alloc(f, b[:31])
	var handle int64
	if handle = realloc(f, h10, b[:31], true); handle != h10 {
		t.Fatal(5, handle, h10)
	}

	free(f, h20)

	used0, total0, err := f.audit() // c+2, c+3
	if err != nil {
		t.Fatal(err)
	}

	c := content(b, 10)
	exp := c[:31]
	if handle = realloc(f, h10, exp, true); handle != h10 {
		t.Fatal(10, handle, h10)
	}

	if got, _ := f.readUsed(h10); !bytes.Equal(got, exp) {
		t.Fatal(20, len(got), len(exp))
	}

	f = reaudit(t, f, *fnFlag)

	if got, _ := f.readUsed(h10); !bytes.Equal(got, exp) {
		t.Fatal(120, len(got), len(exp))
	}

	used, total, err := f.audit() // c+1, c+1
	if err != nil {
		t.Fatal(err)
	}

	if difused, diftotal, free := used-used0, total-total0, total-used; difused != -1 || diftotal != -2 || free != 0 {
		t.Fatal(140, difused, diftotal, free)
	}
}

func TestRelocRealloc4(t *testing.T) {
	f, err := fcreate(*fnFlag)
	if err != nil {
		t.Fatal(5, err)
	}

	defer func() {
		if f != nil {
			if err := f.Close(); err != nil {
				t.Fatal(6, err)
			}
		}

		f = nil
		runtime.GC()
		os.Remove(*fnFlag)
	}()

	b := make([]byte, 61680)

	h10 := alloc(f, nil)
	_ = alloc(f, nil)
	var handle int64
	if handle = realloc(f, h10, b[:47], true); handle != h10 {
		t.Fatal(5, handle, h10)
	}

	_ = alloc(f, nil)

	if handle = realloc(f, h10, b[:31], true); handle != h10 {
		t.Fatal(7, handle, h10)
	}

	used0, total0, err := f.audit() // c+4, c+5
	if err != nil {
		t.Fatal(err)
	}

	c := content(b, 10)
	exp := c[:47]
	if handle = realloc(f, h10, exp, false); handle != h10 {
		t.Fatal(10, handle, h10)
	}

	if got, _ := f.readUsed(h10); !bytes.Equal(got, exp) {
		t.Fatal(20, len(got), len(exp))
	}

	f = reaudit(t, f, *fnFlag)

	if got, _ := f.readUsed(h10); !bytes.Equal(got, exp) {
		t.Fatal(120, len(got), len(exp))
	}

	used, total, err := f.audit() // c+4, c+4
	if err != nil {
		t.Fatal(err)
	}

	if difused, diftotal, free := used-used0, total-total0, total-used; difused != 0 || diftotal != -1 || free != 0 {
		t.Fatal(140, difused, diftotal, free)
	}
}

func TestRelocRealloc4Keep(t *testing.T) {
	f, err := fcreate(*fnFlag)
	if err != nil {
		t.Fatal(5, err)
	}

	defer func() {
		if f != nil {
			if err := f.Close(); err != nil {
				t.Fatal(6, err)
			}
		}

		f = nil
		runtime.GC()
		os.Remove(*fnFlag)
	}()

	b := make([]byte, 61680)

	h10 := alloc(f, nil)
	_ = alloc(f, nil)
	var handle int64
	if handle = realloc(f, h10, b[:47], true); handle != h10 {
		t.Fatal(5, handle, h10)
	}

	_ = alloc(f, nil)

	if handle = realloc(f, h10, b[:31], true); handle != h10 {
		t.Fatal(7, handle, h10)
	}

	used0, total0, err := f.audit() // c+4, c+5
	if err != nil {
		t.Fatal(err)
	}

	c := content(b, 10)
	exp := c[:47]
	if handle = realloc(f, h10, exp, true); handle != h10 {
		t.Fatal(10, handle, h10)
	}

	if got, _ := f.readUsed(h10); !bytes.Equal(got, exp) {
		t.Fatal(20, len(got), len(exp))
	}

	f = reaudit(t, f, *fnFlag)

	if got, _ := f.readUsed(h10); !bytes.Equal(got, exp) {
		t.Fatal(120, len(got), len(exp))
	}

	used, total, err := f.audit() // c+4, c+4
	if err != nil {
		t.Fatal(err)
	}

	if difused, diftotal, free := used-used0, total-total0, total-used; difused != 0 || diftotal != -1 || free != 0 {
		t.Fatal(140, difused, diftotal, free)
	}
}

func TestRelocRealloc5(t *testing.T) {
	f, err := fcreate(*fnFlag)
	if err != nil {
		t.Fatal(5, err)
	}

	defer func() {
		if f != nil {
			if err := f.Close(); err != nil {
				t.Fatal(6, err)
			}
		}

		f = nil
		runtime.GC()
		os.Remove(*fnFlag)
	}()

	b := make([]byte, 61680)

	h10 := alloc(f, nil)
	_ = alloc(f, nil)
	var handle int64
	if handle = realloc(f, h10, b[:31], true); handle != h10 {
		t.Fatal(5, handle, h10)
	}

	_ = alloc(f, nil)

	used0, total0, err := f.audit() // c+4, c+4
	if err != nil {
		t.Fatal(err)
	}

	c := content(b, 10)
	exp := c[:47]
	if handle = realloc(f, h10, exp, false); handle != h10 {
		t.Fatal(10, handle, h10)
	}

	if got, _ := f.readUsed(h10); !bytes.Equal(got, exp) {
		t.Fatal(20, len(got), len(exp))
	}

	f = reaudit(t, f, *fnFlag)

	if got, _ := f.readUsed(h10); !bytes.Equal(got, exp) {
		t.Fatal(120, len(got), len(exp))
	}

	used, total, err := f.audit() // c+4, c+5
	if err != nil {
		t.Fatal(err)
	}

	if difused, diftotal, free := used-used0, total-total0, total-used; difused != 0 || diftotal != 1 || free != 1 {
		t.Fatal(140, difused, diftotal, free)
	}
}

func TestRelocRealloc5Keep(t *testing.T) {
	f, err := fcreate(*fnFlag)
	if err != nil {
		t.Fatal(5, err)
	}

	defer func() {
		if f != nil {
			if err := f.Close(); err != nil {
				t.Fatal(6, err)
			}
		}

		f = nil
		runtime.GC()
		os.Remove(*fnFlag)
	}()

	b := make([]byte, 61680)

	h10 := alloc(f, nil)
	_ = alloc(f, nil)
	var handle int64
	if handle = realloc(f, h10, b[:31], true); handle != h10 {
		t.Fatal(5, handle, h10)
	}

	_ = alloc(f, nil)

	used0, total0, err := f.audit() // c+4, c+4
	if err != nil {
		t.Fatal(err)
	}

	c := content(b, 10)
	exp := c[:47]
	if handle = realloc(f, h10, exp, true); handle != h10 {
		t.Fatal(10, handle, h10)
	}

	if got, _ := f.readUsed(h10); !bytes.Equal(got, exp) {
		t.Fatal(20, len(got), len(exp))
	}

	f = reaudit(t, f, *fnFlag)

	if got, _ := f.readUsed(h10); !bytes.Equal(got, exp) {
		t.Fatal(120, len(got), len(exp))
	}

	used, total, err := f.audit() // c+4, c+5
	if err != nil {
		t.Fatal(err)
	}

	if difused, diftotal, free := used-used0, total-total0, total-used; difused != 0 || diftotal != 1 || free != 1 {
		t.Fatal(140, difused, diftotal, free)
	}
}

func TestRelocRealloc6(t *testing.T) {
	f, err := fcreate(*fnFlag)
	if err != nil {
		t.Fatal(5, err)
	}

	defer func() {
		if f != nil {
			if err := f.Close(); err != nil {
				t.Fatal(6, err)
			}
		}

		f = nil
		runtime.GC()
		os.Remove(*fnFlag)
	}()

	b := make([]byte, 61680)

	h10 := alloc(f, b[:31])
	h20 := alloc(f, nil)
	_ = alloc(f, nil)
	free(f, h20)

	used0, total0, err := f.audit() // c+2, c+3
	if err != nil {
		t.Fatal(err)
	}

	c := content(b, 10)
	exp := c[:15]
	if handle := realloc(f, h10, exp, false); handle != h10 {
		t.Fatal(10, handle, h10)
	}

	if got, _ := f.readUsed(h10); !bytes.Equal(got, exp) {
		t.Fatal(20, len(got), len(exp))
	}

	f = reaudit(t, f, *fnFlag)

	if got, _ := f.readUsed(h10); !bytes.Equal(got, exp) {
		t.Fatal(120, len(got), len(exp))
	}

	used, total, err := f.audit() // c+2, c+3
	if err != nil {
		t.Fatal(err)
	}

	if difused, diftotal, free := used-used0, total-total0, total-used; difused != 0 || diftotal != 0 || free != 1 {
		t.Fatal(140, difused, diftotal, free)
	}
}

func TestRelocRealloc6Keep(t *testing.T) {
	f, err := fcreate(*fnFlag)
	if err != nil {
		t.Fatal(5, err)
	}

	defer func() {
		if f != nil {
			if err := f.Close(); err != nil {
				t.Fatal(6, err)
			}
		}

		f = nil
		runtime.GC()
		os.Remove(*fnFlag)
	}()

	b := make([]byte, 61680)

	h10 := alloc(f, b[:31])
	h20 := alloc(f, nil)
	_ = alloc(f, nil)
	free(f, h20)

	used0, total0, err := f.audit() // c+2, c+3
	if err != nil {
		t.Fatal(err)
	}

	c := content(b, 10)
	exp := c[:15]
	if handle := realloc(f, h10, exp, true); handle != h10 {
		t.Fatal(10, handle, h10)
	}

	if got, _ := f.readUsed(h10); !bytes.Equal(got, exp) {
		t.Fatal(20, len(got), len(exp))
	}

	f = reaudit(t, f, *fnFlag)

	if got, _ := f.readUsed(h10); !bytes.Equal(got, exp) {
		t.Fatal(120, len(got), len(exp))
	}

	used, total, err := f.audit() // c+2, c+3
	if err != nil {
		t.Fatal(err)
	}

	if difused, diftotal, free := used-used0, total-total0, total-used; difused != 0 || diftotal != 0 || free != 1 {
		t.Fatal(140, difused, diftotal, free)
	}
}

func TestFreespaceReuse(t *testing.T) {
	f, err := fcreate(*fnFlag)
	if err != nil {
		t.Fatal(5, err)
	}

	defer func() {
		if f != nil {
			if err := f.Close(); err != nil {
				t.Fatal(6, err)
			}
		}

		f = nil
		runtime.GC()
		os.Remove(*fnFlag)
	}()

	b := make([]byte, 61680)
	c := content(b, 10)

	c10 := c[0 : 0+15]
	c20 := c[16:63]
	c50 := c[64 : 64+15]
	h10 := alloc(f, c10)
	h201 := alloc(f, nil)
	h202 := alloc(f, nil)
	h203 := alloc(f, nil)
	h50 := alloc(f, c50)
	free(f, h201)
	free(f, h202)
	free(f, h203)
	used0, total0, err := f.audit() // c+2, c+3
	if err != nil {
		t.Fatal(err)
	}

	h20 := alloc(f, c20)

	if got, _ := f.readUsed(h10); !bytes.Equal(got, c10) {
		t.Fatal(10)
	}

	if got, _ := f.readUsed(h20); !bytes.Equal(got, c20) {
		t.Fatal(20)
	}

	if got, _ := f.readUsed(h50); !bytes.Equal(got, c50) {
		t.Fatal(30)
	}

	f = reaudit(t, f, *fnFlag)

	if got, _ := f.readUsed(h10); !bytes.Equal(got, c10) {
		t.Fatal(110)
	}

	if got, _ := f.readUsed(h20); !bytes.Equal(got, c20) {
		t.Fatal(120)
	}

	if got, _ := f.readUsed(h50); !bytes.Equal(got, c50) {
		t.Fatal(130)
	}

	used, total, err := f.audit() // c+3, c+3
	if err != nil {
		t.Fatal(err)
	}

	if difused, diftotal, free := used-used0, total-total0, total-used; difused != 1 || diftotal != 0 || free != 0 {
		t.Fatal(140, difused, diftotal, free)
	}
}

func TestFreespaceReuse2(t *testing.T) {
	f, err := fcreate(*fnFlag)
	if err != nil {
		t.Fatal(5, err)
	}

	defer func() {
		if f != nil {
			if err := f.Close(); err != nil {
				t.Fatal(6, err)
			}
		}

		f = nil
		runtime.GC()
		os.Remove(*fnFlag)
	}()

	b := make([]byte, 61680)
	c := content(b, 10)

	c10 := c[0 : 0+15]
	c20 := c[16:47]
	c50 := c[64 : 64+15]
	h10 := alloc(f, c10)
	h201 := alloc(f, nil)
	h202 := alloc(f, nil)
	h203 := alloc(f, nil)
	h50 := alloc(f, c50)
	free(f, h201)
	free(f, h202)
	free(f, h203)
	used0, total0, err := f.audit() // c+2, c+3
	if err != nil {
		t.Fatal(err)
	}

	h20 := alloc(f, c20)

	if got, _ := f.readUsed(h10); !bytes.Equal(got, c10) {
		t.Fatal(10)
	}

	if got, _ := f.readUsed(h20); !bytes.Equal(got, c20) {
		t.Fatal(20)
	}

	if got, _ := f.readUsed(h50); !bytes.Equal(got, c50) {
		t.Fatal(30)
	}

	f = reaudit(t, f, *fnFlag)

	if got, _ := f.readUsed(h10); !bytes.Equal(got, c10) {
		t.Fatal(110)
	}

	if got, _ := f.readUsed(h20); !bytes.Equal(got, c20) {
		t.Fatal(120)
	}

	if got, _ := f.readUsed(h50); !bytes.Equal(got, c50) {
		t.Fatal(130)
	}

	used, total, err := f.audit() // c+3, c+4
	if err != nil {
		t.Fatal(err)
	}

	if difused, diftotal, free := used-used0, total-total0, total-used; difused != 1 || diftotal != 1 || free != 1 {
		t.Fatal(140, difused, diftotal, free)
	}
}

func testBug1(t *testing.T, swap bool) {
	// Free lists table item for size 3856 points to list of free blocks
	// NOT of size 3856 but at least 3856.
	f, err := fcreate(*fnFlag)
	if err != nil {
		t.Fatal(5, err)
	}

	defer func() {
		if f != nil {
			if err := f.Close(); err != nil {
				t.Fatal(6, err)
			}
		}

		f = nil
		runtime.GC()
		os.Remove(*fnFlag)
	}()

	_ = alloc(f, nil)
	b := make([]byte, 61680)
	f1 := alloc(f, b)
	f2 := alloc(f, b)
	_ = alloc(f, nil)

	used0, total0, err := f.audit() // c+4, c+4
	if err != nil {
		t.Fatal(err)
	}

	if swap {
		f1, f2 = f2, f1
	}
	free(f, f1)
	free(f, f2)
	_ = alloc(f, nil)

	f = reaudit(t, f, *fnFlag)

	used, total, err := f.audit() // c+3, c+4
	if err != nil {
		t.Fatal(err)
	}

	if difused, diftotal, free := used-used0, total-total0, total-used; difused != -1 || diftotal != 0 || free != 1 {
		t.Fatal(10, difused, diftotal, free)
	}
}

func TestBug1(t *testing.T) {
	testBug1(t, false)
	testBug1(t, true)
}

func TestMix(t *testing.T) {
	if testing.Short() {
		t.Log("skipped")
		return
	}

	const (
		n = 1 << 10
	)

	if testing.Short() {
		t.Log("skipped")
		return
	}

	t.Log(n)
	f, err := fcreate(*fnFlag)
	if err != nil {
		t.Fatal(5, err)
	}

	defer func() {
		if f != nil {
			if err := f.Close(); err != nil {
				t.Fatal(6, err)
			}
		}

		f = nil
		runtime.GC()
		os.Remove(*fnFlag)
	}()

	b := make([]byte, 61680)
	rng, err := mathutil.NewFC32(0, n-1, true)
	if err != nil {
		t.Fatal(err)
	}

	ha := make([]int64, n)
	payload := 0

	t0 := time.Nanoseconds()
	// Alloc n block with upper half of content
	for _ = range ha {
		r := rng.Next()
		c := content(b, int64(r))
		c = c[len(c)/2:]
		ha[r] = alloc(f, c)
		payload += len(c)
	}
	dt := float64(time.Nanoseconds()-t0) / 1e9
	t.Logf("write time A %.3g", dt)

	// verify
	f = reaudit(t, f, *fnFlag)
	t.Logf("size A %d for %d bytes (fill factor %3.1f%%)", f.atoms<<4, payload, 100*float64(payload)/float64(f.atoms<<4))
	t0 = time.Nanoseconds()
	for _ = range ha {
		r := rng.Next()
		c := content(b, int64(r))
		c = c[len(c)/2:]
		if got, _ := f.readUsed(ha[r]); !bytes.Equal(got, c) {
			t.Fatal(10)
		}
	}
	dt = float64(time.Nanoseconds()-t0) / 1e9
	t.Logf("read time A %.3g", dt)
	// free half of the blocks
	t0 = time.Nanoseconds()
	for i := 0; i < n/2; i++ {
		free(f, ha[i])
		ha[i] = 0
	}
	dt = float64(time.Nanoseconds()-t0) / 1e9
	t.Logf("free time A %.3g", dt)

	// verify
	f = reaudit(t, f, *fnFlag)
	t.Logf("size B %d (freeing half of the blocks)", f.atoms<<4)
	t0 = time.Nanoseconds()
	for _ = range ha {
		r := rng.Next()
		h := ha[r]
		if h == 0 {
			continue
		}

		c := content(b, int64(r))
		c = c[len(c)/2:]
		if got, _ := f.readUsed(h); !bytes.Equal(got, c) {
			t.Fatal(20)
		}
	}
	dt = float64(time.Nanoseconds()-t0) / 1e9
	t.Logf("read time B %.3g", dt)

	// reloc extend
	t0 = time.Nanoseconds()
	for _ = range ha {
		r := rng.Next()
		h := ha[r]
		if h == 0 {
			continue
		}

		c := content(b, int64(r))
		//f = reaudit(t, f, *fnFlag)
		if h2 := realloc(f, h, c, true); h2 != h {
			t.Fatal(30)
		}
	}
	dt = float64(time.Nanoseconds()-t0) / 1e9
	t.Logf("realoc time B %.3g", dt)

	// verify
	f = reaudit(t, f, *fnFlag)
	t.Logf("size C %d for %d bytes (reallocated all used blocks to double size, fill factor %3.1f%%", f.atoms<<4, payload, 100*float64(payload)/float64(f.atoms<<4))

	t0 = time.Nanoseconds()
	for _ = range ha {
		r := rng.Next()
		h := ha[r]
		if h == 0 {
			continue
		}

		c := content(b, int64(r))
		if got, _ := f.readUsed(ha[r]); !bytes.Equal(got, c) {
			t.Fatal(40)
		}
	}
	dt = float64(time.Nanoseconds()-t0) / 1e9
	t.Logf("read time C %.3g", dt)
}

///*

//buf
//none	falloc.BenchmarkIntChan	 5000000	       427 ns/op	   9.37 MB/s
//1		falloc.BenchmarkIntChan	 5000000	       717 ns/op	   5.58 MB/s
//2		falloc.BenchmarkIntChan	 5000000	       456 ns/op	   8.77 MB/s
//4		falloc.BenchmarkIntChan	 5000000	       327 ns/op	  12.23 MB/s
//8		falloc.BenchmarkIntChan	10000000	       249 ns/op	  16.06 MB/s
//16		falloc.BenchmarkIntChan	10000000	       214 ns/op	  18.69 MB/s
//32		falloc.BenchmarkIntChan	10000000	       196 ns/op	  20.41 MB/s
//64		falloc.BenchmarkIntChan	10000000	       189 ns/op	  21.16 MB/s
//128		falloc.BenchmarkIntChan	10000000	       185 ns/op	  21.62 MB/s
//256		falloc.BenchmarkIntChan	10000000	       183 ns/op	  21.86 MB/s
//512		falloc.BenchmarkIntChan	10000000	       183 ns/op	  21.86 MB/s

//*/
//func BenchmarkIntChan(b *testing.B) {
//c := make(chan int, 512)

//go func() {
//for i := 0; i < b.N; i++ {
//c <- i
//}
//}()

//for i := 0; i < b.N; i++ {
//<-c
//}

//b.SetBytes(4)
//}

//var iglobal int32

//func BenchmarkIntAtomicAdd(b *testing.B) {
//for i := 0; i < b.N; i++ {
//atomic.AddInt32(&iglobal, 1)
//}
//b.SetBytes(4)
//}

//func BenchmarkRWM(b *testing.B) {
//var rwm sync.RWMutex

//for i := 0; i < b.N; i++ {
//rwm.Lock()
//rwm.Unlock()
//}
//b.SetBytes(4)
//}
