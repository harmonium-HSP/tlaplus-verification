package leaselock

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

var (
	ErrLockHeld      = errors.New("lock is held by another owner")
	ErrNotOwner      = errors.New("cannot unlock: not the owner")
	ErrLockExpired   = errors.New("lock has expired")
	ErrLockNotHeld   = errors.New("lock is not held")
	ErrContextCancel = errors.New("context cancelled")
)

type LeaseLock struct {
	mu          sync.Mutex
	id          string
	owner       string
	expireAt    time.Time
	ttl         time.Duration
	waiters     []chan struct{}
	stats       LockStats
	clock       Clock
}

type LockStats struct {
	AcquireCount     int64
	ReleaseCount     int64
	RenewCount       int64
	ExpireCount      int64
	WaitTime         time.Duration
	ContentionCount  int64
}

type Clock interface {
	Now() time.Time
}

type systemClock struct{}

func (s systemClock) Now() time.Time {
	return time.Now()
}

func NewLeaseLock(id string, ttl time.Duration) *LeaseLock {
	return &LeaseLock{
		id:    id,
		ttl:   ttl,
		clock: systemClock{},
	}
}

func NewLeaseLockWithClock(id string, ttl time.Duration, clock Clock) *LeaseLock {
	return &LeaseLock{
		id:    id,
		ttl:   ttl,
		clock: clock,
	}
}

func (l *LeaseLock) Lock(ctx context.Context) error {
	l.mu.Lock()
	
	if l.isLockAvailable() {
		l.acquire()
		l.mu.Unlock()
		return nil
	}
	
	l.stats.ContentionCount++
	
	waiter := make(chan struct{}, 1)
	l.waiters = append(l.waiters, waiter)
	l.mu.Unlock()
	
	select {
	case <-ctx.Done():
		l.mu.Lock()
		l.removeWaiter(waiter)
		l.mu.Unlock()
		return ErrContextCancel
	case <-waiter:
		l.mu.Lock()
		if l.isLockAvailable() {
			l.acquire()
			l.mu.Unlock()
			return nil
		}
		l.mu.Unlock()
		return l.Lock(ctx)
	}
}

func (l *LeaseLock) TryLock() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	
	if !l.isLockAvailable() {
		return false
	}
	
	l.acquire()
	return true
}

func (l *LeaseLock) Unlock() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	
	if l.owner != l.id {
		return ErrNotOwner
	}
	
	if !l.IsHeld() {
		return ErrLockNotHeld
	}
	
	l.release()
	return nil
}

func (l *LeaseLock) Renew() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	
	if l.owner != l.id {
		return ErrNotOwner
	}
	
	if !l.IsHeld() {
		return ErrLockExpired
	}
	
	l.expireAt = l.clock.Now().Add(l.ttl)
	l.stats.RenewCount++
	return nil
}

func (l *LeaseLock) IsHeld() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.isHeldLocked()
}

func (l *LeaseLock) isHeldLocked() bool {
	return l.owner == l.id && !l.clock.Now().After(l.expireAt)
}

func (l *LeaseLock) IsHeldBy(id string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.owner == id && !l.clock.Now().After(l.expireAt)
}

func (l *LeaseLock) GetOwner() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.owner
}

func (l *LeaseLock) GetExpireTime() time.Time {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.expireAt
}

func (l *LeaseLock) GetStats() LockStats {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.stats
}

func (l *LeaseLock) ResetStats() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.stats = LockStats{}
}

func (l *LeaseLock) isLockAvailable() bool {
	return l.owner == "" || l.clock.Now().After(l.expireAt)
}

func (l *LeaseLock) acquire() {
	l.owner = l.id
	l.expireAt = l.clock.Now().Add(l.ttl)
	l.stats.AcquireCount++
}

func (l *LeaseLock) release() {
	l.owner = ""
	l.expireAt = time.Time{}
	l.stats.ReleaseCount++
	
	for _, waiter := range l.waiters {
		select {
		case waiter <- struct{}{}:
		default:
		}
	}
	l.waiters = nil
}

func (l *LeaseLock) removeWaiter(waiter chan struct{}) {
	for i, w := range l.waiters {
		if w == waiter {
			l.waiters = append(l.waiters[:i], l.waiters[i+1:]...)
			return
		}
	}
}

func (l *LeaseLock) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return fmt.Sprintf("LeaseLock{id=%s, owner=%s, expires=%v}", l.id, l.owner, l.expireAt)
}

type LeaseLockManager struct {
	mu    sync.Mutex
	locks map[string]*LeaseLock
	ttl   time.Duration
}

func NewLeaseLockManager(ttl time.Duration) *LeaseLockManager {
	return &LeaseLockManager{
		locks: make(map[string]*LeaseLock),
		ttl:   ttl,
	}
}

func (m *LeaseLockManager) GetLock(id string) *LeaseLock {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if lock, exists := m.locks[id]; exists {
		return lock
	}
	
	lock := NewLeaseLock(id, m.ttl)
	m.locks[id] = lock
	return lock
}

func (m *LeaseLockManager) ReleaseLock(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if lock, exists := m.locks[id]; exists {
		lock.Unlock()
		delete(m.locks, id)
	}
}

func (m *LeaseLockManager) CleanupExpired() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	count := 0
	now := time.Now()
	
	for id, lock := range m.locks {
		if now.After(lock.GetExpireTime()) {
			delete(m.locks, id)
			count++
		}
	}
	
	return count
}

func (m *LeaseLockManager) LockCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.locks)
}

func WithLeaseLock(ctx context.Context, lock *LeaseLock, fn func() error) error {
	if err := lock.Lock(ctx); err != nil {
		return err
	}
	
	defer lock.Unlock()
	return fn()
}

func WithLeaseLockRenew(ctx context.Context, lock *LeaseLock, renewInterval time.Duration, fn func(ctx context.Context) error) error {
	if err := lock.Lock(ctx); err != nil {
		return err
	}
	
	defer lock.Unlock()
	
	renewCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	
	go func() {
		ticker := time.NewTicker(renewInterval)
		defer ticker.Stop()
		
		for {
			select {
			case <-renewCtx.Done():
				return
			case <-ticker.C:
				lock.Renew()
			}
		}
	}()
	
	return fn(renewCtx)
}
