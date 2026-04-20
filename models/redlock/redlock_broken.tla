------------------------------- MODULE redlock_fixed -------------------------------
EXTENDS Naturals, Sequences, TLC

CONSTANTS
    Clients,       \* 客户端集合
    Instances,     \* Redis 实例集合
    LockDuration,  \* 锁有效期
    MaxTokens      \* 最大令牌数

ASSUME
    /\ Clients = {"C1", "C2"}
    /\ Instances = {"I1", "I2", "I3", "I4", "I5"}
    /\ LockDuration = 3
    /\ MaxTokens = 10

VARIABLES
    lock,          \* lock[i] = client 表示实例 i 的锁被 client 持有
    token,         \* token[i] = t 表示实例 i 存储的令牌值
    pc,            \* 每个客户端的程序计数器
    client_token,  \* 每个客户端持有的令牌
    global_token_counter,  \* 全局令牌计数器

\* 程序计数器状态
NCS == "ncs"              \* 非临界区
TRYING == "trying"        \* 尝试获取锁
LOCKED == "locked"        \* 持有锁
WRITING == "writing"      \* 写入数据
RELEASING == "releasing"  \* 释放锁

\* 初始状态
Init ==
    /\ lock = [i \in Instances |-> "None"]
    /\ token = [i \in Instances |-> 0]
    /\ pc = [c \in Clients |-> NCS]
    /\ client_token = [c \in Clients |-> 0]
    /\ global_token_counter = 0

\* 获取最大令牌值
MaxToken(client) ==
    LET tokens == {token[i] : i \in Instances}
    IN IF tokens = {} THEN 0 ELSE CHOOSE t \in tokens : \A s \in tokens : s <= t

\* 在单个实例上获取锁
AcquireInstance(client, instance, new_token) ==
    /\ lock[instance] = "None"
    /\ lock' = [lock EXCEPT ![instance] = client]
    /\ token' = [token EXCEPT ![instance] = new_token]

\* 获取锁
ClientAcquireLock(client) ==
    /\ pc[client] = TRYING
    /\ LET
           current_max == MaxToken(client)
           new_token == current_max + 1
           success_instances == {i \in Instances : AcquireInstance(client, i, new_token)}
           quorum == (Cardinality(Instances) \div 2) + 1
       IN
           /\ Cardinality(success_instances) >= quorum
           /\ pc' = [pc EXCEPT ![client] = LOCKED]
           /\ client_token' = [client_token EXCEPT ![client] = new_token]
           /\ global_token_counter' = new_token
           /\ \A i \in Instances:
               /\ lock'[i] = IF i \in success_instances THEN client ELSE lock[i]
               /\ token'[i] = IF i \in success_instances THEN new_token ELSE token[i]

\* 业务写入（带 Fencing 检查）
ClientBusinessWrite(client) ==
    /\ pc[client] = LOCKED
    /\ client_token[client] >= MaxToken(client)
    /\ pc' = [pc EXCEPT ![client] = WRITING]

\* 释放锁
ClientReleaseLock(client) ==
    /\ pc[client] = WRITING
    /\ pc' = [pc EXCEPT ![client] = RELEASING]
    /\ lock' = [lock EXCEPT ![i \in Instances] = IF lock[i] = client THEN "None" ELSE lock[i]]

\* 返回非临界区
ClientReturnToNCS(client) ==
    /\ pc[client] = RELEASING
    /\ pc' = [pc EXCEPT ![client] = NCS]

\* 客户端尝试获取锁
ClientTryAcquire(client) ==
    /\ pc[client] = NCS
    /\ pc' = [pc EXCEPT ![client] = TRYING]

\* 锁过期自动释放
LockExpired(instance) ==
    /\ lock[instance] /= "None"
    /\ lock' = [lock EXCEPT ![instance] = "None"]

\* 所有可能的转换
Next ==
    \/ \E client \in Clients : ClientTryAcquire(client)
    \/ \E client \in Clients : ClientAcquireLock(client)
    \/ \E client \in Clients : ClientBusinessWrite(client)
    \/ \E client \in Clients : ClientReleaseLock(client)
    \/ \E client \in Clients : ClientReturnToNCS(client)
    \/ \E instance \in Instances : LockExpired(instance)

\* 安全属性：互斥性
MutualExclusion ==
    \A c1, c2 \in Clients :
        c1 /= c2 =>
            ~(pc[c1] = LOCKED /\ pc[c2] = LOCKED)

\* 安全属性：只有最新令牌持有者可以写入
FencingSafety ==
    \A client \in Clients :
        pc[client] = WRITING =>
            client_token[client] = MaxToken(client)

\* 安全属性：不会有两个客户端持有相同的令牌
UniqueTokens ==
    \A c1, c2 \in Clients :
        c1 /= c2 =>
            client_token[c1] /= client_token[c2]

\* 活性属性：最终能获取锁
LockEventuallyAvailable ==
    \A client \in Clients :
        []<>(pc[client] = NCS => <>(pc[client] = LOCKED))

\* 活性属性：不会饥饿
StarvationFreedom ==
    \A client \in Clients :
        []<>(pc[client] = TRYING => <>(pc[client] = LOCKED))

\* 不变量
Inv ==
    /\ MutualExclusion
    /\ FencingSafety
    /\ UniqueTokens

\* 强公平性条件
SF_Clients ==
    \A client \in Clients :
        SF_pc(client)

=============================================================================
\* Modification History
\* 1. Initial version - basic Redlock model
\* 2. Added Fencing Token mechanism
\* 3. Added global_token_counter for unique token generation
\* 4. Added FencingSafety invariant
\* 5. Added LockExpired for automatic lock release
=============================================================================
