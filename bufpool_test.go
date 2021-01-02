package bufpool_test

import (
	"errors"
	"fmt"
	"math/rand"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/jsign/bufpool"
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
			t.Fatalf("bad make len=%v cap=%v, want 1, 1", len(buf.B), cap(buf.B))
		}
		buf.Done()
	}
}

func TestHammer(t *testing.T) {
	pool := bufpool.New(100)
	fail := make(chan error, 1000)
	var wg sync.WaitGroup
	for i := 0; i < 1000; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 1000; j++ {
				buf := pool.Make(1)
				if len(buf.B) != 1 || cap(buf.B) != 1 {
					fail <- fmt.Errorf("bad make len=%v cap=%v, want 1, 1", len(buf.B), cap(buf.B))
					return
				}
				// Write to the buffer, so that if anyone else also has non-synchronized
				// access to the same buffer, the race detector will complain.
				buf.B[0] = 'A'
				buf.Done()
			}
		}()
	}
	wg.Wait()

	select {
	case err := <-fail:
		t.Fatal(err)
	default:
	}
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

func BenchmarkChaos(b *testing.B) {
	retries := []int{0, 1, 2, 4, 8}

	for _, r := range retries {
		b.Run(strconv.Itoa(r), func(b *testing.B) {
			b.ReportAllocs()
			pool := bufpool.New(100, bufpool.WithMaxRetries(r))

			var wg sync.WaitGroup
			fail := make(chan error, 100)
			b.ResetTimer()
			for j := 0; j < b.N; j++ {
				for i := 0; i < 100; i++ {
					i := i
					wg.Add(1)
					go func() {
						defer wg.Done()
						rz := newRandz(i)
						for j := 0; j < 1000; j++ {
							sz := rz.genSize()
							buf := pool.Make(sz)
							if len(buf.B) != sz || cap(buf.B) != sz {
								fail <- errors.New("bad make len and cap")
								return
							}
							time.Sleep(rz.genSleep())
							buf.Done()
						}
					}()
				}
				wg.Wait()

				select {
				case err := <-fail:
					b.Fatal(err)
				default:
				}
			}
		})
	}
}

type randz struct {
	r *rand.Rand
}

func newRandz(i int) randz {
	return randz{r: rand.New(rand.NewSource(int64(i)))}
}

func (r *randz) genSize() int {
	sz := int(r.r.NormFloat64()) + 10
	if sz < 0 {
		sz = 0
	}
	return sz
}

func (r *randz) genSleep() time.Duration {
	s := int(r.r.NormFloat64()*.3 + 1)
	if s < 0 {
		s = 0
	}
	return time.Duration(s) * time.Microsecond
}
