// bufpool manages a set of temporary []bytes.
//
// It is similar to sync.Pool.
// Putting []byte in a sync.Pool has problems.
// The cap you get from a slice you get from the pool
// might not be sufficient for your needs.
// And if you put a giant slice in the pool,
// and mostly use it for small slices in the future,
// you're wasting memory.
//
// One workaround is to have multiple sync.Pools,
// each of which contains slices of a similar cap.
// This package takes a different approach.
//
// It allocates large slices and hands out subslices of them.
// (It is effectively a bump-the-pointer allocator.)
// When enough subslices have been returned,
// it can reuse the large slice for future requests.
//
// There is one tunable parameter, how large the large slices are.
// If this number is too big, bufpool will use lots of memory.
// If this number is too small, bufpool will frequently need
// to make new large slices, which may use lots of memory.
//
// Sample usage:
//
//   pool := bufpool.New(1024)
//   buf := pool.Make(15)
//   // buf.B is a []byte with len/cap 15
//   buf.Clear() // if necessary; buf may contain data from previous uses
//   buf.Done() // return to pool for future use
//
// Failure to call buf.Done will be bad.
package bufpool

import (
	"sync"
	"sync/atomic"
)

// A Pool holds reusable []byte.
// It is similar to a sync.Pool, except that it is specialized for []byte.
type Pool struct {
	sz   int
	pool sync.Pool // of *shard
}

// TODO: doc. sz should be pretty big compared to the slices you will actually allocate.
func New(sz int) *Pool {
	return &Pool{
		sz: sz,
		pool: sync.Pool{
			New: func() interface{} {
				return &shard{b: make([]byte, sz)}
			},
		},
	}
}

type shard struct {
	b    []byte
	refs int64
	off  int
}

func (p *Pool) Make(n int) Buffer {
	if n == 0 || n >= p.sz { // TODO: single uint comparison, document this
		return mallocBuffer(n)
	}
	s := p.pool.Get().(*shard)
	// We intentionally do not use defer p.pool.Put(s) here.
	// This is because if we cannot switch to a new region
	// in the current shard, we abandon that shard and make a new one.
	// In that case we want to put the new shard in the pool instead.

	switch {
	case s.off+n < len(s.b):
		// Enough bytes left in this shard to satisfy the request.
	case atomic.LoadInt64(&s.refs) == 0:
		// All old buffers returned; start again at the beginning.
		s.off = 0
	default:
		// Can't use this buffer. Make a new one.
		// TODO: should we try getting a different shard from the pool first?
		// how many times before we give up and make a new one?
		p.pool.Put(s) // better luck next time
		s = p.pool.New().(*shard)
	}

	atomic.AddInt64(&s.refs, 1) // incr refcount
	b := s.b[s.off : s.off+n : s.off+n]
	s.off += n
	p.pool.Put(s)
	return Buffer{B: b, refs: &s.refs}
}

// Buffer is a []byte allocated and managed by a Pool.
// Buffers should be explicitly freed by a call to Done
// when they are no longer in use.
// Buffers are typically used as values (Buffer, not *Buffer),
// to avoid heap allocations.
type Buffer struct {
	B    []byte
	refs *int64
}

// mallocBuffer uses the standard allocator (make) to create a Buffer.
func mallocBuffer(n int) Buffer {
	return Buffer{B: make([]byte, n)}
}

func (b *Buffer) Clear() {
	for i := range b.B {
		b.B[i] = 0
	}
}

func (b *Buffer) Len() int {
	return len(b.B)
}

func (b *Buffer) Done() {
	// b.ctr is nil if b's slice was allocated through a call to make,
	// or if Done has already been called.
	if b.refs != nil {
		atomic.AddInt64(b.refs, -1) // decr from region refcount
		b.refs = nil                // make Done idempotent, since it is cheap and easy to do
	}
}
