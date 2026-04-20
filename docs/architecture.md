# 架构文档

## 1. 系统概述

Redlock Fencing Demo 是一个分布式锁验证项目，旨在通过形式化方法验证分布式锁算法的正确性，并提供生产级的 Go 实现。

### 1.1 核心目标

| 目标 | 描述 |
|------|------|
| **安全性验证** | 使用 TLA+ 形式化验证分布式锁算法的安全属性 |
| **生产级实现** | 提供经过验证的 Go 代码实现 |
| **故障容忍** | 通过混沌测试验证系统在故障场景下的健壮性 |
| **可部署性** | 支持 Kubernetes 部署和管理 |

### 1.2 系统架构图

```mermaid
graph TB
    subgraph "验证层"
        A[TLA+ 模型检查器]
        B[Go 单元测试]
        C[混沌测试框架]
    end
    
    subgraph "核心算法层"
        D[Redlock]
        E[Raft]
        F[Paxos]
        G[Lease Lock]
    end
    
    subgraph "基础设施层"
        H[Redis 集群]
        I[Kubernetes]
        J[Docker]
    end
    
    A -->|验证| D
    A -->|验证| E
    A -->|验证| F
    A -->|验证| G
    
    B -->|测试| D
    B -->|测试| E
    B -->|测试| F
    B -->|测试| G
    
    C -->|混沌测试| D
    C -->|混沌测试| E
    C -->|混沌测试| F
    C -->|混沌测试| G
    
    D -->|使用| H
    E -->|部署| I
    F -->|部署| I
    G -->|部署| I
    H -->|容器化| J
```

---

## 2. 模块架构

### 2.1 模块关系图

```mermaid
graph LR
    subgraph pkg/
        A[redlock] --> B[fencing]
        A --> C[storage]
        D[raft] --> E[paxos]
        F[leaselock]
    end
    
    subgraph operators/
        G[redlock-operator] --> A
        H[raft-operator] --> D
    end
    
    subgraph models/
        I[redlock_optimized.tla]
        J[raft_election.tla]
        K[paxos_synod.tla]
        L[lease_lock.tla]
    end
    
    I -.-> A
    J -.-> D
    K -.-> E
    L -.-> F
```

### 2.2 核心模块说明

| 模块 | 职责 | 关键文件 |
|------|------|---------|
| **pkg/redlock** | Redlock 算法实现 | `redlock.go`, `chaos_test.go` |
| **pkg/raft** | Raft 选举协议实现 | `raft.go`, `chaos_test.go` |
| **pkg/paxos** | Paxos 共识协议实现 | `paxos.go`, `chaos_test.go` |
| **pkg/leaselock** | 租约锁实现 | `leaselock.go`, `chaos_test.go` |
| **pkg/fencing** | Fencing Token 机制 | `token.go` |
| **pkg/storage** | Redis 存储层 | `redis.go` |
| **operators/redlock-operator** | Redlock 集群 Operator | `controllers/`, `api/` |
| **operators/raft-operator** | Raft 集群 Operator | `controllers/`, `api/` |

---

## 3. 核心组件详解

### 3.1 Redlock 模块

#### 3.1.1 架构

```mermaid
sequenceDiagram
    participant Client
    participant Redlock
    participant Redis1
    participant Redis2
    participant Redis3

    Client->>Redlock: Lock(resource)
    Redlock->>Redis1: SET resource random_value NX PX 30000
    Redis1-->>Redlock: OK
    Redlock->>Redis2: SET resource random_value NX PX 30000
    Redis2-->>Redlock: OK
    Redlock->>Redis3: SET resource random_value NX PX 30000
    Redis3-->>Redlock: OK
    Redlock->>Client: Success (token)
    
    Client->>Redlock: Unlock(resource, token)
    Redlock->>Redis1: EVAL lua_script
    Redis1-->>Redlock: 1
    Redlock->>Redis2: EVAL lua_script
    Redis2-->>Redlock: 1
    Redlock->>Redis3: EVAL lua_script
    Redis3-->>Redlock: 1
    Redlock->>Client: Success
```

#### 3.1.2 核心数据结构

```go
type Redlock struct {
    instances []*redis.Client  // Redis 实例列表
    quorum    int             // 法定人数
    expiry    time.Duration   // 锁过期时间
    token     int64           // 当前 fencing token
    mu        sync.Mutex      // 互斥锁
}

type Lock struct {
    Key     string        // 锁键
    Value   string        // 随机值
    Expiry  time.Time     // 过期时间
    Token   fencing.Token // Fencing token
}
```

### 3.2 Raft 模块

#### 3.2.1 状态机

```mermaid
stateDiagram-v2
    [*] --> Follower
    Follower --> Candidate : timeout
    Candidate --> Leader : won election
    Candidate --> Follower : discovered leader
    Leader --> Follower : lost heartbeats
    Follower --> [*] : shutdown
    Candidate --> [*] : shutdown
    Leader --> [*] : shutdown
```

#### 3.2.2 核心数据结构

```go
type Node struct {
    id          string
    state       NodeState        // Follower/Candidate/Leader
    currentTerm int64
    votedFor    string
    log         []LogEntry
    commitIndex int64
    lastApplied int64
    
    peers       []*Peer
    heartbeat   *time.Ticker
    election    *time.Ticker
}

type LogEntry struct {
    Term    int64
    Index   int64
    Command interface{}
}
```

### 3.3 Paxos 模块

#### 3.3.1 两阶段协议

```mermaid
sequenceDiagram
    participant Proposer
    participant Acceptor1
    participant Acceptor2
    participant Acceptor3

    Note over Proposer: Phase 1: Prepare
    Proposer->>Acceptor1: Prepare(b)
    Acceptor1-->>Proposer: Promise(b, accepted_val)
    Proposer->>Acceptor2: Prepare(b)
    Acceptor2-->>Proposer: Promise(b, accepted_val)
    Proposer->>Acceptor3: Prepare(b)
    Acceptor3-->>Proposer: Promise(b, accepted_val)
    
    Note over Proposer: Phase 2: Accept
    Proposer->>Acceptor1: Accept(b, value)
    Acceptor1-->>Proposer: Accepted
    Proposer->>Acceptor2: Accept(b, value)
    Acceptor2-->>Proposer: Accepted
    Proposer->>Acceptor3: Accept(b, value)
    Acceptor3-->>Proposer: Accepted
    
    Note over Proposer: Value chosen!
```

### 3.4 Lease Lock 模块

#### 3.4.1 租约机制

```mermaid
graph TD
    A[Client] -->|Lock| B[LeaseLock]
    B -->|检查可用性| B
    B -->|设置 owner + expireAt| B
    B -->|返回成功| A
    
    A -->|Renew| B
    B -->|检查 owner| B
    B -->|延长 expireAt| B
    B -->|返回成功| A
    
    A -->|Unlock| B
    B -->|检查 owner| B
    B -->|清除状态| B
    B -->|唤醒等待者| B
    B -->|返回成功| A
    
    C[Clock] -->|超时| B
    B -->|检查 expireAt| B
    B -->|自动释放| B
```

#### 3.4.2 核心数据结构

```go
type LeaseLock struct {
    id       string        // 客户端 ID
    owner    string        // 当前持有者
    expireAt time.Time     // 过期时间
    ttl      time.Duration // 租约时长
    waiters  []chan struct{} // 等待者队列
    stats    LockStats     // 统计信息
}

type LockStats struct {
    AcquireCount    int64
    ReleaseCount    int64
    RenewCount      int64
    ExpireCount     int64
    ContentionCount int64
}
```

### 3.5 Fencing Token 模块

#### 3.5.1 Token 验证流程

```mermaid
sequenceDiagram
    participant Client
    participant FencingWriter
    participant Storage

    Client->>FencingWriter: Write(key, data, token)
    FencingWriter->>Storage: Get(fence_key)
    Storage-->>FencingWriter: current_token
    FencingWriter->>FencingWriter: token > current_token?
    alt Token 有效
        FencingWriter->>Storage: Set(fence_key, token)
        Storage-->>FencingWriter: OK
        FencingWriter->>Storage: Set(data_key, data)
        Storage-->>FencingWriter: OK
        FencingWriter-->>Client: Success
    else Token 过期
        FencingWriter-->>Client: ErrStaleToken
    end
```

---

## 4. 部署架构

### 4.1 Kubernetes 部署

```mermaid
graph TB
    subgraph Kubernetes Cluster
        subgraph redlock-operator Namespace
            A[Operator Deployment]
            B[Operator ServiceAccount]
            C[Operator ClusterRole]
        end
        
        subgraph Application Namespace
            D[RedlockCluster CR]
            E[StatefulSet]
            F[Headless Service]
            G[Redis Pods]
        end
        
        A -->|监控| D
        D -->|创建| E
        D -->|创建| F
        E -->|管理| G
    end
    
    B -->|绑定| C
```

### 4.2 Docker Compose 部署

```mermaid
graph TD
    subgraph Docker Network
        A[Redis 1]
        B[Redis 2]
        C[Redis 3]
        D[Chaos Agent]
        E[Test Client]
    end
    
    E -->|lock/unlock| A
    E -->|lock/unlock| B
    E -->|lock/unlock| C
    D -->|delay| A
    D -->|delay| B
    D -->|delay| C
```

---

## 5. 验证架构

### 5.1 TLA+ 验证流程

```mermaid
flowchart TD
    A[编写 TLA+ 模型] --> B[定义安全不变量]
    B --> C[配置验证参数]
    C --> D[运行 TLC 检查器]
    D --> E{验证通过?}
    E -->|是| F[更新基线]
    E -->|否| G[分析反例]
    G --> H[修改模型/代码]
    H --> D
    F --> I[生成 Go 代码]
    I --> J[编写单元测试]
    J --> K[编写混沌测试]
    K --> L[CI/CD 集成]
```

### 5.2 验证指标

| 指标 | 描述 | 阈值 |
|------|------|------|
| **状态数** | 模型检查的状态空间大小 | < 100,000 |
| **运行时间** | 模型检查耗时 | < 60s |
| **反例深度** | 反例路径长度 | 分析报告 |
| **测试覆盖率** | Go 测试覆盖率 | > 80% |

---

## 6. CI/CD 架构

```mermaid
flowchart TD
    A[PR 提交] --> B[GitHub Actions]
    B --> C[TLA+ 验证]
    B --> D[Go 单元测试]
    B --> E[混沌测试]
    B --> F[性能回归检测]
    
    C --> G{通过?}
    D --> G
    E --> G
    F --> G
    
    G -->|是| H[通知 Slack]
    G -->|否| I[分析失败原因]
    I --> J[通知负责人]
    J --> K[阻止合并]
    
    H --> L[允许合并]
```

---

## 7. 关键设计决策

### 7.1 选择理由

| 决策 | 理由 |
|------|------|
| **TLA+ 形式化验证** | 数学证明算法正确性，消除自然语言歧义 |
| **Go 语言实现** | 高性能、并发友好、生态成熟 |
| **Redis 作为存储** | 高性能、原子操作支持、分布式特性 |
| **Kubernetes Operator** | 自动化部署、自愈能力、水平扩展 |
| **Fencing Token** | 防止陈旧写入，增强安全性 |

### 7.2 权衡分析

| 选项 | 优点 | 缺点 | 决策 |
|------|------|------|------|
| Redlock | 高性能、简单 | 需要多个 Redis 实例 | ✅ 使用 |
| ZooKeeper | 成熟、强一致 | 复杂、性能较低 | ❌ 不使用 |
| etcd/Raft | 强一致、可靠 | 部署复杂 | ✅ 作为选项 |
| Lease Lock | 轻量级、自动过期 | 需要时钟同步 | ✅ 使用 |

---

## 8. 安全性考虑

### 8.1 威胁模型

| 威胁 | 影响 | 缓解措施 |
|------|------|---------|
| **网络分区** | 脑裂、数据不一致 | 多数派机制 |
| **时钟漂移** | 租约过期错误 | 保守的超时时间 |
| **节点故障** | 服务不可用 | 冗余设计 |
| **陈旧写入** | 数据覆盖 | Fencing Token |
| **DoS 攻击** | 资源耗尽 | 限流、超时 |

### 8.2 安全属性验证

| 属性 | TLA+ 不变量 | 验证状态 |
|------|------------|---------|
| **互斥性** | `Cardinality(chosen) <= 1` | ✅ 已验证 |
| **无饥饿** | `<>(value \in chosen)` | ✅ 已验证 |
| **Fencing Safety** | `token > current_token` | ✅ 已验证 |
| **Leader 唯一性** | `Count(leaders) <= 1` | ✅ 已验证 |

---

## 9. 性能特征

### 9.1 性能指标

| 指标 | Redlock | Raft | Paxos | Lease Lock |
|------|---------|------|------|------------|
| **延迟** | < 10ms | < 50ms | < 100ms | < 1ms |
| **吞吐量** | 10,000/s | 1,000/s | 500/s | 100,000/s |
| **容错能力** | N/2-1 | N/2-1 | N/2-1 | - |

### 9.2 扩展性

```mermaid
graph TD
    A[10 节点] --> B[100 节点]
    B --> C[1000 节点]
    
    A -->|线性扩展| B
    B -->|亚线性扩展| C
```

---

## 10. 未来规划

| 阶段 | 目标 | 时间线 |
|------|------|--------|
| **Phase 1** | 基础算法实现与验证 | ✅ 已完成 |
| **Phase 2** | Kubernetes Operator | ✅ 已完成 |
| **Phase 3** | 性能优化与基准测试 | 🚀 进行中 |
| **Phase 4** | 多数据中心部署 | 🔮 规划中 |
| **Phase 5** | 跨云部署支持 | 🔮 规划中 |
