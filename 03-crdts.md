# CRDTs

## Difficulty: ★★★☆☆

> Maelstrom Chapter: 4 (CRDTs)

## What This Chapter Builds

An eventually consistent, totally available data store using Conflict-free Replicated Data Types. Progresses from a Grow-only Set (G-Set) to a Grow-only Counter (G-Counter) to a Positive-Negative Counter (PN-Counter), demonstrating how CRDTs compose.

---

## Concepts & Theory

### 1. Eventual Consistency

**What it is.** A consistency model where replicas may temporarily disagree, but if updates stop, all replicas will *eventually* converge to the same state. There is no bound on how long convergence takes.

**How Maelstrom uses it.** CRDT nodes replicate their full state to all other nodes every 5 seconds. Between replications, nodes may return different values for reads. Maelstrom verifies that after a quiet period, all nodes agree.

**Theory.** Eventual consistency was formalized in the context of Bayou (Terry et al., 1995) and later popularized by Amazon's Dynamo paper (DeCanio et al., 2007). It's the weakest useful consistency guarantee:

- **Strong eventual consistency (SEC):** A stronger variant where replicas that have received the same set of updates are *guaranteed* to be in the same state — no conflict resolution needed. CRDTs provide SEC.
- **Plain eventual consistency:** Replicas converge, but may need conflict resolution (last-write-wins, application-level merge, etc.).

The key insight: SEC eliminates the need for consensus. If your merge function is deterministic and commutative, replicas converge automatically.

### 2. State-Based CRDTs (CvRDTs)

**What it is.** A data structure where the entire state can be sent to another replica, and the two states can be *merged* into a combined state. The merge function must be:
- **Commutative:** merge(A, B) = merge(B, A)
- **Associative:** merge(A, merge(B, C)) = merge(merge(A, B), C)
- **Idempotent:** merge(A, A) = A

These three properties make the merge function a *join semilattice*, which guarantees convergence regardless of message ordering, duplication, or timing.

**How Maelstrom uses it.** Nodes periodically send their entire CRDT state to all other nodes via `replicate` messages. On receiving a replicate message, the node merges the remote state with its local state. No acknowledgement needed — if a message is lost, the next replication round will include the same data.

**Theory.** State-based CRDTs (Convergent Replicated Data Types, or CvRDTs) were formalized by Shapiro et al. (2011). The alternative is *operation-based CRDTs* (CmRDTs), where you send individual operations instead of full state. Op-based CRDTs require reliable, exactly-once delivery but use less bandwidth. State-based CRDTs tolerate message loss and duplication but send more data.

| Property | State-based (CvRDT) | Operation-based (CmRDT) |
|----------|-------------------|------------------------|
| What's sent | Full state | Individual operations |
| Delivery requirement | None (idempotent merge) | Exactly-once, causal order |
| Bandwidth | Grows with state size | Constant per operation |
| Fault tolerance | Very high | Needs reliable delivery |

Maelstrom uses state-based CRDTs because they're simpler and more fault-tolerant.

### 3. G-Set (Grow-Only Set)

**What it is.** A set that only supports adding elements — never removing them. Merge is set union.

**How Maelstrom uses it.** Each node maintains a local `Set`. On `add`, it inserts the element. On `replicate`, it sends the full set. On receiving a replicate, it takes the union. On `read`, it returns the set contents.

**Theory.** The G-Set is the simplest possible CRDT. Its merge function (set union) is trivially a join semilattice:
- Union is commutative: A ∪ B = B ∪ A
- Union is associative: A ∪ (B ∪ C) = (A ∪ B) ∪ C
- Union is idempotent: A ∪ A = A

The limitation is obvious: you can never remove an element. This is a fundamental constraint — supporting removal requires more sophisticated structures like the OR-Set (Observed-Remove Set) or the 2P-Set (Two-Phase Set, where removed elements go into a "tombstone" set).

**Real-world examples.** Event logs (append-only), blockchain transaction sets, "seen message" sets in gossip protocols.

### 4. G-Counter (Grow-Only Counter)

**What it is.** A counter that only goes up. Internally, it's a map of `{node_id → count}`. Each node only increments its own entry. The effective value is the sum of all entries. Merge takes the max of each node's entry.

**How Maelstrom uses it.** On `add(delta)`, the node increments its own entry: `counters[node_id] += delta`. On merge, for each node ID, it takes `max(local, remote)`. On read, it sums all values.

**Theory.** The G-Counter solves a problem that a naive shared counter can't: concurrent increments on different nodes. If two nodes both increment a shared counter from 5 to 6, a naive merge would pick 6 — losing one increment. The per-node vector avoids this by tracking *who* incremented *how much*.

The merge function (element-wise max) forms a join semilattice because max is commutative, associative, and idempotent.

This is a *vector clock*-like structure. Vector clocks track causal ordering; G-Counters track cumulative increments. Both use per-node entries merged via element-wise max.

### 5. PN-Counter (Positive-Negative Counter)

**What it is.** A counter that supports both increments and decrements. Internally, it's *two* G-Counters: one for positive deltas (`inc`) and one for negative deltas (`dec`). The effective value is `inc.read() - dec.read()`.

**How Maelstrom uses it.** On `add(delta)`: if delta ≥ 0, add to `inc`; if delta < 0, add `|delta|` to `dec`. Merge merges both G-Counters independently. Read returns the difference.

**Theory.** The PN-Counter demonstrates a critical CRDT property: **composition**. If you have two CRDTs and combine them with a function that preserves the semilattice properties, the result is also a CRDT.

Why can't a single G-Counter handle decrements? Because the merge function takes the *max* of each node's entry. If node n1 has counter value 5 and decrements to 3, then merges with a replica that still has 5, the max is 5 — the decrement is lost. Separating increments and decrements into independent monotonically-increasing counters avoids this.

**Real-world examples.** Like/dislike counters, inventory levels (add stock / sell stock), bank account balances in eventually consistent systems.

### 6. CRDT Composition

**What it is.** Building complex CRDTs by combining simpler ones. If each component is a CRDT with a valid merge function, and the composition preserves the semilattice properties, the composite is also a CRDT.

**How Maelstrom uses it.** The PN-Counter is composed of two G-Counters. Maelstrom's code abstracts the CRDT interface (`read`, `merge`, `add`, `to_json`, `from_json`) so that the replication logic is identical regardless of the underlying data type.

**Theory.** This is the power of the algebraic approach. CRDTs form a *category* where:
- Objects are data types with semilattice merge
- Morphisms are functions that preserve the semilattice structure

You can build arbitrarily complex structures:
- A **Map CRDT** where each value is itself a CRDT
- A **Person** with a G-Counter for "steps walked" and an OR-Set for "friends"
- A **Document** with a sequence CRDT for text and a Map CRDT for metadata

As long as each field's merge is a valid semilattice join, the whole structure converges.

**Real-world examples.** Automerge and Yjs (collaborative editing) compose sequence CRDTs, map CRDTs, and counter CRDTs into full document models.

### 7. Total Availability

**What it is.** A system is *totally available* if every request to a non-crashed node receives a response, regardless of network conditions. No request ever blocks waiting for another node.

**How Maelstrom uses it.** CRDT nodes respond to reads and writes immediately using local state. Replication happens asynchronously in the background. Even during a network partition, every node can serve every request.

**Theory.** This is the "A" in the CAP theorem (Brewer, 2000; Gilbert & Lynch, 2002). The CAP theorem states that a distributed system can provide at most two of:
- **Consistency** (all nodes see the same data at the same time)
- **Availability** (every request gets a response)
- **Partition tolerance** (the system works despite network splits)

Since partitions are inevitable in real networks, the practical choice is between CP (consistent but sometimes unavailable) and AP (available but sometimes inconsistent). CRDTs are firmly AP — they sacrifice consistency (allowing stale reads) for total availability.

The PACELC extension (Abadi, 2012) adds: even when there's no partition, there's a tradeoff between latency and consistency. CRDTs choose low latency (local reads) over consistency.

### 8. Anti-Entropy Replication

**What it is.** Nodes periodically send their full state to peers, regardless of whether anything changed. This is "anti-entropy" because it reduces the disorder (entropy) between replicas.

**How Maelstrom uses it.** Every 5 seconds, each node sends its full CRDT state to all other nodes. This is simple but effective: message count is constant per time interval, independent of the operation rate.

**Theory.** Anti-entropy has a key advantage over operation-based replication: **it's self-healing**. If a message is lost, the next round includes all the data anyway. There's no need for acknowledgements, retries, or sequence numbers.

The cost is bandwidth: you send the full state even if nothing changed. For small states (counters, small sets), this is negligible. For large states (millions of keys), you'd use:
- **Merkle trees** to identify which parts of the state differ (used by Cassandra, Dynamo)
- **Delta-state CRDTs** that only send the parts that changed since the last sync
- **Hybrid approaches** that use operation-based replication normally and fall back to anti-entropy for repair

---

## Key Tradeoffs

| Dimension | Less ← → More |
|-----------|---------------|
| **Replication frequency** | Less bandwidth, higher staleness ← → Lower staleness, more bandwidth |
| **State vs. operation based** | Simpler, more bandwidth ← → Less bandwidth, needs reliable delivery |
| **Data model expressiveness** | Simple (sets, counters), easy to reason about ← → Complex (sequences, maps), harder to design |
| **Consistency vs. availability** | CRDTs choose availability — always respond, eventually converge |

---

## Prerequisites for Next Chapter

Before moving to Transactions, you should be comfortable with:

- What eventual consistency means and when it's acceptable
- How merge functions (semilattice joins) guarantee convergence
- The G-Set, G-Counter, and PN-Counter structures
- Why CRDTs compose and what that enables
- The CAP theorem and where CRDTs sit (AP)
- The difference between state-based and operation-based replication
