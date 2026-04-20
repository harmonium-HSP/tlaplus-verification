package redlock

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/example/redlock-fencing-demo/pkg/fencing"
	"github.com/go-redis/redis/v8"
)

var globalTokenStore = make(map[string]int64)
var globalTokenMu sync.Mutex

type MockInstance struct {
	data      map[string]string
	expiry    map[string]time.Time
	fail      bool
	delay     time.Duration
	mu        sync.Mutex
	enableTTL bool
}

func NewMockInstance() *MockInstance {
	return &MockInstance{
		data:      make(map[string]string),
		expiry:    make(map[string]time.Time),
		enableTTL: true,
	}
}

func (m *MockInstance) Get(ctx context.Context, key string) (string, error) {
	if m.delay > 0 {
		time.Sleep(m.delay)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.fail {
		return "", errors.New("mock failure")
	}

	if m.enableTTL {
		if expiry, ok := m.expiry[key]; ok && time.Now().After(expiry) {
			delete(m.data, key)
			delete(m.expiry, key)
			return "", redis.Nil
		}
	}

	val, ok := m.data[key]
	if !ok {
		return "", redis.Nil
	}
	return val, nil
}

func (m *MockInstance) SetNX(ctx context.Context, key string, value interface{}, ttl time.Duration) (bool, error) {
	if m.delay > 0 {
		time.Sleep(m.delay)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.fail {
		return false, errors.New("mock failure")
	}

	if m.enableTTL {
		if expiry, ok := m.expiry[key]; ok && time.Now().After(expiry) {
			delete(m.data, key)
			delete(m.expiry, key)
		}
	}

	_, ok := m.data[key]
	if ok {
		return false, nil
	}
	m.data[key] = fmt.Sprintf("%v", value)
	if m.enableTTL && ttl > 0 {
		m.expiry[key] = time.Now().Add(ttl)
	}
	return true, nil
}

func (m *MockInstance) Eval(ctx context.Context, script string, keys []string, args ...interface{}) (interface{}, error) {
	if m.delay > 0 {
		time.Sleep(m.delay)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.fail {
		return 0, errors.New("mock failure")
	}

	if len(keys) > 0 {
		key := keys[0]

		if m.enableTTL {
			if expiry, ok := m.expiry[key]; ok && time.Now().After(expiry) {
				delete(m.data, key)
				delete(m.expiry, key)
			}
		}

		if strings.Contains(script, "redis.call('get', key)") && strings.Contains(script, "redis.call('set', key") && len(keys) == 1 {
			_, exists := m.data[key]
			if !exists {
				m.data[key] = fmt.Sprintf("%v", args[0])
				if len(args) > 1 {
					if ttl, ok := args[1].(int64); ok && ttl > 0 {
						m.expiry[key] = time.Now().Add(time.Duration(ttl) * time.Millisecond)
					}
				}
				return int64(1), nil
			}
			return int64(0), nil
		}

		if strings.Contains(script, "redis.call('get',") && strings.Contains(script, "redis.call('del',") && len(keys) == 1 {
			current, exists := m.data[key]
			if exists && current == fmt.Sprintf("%v", args[0]) {
				delete(m.data, key)
				delete(m.expiry, key)
				return int64(1), nil
			}
			return int64(0), nil
		}

		if len(keys) >= 2 {
			fenceKey := keys[0]
			dataKey := keys[1]
			if strings.Contains(script, "fenceKey") || strings.Contains(script, "dataKey") {
				currentTokenStr, exists := m.data[fenceKey]
				var currentToken int64 = 0
				if exists {
					fmt.Sscanf(currentTokenStr, "%d", &currentToken)
				}
				newToken := args[0].(fencing.Token)
				if int64(newToken) > currentToken {
					m.data[fenceKey] = fmt.Sprintf("%d", newToken)
					m.data[dataKey] = fmt.Sprintf("%v", args[1])
					return int64(1), nil
				}
				return int64(0), nil
			}
		}

		if strings.Contains(script, "get") && !strings.Contains(script, "set") {
			val, ok := m.data[key]
			if !ok {
				return nil, nil
			}
			return val, nil
		}

		if strings.Contains(script, "set") && !strings.Contains(script, "del") && len(keys) == 1 {
			m.data[key] = fmt.Sprintf("%v", args[0])
			return int64(1), nil
		}

		if strings.Contains(script, "del") && len(keys) == 1 {
			delete(m.data, key)
			delete(m.expiry, key)
			return int64(1), nil
		}
	}
	return int64(1), nil
}

func (m *MockInstance) Del(ctx context.Context, keys ...string) (int64, error) {
	if m.delay > 0 {
		time.Sleep(m.delay)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.fail {
		return 0, errors.New("mock failure")
	}
	count := int64(0)
	for _, key := range keys {
		if _, ok := m.data[key]; ok {
			delete(m.data, key)
			delete(m.expiry, key)
			count++
		}
	}
	return count, nil
}

func (m *MockInstance) Incr(ctx context.Context, key string) (int64, error) {
	if m.delay > 0 {
		time.Sleep(m.delay)
	}
	if m.fail {
		return 0, errors.New("mock failure")
	}
	globalTokenMu.Lock()
	defer globalTokenMu.Unlock()
	globalTokenStore[key]++
	return globalTokenStore[key], nil
}

func resetTokenStore() {
	globalTokenMu.Lock()
	defer globalTokenMu.Unlock()
	globalTokenStore = make(map[string]int64)
}

func TestSingleClientLock(t *testing.T) {
	resetTokenStore()
	mocks := []Instance{NewMockInstance(), NewMockInstance(), NewMockInstance()}
	rl := NewRedlock(mocks, 10*time.Second)

	ctx := context.Background()
	token, err := rl.Lock(ctx, "test-lock")
	if err != nil {
		t.Fatalf("Failed to acquire lock: %v", err)
	}

	err = rl.Unlock(ctx, "test-lock", token)
	if err != nil {
		t.Fatalf("Failed to release lock: %v", err)
	}
}

func TestTwoClientsMutualExclusion(t *testing.T) {
	resetTokenStore()
	mock1, mock2, mock3 := NewMockInstance(), NewMockInstance(), NewMockInstance()
	rl := NewRedlock([]Instance{mock1, mock2, mock3}, 10*time.Second)

	ctx := context.Background()

	token1, err := rl.Lock(ctx, "test-lock")
	if err != nil {
		t.Fatalf("Client 1 failed to acquire lock: %v", err)
	}

	_, err = rl.Lock(ctx, "test-lock")
	if err == nil {
		t.Fatal("Client 2 should not be able to acquire lock")
	}

	err = rl.Unlock(ctx, "test-lock", token1)
	if err != nil {
		t.Fatalf("Failed to release lock: %v", err)
	}

	mock4, mock5, mock6 := NewMockInstance(), NewMockInstance(), NewMockInstance()
	rl2 := NewRedlock([]Instance{mock4, mock5, mock6}, 10*time.Second)

	token2, err := rl2.Lock(ctx, "test-lock")
	if err != nil {
		t.Fatalf("Client 2 failed to acquire lock after release: %v", err)
	}

	if token2 <= token1 {
		t.Errorf("Expected token > %d, got %d", token1, token2)
	}
}

func TestLockExpiry(t *testing.T) {
	resetTokenStore()
	mock1, mock2, mock3 := NewMockInstance(), NewMockInstance(), NewMockInstance()
	rl := NewRedlock([]Instance{mock1, mock2, mock3}, 100*time.Millisecond)

	ctx := context.Background()

	token1, err := rl.Lock(ctx, "test-lock")
	if err != nil {
		t.Fatalf("Failed to acquire lock: %v", err)
	}

	time.Sleep(150 * time.Millisecond)

	token2, err := rl.Lock(ctx, "test-lock")
	if err != nil {
		t.Fatalf("Failed to acquire lock after expiry: %v", err)
	}

	if token2 <= token1 {
		t.Errorf("Expected token > %d, got %d", token1, token2)
	}
}

func TestPartialLockRelease(t *testing.T) {
	resetTokenStore()
	mock1, mock2, mock3 := NewMockInstance(), NewMockInstance(), NewMockInstance()
	rl := NewRedlock([]Instance{mock1, mock2, mock3}, 10*time.Second)

	ctx := context.Background()

	mock1.fail = true
	mock2.fail = true

	_, err := rl.Lock(ctx, "test-lock")
	if err == nil {
		t.Fatal("Should fail when quorum not met")
	}

	if len(mock3.data) > 0 {
		t.Error("Locks should have been released after failed acquisition")
	}
}

func TestFencingWriter(t *testing.T) {
	mock := NewMockInstance()
	writer := fencing.NewWriter(mockFencingStore{inst: mock})

	ctx := context.Background()

	err := writer.Write(ctx, "test-key", "data1", fencing.NewToken(1))
	if err != nil {
		t.Fatalf("First write failed: %v", err)
	}

	err = writer.Write(ctx, "test-key", "data2", fencing.NewToken(2))
	if err != nil {
		t.Fatalf("Second write failed: %v", err)
	}

	err = writer.Write(ctx, "test-key", "data3", fencing.NewToken(1))
	if err == nil {
		t.Fatal("Stale token write should fail")
	}

	data, token, err := writer.Read(ctx, "test-key")
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}

	if data != "data2" {
		t.Errorf("Expected data2, got %s", data)
	}

	if token != fencing.NewToken(2) {
		t.Errorf("Expected token 2, got %d", token)
	}
}

type mockFencingStore struct {
	inst *MockInstance
}

func (s mockFencingStore) Get(ctx context.Context, key string) (string, error) {
	return s.inst.Get(ctx, key)
}

func (s mockFencingStore) Set(ctx context.Context, key string, value string) error {
	s.inst.mu.Lock()
	defer s.inst.mu.Unlock()
	s.inst.data[key] = value
	return nil
}

func (s mockFencingStore) Incr(ctx context.Context, key string) (int64, error) {
	return s.inst.Incr(ctx, key)
}
