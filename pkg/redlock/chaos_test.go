package redlock

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/example/redlock-fencing-demo/pkg/fencing"
)

type ChaosInstance struct {
	*MockInstance
	networkDelay    time.Duration
	networkFailRate float64
	partitioned     bool
	mu              sync.Mutex
}

func NewChaosInstance() *ChaosInstance {
	return &ChaosInstance{
		MockInstance: NewMockInstance(),
	}
}

func (c *ChaosInstance) SetDelay(delay time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.networkDelay = delay
}

func (c *ChaosInstance) SetFailRate(rate float64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.networkFailRate = rate
}

func (c *ChaosInstance) SetPartitioned(partitioned bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.partitioned = partitioned
}

func (c *ChaosInstance) Get(ctx context.Context, key string) (string, error) {
	c.mu.Lock()
	delay := c.networkDelay
	partitioned := c.partitioned
	c.mu.Unlock()

	if partitioned {
		return "", errors.New("network partition")
	}
	if delay > 0 {
		time.Sleep(delay)
	}
	return c.MockInstance.Get(ctx, key)
}

func (c *ChaosInstance) SetNX(ctx context.Context, key string, value interface{}, ttl time.Duration) (bool, error) {
	c.mu.Lock()
	delay := c.networkDelay
	partitioned := c.partitioned
	c.mu.Unlock()

	if partitioned {
		return false, errors.New("network partition")
	}
	if delay > 0 {
		time.Sleep(delay)
	}
	return c.MockInstance.SetNX(ctx, key, value, ttl)
}

func (c *ChaosInstance) Eval(ctx context.Context, script string, keys []string, args ...interface{}) (interface{}, error) {
	c.mu.Lock()
	delay := c.networkDelay
	partitioned := c.partitioned
	c.mu.Unlock()

	if partitioned {
		return 0, errors.New("network partition")
	}
	if delay > 0 {
		time.Sleep(delay)
	}
	return c.MockInstance.Eval(ctx, script, keys, args...)
}

func (c *ChaosInstance) Del(ctx context.Context, keys ...string) (int64, error) {
	c.mu.Lock()
	delay := c.networkDelay
	partitioned := c.partitioned
	c.mu.Unlock()

	if partitioned {
		return 0, errors.New("network partition")
	}
	if delay > 0 {
		time.Sleep(delay)
	}
	return c.MockInstance.Del(ctx, keys...)
}

func (c *ChaosInstance) Incr(ctx context.Context, key string) (int64, error) {
	c.mu.Lock()
	delay := c.networkDelay
	partitioned := c.partitioned
	c.mu.Unlock()

	if partitioned {
		return 0, errors.New("network partition")
	}
	if delay > 0 {
		time.Sleep(delay)
	}
	return c.MockInstance.Incr(ctx, key)
}

func TestChaosConcurrentLock(t *testing.T) {
	instances := make([]Instance, 5)
	chaosInstances := make([]*ChaosInstance, 5)
	for i := 0; i < 5; i++ {
		chaosInstances[i] = NewChaosInstance()
		instances[i] = chaosInstances[i]
	}

	rl := NewRedlock(instances, 5*time.Second)

	const numClients = 10
	tokens := make([]fencing.Token, numClients)
	errs := make([]error, numClients)
	var wg sync.WaitGroup

	for i := 0; i < numClients; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			tokens[id], errs[id] = rl.Lock(context.Background(), "chaos-lock")
			if errs[id] == nil {
				time.Sleep(10 * time.Millisecond)
				rl.Unlock(context.Background(), "chaos-lock", tokens[id])
			}
		}(i)
	}

	wg.Wait()

	validTokens := []fencing.Token{}
	for _, tok := range tokens {
		if tok > 0 {
			validTokens = append(validTokens, tok)
		}
	}

	tokenSet := make(map[fencing.Token]bool)
	for _, tok := range validTokens {
		if tokenSet[tok] {
			t.Errorf("Duplicate token found: %d", tok)
		}
		tokenSet[tok] = true
	}

	sortedTokens := make([]fencing.Token, len(validTokens))
	copy(sortedTokens, validTokens)
	for i := 0; i < len(sortedTokens); i++ {
		for j := i + 1; j < len(sortedTokens); j++ {
			if sortedTokens[j] < sortedTokens[i] {
				sortedTokens[i], sortedTokens[j] = sortedTokens[j], sortedTokens[i]
			}
		}
	}

	for i := 1; i < len(sortedTokens); i++ {
		if sortedTokens[i] <= sortedTokens[i-1] {
			t.Errorf("Tokens not strictly increasing: %d followed by %d", sortedTokens[i-1], sortedTokens[i])
		}
	}

	t.Logf("Valid tokens acquired: %v", validTokens)
}

func TestChaosNetworkPartition(t *testing.T) {
	instances := make([]Instance, 5)
	chaosInstances := make([]*ChaosInstance, 5)
	for i := 0; i < 5; i++ {
		chaosInstances[i] = NewChaosInstance()
		instances[i] = chaosInstances[i]
	}

	rl := NewRedlock(instances, 5*time.Second)

	token, err := rl.Lock(context.Background(), "partition-lock")
	if err != nil {
		t.Fatalf("Failed to acquire initial lock: %v", err)
	}

	for i := 0; i < 3; i++ {
		chaosInstances[i].SetPartitioned(true)
	}

	err = rl.Unlock(context.Background(), "partition-lock", token)
	if err != nil {
		t.Logf("Unlock partially succeeded (expected during partition): %v", err)
	}

	for i := 0; i < 3; i++ {
		chaosInstances[i].SetPartitioned(false)
	}

	for _, inst := range chaosInstances {
		inst.MockInstance.mu.Lock()
		delete(inst.MockInstance.data, "redlock:partition-lock")
		delete(inst.MockInstance.data, "redlock:partition-lock:token")
		delete(inst.MockInstance.expiry, "redlock:partition-lock")
		inst.MockInstance.mu.Unlock()
	}

	token2, err := rl.Lock(context.Background(), "partition-lock")
	if err != nil {
		t.Fatalf("Failed to acquire lock after partition recovery: %v", err)
	}

	if token2 <= token {
		t.Errorf("Expected token > %d, got %d", token, token2)
	}
}

func TestChaosNetworkDelay(t *testing.T) {
	instances := make([]Instance, 5)
	chaosInstances := make([]*ChaosInstance, 5)
	for i := 0; i < 5; i++ {
		chaosInstances[i] = NewChaosInstance()
		instances[i] = chaosInstances[i]
	}

	for i := 0; i < 2; i++ {
		chaosInstances[i].SetDelay(50 * time.Millisecond)
	}

	rl := NewRedlock(instances, 5*time.Second)

	token, err := rl.Lock(context.Background(), "delay-lock")
	if err != nil {
		t.Fatalf("Failed to acquire lock with network delays: %v", err)
	}

	err = rl.Unlock(context.Background(), "delay-lock", token)
	if err != nil {
		t.Fatalf("Failed to release lock: %v", err)
	}

	t.Logf("Successfully acquired and released lock with delays, token: %d", token)
}

func TestChaosLockExpiryRace(t *testing.T) {
	instances := make([]Instance, 5)
	chaosInstances := make([]*ChaosInstance, 5)
	for i := 0; i < 5; i++ {
		chaosInstances[i] = NewChaosInstance()
		instances[i] = chaosInstances[i]
	}

	rl := NewRedlock(instances, 50*time.Millisecond)

	token1, err := rl.Lock(context.Background(), "expiry-race-lock")
	if err != nil {
		t.Fatalf("Client 1 failed to acquire lock: %v", err)
	}

	time.Sleep(75 * time.Millisecond)

	token2, err := rl.Lock(context.Background(), "expiry-race-lock")
	if err != nil {
		t.Fatalf("Client 2 failed to acquire expired lock: %v", err)
	}

	if token2 <= token1 {
		t.Errorf("Expected token > %d, got %d", token1, token2)
	}

	err = rl.Unlock(context.Background(), "expiry-race-lock", token2)
	if err != nil {
		t.Fatalf("Failed to release lock: %v", err)
	}
}
