------------------------------ MODULE raft_election ------------------------------
EXTENDS Naturals, FiniteSets, Sequences, TLC

CONSTANTS Nodes, MaxTerm, TimeoutRange

ASSUME Nodes /= {}
ASSUME MaxTerm >= 1
ASSUME TimeoutRange /= {}

VARIABLES
    currentTerm,      \* [node -> term] 每个节点的当前任期
    votedFor,         \* [node -> (node \cup {NULL})] 每个节点在当前任期的投票
    role,             \* [node -> {"Follower", "Candidate", "Leader"}] 节点角色
    votesReceived,    \* [node -> (SUBSET Nodes)] 当前任期收到的选票
    electionTimeout,  \* [node -> TimeoutRange] 选举超时时间
    timeoutRemaining, \* [node -> Nat] 剩余超时时间

\* 角色常量
Follower == "Follower"
Candidate == "Candidate"
Leader == "Leader"

\* 初始状态
Init ==
    /\ currentTerm = [n \in Nodes |-> 1]
    /\ votedFor = [n \in Nodes |-> NULL]
    /\ role = [n \in Nodes |-> Follower]
    /\ votesReceived = [n \in Nodes |-> {}]
    /\ electionTimeout = [n \in Nodes |-> CHOOSE t \in TimeoutRange : TRUE]
    /\ timeoutRemaining = electionTimeout

\* 获取节点数量
NumNodes == Cardinality(Nodes)

\* 多数派数量
Quorum == (NumNodes \div 2) + 1

\* 选举超时条件
Timeout(n) == timeoutRemaining[n] = 0

\* 减少超时时间
DecrementTimeout ==
    /\ timeoutRemaining' = [n \in Nodes |->
        IF timeoutRemaining[n] > 0 THEN timeoutRemaining[n] - 1 ELSE 0]
    /\ UNCHANGED <<currentTerm, votedFor, role, votesReceived, electionTimeout>>

\* Follower 转换为 Candidate（选举超时）
StartElection(n) ==
    /\ role[n] = Follower
    /\ Timeout(n)
    /\ currentTerm[n] <= MaxTerm
    /\ currentTerm' = [m \in Nodes |->
        IF m = n THEN currentTerm[n] + 1 ELSE currentTerm[m]]
    /\ votedFor' = [m \in Nodes |->
        IF m = n THEN n ELSE votedFor[m]]
    /\ role' = [m \in Nodes |->
        IF m = n THEN Candidate ELSE role[m]]
    /\ votesReceived' = [m \in Nodes |->
        IF m = n THEN {n} ELSE votesReceived[m]]
    /\ timeoutRemaining' = [m \in Nodes |->
        IF m = n THEN electionTimeout[m] ELSE timeoutRemaining[m]]
    /\ UNCHANGED electionTimeout

\* Candidate 收到投票
ReceiveVote(candidate, voter) ==
    /\ role[candidate] = Candidate
    /\ voter \in Nodes
    /\ voter /= candidate
    /\ votedFor[voter] = NULL
    /\ currentTerm[voter] = currentTerm[candidate]
    /\ votesReceived[candidate]' = votesReceived[candidate] \cup {voter}
    /\ votedFor' = [m \in Nodes |->
        IF m = voter THEN candidate ELSE votedFor[m]]
    /\ UNCHANGED <<currentTerm, role, votesReceived, electionTimeout, timeoutRemaining>>

\* Candidate 赢得选举（成为 Leader）
WinElection(n) ==
    /\ role[n] = Candidate
    /\ Cardinality(votesReceived[n]) >= Quorum
    /\ role' = [m \in Nodes |->
        IF m = n THEN Leader ELSE role[m]]
    /\ votesReceived' = [m \in Nodes |-> {}]
    /\ timeoutRemaining' = electionTimeout
    /\ UNCHANGED <<currentTerm, votedFor, electionTimeout>>

\* Leader 发送心跳（重置 Follower 超时）
SendHeartbeat(leader, follower) ==
    /\ role[leader] = Leader
    /\ follower \in Nodes
    /\ follower /= leader
    /\ role[follower] = Follower
    /\ currentTerm[leader] = currentTerm[follower]
    /\ timeoutRemaining' = [m \in Nodes |->
        IF m = follower THEN electionTimeout[m] ELSE timeoutRemaining[m]]
    /\ UNCHANGED <<currentTerm, votedFor, role, votesReceived, electionTimeout>>

\* 发现更高任期（降级）
DiscoverHigherTerm(node, newTerm) ==
    /\ newTerm > currentTerm[node]
    /\ newTerm <= MaxTerm
    /\ currentTerm' = [m \in Nodes |->
        IF m = node THEN newTerm ELSE currentTerm[m]]
    /\ votedFor' = [m \in Nodes |->
        IF m = node THEN NULL ELSE votedFor[m]]
    /\ role' = [m \in Nodes |->
        IF m = node THEN Follower ELSE role[m]]
    /\ votesReceived' = [m \in Nodes |-> {}]
    /\ timeoutRemaining' = [m \in Nodes |->
        IF m = node THEN electionTimeout[node] ELSE timeoutRemaining[m]]
    /\ UNCHANGED electionTimeout

\* 所有可能的动作
Next ==
    \/ (\E n \in Nodes : StartElection(n))
    \/ (\E c \in Nodes, v \in Nodes : ReceiveVote(c, v))
    \/ (\E n \in Nodes : WinElection(n))
    \/ (\E l \in Nodes, f \in Nodes : SendHeartbeat(l, f))
    \/ (\E n \in Nodes, t \in 1..MaxTerm : DiscoverHigherTerm(n, t))
    \/ DecrementTimeout

\* 安全性不变量：一个任期内最多只有一个 Leader
Safety ==
    \A t \in 1..MaxTerm :
        Cardinality({n \in Nodes : role[n] = Leader /\ currentTerm[n] = t}) <= 1

\* 活性属性：最终会选出 Leader（在有限步内）
Liveness ==
    <> (\E n \in Nodes : role[n] = Leader)

\* 规范
Spec == Init /\ [][Next]_<<currentTerm, votedFor, role, votesReceived, timeoutRemaining>>

\* 对称性
SYMMETRY Nodes
=============================================================================
