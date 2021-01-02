# bufpool

`bufpool` manages a set of []bytes in a `sync.Pool` to provide smaller 
[]byte subslices.

## Usage
Sample usage:
```
pool := bufpool.New(1024) // Size of shards
buf := pool.Make(15)
// buf.B is an unzeroed []byte with len/cap 15
buf.Clear() // if necessary; buf may contain data from previous uses
buf.Done() // return to pool for future use
```

## How this compares to usual `sync.Pool` solutions?
Using a single `sync.Pool` of []bytes has known problems, like receiving a low capacity slice, infecting the pool with big slices produced by even rare outliers, etc.

A usual solution is to bucketize requests using different pools, which prevents outliers from infecting the pool with worst-case slices of high capacity and thus wasting memory.

This library takes a different approach. It manages a single `sync.Pool` of _big_[]bytes called _shards_. A request of []byte of size _n_  results in a slice from a shard in the pool. It is effectively a bump-the-pointer allocator within each shard.

## Configuration
This library has two knobs:
1. `sz`: The size of the shards.
2. `maxRetries` (opt, default `0`): Maximum retries to find a suitable shard before making a new one.

## How it works
Whenever a new request of size `n` is received, a shard is pulled from the `sync.Pool` and checked if it contains enough free space for the request. If that is not the case, it will retry `maxRetries` times to see if other shards can fit the request. A new shard is created if none is found.

Since each shard behaves as a bump-the-pointer allocator, ideally, subslices need to be returned fast to avoid bad consequences, like:
1. A shard will be alive and not be GCed since a single subslice is still holding a reference to it.
2. The ratio of used vs. free space of a shard will decrease rapidly since a single subslice prevents the shard pointer from starting from zero again.

A shard pointer moved to zero-position when all subslices were returned, and a request cannot be fulfilled.

## Tradeoffs
The two available knobs produce different tradeoffs to the allocator.

If the shard size is too small, it behaves as a direct `make` with extra overhead. If the shard size is too big, it will waste memory since a portion of the shard will not be ever subsliced.

Regarding `maxRetries`, whenever a shard is pulled from the pool, it might not contain enough free space to generate the needed subslice size. At this point, the allocator has to create a new shard or try another pooled shard that might have enough free space.

A low value will only try a few times (minimum of 1) to find a suitable shard and move to create a new shard if needed. This might create new shards when other shards in the pool also have enough free space for the needed size.

A big `maxRetries` will inspect all existing shards for free space before creating a new shard, thus only allocating a new shard if necessary. On the other hand, it may increase latency/contention due to doing multiple `Get` from `sync.Pool`. It might also make a high concurrency scenario worse since pools are not returned to the pool until a request has finished, thus potentially creating new shards when unreturned ones might be fine to use.

The above analysis is purely theoretical and most probably useless to make decisions. The only way to know the correct values for each knob depends on the use-case and most probably will involve doing benchmarks to find optimal values. See the _Benchmark_ section for some naive simulation results.

## Benchmarks

This section contains available benchmark results.

### BenchmarkVsMalloc
This benchmark compares a single (thus, non-concurrent) client for different sizes (4, 64, 512, 16k bytes):
- `malloc`: Using `make` directly.
- `overhead`: Asking for a size greater than shard size results in a direct `make` call and thus measures the overhead cost compared to the above case.
- `reuse`: Ask-and-return serially.  

```
goos: linux
goarch: amd64
pkg: github.com/jsign/bufpool
BenchmarkVsMalloc/4/malloc-16           100000000               16.0 ns/op             4 B/op          1 allocs/op
BenchmarkVsMalloc/4/overhead-16         67191996                19.1 ns/op             4 B/op          1 allocs/op
BenchmarkVsMalloc/4/reuse-16            19410166                53.8 ns/op             0 B/op          0 allocs/op
BenchmarkVsMalloc/64/malloc-16          27107473                43.0 ns/op            64 B/op          1 allocs/op
BenchmarkVsMalloc/64/overhead-16        24714236                49.3 ns/op            64 B/op          1 allocs/op
BenchmarkVsMalloc/64/reuse-16           20522583                54.3 ns/op             0 B/op          0 allocs/op
BenchmarkVsMalloc/512/malloc-16          8445880               138 ns/op             512 B/op          1 allocs/op
BenchmarkVsMalloc/512/overhead-16        8599345               140 ns/op             512 B/op          1 allocs/op
BenchmarkVsMalloc/512/reuse-16          18668682                54.3 ns/op             0 B/op          0 allocs/op
BenchmarkVsMalloc/16384/malloc-16         355327              3334 ns/op           16384 B/op          1 allocs/op
BenchmarkVsMalloc/16384/overhead-16       340364              3293 ns/op           16384 B/op          1 allocs/op
BenchmarkVsMalloc/16384/reuse-16        20684917                54.8 ns/op             0 B/op          0 allocs/o
```

### BenchmarkChaos
This benchmark creates a scenario of concurrent calls with some non-uniform `Make` and `Done` calls, as _simulate_ some real use-case, with different shard retries to see the impact in allocations.

```
goos: linux
goarch: amd64
pkg: github.com/jsign/bufpool
BenchmarkChaos/0-16          134           8914546 ns/op        18926198 B/op     200415 allocs/op
BenchmarkChaos/1-16          249           5057267 ns/op         2493774 B/op      21583 allocs/op
BenchmarkChaos/2-16          234           5225900 ns/op         1411009 B/op       9802 allocs/op
BenchmarkChaos/4-16          241           4752396 ns/op          850502 B/op       3594 allocs/op
BenchmarkChaos/8-16          250           4656146 ns/op          608421 B/op        925 allocs/op
```
The benchmark results should only be compared in a relative sense since the benchmark scenario adds work overhead unrelated to the allocator.

The benchmark is a synthetic scenario to show some numbers on available knobs.

## Credits
This repository is based on a Playground snippet created by Josh Bleecher Snyder (@josharian). His original work can be found unchanged in the second commit of this repo.

