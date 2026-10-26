//go:build integration

package challenge

import (
	"context"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/itsivali/lottery-platform/internal/platform/testvalkey"
)

func TestChallenge_IsUsableOnceAndOnlyWhileFresh(t *testing.T) {
	vk := testvalkey.Start(t).Client(t)
	s := NewStore(vk, time.Second)
	ctx := context.Background()
	c, exp, err := s.Issue(ctx)
	if err != nil || len(c) < 32 || time.Until(exp) > time.Second {
		t.Fatalf("issue: %q %v %v", c, exp, err)
	}
	if ok, err := s.Consume(ctx, c); err != nil || !ok {
		t.Fatalf("first use: %v %v", ok, err)
	}
	if ok, _ := s.Consume(ctx, c); ok {
		t.Fatal("a challenge was used twice")
	}
	if ok, _ := s.Consume(ctx, "never-issued"); ok {
		t.Fatal("an unknown challenge was accepted")
	}
	old, _, _ := s.Issue(ctx)
	// wait (by polling, not sleeping) until the store has forgotten it
	deadline := time.Now().Add(10 * time.Second)
	for {
		n, err := vk.Do(ctx, vk.B().Exists().Key(key(old)).Build()).AsInt64()
		if err != nil {
			t.Fatal(err)
		}
		if n == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the challenge never expired")
		}
		runtime.Gosched()
	}
	if ok, _ := s.Consume(ctx, old); ok {
		t.Fatal("an expired challenge was accepted")
	}
}

func TestChallenge_ConcurrentUsesSucceedOnce(t *testing.T) {
	s := NewStore(testvalkey.Start(t).Client(t), time.Minute)
	ctx := context.Background()
	c, _, _ := s.Issue(ctx)
	var wins atomic.Int64
	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if ok, _ := s.Consume(ctx, c); ok {
				wins.Add(1)
			}
		}()
	}
	wg.Wait()
	if wins.Load() != 1 {
		t.Fatalf("%d uses succeeded, want 1", wins.Load())
	}
}
