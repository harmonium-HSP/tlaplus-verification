package main

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"
)

func main() {
	ctx := context.Background()

	redisAddrs := []string{
		"localhost:6379",
		"localhost:6380",
		"localhost:6381",
		"localhost:6382",
		"localhost:6383",
	}

	fmt.Println("=== Redlock + Fencing Token Demo ===")
	fmt.Println()

	// 演示1: 正常获取锁场景
	fmt.Println("1. Normal Lock Acquisition")
	fmt.Println("---------------------------")
	demoNormalLock(ctx, redisAddrs)

	fmt.Println()

	// 演示2: TLA+ 反例场景 - 两个客户端交错获取锁
	fmt.Println("2. TLA+ Counterexample Scenario")
	fmt.Println("-------------------------------")
	demoCounterexample(ctx, redisAddrs)

	fmt.Println()
	fmt.Println("=== Demo Complete ===")
}

// demoNormalLock 演示正常获取锁场景
func demoNormalLock(ctx context.Context, addrs []string) {
	client := NewRedlockClient(addrs, "demo:normal:lock", "demo:normal:token")
	defer client.Close()

	// 获取锁
	token, err := client.Lock(ctx, 5*time.Second)
	if err != nil {
		log.Printf("Failed to acquire lock: %v", err)
		return
	}
	fmt.Printf("✓ Acquired lock with token: %d\n", token)

	// 执行业务操作
	time.Sleep(1 * time.Second)

	// 带 fencing token 写入
	err = client.WriteWithFencing(ctx, token, "critical business data")
	if err != nil {
		log.Printf("Write failed: %v", err)
		return
	}
	fmt.Println("✓ Write operation successful")

	// 释放锁
	err = client.Unlock(ctx, token)
	if err != nil {
		log.Printf("Failed to release lock: %v", err)
		return
	}
	fmt.Println("✓ Lock released")
}

// demoCounterexample 演示 TLA+ 发现的反例场景
func demoCounterexample(ctx context.Context, addrs []string) {
	var wg sync.WaitGroup
	var mu sync.Mutex
	staleWriteRejected := false

	wg.Add(2)

	// Client 1 - 获取锁后模拟延迟
	go func() {
		defer wg.Done()
		client := NewRedlockClient(addrs, "demo:counter:lock", "demo:counter:token")
		defer client.Close()

		token1, err := client.Lock(ctx, 1*time.Second)
		if err != nil {
			fmt.Println("Client 1: Failed to acquire lock")
			return
		}
		fmt.Printf("Client 1: Acquired lock with token %d\n", token1)

		// 模拟长操作 - 超过锁的 TTL
		time.Sleep(2 * time.Second)

		// 尝试写入 - 应该被拒绝（token 已过期）
		err = client.WriteWithFencing(ctx, token1, "client-1-data")
		if err != nil {
			mu.Lock()
			staleWriteRejected = true
			mu.Unlock()
			fmt.Printf("Client 1: Write REJECTED (stale token) - %v\n", err)
		} else {
			fmt.Println("Client 1: Write successful (should have been rejected!)")
		}

		client.Unlock(ctx, token1)
	}()

	// Client 2 - 在 Client 1 的锁过期后获取新锁
	time.Sleep(500 * time.Millisecond)
	go func() {
		defer wg.Done()
		client := NewRedlockClient(addrs, "demo:counter:lock", "demo:counter:token")
		defer client.Close()

		// 等待 Client 1 的锁过期（1秒TTL后）
		time.Sleep(1000 * time.Millisecond)

		token2, err := client.Lock(ctx, 5*time.Second)
		if err != nil {
			fmt.Println("Client 2: Failed to acquire lock")
			return
		}
		fmt.Printf("Client 2: Acquired lock with token %d\n", token2)

		// 写入应该成功
		err = client.WriteWithFencing(ctx, token2, "client-2-data")
		if err != nil {
			fmt.Printf("Client 2: Write failed - %v\n", err)
		} else {
			fmt.Println("Client 2: Write successful")
		}

		client.Unlock(ctx, token2)
	}()

	wg.Wait()

	if staleWriteRejected {
		fmt.Println("\n✓ SUCCESS: Fencing Token prevented stale write!")
	} else {
		fmt.Println("\n✗ FAILURE: Stale write was NOT prevented!")
	}
}
