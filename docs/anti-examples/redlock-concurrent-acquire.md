# Redlock 并发获取反例

## 问题描述

在并发场景下，如果多个客户端同时尝试获取同一锁，可能会出现以下问题：

1. **重复令牌**：多个客户端获得相同的令牌值
2. **锁竞争**：多个客户端同时认为自己持有锁

## 根本原因

### 原实现缺陷

```go
// 错误的实现
func (r *Redlock) Lock(ctx context.Context, lockName string) (Token, error) {
    var wg sync.WaitGroup
    results := make(chan bool, len(r.instances))
    
    for _, inst := range r.instances {
        wg.Add(1)
        go func(inst Instance) {
            defer wg.Done()
            token, _ := inst.Incr(ctx, tokenKey)  // 每个实例独立递增
            success, _ := inst.SetNX(ctx, lockKey, token, ttl)
            results <- success
        }(inst)
    }
    
    wg.Wait()
    // 统计成功数...
}
```

**问题**：每个 Redis 实例独立递增令牌，导致不同实例返回不同的令牌值。

### 时序图

```
Client1                  Client2                  Redis1      Redis2
  │                         │                       │           │
  │─── INCR token ───────────│                       │           │
  │                         │─── INCR token ────────│           │
  │                         │                       │           │
  │◄── token=1 ─────────────│                       │           │
  │                         │◄── token=1 ───────────│           │
  │                         │                       │           │
  │─── SETNX lock=1 ────────│                       │           │
  │                         │─── SETNX lock=1 ────────────────►│
  │                         │                       │           │
  │◄── OK ──────────────────│                       │           │
  │                         │◄── OK ───────────────────────────│
```

两个客户端都获得了令牌 `1`，并且都成功设置了锁！

## 修复方案

### 使用单个实例生成令牌

```go
// 正确的实现
func (r *Redlock) Lock(ctx context.Context, lockName string) (Token, error) {
    tokenKey := r.keyPrefix + lockName + ":token"
    lockKey := r.keyPrefix + lockName
    
    // 使用第一个实例生成令牌
    newToken, err := r.instances[0].Incr(ctx, tokenKey)
    if err != nil {
        return 0, err
    }
    
    // 同步令牌到其他实例
    for i := 1; i < len(r.instances); i++ {
        r.instances[i].Incr(ctx, tokenKey)
    }
    
    // 使用相同的令牌尝试获取锁
    acquired := 0
    for _, inst := range r.instances {
        success, _ := r.tryAcquire(ctx, inst, lockKey, Token(newToken))
        if success {
            acquired++
        }
    }
    
    if acquired >= r.quorum {
        return Token(newToken), nil
    }
    
    r.forceRelease(ctx, lockName)
    return 0, errors.New("failed to acquire lock")
}
```

### 修复后的时序图

```
Client1                  Client2                  Redis1      Redis2
  │                         │                       │           │
  │─── INCR token ──────────────────────────────────►           │
  │                         │                       │           │
  │◄── token=1 ─────────────────────────────────────│           │
  │                         │                       │           │
  │─── SETNX lock=1 ────────│                       │           │
  │                         │                       │           │
  │◄── OK ──────────────────│                       │           │
  │                         │                       │           │
  │                         │─── INCR token ────────►           │
  │                         │                       │           │
  │                         │◄── token=2 ───────────│           │
  │                         │                       │           │
  │                         │─── SETNX lock=2 ──────│           │
  │                         │                       │           │
  │                         │◄── FAIL (lock exists)│           │
```

## 验证结果

### TLA+ 模型验证

| 属性 | 修复前 | 修复后 |
|------|--------|--------|
| MutualExclusion | ❌ 失败 | ✅ 通过 |
| FencingSafety | ❌ 失败 | ✅ 通过 |
| NoStaleWrite | ✅ 通过 | ✅ 通过 |

### 单元测试

```bash
go test -v -run TestTwoClientsMutualExclusion ./pkg/...
```

**修复前**：测试失败（两个客户端都获取到了锁）

**修复后**：测试通过（只有一个客户端能获取锁）

## 关键要点

1. **原子性**：令牌生成必须是原子操作，所有客户端必须使用相同的令牌值
2. **一致性**：令牌值必须在所有 Redis 实例之间保持一致
3. **同步**：令牌生成和锁获取必须在同一个事务中完成（或使用脚本保证原子性）

## 相关资源

- [Redlock 算法官方文档](https://redis.io/docs/manual/patterns/distributed-locks/)
- [Fencing Token 概念](https://martin.kleppmann.com/2016/02/08/how-to-do-distributed-locking.html)
- [TLA+ 模型文件](../../models/redlock/redlock_optimized.tla)
