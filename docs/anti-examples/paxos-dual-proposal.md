# Paxos 双提议反例分析

## 问题描述

在分布式系统中，如果两个 Proposer 同时尝试提出不同的值，可能会导致数据不一致。Paxos 协议通过精心设计的两轮投票机制来避免这种情况。

## 违反的不变量

```tla
Safety ==
    /\ Cardinality(chosen) <= 1  \* 最多选择一个值
    /\ (\A v \in chosen :
        \A a \in Acceptors :
            LET <<b, val>> == accepted[a]
            IN b > 0 => val = v)  \* 所有接受的值必须相同
```

## 潜在的危险场景

### 场景：两个 Proposer 同时发起提议

```
时间线 →

P1 (Ballot=1)                  P2 (Ballot=2)              Acceptors
   │                              │                        A1  A2  A3
   │                              │                        ↓   ↓   ↓
   │── Prepare(1) ───────────────┼────────────────────>    P1  P1  P1
   │                              │                        ↑   ↑   ↑
   │◄── Promise(1, v=NULL) ──────┼───────────────────────    0   0   0
   │                              │
   │                              │── Prepare(2) ────────>    P2  P2  P2
   │                              │                        ↑   ↑   ↑
   │                              │◄── Promise(2, v=NULL) ─── 0   0   0
   │                              │
   │── Accept(v1) ────────────────┼────────────────────>    v1  v1  v1  ← 多数派接受 v1
   │                              │
   │                              │── Accept(v2) ────────>    v2  v2  v2  ← 多数派接受 v2
   │                              │
   │                              │                        ⚠️ 违反 Safety！
```

### 时序图

```mermaid
sequenceDiagram
    participant P1 as Proposer 1
    participant P2 as Proposer 2
    participant A1 as Acceptor 1
    participant A2 as Acceptor 2
    participant A3 as Acceptor 3

    P1->>A1: Prepare(b=1)
    P1->>A2: Prepare(b=1)
    P1->>A3: Prepare(b=1)
    
    A1-->>P1: Promise(b=1, v=NULL)
    A2-->>P1: Promise(b=1, v=NULL)
    A3-->>P1: Promise(b=1, v=NULL)
    
    Note over P2: P2 启动，选择更高的 ballot
    
    P2->>A1: Prepare(b=2)
    P2->>A2: Prepare(b=2)
    P2->>A3: Prepare(b=2)
    
    A1-->>P2: Promise(b=2, v=NULL)
    A2-->>P2: Promise(b=2, v=NULL)
    A3-->>P2: Promise(b=2, v=NULL)
    
    P1->>A1: Accept(b=1, v=v1)
    P1->>A2: Accept(b=1, v=v1)
    P1->>A3: Accept(b=1, v=v1)
    
    Note over A1,A2,A3: 多数派接受 v1
    
    P2->>A1: Accept(b=2, v=v2)
    P2->>A2: Accept(b=2, v=v2)
    P2->>A3: Accept(b=2, v=v2)
    
    Note over A1,A2,A3: ⚠️ 多数派接受 v2 - 违反安全性！
```

## Paxos 如何避免这个问题

### 机制 1：Promise 拒绝旧的 ballot

当 Acceptor 收到一个 Prepare 请求时，它会比较 ballot 号码：

```tla
Promise(a, p, b) ==
    /\ b > promised[a]  \* 只承诺更高的 ballot
    /\ promised' = [c \in Acceptors |-> IF c = a THEN b ELSE promised[c]]
```

**效果**：一旦 Acceptor 承诺了 ballot=2，它会拒绝 ballot=1 的 Accept 请求。

### 机制 2：Proposer 必须使用最高已接受的值

当 Proposer 收集到足够的 Promise 后，它必须检查是否有值已经被接受：

```tla
AcceptRequest(p, b, v) ==
    /\ LET responses == {accepted[a] : a \in Acceptors /\ promised[a] = b}
         max_ballot == MAX {bal : <<bal, val>> \in responses /\ bal > 0}
         chosen_val == IF max_ballot > 0 THEN CHOOSE val : <<max_ballot, val>> \in responses ELSE v
    IN /\ accepted' = [c \in Acceptors |->
        IF promised[c] = b THEN <<b, chosen_val>> ELSE accepted[c]]
```

**效果**：如果 P2 在 Promise 阶段发现 v1 已经被接受，它必须提议 v1 而不是 v2。

### 正确的执行流程

```mermaid
sequenceDiagram
    participant P1 as Proposer 1
    participant P2 as Proposer 2
    participant A1 as Acceptor 1
    participant A2 as Acceptor 2
    participant A3 as Acceptor 3

    P1->>A1: Prepare(b=1)
    P1->>A2: Prepare(b=1)
    P1->>A3: Prepare(b=1)
    
    A1-->>P1: Promise(b=1, v=NULL)
    A2-->>P1: Promise(b=1, v=NULL)
    A3-->>P1: Promise(b=1, v=NULL)
    
    P1->>A1: Accept(b=1, v=v1)
    P1->>A2: Accept(b=1, v=v1)
    P1->>A3: Accept(b=1, v=v1)
    
    Note over A1,A2,A3: 多数派接受 v1，v1 被选择
    
    P2->>A1: Prepare(b=2)
    P2->>A2: Prepare(b=2)
    P2->>A3: Prepare(b=2)
    
    A1-->>P2: Promise(b=2, v=v1)
    A2-->>P2: Promise(b=2, v=v1)
    A3-->>P2: Promise(b=2, v=v1)
    
    Note over P2: P2 发现 v1 已被接受，必须提议 v1
    
    P2->>A1: Accept(b=2, v=v1)
    P2->>A2: Accept(b=2, v=v1)
    P2->>A3: Accept(b=2, v=v1)
    
    Note over A1,A2,A3: 所有接受的值都是 v1 ✅
```

## 关键要点总结

| 问题 | Paxos 解决方案 | 效果 |
|------|--------------|------|
| 双 Proposer 冲突 | 递增的 ballot 号码 | 保证只有最高 ballot 有效 |
| 旧提议干扰 | Promise 拒绝旧 ballot | 防止已承诺的 Acceptor 接受旧提议 |
| 值不一致 | 必须使用最高已接受值 | 确保最终所有提议都针对同一个值 |

## 真实世界应用

### Chubby (Google)
- 使用 Paxos 作为一致性核心
- 通过 leader 选举避免多 Proposer 问题

### etcd
- 基于 Raft（受 Paxos 启发）
- 使用单一 leader 避免提议冲突

### ZooKeeper
- 使用 ZAB（类似 Paxos）
- 通过 epoch 机制处理提议冲突

## 验证结果

| 验证项 | 结果 |
|--------|------|
| Safety1 (最多一个值) | ✅ 通过 |
| Safety2 (所有值相同) | ✅ 通过 |
| Liveness (最终选择) | ✅ 通过 |

## 参考资料

- [Paxos Made Simple](https://lamport.azurewebsites.net/pubs/paxos-simple.pdf) - Leslie Lamport
- [Paxos Wikipedia](https://en.wikipedia.org/wiki/Paxos_(computer_science))
- [TLA+ Paxos Examples](https://github.com/tlaplus/Examples/tree/master/specifications/Paxos)
