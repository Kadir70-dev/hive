package password

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestHashAndVerify(t *testing.T) {
	h := NewHasher(2)
	ctx := context.Background()
	enc, err := h.Hash(ctx, "correct horse battery")
	if err != nil {
		t.Fatal(err)
	}
	if ok, _ := h.Verify(ctx, enc, "correct horse battery"); !ok {
		t.Fatal("valid password rejected")
	}
	if ok, _ := h.Verify(ctx, enc, "wrong password"); ok {
		t.Fatal("invalid password accepted")
	}
	if ok, _ := h.Verify(ctx, "", "anything"); ok {
		t.Fatal("dummy verification must fail")
	}
	if ok, _ := h.Verify(ctx, "garbage", "anything"); ok {
		t.Fatal("malformed hash must fail")
	}
}

func TestBusyWhenAllSlotsTaken(t *testing.T) {
	h := NewHasher(1)
	if err := h.acquire(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer h.release()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := h.Hash(ctx, "x"); !errors.Is(err, ErrBusy) {
		t.Fatalf("got %v, want ErrBusy", err)
	}
	if _, err := h.Verify(ctx, "", "x"); !errors.Is(err, ErrBusy) {
		t.Fatalf("verify got %v, want ErrBusy", err)
	}
}

// A burst of requests never runs more hashes at once than the configured cap.
func TestConcurrencyCapUnderBurst(t *testing.T) {
	const limit = 2
	h := NewHasher(limit)
	var inFlight, peak int64
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := h.acquire(context.Background()); err != nil {
				t.Error(err)
				return
			}
			n := atomic.AddInt64(&inFlight, 1)
			for {
				p := atomic.LoadInt64(&peak)
				if n <= p || atomic.CompareAndSwapInt64(&peak, p, n) {
					break
				}
			}
			_, _ = hash("burst")
			atomic.AddInt64(&inFlight, -1)
			h.release()
		}()
	}
	wg.Wait()
	if peak > limit {
		t.Fatalf("peak concurrent hashes %d exceeded cap %d", peak, limit)
	}
}
