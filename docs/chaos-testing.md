# 混沌测试指南

## 概述

混沌测试是一种通过主动注入故障来验证系统鲁棒性的方法。本项目使用混沌测试来验证分布式锁在各种故障场景下的行为。

## 混沌测试框架

### 核心组件

#### ChaosInstance

ChaosInstance 是一个包装了 MockInstance 的混沌代理，支持以下故障注入：

| 故障类型 | 方法 | 描述 |
|---------|------|------|
| 网络延迟 | `SetDelay(delay)` | 为所有操作添加固定延迟 |
| 网络分区 | `SetPartitioned(true)` | 模拟网络分区，拒绝所有请求 |
| 故障注入 | `SetFailRate(rate)` | 以指定概率随机失败 |

#### 使用示例

```go
// 创建混沌实例
chaosInstance := NewChaosInstance()

// 注入网络延迟
chaosInstance.SetDelay(50 * time.Millisecond)

// 注入网络分区
chaosInstance.SetPartitioned(true)

// 使用混沌实例创建 Redlock
rl := NewRedlock([]Instance{chaosInstance, ...}, 5*time.Second)
```

## 测试场景

### 场景 1：网络分区

**目标**：验证系统在网络分区时的行为

**设置**：
- 5 个 Redis 实例
- 将 3 个实例设置为分区状态
- 尝试获取锁

**预期结果**：
- 获取锁失败（达不到 quorum）
- 自动释放已获取的部分锁
- 分区恢复后可正常获取锁

**测试代码**：`TestChaosNetworkPartition`

### 场景 2：网络延迟

**目标**：验证系统在网络延迟下的性能和正确性

**设置**：
- 5 个 Redis 实例
- 2 个实例添加 50ms 延迟
- 尝试获取锁

**预期结果**：
- 锁获取成功（达到 quorum）
- 延迟会增加锁获取时间
- 锁的正确性不受影响

**测试代码**：`TestChaosNetworkDelay`

### 场景 3：并发竞争

**目标**：验证大量并发客户端竞争同一锁时的行为

**设置**：
- 5 个 Redis 实例
- 10 个并发客户端
- 同时请求同一锁

**预期结果**：
- 只有一个客户端能获取锁
- 令牌严格递增
- 没有重复令牌

**测试代码**：`TestChaosConcurrentLock`

### 场景 4：锁过期竞争

**目标**：验证锁过期时的竞争场景

**设置**：
- 5 个 Redis 实例
- 短 TTL（50ms）
- 客户端 1 获取锁后等待超过 TTL
- 客户端 2 尝试获取锁

**预期结果**：
- 客户端 1 的锁过期
- 客户端 2 成功获取新锁
- 客户端 2 的令牌大于客户端 1

**测试代码**：`TestChaosLockExpiryRace`

## 运行混沌测试

### 基本命令

```bash
# 运行所有混沌测试
make chaos

# 运行特定混沌测试
go test -v -run TestChaosNetworkPartition ./pkg/...

# 多次运行以增加覆盖率
go test -v -run TestChaos -count=10 ./pkg/...
```

### 带竞态检测

```bash
go test -race -v -run TestChaos ./pkg/...
```

## 测试配置

### 配置文件

在 `configs/chaos-config.yaml` 中配置测试参数：

```yaml
chaos:
  # 网络延迟范围
  minDelay: 10ms
  maxDelay: 100ms
  
  # 故障概率（0-1）
  failRate: 0.1
  
  # 分区实例数量
  partitionCount: 2
  
  # 并发客户端数量
  clientCount: 10
  
  # 测试迭代次数
  iterations: 10
  
  # 锁 TTL
  lockTTL: 5s
```

### 自定义测试

可以通过修改测试代码来创建自定义混沌场景：

```go
func TestCustomChaosScenario(t *testing.T) {
    // 创建实例
    instances := make([]Instance, 5)
    chaosInstances := make([]*ChaosInstance, 5)
    for i := 0; i < 5; i++ {
        chaosInstances[i] = NewChaosInstance()
        instances[i] = chaosInstances[i]
    }
    
    // 自定义故障注入
    chaosInstances[0].SetDelay(100 * time.Millisecond)
    chaosInstances[1].SetFailRate(0.3)
    chaosInstances[2].SetPartitioned(true)
    
    // 创建 Redlock
    rl := NewRedlock(instances, 5*time.Second)
    
    // 执行测试逻辑
    token, err := rl.Lock(context.Background(), "custom-lock")
    // ...
}
```

## 混沌测试最佳实践

### 1. 定义明确的预期结果

每个混沌测试都应该有明确的预期结果，例如：
- 操作应该成功/失败
- 数据应该保持一致
- 系统应该在合理时间内恢复

### 2. 覆盖多种故障组合

单一故障场景可能不足以暴露系统的弱点，应该测试多种故障的组合：
- 网络延迟 + 部分分区
- 高并发 + 锁过期
- 故障注入 + 网络分区

### 3. 多次运行测试

混沌测试的结果可能具有随机性，应该多次运行以提高覆盖率：

```bash
go test -run TestChaos -count=20 ./pkg/...
```

### 4. 结合监控

在运行混沌测试时，应该监控系统的关键指标：
- 锁获取成功率
- 平均延迟
- 错误率

### 5. 逐步增加故障强度

从较小的故障强度开始，逐步增加：
1. 低延迟 → 高延迟
2. 低故障概率 → 高故障概率
3. 部分分区 → 完全分区

## CI/CD 集成

### GitHub Actions 配置

在 `.github/workflows/chaos-test.yml` 中配置：

```yaml
name: Chaos Tests

on:
  push:
    branches: [ main ]
  pull_request:
    branches: [ main ]
  schedule:
    - cron: '0 0 * * *'  # 每天凌晨运行

jobs:
  chaos:
    runs-on: ubuntu-latest
    steps:
    - uses: actions/checkout@v2
    
    - name: Set up Go
      uses: actions/setup-go@v2
      with:
        go-version: 1.21
    
    - name: Run chaos tests
      run: |
        go test -v -race -run TestChaos -count=10 ./pkg/...
```

### 定时运行

混沌测试可以配置为定时运行，以持续验证系统的稳定性：

```yaml
schedule:
  - cron: '0 0 * * *'  # 每天凌晨
  - cron: '0 12 * * *' # 每天中午
```

## 故障注入工具

### 推荐工具

除了内置的混沌测试框架，还可以使用以下工具进行更复杂的混沌测试：

| 工具 | 描述 | 适用场景 |
|------|------|---------|
| Pumba | Docker 网络模拟工具 | 容器化环境 |
| Chaos Mesh | Kubernetes 混沌工程平台 | K8s 集群 |
| Gremlin | 企业级混沌工程平台 | 生产环境 |

### Docker 环境中的混沌测试

使用 Docker Compose 配置测试环境：

```yaml
version: '3.8'
services:
  redis1:
    image: redis:6
    ports:
      - "6379:6379"
  
  redis2:
    image: redis:6
    ports:
      - "6380:6379"
  
  # ... 更多 Redis 实例

  chaos-proxy:
    image: gaiaadm/pumba:0.10.0
    command: "netem --duration 30s delay 50ms re2d:redis1"
```

## 总结

混沌测试是验证分布式系统可靠性的重要手段。通过主动注入故障，可以发现系统在极端条件下的行为，提前暴露潜在问题，从而提高系统的稳定性和可靠性。
