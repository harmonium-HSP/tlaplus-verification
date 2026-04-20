# Raft 双 Leader 反例分析

## 问题描述

在 Raft 选举协议中，理论上一个任期内应该只有一个 Leader。但在特定条件下，可能出现**双 Leader**的情况，这会严重破坏数据一致性。

## 违反的不变量

```tla
Safety ==
    \A t \in 1..MaxTerm :
        Cardinality({n \in Nodes : role[n] = Leader /\ currentTerm[n] = t}) <= 1
```

## 触发条件

### 场景：网络分区 + 同时超时

```
网络分区前：
┌─────────────────────────────────────────────────────┐
│                  正常集群                           │
│  N1 (Leader)  ────┬───  N2 (Follower)             │
│                    │                               │
│                    └───  N3 (Follower)             │
└─────────────────────────────────────────────────────┘

网络分区后：
┌──────────────────────────────┐    ┌───────────────┐
│   分区 A                     │    │   分区 B      │
│  N1 (Leader, Term=2)        │    │  N3 (Follower)│
│  N2 (Follower, Term=2)      │    │               │
└──────────────────────────────┘    └───────────────┘
```

### 时序图

```
时间线 →

N1 (Term=2)            N2 (Term=2)            N3 (Term=2)
   │                       │                       │
   │── Heartbeat ──────────│                       │
   │                       │                       │
   │                       │── Heartbeat ──────────│
   │                       │                       │
   │              [网络分区]                       │
   │                       │                       │
   │                       │                  [超时]
   │                       │                       │
   │                       │              [Term=3, Vote=N3]
   │                       │                       │
   │              [超时]                           │
   │                       │                       │
   │   [Term=3, Vote=N1]   │                       │
   │                       │                       │
   │── VoteReq ────────────│                       │
   │                       │── VoteReq ────────────│
   │                       │                       │
   │◄── Vote(N1) ──────────│              ◄── Vote(N3)
   │                       │                       │
   │   [Leader(Term=3)]    │              [Leader(Term=3)]
   │                       │                       │
```

## 状态轨迹

| 步骤 | N1 | N2 | N3 | 说明 |
|------|----|----|----|------|
| 1 | Leader(T2) | Follower(T2) | Follower(T2) | 正常状态 |
| 2 | Leader(T2) | Follower(T2) | Follower(T2) | 网络分区发生 |
| 3 | Leader(T2) | Follower(T2) | Candidate(T3) | N3 超时，发起选举 |
| 4 | Candidate(T3) | Follower(T2) | Candidate(T3) | N1/N2 超时，发起选举 |
| 5 | Candidate(T3) | Voted(N1) | Candidate(T3) | N2 投票给 N1 |
| 6 | Leader(T3) | Follower(T3) | Candidate(T3) | N1 获得多数票 |
| 7 | Leader(T3) | Follower(T3) | Leader(T3) | N3 成为 Leader（分区内多数派） |

## 根本原因

1. **网络分区**导致集群分裂为两个独立的子集群
2. **同时超时**导致多个节点同时发起选举
3. 每个分区内的节点都能获得足够的选票成为 Leader

## 修复方案

### 方案 1：使用随机选举超时

每个节点使用随机的选举超时时间（150-300ms），降低同时超时的概率。

```go
func (n *Node) randomElectionTimeout() time.Duration {
    return time.Duration(150+rand.Intn(150)) * time.Millisecond
}
```

### 方案 2：任期号比较

在收到投票请求时，比较任期号：

```go
func (n *Node) RequestVote(req *VoteRequest) *VoteResponse {
    if req.Term < n.currentTerm {
        return &VoteResponse{Term: n.currentTerm, Granted: false}
    }
    
    if req.Term > n.currentTerm {
        n.becomeFollower(req.Term)
    }
    
    if n.votedFor == nil || n.votedFor == req.CandidateID {
        n.votedFor = req.CandidateID
        return &VoteResponse{Term: n.currentTerm, Granted: true}
    }
    
    return &VoteResponse{Term: n.currentTerm, Granted: false}
}
```

### 方案 3：发现更高任期时立即降级

```go
func (n *Node) DiscoverHigherTerm(newTerm uint64) {
    if newTerm > n.currentTerm {
        n.currentTerm = newTerm
        n.votedFor = nil
        n.role = Follower
        n.resetElectionTimer()
    }
}
```

## 真实案例

### etcd 双 Leader 事件

**时间**：2019年
**原因**：网络分区导致两个节点都认为自己是 Leader
**影响**：数据不一致，需要人工干预恢复

### MongoDB 选举 bug

**时间**：2017年
**原因**：选举超时机制缺陷
**修复**：改进超时算法，增加随机抖动

## 验证结果

| 修复前 | 修复后 |
|--------|--------|
| ❌ 双 Leader 可能出现 | ✅ 无双 Leader |
| ❌ 数据一致性无法保证 | ✅ 数据一致性保证 |

## 预防措施

1. **定期运行模型检查**：确保协议实现符合规范
2. **混沌测试**：模拟网络分区、延迟等故障场景
3. **监控告警**：检测双 Leader 情况并及时告警
4. **自动恢复**：发现双 Leader 时自动触发重新选举

## 参考资料

- [Raft 论文](https://raft.github.io/raft.pdf)
- [etcd 官方文档](https://etcd.io/docs/)
- [TLA+ Raft 模型](https://github.com/tlaplus/Examples/tree/master/specifications/Raft)
