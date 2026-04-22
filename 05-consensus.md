# Consensus

## Difficulty: ★★★★★

> Maelstrom Chapter: 6 (Raft)

## What This Chapter Builds

A linearizable key-value store using the Raft consensus protocol. Progresses through: single-node state machine → leader election with terms and votes → log replication via AppendEntries → commit index advancement → state machine application from committed log entries. Also covers proxying requests to the leader and leader stepdown on timeout.

---

## Concepts & Theory

### 1. The Consensus Problem

**What it is.** Getting a group of nodes to agree on a single value, even if some nodes crash or messages are lost. Once a value is decided, it cannot be changed.

**Theory.** Consensus is formally defined by three properties:
- **Validity:** The decided value was proposed by some node (no values from nowhere)
- **Agreement:** All correct nodes decide the same value
- **Termination:** All correct nodes eventually decide (liveness)

The FLP impossibility result (Fischer, Lynch, Paterson, 1985) proves that *deterministic* consensus is impossible in an asynchronous system where even one node can crash. Every practical consensus protocol (Paxos, Raft, Viewstamped Replication) works around this by using timeouts — technically violating the pure asynchronous model, but working in practice.

**Why it matters.** Consensus is the foundation of all strongly consistent distributed systems. If you can solve consensus, you can build:
- Linearizable key-value stores
- Distributed locks
- Atomic broadcast (total order broadcast)
- Replicated state machines

### 2. Replicated State Machines

**What it is.** Multiple nodes maintain identical copies of a state machine. They agree on the *sequence of inputs* (via consensus on a log), and since the state machine is deterministic, they all arrive at the same state.

**How Maelstrom uses it.** Each Raft node has a `Map` state machine that supports read, write, and CAS operations. The Raft log contains these operations. When an entry is committed (agreed upon by a majority), every node applies it to its local state machine, producing identical results.

**Theory.** The replicated state machine approach was described by Schneider (1990). The key insight: **consensus on a log of operations is equivalent to consensus on state**. If all nodes apply the same operations in the same order to the same initial state, they arrive at the same final state.

This decouples the consensus mechanism (agreeing on log order) from the application logic (what the state machine does). You can swap in any deterministic state machine — a KV store, a database, a lock service — without changing the consensus protocol.

**Real-world examples.** etcd (Raft + KV store), ZooKeeper (ZAB + hierarchical namespace), CockroachDB (Raft + SQL engine), TiKV (Raft + KV store).

### 3. Raft Overview

**What it is.** A consensus protocol designed for understandability (Ongaro & Ousterhout, 2014). It decomposes consensus into three sub-problems:
1. **Leader election:** Choose one node to coordinate
2. **Log replication:** The leader sends log entries to followers
3. **Safety:** Ensure committed entries are never lost

**Theory.** Raft is equivalent in power to Multi-Paxos but is structured differently. Where Paxos describes consensus on a single value and leaves multi-value consensus as an exercise, Raft directly specifies a replicated log with a stable leader.

Key Raft invariants:
- **Election safety:** At most one leader per term
- **Leader append-only:** A leader never overwrites or deletes log entries
- **Log matching:** If two logs contain an entry with the same index and term, all preceding entries are identical
- **Leader completeness:** If an entry is committed in a given term, it will be present in the logs of all leaders for higher terms
- **State machine safety:** If a node applies a log entry at a given index, no other node applies a different entry at that index

### 4. Terms

**What it is.** A monotonically increasing integer that acts as a *logical clock* for the cluster. Each election increments the term. All messages carry the sender's term. If a node receives a message with a higher term, it immediately steps down to follower.

**How Maelstrom uses it.** The `@term` variable starts at 0. When a node becomes a candidate, it increments its term. When it receives a message with a higher term, it calls `advance_term!` and becomes a follower. Terms are included in every `request_vote` and `append_entries` message.

**Theory.** Terms serve multiple purposes:
- **Stale leader detection:** A leader from term 3 that was partitioned away will discover term 5 exists when the partition heals, and step down immediately.
- **Vote scoping:** Each node votes at most once per term, preventing split-brain.
- **Log consistency:** Entries are tagged with the term they were created in, enabling the log matching property.

Terms are a form of *logical clock* (Lamport, 1978), but simpler than vector clocks. They only need to be monotonic, not track causality between all events.

### 5. Leader Election

**What it is.** When a follower hasn't heard from a leader within its election timeout, it becomes a candidate, increments its term, votes for itself, and requests votes from all other nodes. If it receives votes from a majority, it becomes the leader.

**How Maelstrom uses it.** The election process:
1. Election timeout expires → become candidate
2. Increment term, vote for self, reset election deadline
3. Send `request_vote` to all other nodes
4. Collect responses; if majority grants votes → become leader
5. If another node has a higher term → step down to follower

**Theory.** The election protocol ensures *election safety*: at most one leader per term. This follows from:
- Each node votes at most once per term
- A candidate needs a majority of votes
- Two majorities must overlap in at least one node
- That overlapping node can only have voted for one candidate

**Randomized timeouts** prevent *split votes* (where multiple candidates start elections simultaneously and nobody gets a majority). Each node's election timeout is randomized within a range, so elections are staggered. This is a probabilistic solution — split votes can still happen, but become increasingly unlikely with each round.

**The log completeness check:** A node only grants its vote if the candidate's log is at least as up-to-date as its own (compared by last entry's term, then by length). This ensures that the elected leader has all committed entries — a critical safety property.

### 6. Log Replication (AppendEntries)

**What it is.** The leader sends `append_entries` RPCs to followers containing new log entries. Each message includes the term and index of the entry *before* the new ones, so the follower can verify log consistency. Followers acknowledge successful appends.

**How Maelstrom uses it.** The leader maintains two maps:
- `@next_index[node]`: The next log index to send to that node
- `@match_index[node]`: The highest log index known to be replicated on that node

On each replication round, the leader sends entries from `next_index` onward. If the follower accepts, `next_index` and `match_index` advance. If the follower rejects (log mismatch), `next_index` backs up by one and retries.

**Theory.** The `prev_log_index` and `prev_log_term` fields implement an *inductive consistency check*. If the follower agrees on entry N, and the leader sends entry N+1 with a matching prev_log reference, then entries 1 through N+1 are guaranteed identical. This is the *log matching property*.

When a follower's log diverges (e.g., it accepted entries from a previous leader that was later superseded), the leader backs up `next_index` until it finds a common prefix, then overwrites the divergent suffix. This is safe because uncommitted entries can be overwritten — only committed entries are permanent.

**AppendEntries also serves as a heartbeat.** Even when there are no new entries, the leader periodically sends empty AppendEntries to prevent followers from starting elections.

### 7. Commit Index and Majority Acknowledgement

**What it is.** An entry is *committed* when it has been replicated to a majority of nodes. The leader tracks this via the `commit_index`, which advances to the median of all `match_index` values (including the leader's own log size).

**How Maelstrom uses it.** After each successful `append_entries` response, the leader calls `advance_commit_index!`, which computes the median match index. If this is higher than the current commit index *and* the entry at that index is from the current term, the commit index advances.

**Theory.** The majority requirement ensures *durability*: any future leader must have received votes from a majority, and that majority overlaps with the majority that acknowledged the committed entry. Therefore, the new leader must have the committed entry in its log.

The "current term" restriction is subtle but critical. A leader cannot commit entries from *previous* terms by counting replicas alone — it can only commit them indirectly, by committing a new entry from its own term that comes after them. This prevents a specific scenario where a leader could commit an entry, get replaced, and the new leader could overwrite it. (See Section 5.4.2 of the Raft paper for the detailed example.)

### 8. State Machine Application

**What it is.** Once an entry is committed, it can be safely applied to the state machine. The `last_applied` index tracks the most recently applied entry. When `commit_index > last_applied`, the node applies entries sequentially until caught up.

**How Maelstrom uses it.** `advance_state_machine!` loops from `last_applied + 1` to `commit_index`, applying each entry's operation to the `Map` state machine. If the node is the leader, it also sends the response back to the client.

**Theory.** The separation between *committed* and *applied* is important:
- **Committed:** The entry is durable — it will never be lost or overwritten
- **Applied:** The entry's effect is visible in the state machine

A node might have committed entries it hasn't applied yet (e.g., after recovering from a crash). The state machine must apply entries in order, because operations may depend on previous state.

**The critical bug Maelstrom teaches:** If you apply operations to the state machine *before* they're committed (as the initial implementation does), you get linearizability violations. Two leaders in different terms might apply conflicting operations to their local state machines, and clients observe inconsistent results. The fix: only apply operations after they're committed via majority acknowledgement.

### 9. Leader Proxying

**What it is.** Non-leader nodes forward client requests to the current leader, rather than rejecting them. This improves availability — clients can talk to any node.

**How Maelstrom uses it.** Each node tracks `@leader`, set when it receives `append_entries` from a leader. When a non-leader receives a client request, it forwards the request body to `@leader` via RPC and relays the response back.

**Theory.** This is a common pattern in leader-based systems:
- **etcd:** Followers proxy writes to the leader
- **CockroachDB:** Leaseholder nodes handle reads; writes go to the Raft leader
- **ZooKeeper:** Followers forward write requests to the leader

The tradeoff: proxying adds one extra network hop for non-leader requests, but eliminates the need for clients to discover and track the leader themselves.

### 10. Leader Stepdown

**What it is.** A leader that hasn't received acknowledgements from followers within a timeout voluntarily steps down to follower. This prevents a partitioned leader from accepting writes that can never be committed.

**How Maelstrom uses it.** The `@step_down_deadline` is reset whenever the leader receives an `append_entries` response. A periodic task checks if the deadline has passed and calls `become_follower!` if so.

**Theory.** Without stepdown, a partitioned leader would:
1. Accept client writes
2. Append them to its local log
3. Never get majority acknowledgement (partitioned)
4. Never commit them
5. Never respond to clients (they time out)

Meanwhile, the other partition elects a new leader and makes progress. When the partition heals, the old leader discovers the higher term and steps down, but its uncommitted entries are overwritten. Stepdown limits the window during which this can happen.

### 11. Quorums and the Majority Requirement

**What it is.** A *quorum* is the minimum number of nodes that must participate in a decision for it to be valid. In Raft, the quorum is a simple majority: ⌊n/2⌋ + 1 nodes out of n.

**Theory.** The majority requirement is the foundation of Raft's safety. It relies on the *quorum intersection property*: any two majorities of the same set must share at least one member. This means:
- The set of nodes that voted for a leader overlaps with the set that acknowledged a committed entry
- Therefore, the leader must have the committed entry (because the overlapping voter wouldn't have voted for a candidate with a shorter log)

Raft uses uniform majorities, but other systems use different quorum structures:
- **Flexible Paxos:** Read and write quorums can be different sizes, as long as they overlap
- **Weighted quorums:** Nodes have different voting weights (useful for geo-distributed systems)
- **Witness replicas:** Nodes that vote but don't store data (reduces storage cost)

**Fault tolerance:** A cluster of 2f+1 nodes tolerates f failures. A 3-node cluster tolerates 1 failure. A 5-node cluster tolerates 2. Adding nodes improves fault tolerance but increases the number of acknowledgements needed per commit.

---

## Key Tradeoffs

| Dimension | Less ← → More |
|-----------|---------------|
| **Cluster size** | Fewer nodes, less fault tolerance, faster commits ← → More nodes, more fault tolerance, slower commits |
| **Election timeout** | Shorter, faster failover, more spurious elections ← → Longer, fewer spurious elections, slower failover |
| **Heartbeat interval** | Less frequent, less network traffic, slower failure detection ← → More frequent, faster detection, more traffic |
| **Leader vs. leaderless** | Simpler, single bottleneck ← → Higher throughput, more complex |
| **Reads through log vs. local** | Linearizable reads, higher latency ← → Faster reads, potentially stale |

---

## Prerequisites for Next Chapter

Before moving to the Patterns Overview, you should be comfortable with:

- Why consensus is hard (FLP impossibility)
- How Raft decomposes consensus into election + replication + safety
- The role of terms, quorums, and the log matching property
- Why entries must be committed before being applied to the state machine
- The difference between committed and applied
- How leader election ensures at most one leader per term
