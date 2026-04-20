package leaselock

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type mockClock struct {
	now time.Time
}

func (m *mockClock) Now() time.Time {
	return m.now
}

func (m *mockClock) Advance(d time.Duration) {
	m.now = m.now.Add(d)
}

func TestBasicLock(t *testing.T) {
	lock := NewLeaseLock("test", time.Second)

	err := lock.Lock(context.Background())
	if err != nil {
		t.Fatalf("Failed to acquire lock: %v", err)
	}

	if !lock.IsHeld() {
		t.Error("Lock should be held")
	}

	err = lock.Unlock()
	if err != nil {
		t.Fatalf("Failed to release lock: %v", err)
	}

	if lock.IsHeld() {
		t.Error("Lock should not be held after release")
	}
}

func TestMutualExclusion(t *testing.T) {
	lock1 := NewLeaseLock("lock1", time.Second)
	lock2 := NewLeaseLock("lock1", time.Second)

	err := lock1.Lock(context.Background())
	if err != nil {
		t.Fatalf("First lock should acquire: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err = lock2.Lock(ctx)
	if err != ErrContextCancel {
		t.Errorf("Second lock should timeout, got: %v", err)
	}

	err = lock1.Unlock()
	if err != nil {
		t.Fatalf("Failed to unlock: %v", err)
	}

	err = lock2.Lock(context.Background())
	if err != nil {
		t.Errorf("Second lock should acquire after first releases: %v", err)
	}
}

func TestLockExpiry(t *testing.T) {
	now := time.Now()
	clock := &mockClock{now: now}
	lock := NewLeaseLockWithClock("test", 100*time.Millisecond, clock)

	err := lock.Lock(context.Background())
	if err != nil {
		t.Fatalf("Failed to acquire lock: %v", err)
	}

	if !lock.IsHeld() {
		t.Error("Lock should be held")
	}

	clock.Advance(150 * time.Millisecond)

	if lock.IsHeld() {
		t.Error("Lock should not be held after expiry")
	}

	err = lock.Unlock()
	if err != ErrLockNotHeld {
		t.Errorf("Expected ErrLockNotHeld, got: %v", err)
	}
}

func TestTryLock(t *testing.T) {
	lock1 := NewLeaseLock("test", time.Second)
	lock2 := NewLeaseLock("test", time.Second)

	success := lock1.TryLock()
	if !success {
		t.Error("First TryLock should succeed")
	}

	success = lock2.TryLock()
	if success {
		t.Error("Second TryLock should fail")
	}

	err := lock1.Unlock()
	if err != nil {
		t.Fatalf("Failed to unlock: %v", err)
	}

	success = lock2.TryLock()
	if !success {
		t.Error("TryLock should succeed after release")
	}
}

func TestRenew(t *testing.T) {
	now := time.Now()
	clock := &mockClock{now: now}
	lock := NewLeaseLockWithClock("test", 100*time.Millisecond, clock)

	err := lock.Lock(context.Background())
	if err != nil {
		t.Fatalf("Failed to acquire lock: %v", err)
	}

	clock.Advance(50 * time.Millisecond)

	err = lock.Renew()
	if err != nil {
		t.Fatalf("Failed to renew lock: %v", err)
	}

	clock.Advance(80 * time.Millisecond)

	if !lock.IsHeld() {
		t.Error("Lock should still be held after renew")
	}

	clock.Advance(50 * time.Millisecond)

	if lock.IsHeld() {
		t.Error("Lock should expire after TTL from renew")
	}
}

func TestContextCancellation(t *testing.T) {
	lock1 := NewLeaseLock("test", time.Second)
	lock2 := NewLeaseLock("test", time.Second)

	err := lock1.Lock(context.Background())
	if err != nil {
		t.Fatalf("Failed to acquire lock: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err = lock2.Lock(ctx)
	if err != ErrContextCancel {
		t.Errorf("Expected ErrContextCancel, got: %v", err)
	}
}

func TestStats(t *testing.T) {
	lock := NewLeaseLock("test", time.Second)

	for i := 0; i < 5; i++ {
		err := lock.Lock(context.Background())
		if err != nil {
			t.Fatalf("Failed to acquire lock: %v", err)
		}

		err = lock.Renew()
		if err != nil {
			t.Fatalf("Failed to renew lock: %v", err)
		}

		err = lock.Unlock()
		if err != nil {
			t.Fatalf("Failed to unlock: %v", err)
		}
	}

	stats := lock.GetStats()
	if stats.AcquireCount != 5 {
		t.Errorf("Expected AcquireCount=5, got %d", stats.AcquireCount)
	}
	if stats.ReleaseCount != 5 {
		t.Errorf("Expected ReleaseCount=5, got %d", stats.ReleaseCount)
	}
	if stats.RenewCount != 5 {
		t.Errorf("Expected RenewCount=5, got %d", stats.RenewCount)
	}
}

func TestManager(t *testing.T) {
	manager := NewLeaseLockManager(time.Second)

	lock1 := manager.GetLock("resource1")
	lock2 := manager.GetLock("resource1")

	if lock1 != lock2 {
		t.Error("Same resource should return same lock instance")
	}

	lock3 := manager.GetLock("resource2")
	if lock1 == lock3 {
		t.Error("Different resources should return different lock instances")
	}

	err := lock1.Lock(context.Background())
	if err != nil {
		t.Fatalf("Failed to acquire lock: %v", err)
	}

	count := manager.LockCount()
	if count != 2 {
		t.Errorf("Expected 2 locks, got %d", count)
	}

	manager.ReleaseLock("resource1")
	count = manager.LockCount()
	if count != 1 {
		t.Errorf("Expected 1 lock after release, got %d", count)
	}
}

func TestWithLeaseLock(t *testing.T) {
	lock := NewLeaseLock("test", time.Second)
	called := false

	err := WithLeaseLock(context.Background(), lock, func() error {
		if !lock.IsHeld() {
			t.Error("Lock should be held inside callback")
		}
		called = true
		return nil
	})

	if err != nil {
		t.Fatalf("WithLeaseLock failed: %v", err)
	}

	if !called {
		t.Error("Callback should have been called")
	}

	if lock.IsHeld() {
		t.Error("Lock should be released after WithLeaseLock")
	}
}

func TestWithLeaseLockRenew(t *testing.T) {
	now := time.Now()
	clock := &mockClock{now: now}
	lock := NewLeaseLockWithClock("test", 100*time.Millisecond, clock)

	err := WithLeaseLockRenew(context.Background(), lock, 30*time.Millisecond, func(ctx context.Context) error {
		for i := 0; i < 5; i++ {
			clock.Advance(40 * time.Millisecond)
			if !lock.IsHeld() {
				return errors.New("lock expired during callback")
			}
			time.Sleep(10 * time.Millisecond)
		}
		return nil
	})

	if err != nil {
		t.Fatalf("WithLeaseLockRenew failed: %v", err)
	}

	if lock.IsHeld() {
		t.Error("Lock should be released after WithLeaseLockRenew")
	}
}

func TestConcurrentAccess(t *testing.T) {
	lock := NewLeaseLock("test", time.Second)
	var wg sync.WaitGroup
	count := 0

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := lock.Lock(context.Background())
			if err != nil {
				return
			}
			count++
			time.Sleep(10 * time.Millisecond)
			lock.Unlock()
		}()
	}

	wg.Wait()

	if count != 10 {
		t.Errorf("Expected 10 successful acquires, got %d", count)
	}
}
