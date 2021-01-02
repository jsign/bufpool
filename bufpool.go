package bufpool

import (
	"sync"
	"sync/atomic"
)

// A Pool holds reusable set of []byte to be arbitrarly subsliced in
// future requests.
type Pool struct {
	sz         int
	pool       sync.Pool // of *shard
	maxRetries int
}

// New returns a new *Pool, where each shard has `sz` bytes size, and
// optionally a maxRetries for the maximum times to look for a suitable
// shard before creating a new one.
func New(sz int, opts ...Option) *Pool {
	cfg := defaultCfg
	for _, o := range opts {
		o(&cfg)
	}
	return &Pool{
		sz: sz,
		pool: sync.Pool{
			New: func() interface{} {
				return &shard{b: make([]byte, sz)}
			},
		},
		maxRetries: cfg.maxRetries,
	}
}

type shard struct {
	b    []byte
	refs int64
	off  int
}

// Make returns a Buffer which contains a []byte with lenght/capacity
// equal to `n` bytes.
func (p *Pool) Make(n int) Buffer {
	if n < 0 {
		panic("size should be greater than zero")
	}
	if n == 0 || n >= p.sz {
		return mallocBuffer(n)
	}
	var s *shard
	for i := 0; i < p.maxRetries && s == nil; i++ {
		st := p.pool.Get().(*shard)
		// Return pools after we got the subslice, as to
		// avoid getting the same shards again in further iterations
		// of this loop.
		// TODO(jsign): ^ might have an impact on other concurrent calls
		// that might not find shards in the pool since still have to be
		// returned. Most probably, that's fine since defered returns
		// means those shards where somewhat "full". Of course, depends
		// on the requested size but there's a reasoanble chance that won't
		// be useful for other callers. (hand-wavy argument).
		defer p.pool.Put(st) // better luck next time

		switch {
		case st.off+n < len(st.b):
			// Enough bytes left in this shard to satisfy the request.
			s = st
		case atomic.LoadInt64(&st.refs) == 0:
			// All old buffers returned; start again at the beginning.
			s = st
			s.off = 0
		}
	}
	if s == nil {
		s = p.pool.New().(*shard)
		defer p.pool.Put(s)
	}

	atomic.AddInt64(&s.refs, 1) // incr refcount
	b := s.b[s.off : s.off+n : s.off+n]
	s.off += n
	return Buffer{B: b, refs: &s.refs}
}

// Buffer is a []byte allocated and managed by a Pool.
// Buffers should be explicitly freed by a call to Done
// when they are no longer in use. Buffers are typically
// used as values (Buffer, not *Buffer), to avoid heap
// allocations.
type Buffer struct {
	B    []byte
	refs *int64
}

func mallocBuffer(n int) Buffer {
	return Buffer{B: make([]byte, n)}
}

// Clear zeroes the underlying []byte.
func (b *Buffer) Clear() {
	for i := range b.B {
		b.B[i] = 0
	}
}

// Len returns the length of the buffer, which is equal to
// its capacity.
func (b *Buffer) Len() int {
	return len(b.B)
}

// Done returns the buffer space to the allocator for future use.
// It's important to call this method when done using the Buffer, since
// not doing so will leak a complete internal shard. Multiple calls
// to Done are idempotent and thus safe.
func (b *Buffer) Done() {
	// b.ctr is nil if b's slice was allocated through a call to make,
	// or if Done has already been called.
	if b.refs != nil {
		atomic.AddInt64(b.refs, -1) // decr from region refcount
		b.refs = nil
	}
}
