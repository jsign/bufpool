package bufpool_test

import (
	"strconv"
	"sync"
	"testing"

	"tailscale.com/util/bufpool"
)

func TestBasics(t *testing.T) {
	pool := bufpool.New(100)
	for i := 0; i < 10; i++ {
		buf := pool.Make(20)
		if len(buf.B) != 20 || cap(buf.B) != 20 {
			t.Fatalf("bad make len=%v cap=%v, want 20, 20", len(buf.B), cap(buf.B))
		}
		buf.Done()
	}
}

func TestWrap(t *testing.T) {
	pool := bufpool.New(100)
	for i := 0; i < 200; i++ {
		buf := pool.Make(1)
		if len(buf.B) != 1 || cap(buf.B) != 1 {
			t.Fatalf("bad make len=%v cap=%v, want 20, 20", len(buf.B), cap(buf.B))
		}
		buf.Done()
	}
}

func TestHammer(t *testing.T) {
	pool := bufpool.New(100)
	var wg sync.WaitGroup
	for i := 0; i < 1000; i++ {
		wg.Add(1)
		go func() {
			for j := 0; j < 1000; j++ {
				buf := pool.Make(1)
				if len(buf.B) != 1 || cap(buf.B) != 1 {
					t.Fatalf("bad make len=%v cap=%v, want 20, 20", len(buf.B), cap(buf.B))
				}
				// Write to the buffer, so that if anyone else also has non-synchronized
				// access to the same buffer, the race detector will complain.
				buf.B[0] = 'A'
				buf.Done()
			}
			wg.Done()
		}()
	}
	wg.Wait()
}

var sinkSlice []byte

func BenchmarkVsMalloc(b *testing.B) {
	for _, sz := range []int{4, 64, 512, 16384} {
		b.Run(strconv.Itoa(sz), func(b *testing.B) {
			b.Run("malloc", func(b *testing.B) {
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					sinkSlice = make([]byte, sz)
				}
			})

			b.Run("overhead", func(b *testing.B) {
				pool := bufpool.New(sz / 2) // too small for any of our allocations -- TODO: is this still true?
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					buf := pool.Make(sz)
					buf.Done()
				}
			})

			b.Run("reuse", func(b *testing.B) {
				pool := bufpool.New(sz * 2) // big enough for all of our allocations -- TODO: is this still true?
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					buf := pool.Make(sz)
					buf.Done()
				}
			})
		})
	}
}
