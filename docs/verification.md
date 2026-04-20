# 验证流程说明

## 概述

本项目采用多层验证策略，确保分布式锁实现的正确性和可靠性。

## 验证层次

| 层次 | 验证方法 | 工具 | 目的 |
|------|---------|------|------|
| 形式化验证 | TLA+ 模型检查 | TLC | 证明安全性和活性属性 |
| 单元测试 | Go 单元测试 | Go Test | 验证单个组件的正确性 |
| 集成测试 | 端到端测试 | Go Test | 验证组件协作 |
| 混沌测试 | 故障注入 | Go Test + ChaosInstance | 验证系统鲁棒性 |
| 性能测试 | 基准测试 | Go Benchmark | 验证性能指标 |

## TLA+ 模型验证

### 模型文件结构

```
models/
├── redlock/
│   ├── redlock_optimized.tla    # 主模型（已优化）
│   ├── redlock_optimized.cfg    # 模型配置
│   └── redlock_broken.tla       # 有漏洞版本（用于教学）
├── lease-lock/
│   └── lease_lock_fixed.tla
└── shared/
    └── common.tla               # 公共定义
```

### 验证属性

#### 安全属性（Safety Properties）

1. **MutualExclusion**：在任意时刻，最多只有一个客户端持有锁

```tla
MutualExclusion == 
    \A c1, c2 \in Clients: c1 /= c2 => 
        ~(LockHeld(c1) /\ LockHeld(c2))
```

2. **FencingSafety**：只有持有最新令牌的客户端才能写入

```tla
FencingSafety ==
    \A c \in Clients, r \in Resources:
        WriteAllowed(c, r) => c.token = r.currentToken
```

3. **NoStaleWrite**：过期锁持有者无法写入资源

```tla
NoStaleWrite ==
    \A c \in Clients:
        ~LockHeld(c) => ~WriteAllowed(c, ANY)
```

#### 活性属性（Liveness Properties）

1. **EventuallyAcquire**：请求锁的客户端最终会获得锁

```tla
EventuallyAcquire ==
    \A c \in Clients:
        RequestLock(c) => <>LockHeld(c)
```

2. **NoStarvation**：没有客户端被永久饥饿

```tla
NoStarvation ==
    \A c \in Clients:
        []<>(RequestLock(c) => <>(LockHeld(c)))
```

### 运行验证

```bash
# 运行单个模型验证
java -jar tla2tools.jar redlock_optimized.tla

# 使用配置文件
java -jar tla2tools.jar -config redlock_optimized.cfg redlock_optimized.tla
```

### CI 集成

在 `.github/workflows/verify-models.yml` 中配置自动验证：

```yaml
- name: Run TLA+ verification
  run: |
    cd models/redlock
    java -jar /path/to/tla2tools.jar -config redlock_optimized.cfg redlock_optimized.tla
```

## 单元测试

### 测试覆盖范围

| 测试类别 | 测试方法 | 覆盖场景 |
|---------|---------|---------|
| 基本功能 | `TestSingleClientLock` | 单客户端获取和释放锁 |
| 互斥性 | `TestTwoClientsMutualExclusion` | 双客户端竞争同一锁 |
| 锁过期 | `TestLockExpiry` | 锁过期后自动释放 |
| 部分失败 | `TestPartialLockRelease` | 部分实例失败时的行为 |
| Fencing Token | `TestFencingWriter` | 令牌验证和写入控制 |

### 运行单元测试

```bash
make test
# 或
go test -v ./pkg/...
```

## 混沌测试

### 故障注入场景

| 场景 | 故障类型 | 测试方法 |
|------|---------|---------|
| 网络分区 | 部分实例不可达 | `TestChaosNetworkPartition` |
| 网络延迟 | 增加请求延迟 | `TestChaosNetworkDelay` |
| 并发竞争 | 大量并发请求 | `TestChaosConcurrentLock` |
| 锁过期竞争 | 短 TTL + 延迟 | `TestChaosLockExpiryRace` |

### 运行混沌测试

```bash
make chaos
# 或
go test -v -run TestChaos -count=10 ./pkg/...
```

### 混沌测试配置

在 `configs/chaos-config.yaml` 中配置混沌参数：

```yaml
chaos:
  networkDelay: 50ms
  failRate: 0.1
  partitionCount: 2
  iterations: 10
```

## 性能测试

### 基准测试指标

| 指标 | 描述 | 单位 |
|------|------|------|
| 锁获取延迟 | 单次锁获取的平均时间 | 微秒 |
| 吞吐量 | 每秒获取的锁数量 | ops/s |
| 内存分配 | 每次操作的内存分配量 | bytes/op |

### 运行基准测试

```bash
make benchmark
# 或
go test -bench=. -benchmem ./pkg/redlock/...
```

### 性能基线

性能基线存储在 `configs/model-baseline.json`：

```json
{
  "redlock_optimized": {
    "states": 14850,
    "distinctStates": 11230,
    "runtimeSeconds": 1.2,
    "date": "2024-04-18"
  }
}
```

### 性能回归检测

在 `.github/workflows/performance.yml` 中配置回归检测：

```yaml
- name: Check performance regression
  run: scripts/check-performance.sh
```

脚本会对比当前性能与基线，超过阈值时告警。

## 验证流程总结

```
PR 提交
    │
    ▼
┌──────────────────────────────┐
│  GitHub Actions 触发         │
└──────────────────────────────┘
    │
    ├──► TLA+ 模型验证
    │       │
    │       ▼
    │   验证安全属性
    │   验证活性属性
    │
    ├──► 单元测试
    │       │
    │       ▼
    │   基本功能测试
    │   边界情况测试
    │
    ├──► 混沌测试
    │       │
    │       ▼
    │   故障注入测试
    │   并发竞争测试
    │
    ├──► 性能回归检测
    │       │
    │       ▼
    │   对比性能基线
    │   超过阈值告警
    │
    └──► Slack/Confluence 通知
            │
            ▼
        验证通过/失败通知
```

## 验证失败处理

### TLA+ 验证失败

1. 检查反例输出
2. 分析导致失败的执行路径
3. 修复代码或模型
4. 重新运行验证

### 单元测试失败

1. 检查失败的测试用例
2. 分析失败原因
3. 修复代码
4. 重新运行测试

### 混沌测试失败

1. 检查故障场景
2. 分析系统在故障下的行为
3. 增强容错机制
4. 重新运行测试

### 性能回归

1. 分析性能下降原因
2. 优化代码或配置
3. 更新性能基线（如果是预期的变化）
4. 重新运行基准测试
