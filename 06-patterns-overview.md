# Patterns Overview

## Difficulty: ★★★☆☆ (synthesis — assumes you've read the previous docs)

> Cross-cutting: ties together all Maelstrom chapters

## Purpose

This document zooms out from individual patterns to show how they relate, when to use each, and the fundamental tradeoffs that govern all distributed systems design.

---

## The Consistency Spectrum

Maelstrom teaches patterns across the full consistency spectrum. From weakest to strongest:

```
Eventual          Causal          Sequential       Serializable     Strict Serializable
Consistency       Consistency     Consistency      (+ real-time)    = Linearizable txns
    │                                                  │                    │
    ▼                                                  ▼                    ▼
  CRDTs                                          Datomic (stale       Datomic (with
  Gossip                                         read optimization)   CAS fence)
                                                                      Raft
```

Each step to the right buys you stronger guarantees but costs you in availability, latency, or throughput.

### Formal Definitions

| Model | Guarantee | Intuition |
|-------|-----------|-----------|
| **Eventual consistency** | If updates stop, all replicas converge | "Everyone agrees... eventually" |
| **Strong eventual consistency** | Replicas with the same updates are in the same state | "Same inputs → same state, always" |
| **Causal consistency** | If A caused B, everyone sees A before B | "Cause precedes effect" |
| **Sequential consistency** | All nodes see operations in the same order (but not necessarily real-time order) | "One global timeline, but it might not match wall clocks" |
| **Linearizability** | Operations appear to take effect at a single instant between invocation and response | "Behaves like a single machine" |
| **Serializability** | Transactions appear to execute in some serial order | "As if one transaction at a time" |
| **Strict serializability** | Serializable + real-time ordering | "Linearizability for transactions" |

---

## The CAP Theorem

**Statement (Brewer, 2000; Gilbert & Lynch, 2002):** A distributed system can provide at most two of three properties:
- **C**onsistency: Every read receives the most recent write
- **A**vailability: Every request to a non-crashed node receives a response
- **P**artition tolerance: The system continues to operate despite network splits

Since network partitions are inevitable, the practical choice is **CP** or **AP**.

### Where Maelstrom's Patterns Fall

| Pattern | CAP | During Partition |
|---------|-----|-----------------|
| Gossip broadcast | AP | Nodes in each partition continue independently; merge after healing |
| CRDTs | AP | Total availability; reads/writes always succeed; convergence delayed |
| Datomic transactor | CP | CAS fails if the linearizable store is unreachable; transactions abort |
| Raft | CP | Minority partition is unavailable; majority partition elects a leader and continues |

### PACELC Extension

The CAP theorem only describes behavior *during* partitions. PACELC (Abadi, 2012) adds: even when there's **no partition (E)**, there's a tradeoff between **latency (L)** and **consistency (C)**.

| Pattern | During Partition | Else (no partition) |
|---------|-----------------|-------------------|
| CRDTs | PA (available) | EL (low latency, local reads) |
| Raft | PC (consistent) | EC (consistent, higher latency) |
| Datomic | PC (consistent) | EC (consistent) or EL (stale reads) |

---

## Pattern Comparison

### By Complexity

| # | Pattern | Concepts Required | Implementation Difficulty |
|---|---------|-------------------|-------------------------|
| 1 | Unique ID generation | Node identity | Trivial |
| 2 | Gossip broadcast (basic) | Message passing, deduplication | Easy |
| 3 | Reliable delivery (retries) | RPC, acknowledgements, concurrency | Easy-Medium |
| 4 | Topology optimization | Graph theory, latency analysis | Medium |
| 5 | CRDTs | Semilattices, merge functions, CAP | Medium |
| 6 | Kafka-style log | Offsets, consumer groups, ordering | Medium |
| 7 | Datomic transactor | Serializability, CAS, immutable data | Hard |
| 8 | Raft consensus | Elections, log replication, quorums, safety proofs | Very Hard |

### By Throughput and Latency

| Pattern | Read Latency | Write Latency | Throughput Bottleneck |
|---------|-------------|--------------|----------------------|
| CRDTs | Local (μs) | Local (μs) | None — fully parallel |
| Gossip | Local (μs) | Local + propagation | Network bandwidth |
| Datomic | 1 round-trip (or cached) | 2+ round-trips (read + CAS) | Single CAS point |
| Raft | 1 round-trip (leader) or local (stale) | 1 majority round-trip | Leader throughput |

### By Fault Tolerance

| Pattern | Tolerates | Requires |
|---------|-----------|----------|
| CRDTs | Any number of failures | At least 1 node alive |
| Gossip + retries | Any number of transient failures | Eventual connectivity |
| Datomic | lin-kv failures (depends on its replication) | lin-kv available |
| Raft | f failures in 2f+1 nodes | Majority alive and connected |

---

## When to Use What

### Use CRDTs / Gossip when:
- You need **total availability** (every request succeeds, always)
- Temporary inconsistency is acceptable
- The data model fits (counters, sets, flags, registers)
- You're building: shopping carts, collaborative editing, view counters, cluster membership, feature flags

### Use the Datomic/transactor pattern when:
- You need **strict serializability** for transactions
- You already have a linearizable store available
- Write throughput is moderate (single-writer bottleneck is acceptable)
- You're building: metadata stores, configuration management, coordination services

### Use Raft / consensus when:
- You need **linearizable reads and writes** without an external dependency
- You need to **build** the linearizable store itself
- You can tolerate brief unavailability during leader elections
- You're building: distributed databases, lock services, coordination services (etcd, ZooKeeper)

### Decision Flowchart

```
Do you need strong consistency?
├── No → Can your data model use CRDTs?
│        ├── Yes → Use CRDTs (total availability, eventual consistency)
│        └── No  → Use gossip + application-level conflict resolution
└── Yes → Do you have an existing linearizable store?
          ├── Yes → Use the Datomic/transactor pattern (CAS over lin-kv)
          └── No  → Build one with Raft (or use an existing one: etcd, ZooKeeper)
```

---

## Cross-Cutting Concepts

These concepts appear across multiple chapters:

### Idempotency
- **Gossip:** Deduplication makes message processing idempotent
- **CRDTs:** Merge is idempotent by definition
- **Datomic:** CAS is idempotent (same CAS applied twice has no additional effect)
- **Raft:** AppendEntries is idempotent (re-sending the same entries is safe)

### Monotonicity
- **Gossip:** The seen-set only grows
- **CRDTs:** State only moves "up" in the semilattice
- **Raft:** Terms only increase; commit index only advances; logs only grow (on the leader)

### Quorums and Majorities
- **Raft:** Majority for election, majority for commit
- **Datomic:** Single-point CAS (quorum of 1, delegated to lin-kv)
- **CRDTs:** No quorum needed (every node is authoritative)

### Immutability
- **CRDTs:** Merge produces new values; old values are never modified
- **Datomic:** Thunks are immutable; the database is a succession of immutable snapshots
- **Raft:** Committed log entries are never overwritten

### Retries and Convergence
- **Gossip:** Retry unacknowledged broadcasts
- **CRDTs:** Periodic anti-entropy replication (implicit retry)
- **Datomic:** Retry transactions on CAS failure
- **Raft:** Leader retries AppendEntries; candidates retry elections

---

## The Fundamental Tradeoff Map

Every distributed systems design decision maps to a point in this space:

```
                        Strong Consistency
                              │
                              │
                    Raft ●    │    ● Datomic
                              │
                              │
   High Availability ─────────┼───────── Low Availability
                              │
                              │
                  CRDTs ●     │    ● Single-node DB
                              │
                              │
                       Weak Consistency
```

- **Top-left (Raft):** Strong consistency + reasonable availability (majority must be up)
- **Top-right (Datomic):** Strong consistency + depends on external store availability
- **Bottom-left (CRDTs):** Weak consistency + total availability
- **Bottom-right (Single-node):** Strong consistency + zero fault tolerance

There is no top-left-and-bottom-left position. That's the CAP theorem.

---

## References

- Brewer, E. (2000). "Towards Robust Distributed Systems" (CAP conjecture)
- Gilbert, S. & Lynch, N. (2002). "Brewer's Conjecture and the Feasibility of Consistent, Available, Partition-Tolerant Web Services"
- Abadi, D. (2012). "Consistency Tradeoffs in Modern Distributed Database System Design" (PACELC)
- Fischer, M., Lynch, N., & Paterson, M. (1985). "Impossibility of Distributed Consensus with One Faulty Process" (FLP)
- Shapiro, M. et al. (2011). "Conflict-free Replicated Data Types"
- Ongaro, D. & Ousterhout, J. (2014). "In Search of an Understandable Consensus Algorithm" (Raft)
- Herlihy, M. & Wing, J. (1990). "Linearizability: A Correctness Condition for Concurrent Objects"
- Lamport, L. (1978). "Time, Clocks, and the Ordering of Events in a Distributed System"
- Schneider, F. (1990). "Implementing Fault-Tolerant Services Using the State Machine Approach"
- DeCandia, G. et al. (2007). "Dynamo: Amazon's Highly Available Key-value Store"
