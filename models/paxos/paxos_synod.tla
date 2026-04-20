------------------------------ MODULE paxos_synod ------------------------------
EXTENDS Naturals, FiniteSets, Sequences, TLC

CONSTANTS Acceptors, Proposers, Values, MaxBallot

ASSUME Acceptors /= {}
ASSUME Proposers /= {}
ASSUME Values /= {}
ASSUME MaxBallot >= 1

\* 类型定义
Ballot == 1..MaxBallot
Value == Values

\* 状态变量
VARIABLES
    promised,      \* [Acceptor -> Ballot \cup {0}] 每个Acceptor承诺的最高ballot
    accepted,      \* [Acceptor -> (Ballot \times Value) \cup {<<0, NULL>>}] 每个Acceptor接受的ballot和value
    chosen,        \* SUBSET Value 已经被选择的值集合
    lastBallot,    \* [Proposer -> Ballot] 每个Proposer的最后ballot
    active,        \* [Proposer -> BOOLEAN] Proposer是否活跃

\* 辅助函数
MaxBallotValue(acceptors) ==
    IF acceptors = {} THEN <<0, NULL>>
    ELSE LET max_b == MAX {b : <<b, v>> \in {accepted[a] : a \in acceptors} /\ b > 0}
         IN CHOOSE v : <<max_b, v>> \in {accepted[a] : a \in acceptors}

\* 多数派
Quorum == (Cardinality(Acceptors) \div 2) + 1

\* 初始状态
Init ==
    /\ promised = [a \in Acceptors |-> 0]
    /\ accepted = [a \in Acceptors |-> <<0, NULL>>]
    /\ chosen = {}
    /\ lastBallot = [p \in Proposers |-> 0]
    /\ active = [p \in Proposers |-> TRUE]

\* Proposer: 选择一个新的ballot号码
Prepare(p, b) ==
    /\ active[p]
    /\ b > lastBallot[p]
    /\ lastBallot' = [q \in Proposers |-> IF q = p THEN b ELSE lastBallot[q]]
    /\ UNCHANGED <<promised, accepted, chosen, active>>

\* Acceptor: 收到Prepare请求，返回Promise或拒绝
Promise(a, p, b) ==
    /\ b > promised[a]
    /\ promised' = [c \in Acceptors |-> IF c = a THEN b ELSE promised[c]]
    /\ UNCHANGED <<accepted, chosen, lastBallot, active>>

\* Proposer: 收到足够的Promise后，发送Accept请求
AcceptRequest(p, b, v) ==
    /\ lastBallot[p] = b
    /\ LET quorum == {a \in Acceptors : promised[a] = b}
         IN Cardinality(quorum) >= Quorum
    /\ LET responses == {accepted[a] : a \in Acceptors /\ promised[a] = b}
         max_ballot == MAX {bal : <<bal, val>> \in responses /\ bal > 0}
         chosen_val == IF max_ballot > 0 THEN CHOOSE val : <<max_ballot, val>> \in responses ELSE v
    IN /\ accepted' = [c \in Acceptors |->
        IF promised[c] = b THEN <<b, chosen_val>> ELSE accepted[c]]
    /\ UNCHANGED <<promised, chosen, lastBallot, active>>

\* 检查是否有值被选择（多数派接受了相同的ballot和value）
CheckChosen ==
    \E b \in Ballot, v \in Value :
        /\ Cardinality({a \in Acceptors : accepted[a] = <<b, v>>}) >= Quorum
        /\ chosen' = chosen \cup {v}
    /\ UNCHANGED <<promised, accepted, lastBallot, active>>

\* Proposer停止
StopProposer(p) ==
    /\ active[p]
    /\ active' = [q \in Proposers |-> IF q = p THEN FALSE ELSE active[q]]
    /\ UNCHANGED <<promised, accepted, chosen, lastBallot>>

\* 所有可能的动作
Next ==
    \/ (\E p \in Proposers, b \in Ballot : Prepare(p, b))
    \/ (\E a \in Acceptors, p \in Proposers, b \in Ballot : Promise(a, p, b))
    \/ (\E p \in Proposers, b \in Ballot, v \in Value : AcceptRequest(p, b, v))
    \/ CheckChosen
    \/ (\E p \in Proposers : StopProposer(p))

\* 安全性不变量1: 最多只能选择一个值
Safety1 ==
    Cardinality(chosen) <= 1

\* 安全性不变量2: 如果选择了一个值v，那么所有被接受的值都必须是v
Safety2 ==
    \A v \in chosen :
        \A a \in Acceptors :
            LET <<b, val>> == accepted[a]
            IN b > 0 => val = v

\* 综合安全不变量
Safety == Safety1 /\ Safety2

\* 活性属性: 如果一个值被多数派接受，最终会被选择
Liveness ==
    \A b \in Ballot, v \in Value :
        (Cardinality({a \in Acceptors : accepted[a] = <<b, v>>}) >= Quorum)
        => (<> v \in chosen)

\* 规范
Spec == Init /\ [][Next]_<<promised, accepted, chosen, lastBallot, active>>

\* 对称性
SYMMETRY Acceptors
SYMMETRY Proposers
=============================================================================
