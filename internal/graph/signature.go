package graph

import (
	"encoding/binary"
	"hash/maphash"
	"io/fs"

	"github.com/fiatcode-gh/weft/v3/internal/layout"
)

// Signature identifies the walked state of a graph: every indexed .md
// file's graph-relative path, size and modification time, in walk order.
// Equal signatures mean no file was added, removed or changed (per stat);
// values are only comparable within one process.
type Signature uint64

// ScanSignature walks the graph with BuildIndex's rules and stats every
// file without reading any; read-only.
func ScanSignature(graphPath string, lay layout.Layout) (Signature, error) {
	var sb sigBuilder
	err := walkGraph(graphPath, lay, func(string, ...any) {}, func(rel string, d fs.DirEntry) {
		info, err := d.Info()
		sb.add(rel, info, err)
	})
	if err != nil {
		return 0, err
	}
	return sb.sum(), nil
}

// Signature is the signature of the walk this index was built from.
func (idx *Index) Signature() Signature { return idx.signature }

var sigSeed = maphash.MakeSeed()

// sigBuilder accumulates a Signature; its zero value is ready to use.
type sigBuilder struct {
	h    maphash.Hash
	buf  [16]byte
	init bool
}

// add records one walked file. A failed stat (err != nil) counts as size -1
// and mtime 0 so both walkers agree.
func (b *sigBuilder) add(rel string, info fs.FileInfo, err error) {
	if !b.init {
		b.h.SetSeed(sigSeed)
		b.init = true
	}
	size, mtime := int64(-1), int64(0)
	if err == nil {
		size, mtime = info.Size(), info.ModTime().UnixNano()
	}
	b.h.WriteString(rel)
	b.h.WriteByte(0)
	binary.LittleEndian.PutUint64(b.buf[:8], uint64(size))
	binary.LittleEndian.PutUint64(b.buf[8:], uint64(mtime))
	b.h.Write(b.buf[:])
}

func (b *sigBuilder) sum() Signature {
	if !b.init {
		b.h.SetSeed(sigSeed)
		b.init = true
	}
	return Signature(b.h.Sum64())
}
