package async

import (
	"errors"
	"sync"
	"testing"
)

func TestLatestCoalescesBurstAndDefersReentrantValue(t *testing.T) {
	var queue []func()
	var values []int
	var latest *Latest[int]
	latest, _ = NewLatest(func(fn func()) { queue = append(queue, fn) }, func(v int) {
		values = append(values, v)
		if v == 99999 {
			latest.Publish(100000)
		}
	})
	for i := 0; i < 100000; i++ {
		if !latest.Publish(i) {
			t.Fatal("rejected")
		}
	}
	if len(queue) != 1 {
		t.Fatal("unbounded delivery queue", len(queue))
	}
	fn := queue[0]
	queue = nil
	fn()
	if len(values) != 1 || values[0] != 99999 || len(queue) != 1 {
		t.Fatal(values, len(queue))
	}
	queue[0]()
	if len(values) != 2 || values[1] != 100000 {
		t.Fatal(values)
	}
	latest.Close()
	if latest.Publish(3) {
		t.Fatal("accepted after close")
	}
}

func TestLatestCloseSuppressesQueuedAndReleasesReferences(t *testing.T) {
	var fn func()
	latest, _ := NewLatest(func(f func()) { fn = f }, func(*int) { t.Fatal("closed callback ran") })
	value := 3
	latest.Publish(&value)
	latest.Close()
	latest.Close()
	if latest.value != nil || latest.consume != nil || latest.dispatch != nil {
		t.Fatal("retained owners")
	}
	fn()
	var zero Latest[int]
	if zero.Publish(1) {
		t.Fatal("uninitialized latest accepted")
	}
	zero.Close()
	if _, err := NewLatest[int](nil, func(int) {}); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	if _, err := NewLatest[int](func(func()) {}, nil); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
}

func TestLatestConcurrentPublishAndPanicRecovery(t *testing.T) {
	var mu sync.Mutex
	var queue []func()
	calls := 0
	latest, _ := NewLatest(func(fn func()) { mu.Lock(); queue = append(queue, fn); mu.Unlock() }, func(int) {
		calls++
		if calls == 1 {
			panic("consumer")
		}
	})
	var wg sync.WaitGroup
	for g := 0; g < 16; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 1000; i++ {
				latest.Publish(g*1000 + i)
			}
		}(g)
	}
	wg.Wait()
	if len(queue) != 1 {
		t.Fatal(len(queue))
	}
	func() {
		defer func() {
			if recover() == nil {
				t.Error("missing consumer panic")
			}
		}()
		queue[0]()
	}()
	latest.Publish(7)
	if len(queue) != 2 {
		t.Fatal("panic wedged delivery", len(queue))
	}
	queue[1]()
	latest.Close()
	if calls != 2 {
		t.Fatal(calls)
	}
}

func TestLatestPublishWhileDeliveringAndCloseDuringConsumer(t *testing.T) {
	queue := make(chan func(), 4)
	entered, release := make(chan struct{}), make(chan struct{})
	latest, _ := NewLatest(func(fn func()) { queue <- fn }, func(int) { close(entered); <-release })
	latest.Publish(1)
	finished := make(chan struct{})
	go func() { (<-queue)(); close(finished) }()
	<-entered
	for i := 0; i < 1000; i++ {
		latest.Publish(i)
	}
	if len(queue) != 0 {
		t.Fatal("overlapping delivery")
	}
	latest.Close()
	close(release)
	<-finished
	if len(queue) != 0 {
		t.Fatal("close queued a new delivery")
	}
}

func BenchmarkLatestPublishQueued(b *testing.B) {
	posts := 0
	latest, _ := NewLatest(func(func()) { posts++ }, func(int) {})
	defer latest.Close()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		latest.Publish(i)
	}
	b.StopTimer()
	if posts != 1 {
		b.Fatal(posts)
	}
}
