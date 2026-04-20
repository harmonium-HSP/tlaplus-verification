----------------------------- MODULE redlock_optimized -----------------------------
EXTENDS Naturals, TLC, FiniteSets

CONSTANTS
  Clients, LockDuration, MaxSteps

VARIABLES
  locked_by,    \* [1..5 -> Clients \cup {"None"}]
  lock_token,   \* [1..5 -> 0..MaxSteps]
  lock_expire,  \* [1..5 -> 0..MaxSteps + LockDuration]
  step,         \* 时间步
  pc            \* [Clients -> {"Idle", "Acquiring", "Acquired", "Releasing"}]

Majority == 3

HasMajority(client) ==
  Cardinality({i \in 1..5 : locked_by[i] = client}) >= Majority

Safety ==
  \A c1, c2 \in Clients:
    (c1 /= c2) => ~(HasMajority(c1) /\ HasMajority(c2))

TypeOK ==
  /\ locked_by \in [1..5 -> Clients \cup {"None"}]
  /\ lock_token \in [1..5 -> 0..MaxSteps]
  /\ lock_expire \in [1..5 -> 0..MaxSteps + LockDuration]
  /\ pc \in [Clients -> {"Idle", "Acquiring", "Acquired", "Releasing"}]

Init ==
  /\ locked_by = [i \in 1..5 |-> "None"]
  /\ lock_token = [i \in 1..5 |-> 0]
  /\ lock_expire = [i \in 1..5 |-> 0]
  /\ step = 0
  /\ pc = [c \in Clients |-> "Idle"]

AcquireLock(client, instance) ==
  /\ locked_by[instance] = "None" \/ lock_expire[instance] <= step
  /\ locked_by' = [locked_by EXCEPT ![instance] = client]
  /\ lock_token' = [lock_token EXCEPT ![instance] = step + 1]
  /\ lock_expire' = [lock_expire EXCEPT ![instance] = step + LockDuration]

ReleaseLock(client, instance) ==
  /\ locked_by[instance] = client
  /\ locked_by' = [locked_by EXCEPT ![instance] = "None"]

Next ==
  \/ \E client \in Clients:
       /\ pc[client] = "Idle"
       /\ pc' = [pc EXCEPT ![client] = "Acquiring"]
       /\ UNCHANGED <<locked_by, lock_token, lock_expire, step>>
  
  \/ \E client \in Clients, i \in 1..5:
       /\ pc[client] = "Acquiring"
       /\ locked_by[i] = "None" \/ lock_expire[i] <= step
       /\ locked_by' = [locked_by EXCEPT ![i] = client]
       /\ lock_token' = [lock_token EXCEPT ![i] = step + 1]
       /\ lock_expire' = [lock_expire EXCEPT ![i] = step + LockDuration]
       /\ UNCHANGED <<pc, step>>
  
  \/ \E client \in Clients:
       /\ pc[client] = "Acquiring"
       /\ ~(\E i \in 1..5: locked_by[i] = "None" \/ lock_expire[i] <= step)
       /\ IF HasMajority(client)
          THEN pc' = [pc EXCEPT ![client] = "Acquired"]
          ELSE pc' = [pc EXCEPT ![client] = "Releasing"]
       /\ UNCHANGED <<locked_by, lock_token, lock_expire, step>>
  
  \/ \E client \in Clients:
       /\ pc[client] = "Acquired"
       /\ pc' = [pc EXCEPT ![client] = "Releasing"]
       /\ UNCHANGED <<locked_by, lock_token, lock_expire, step>>
  
  \/ \E client \in Clients, i \in 1..5:
       /\ pc[client] = "Releasing"
       /\ locked_by[i] = client
       /\ locked_by' = [locked_by EXCEPT ![i] = "None"]
       /\ UNCHANGED <<lock_token, lock_expire, pc, step>>
  
  \/ \E client \in Clients:
       /\ pc[client] = "Releasing"
       /\ ~(\E i \in 1..5: locked_by[i] = client)
       /\ pc' = [pc EXCEPT ![client] = "Idle"]
       /\ UNCHANGED <<locked_by, lock_token, lock_expire, step>>
  
  \/ /\ step < MaxSteps
     /\ step' = step + 1
     /\ UNCHANGED <<locked_by, lock_token, lock_expire, pc>>

Spec == Init /\ [][Next]_<<locked_by, lock_token, lock_expire, step, pc>>

=============================================================================
