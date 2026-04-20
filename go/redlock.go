package main

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedlockClient 封装 Redlock + Fencing Token 客户端
type RedlockClient struct {
	clients  []*redis.Client
	quorum   int
	lockKey  string
	tokenKey string
}

// NewRedlockClient 创建新的 Redlock 客户端
func NewRedlockClient(addrs []string, lockKey, tokenKey string) *RedlockClient {
	var clients []*redis.Client
	for _, addr := range addrs {
		clients = append(clients, redis.NewClient(&redis.Options{
			Addr:         addr,
			DialTimeout:  5 * time.Second,
			ReadTimeout:  3 * time.Second,
			WriteTimeout: 3 * time.Second,
		}))
	}
	return &RedlockClient{
		clients:  clients,
		quorum:   (len(clients)/2 + 1),
		lockKey:  lockKey,
		tokenKey: tokenKey,
	}
}

// Lock 尝试获取分布式锁，返回唯一的 Fencing Token
// 对应 TLA+ 中的 TryAcquire 动作
func (r *RedlockClient) Lock(ctx context.Context, ttl time.Duration) (int64, error) {
	// 1. 获取全局最大 token
	maxToken, err := r.getMaxToken(ctx)
	if err != nil {
		return 0, err
	}
	newToken := maxToken + 1

	// 2. 尝试在多数派实例上获取锁
	successCount := 0
	var wg sync.WaitGroup
	var mu sync.Mutex

	for _, client := range r.clients {
		wg.Add(1)
		go func(c *redis.Client) {
			defer wg.Done()
			if r.acquireInstance(ctx, c, newToken, ttl) {
				mu.Lock()
				successCount++
				mu.Unlock()
			}
		}(client)
	}
	wg.Wait()

	// 3. 检查是否获得多数派
	if successCount >= r.quorum {
		// 更新所有实例的 token
		for _, client := range r.clients {
			client.Set(ctx, r.tokenKey, newToken, 0)
		}
		return newToken, nil
	}

	// 4. 获取失败，清理已获取的锁
	r.Unlock(ctx, newToken)
	return 0, errors.New("failed to acquire lock: not enough instances")
}

// WriteWithFencing 写入数据时检查 Fencing Token
// 对应 TLA+ 中的 BusinessWrite 不变量
func (r *RedlockClient) WriteWithFencing(ctx context.Context, token int64, data string) error {
	// 获取当前最大 token
	currentMaxToken, err := r.getMaxToken(ctx)
	if err != nil {
		return err
	}

	// Fencing 检查：只有 token 最大的客户端才能写入
	if token < currentMaxToken {
		return errors.New("stale token rejected: another client holds the lock")
	}

	// 执行写入操作
	println("Write successful with token:", token, "data:", data)
	return nil
}

// Unlock 释放锁
func (r *RedlockClient) Unlock(ctx context.Context, token int64) error {
	return r.unlockAll(ctx, token)
}

// acquireInstance 在单个 Redis 实例上获取锁
func (r *RedlockClient) acquireInstance(ctx context.Context, client *redis.Client, token int64, ttl time.Duration) bool {
	script := `
		local lock = redis.call('GET', KEYS[1])
		if not lock then
			redis.call('SET', KEYS[1], ARGV[1], 'PX', ARGV[2])
			redis.call('SET', KEYS[2], ARGV[1])
			return 1
		end
		return 0
	`
	result, err := client.Eval(ctx, script, []string{r.lockKey, r.tokenKey}, token, int(ttl.Milliseconds())).Result()
	if err != nil {
		return false
	}
	return result == int64(1)
}

// getMaxToken 从所有实例读取当前最大 token
func (r *RedlockClient) getMaxToken(ctx context.Context) (int64, error) {
	maxToken := int64(0)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var errs []error

	for _, client := range r.clients {
		wg.Add(1)
		go func(c *redis.Client) {
			defer wg.Done()
			val, err := c.Get(ctx, r.tokenKey).Int64()
			if err != nil && err != redis.Nil {
				mu.Lock()
				errs = append(errs, err)
				mu.Unlock()
				return
			}
			mu.Lock()
			if val > maxToken {
				maxToken = val
			}
			mu.Unlock()
		}(client)
	}
	wg.Wait()

	if len(errs) > 0 {
		return 0, errs[0]
	}
	return maxToken, nil
}

// unlockAll 释放所有实例上的锁
func (r *RedlockClient) unlockAll(ctx context.Context, token int64) error {
	var wg sync.WaitGroup
	var mu sync.Mutex
	var errs []error

	for _, client := range r.clients {
		wg.Add(1)
		go func(c *redis.Client) {
			defer wg.Done()
			script := `
				local current = redis.call('GET', KEYS[1])
				if current == tostring(ARGV[1]) then
					redis.call('DEL', KEYS[1])
				end
				return 1
			`
			_, err := c.Eval(ctx, script, []string{r.lockKey}, token).Result()
			if err != nil {
				mu.Lock()
				errs = append(errs, err)
				mu.Unlock()
			}
		}(client)
	}
	wg.Wait()

	if len(errs) > 0 {
		return errs[0]
	}
	return nil
}

// Close 关闭所有客户端连接
func (r *RedlockClient) Close() {
	for _, client := range r.clients {
		client.Close()
	}
}
