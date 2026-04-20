package leaselock

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestChaosHighContention(t *testing.T) {
	const goroutines = 100
	const iterations = 10
	
	var wg sync.WaitGroup
	acquired := make([]bool, goroutines)
	var mu sync.Mutex
	
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			lock := NewLeaseLock("high-contention", time.Millisecond*50)
			
			for j := 0; j < iterations; j++ {
				ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
				err := lock.Lock(ctx)
				cancel()
				
				if err == nil {
					mu.Lock()
					acquired[idx] = true
					mu.Unlock()
					
					time.Sleep(time.Millisecond * 5)
					lock.Unlock()
				}
			}
		}(i)
	}
	
	wg.Wait()
	
	mu.Lock()
	count := 0
	for _, success := range acquired {
		if success {
			count++
		}
	}
	mu.Unlock()
	
	if count == 0 {
		t.Error("No goroutine acquired the lock")
	}
	
	t.Logf("High contention test: %d/%d goroutines successfully acquired lock", count, goroutines)
}

func TestChaosLockExpiryRace(t *testing.T) {
	const goroutines = 10
	const iterations = 20
	const ttl = time.Millisecond * 50
	
	for iteration := 0; iteration < iterations; iteration++ {
		var wg sync.WaitGroup
		var holder string
		var holderMu sync.Mutex
		
		for i := 0; i < goroutines; i++ {
			wg.Add(1)
			go func(id string) {
				defer wg.Done()
				lock := NewLeaseLock(id, ttl)
				
				ctx, cancel := context.WithTimeout(context.Background(), time.Second)
				err := lock.Lock(ctx)
				cancel()
				
				if err != nil {
					return
				}
				
				holderMu.Lock()
				if holder != "" && holder != id {
					t.Errorf("Iteration %d: Two goroutines hold lock simultaneously: %s and %s", iteration, holder, id)
				}
				holder = id
				holderMu.Unlock()
				
				time.Sleep(time.Millisecond * 10)
				
				holderMu.Lock()
				if holder == id {
					holder = ""
				}
				holderMu.Unlock()
				
				lock.Unlock()
			}(string(rune('A' + i)))
		}
		
		wg.Wait()
	}
}

func TestChaosRenewRace(t *testing.T) {
	const goroutines = 5
	const renewals = 100
	
	now := time.Now()
	clock := &mockClock{now: now}
	lock := NewLeaseLockWithClock("renew-race", time.Second, clock)
	
	err := lock.Lock(context.Background())
	if err != nil {
		t.Fatalf("Failed to acquire lock: %v", err)
	}
	
	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < renewals; j++ {
				lock.Renew()
				time.Sleep(time.Millisecond)
			}
		}()
	}
	
	wg.Wait()
	
	if !lock.IsHeld() {
		t.Error("Lock should still be held after concurrent renewals")
	}
	
	stats := lock.GetStats()
	if stats.RenewCount != int64(goroutines*renewals) {
		t.Errorf("Expected %d renewals, got %d", goroutines*renewals, stats.RenewCount)
	}
	
	lock.Unlock()
}

func TestChaosExpiryAndAcquireRace(t *testing.T) {
	const iterations = 10
	
	for i := 0; i < iterations; i++ {
		now := time.Now()
		clock := &mockClock{now: now}
		lock1 := NewLeaseLockWithClock("expiry-race", time.Millisecond*100, clock)
		lock2 := NewLeaseLockWithClock("expiry-race", time.Millisecond*100, clock)
		
		err := lock1.Lock(context.Background())
		if err != nil {
			t.Fatalf("Failed to acquire lock: %v", err)
		}
		
		var wg sync.WaitGroup
		wg.Add(2)
		
		go func() {
			defer wg.Done()
			for {
				clock.Advance(time.Millisecond * 50)
				time.Sleep(time.Millisecond * 5)
				if !lock1.IsHeld() {
					break
				}
			}
		}()
		
		go func() {
			defer wg.Done()
			for {
				ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond*20)
				err := lock2.Lock(ctx)
				cancel()
				if err == nil {
					time.Sleep(time.Millisecond * 10)
					lock2.Unlock()
					break
				}
				time.Sleep(time.Millisecond * 5)
			}
		}()
		
		wg.Wait()
	}
}

func TestChaosMultipleLocks(t *testing.T) {
	const numLocks = 10
	const goroutinesPerLock = 5
	
	manager := NewLeaseLockManager(time.Millisecond * 50)
	var wg sync.WaitGroup
	
	for lockIdx := 0; lockIdx < numLocks; lockIdx++ {
		for gIdx := 0; gIdx < goroutinesPerLock; gIdx++ {
			wg.Add(1)
			go func(lockName string) {
				defer wg.Done()
				lock := manager.GetLock(lockName)
				
				ctx, cancel := context.WithTimeout(context.Background(), time.Second*2)
				err := lock.Lock(ctx)
				cancel()
				
				if err == nil {
					time.Sleep(time.Millisecond * 10)
					lock.Unlock()
				}
			}(string(rune('A' + lockIdx)))
		}
	}
	
	wg.Wait()
	
	t.Logf("Multiple locks test completed with %d locks", numLocks)
}
