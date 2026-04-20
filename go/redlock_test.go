package main

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestLockAcquisition(t *testing.T) {
	ctx := context.Background()
	addrs := []string{
		"localhost:6379",
		"localhost:6380",
		"localhost:6381",
		"localhost:6382",
		"localhost:6383",
	}

	client := NewRedlockClient(addrs, "test:lock", "test:token")
	defer client.Close()

	// 测试获取锁
	token, err := client.Lock(ctx, 5*time.Second)
	assert.NoError(t, err)
	assert.Greater(t, token, int64(0))

	// 测试带 fencing 的写入
	err = client.WriteWithFencing(ctx, token, "test data")
	assert.NoError(t, err)

	// 释放锁
	err = client.Unlock(ctx, token)
	assert.NoError(t, err)
}

func TestConcurrentLocking(t *testing.T) {
	ctx := context.Background()
	addrs := []string{
		"localhost:6379",
		"localhost:6380",
		"localhost:6381",
		"localhost:6382",
		"localhost:6383",
	}

	var wg sync.WaitGroup
	tokens := make([]int64, 10)
	errors := make([]error, 10)

	// 10个并发客户端
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			client := NewRedlockClient(addrs, "test:concurrent:lock", "test:concurrent:token")
			defer client.Close()

			token, err := client.Lock(ctx, 1*time.Second)
			tokens[idx] = token
			errors[idx] = err

			if err == nil {
				time.Sleep(100 * time.Millisecond)
				client.Unlock(ctx, token)
			}
		}(i)
	}

	wg.Wait()

	// 至少有一些客户端应该成功获取锁
	successCount := 0
	for _, err := range errors {
		if err == nil {
			successCount++
		}
	}
	assert.True(t, successCount > 0, "At least one client should acquire lock")
}

func TestStaleTokenRejection(t *testing.T) {
	ctx := context.Background()
	addrs := []string{
		"localhost:6379",
		"localhost:6380",
		"localhost:6381",
		"localhost:6382",
		"localhost:6383",
	}

	client1 := NewRedlockClient(addrs, "test:stale:lock", "test:stale:token")
	defer client1.Close()

	client2 := NewRedlockClient(addrs, "test:stale:lock", "test:stale:token")
	defer client2.Close()

	// Client 1 获取锁
	token1, err := client1.Lock(ctx, 1*time.Second)
	assert.NoError(t, err)
	assert.Greater(t, token1, int64(0))

	// 等待锁过期
	time.Sleep(1500 * time.Millisecond)

	// Client 2 获取新锁
	token2, err := client2.Lock(ctx, 5*time.Second)
	assert.NoError(t, err)
	assert.Greater(t, token2, token1)

	// Client 1 尝试用旧 token 写入 - 应该被拒绝
	err = client1.WriteWithFencing(ctx, token1, "stale data")
	assert.Error(t, err, "Stale token should be rejected")

	// Client 2 用新 token 写入 - 应该成功
	err = client2.WriteWithFencing(ctx, token2, "fresh data")
	assert.NoError(t, err)

	client2.Unlock(ctx, token2)
}

func TestFencingTokenOrdering(t *testing.T) {
	ctx := context.Background()
	addrs := []string{
		"localhost:6379",
		"localhost:6380",
		"localhost:6381",
		"localhost:6382",
		"localhost:6383",
	}

	client := NewRedlockClient(addrs, "test:order:lock", "test:order:token")
	defer client.Close()

	// 获取多次锁，验证 token 递增
	var prevToken int64 = 0
	for i := 0; i < 5; i++ {
		token, err := client.Lock(ctx, 1*time.Second)
		assert.NoError(t, err)
		assert.Greater(t, token, prevToken)
		prevToken = token
		client.Unlock(ctx, token)
		time.Sleep(100 * time.Millisecond)
	}
}

func TestChaosNetworkDelay(t *testing.T) {
	ctx := context.Background()
	addrs := []string{
		"localhost:6379",
		"localhost:6380",
		"localhost:6381",
		"localhost:6382",
		"localhost:6383",
	}

	client1 := NewRedlockClient(addrs, "test:chaos:lock", "test:chaos:token")
	defer client1.Close()

	client2 := NewRedlockClient(addrs, "test:chaos:lock", "test:chaos:token")
	defer client2.Close()

	// 模拟网络延迟场景
	token1, err := client1.Lock(ctx, 500*time.Millisecond)
	assert.NoError(t, err)

	// 模拟延迟超过 TTL
	time.Sleep(600 * time.Millisecond)

	// Client 2 应该能获取锁
	token2, err := client2.Lock(ctx, 5*time.Second)
	assert.NoError(t, err)
	assert.Greater(t, token2, token1)

	// Client 1 的写入应该被拒绝
	err = client1.WriteWithFencing(ctx, token1, "delayed write")
	assert.Error(t, err)

	client2.Unlock(ctx, token2)
}
