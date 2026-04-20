----------------------------- MODULE redlock_optimized -----------------------------
EXTENDS Naturals, TLC, FiniteSets

(* --algorithm redlock_optimized

variables
  (* 优化 1：用集合替代 5 个独立实例，但保留完整信息 *)
  locked_by = [i \in 1..5 |-> "None"],
  lock_token = [i \in 1..5 |-> 0],
  lock_expire = [i \in 1..5 |-> 0];

define
  Majority == 3
  
  (* 优化 2：简化 token 有效性检查 *)
  HasMajority(client) ==
    Cardinality({i \in 1..5 : locked_by[i] = client}) >= Majority
  
  (* Safety：两个客户端不能同时拥有多数派 *)
  Safety ==
    \A c1, c2 \in Clients:
      (c1 /= c2) => ~(HasMajority(c1) /\ HasMajority(c2))
  
  (* 类型不变量 *)
  TypeOK ==
    /\ locked_by \in [1..5 -> Clients \union {"None"}]
    /\ lock_token \in [1..5 -> 0..MaxSteps]
    /\ lock_expire \in [1..5 -> 0..MaxSteps + LockDuration]
end define;

constants Clients, LockDuration, MaxSteps

(* 优化 3：用事件步数替代连续时钟 *)
variable step = 0;

process Client \in Clients
variable
  my_name = self,
  acquired = FALSE,
  my_token = 0,
  acquired_set = {};

begin P:
  while step < MaxSteps do
    TryAcquire:
      (* 优化 4：用步数作为 token，减少状态空间 *)
      my_token := step + 1;
      acquired_set := {};
      
      (* 尝试在所有实例上获取锁 *)
      for i \in 1..5 do
        if locked_by[i] = "None" \/ lock_expire[i] <= step then
          locked_by[i] := my_name;
          lock_token[i] := my_token;
          lock_expire[i] := step + LockDuration;
          acquired_set := acquired_set \cup {i};
        end if;
      end for;
      
      (* 检查是否获得多数派 *)
      if Cardinality(acquired_set) >= Majority then
        LockHeld:
          skip;  (* 持有锁 *)
        Release:
          for i \in acquired_set do
            if locked_by[i] = my_name then
              locked_by[i] := "None";
            end if;
          end for;
      else
        (* 获取失败，释放部分已获取的锁 *)
        for i \in acquired_set do
          if locked_by[i] = my_name then
            locked_by[i] := "None";
          end if;
        end for;
      end if;
  end while;
end process;

(* 优化 5：独立的时间步进进程 *)
fair process StepProcess = "step"
begin Step:
  while step < MaxSteps do
    step := step + 1;
  end while;
end process;

end algorithm; *)

=============================================================================
