# Distributed Systems — Concept & Theory Notes

Study notes based on [Jepsen's Maelstrom](https://github.com/jepsen-io/maelstrom), a workbench for learning distributed systems by writing your own.

## Reading Order

Each doc builds on the previous. Difficulty ranges from 1 (introductory) to 5 (advanced).

| # | Document | Covers | Difficulty |
|---|----------|--------|------------|
| 1 | [Foundations](01-foundations.md) | Node model, message passing, RPC, request-response | ★☆☆☆☆ |
| 2 | [Gossip & Broadcast](02-gossip-and-broadcast.md) | Epidemic protocols, topologies, reliable delivery, retries | ★★☆☆☆ |
| 3 | [CRDTs](03-crdts.md) | Eventual consistency, state-based CRDTs, CRDT composition | ★★★☆☆ |
| 4 | [Transactions](04-transactions.md) | Serializability, compare-and-set, immutable persistent data | ★★★★☆ |
| 5 | [Consensus](05-consensus.md) | Leader election, log replication, Raft, state machine replication | ★★★★★ |
| 6 | [Patterns Overview](06-patterns-overview.md) | CAP theorem, consistency spectrum, pattern comparison, tradeoff map | ★★★☆☆ |
| — | [Building a Node](07-building-a-node.md) | Practical guide: protocol spec, node skeleton, workload checklist, services API | ★★☆☆☆ |
| — | [Node Architecture](08-node-architecture.md) | Layered design: transport, codec, router, RPC, scheduler, testability | ★★☆☆☆ |

## Maelstrom Source

These notes reference the Maelstrom repo at `~/repos/maelstrom`. The chapters map as follows:

- Maelstrom Ch1 (Getting Ready) + Ch2 (Echo) → `01-foundations.md`
- Maelstrom Ch3 (Broadcast) → `02-gossip-and-broadcast.md`
- Maelstrom Ch4 (CRDTs) → `03-crdts.md`
- Maelstrom Ch5 (Datomic) → `04-transactions.md`
- Maelstrom Ch6 (Raft) → `05-consensus.md`
- Cross-cutting synthesis → `06-patterns-overview.md`

## Appendixes — System Design Interview (Alex Xu)

Supplementary notes from [liquidslr/system-design-notes](https://github.com/liquidslr/system-design-notes) (based on *System Design Interview* Vol 1 & 2 by Alex Xu). These chapters overlap with or extend the Maelstrom concepts above.

### A. Direct Overlaps

| Maelstrom Chapter | Alex Xu Chapter | Shared Concept |
|-------------------|-----------------|----------------|
| 02 - Gossip & Broadcast | 06 - Key-Value Store | Gossip protocol for failure detection & state propagation |
| 03 - CRDTs | 06 - Key-Value Store | Eventual consistency, conflict resolution (Dynamo-style) |
| 05 - Consensus (Raft) | 06 - Key-Value Store | Quorum replication, consistency guarantees |
| 05 - Consensus (Raft) | 19 - Distributed Message Queue | Leader-based replication, log ordering |
| 06 - Patterns (CAP/PACELC) | 06 - Key-Value Store | CAP tradeoffs, consistency spectrum |
| 01 - Foundations (Unique ID) | 07 - Unique ID Generator | Snowflake, node-identity-based ID generation |

### B. Conceptual Overlaps

| Maelstrom Concept | Alex Xu Application |
|-------------------|-------------------|
| Reliable broadcast + retries | 10 - Notification System (fan-out, delivery guarantees) |
| Idempotency (cross-cutting) | 26 - Payment System, 27 - Digital Wallet |
| Log replication (Raft) | 19 - Message Queue (commit log, offset tracking) |
| CAS / linearizability | 22 - Hotel Reservation (concurrency control) |
| Consistent hashing (Dynamo theory) | 05 - Consistent Hashing |
| Anti-entropy / state reconciliation | 15 - Google Drive (sync conflicts) |

### C. Implementable Extensions

Chapters from Alex Xu that can be built on top of the Maelstrom node framework:

| Chapter | What to Build | Builds On |
|---------|--------------|-----------|
| 05 - Consistent Hashing | Hash ring with virtual nodes | Foundations (node identity) |
| 04 - Rate Limiter | Token bucket / sliding window | Foundations (RPC) |
| 07 - Unique ID Generator | Snowflake-style generator | Foundations (node identity) |
| 06 - Key-Value Store | Dynamo-style KV with quorums | Gossip + CRDTs + Consensus |
| 19 - Distributed Message Queue | Partitioned log with offsets | Consensus (Raft log) |

### D. CodeCrafters — Systems Internals

[CodeCrafters](https://codecrafters.io) challenges that complement the distributed systems theory above. Maelstrom teaches how nodes coordinate; CodeCrafters teaches what's inside each node.

| Challenge | Overlap | Builds On |
|-----------|---------|-----------|
| Build your own Redis | KV store, replication, expiry | CRDTs (Ch 03), Key-Value Store (Alex Xu 06) |
| Build your own Kafka | Commit log, partitions, consumer offsets | Consensus/Raft log (Ch 05), Message Queue (Alex Xu 19) |
| Build your own SQLite | B-tree, pages, transactions | Transactions (Ch 04) |
| Build your own DNS | Message parsing, request routing | Transport/codec layer (Ch 08) |
| Build your own HTTP server | Concurrent connections, request/response | Foundations (Ch 01) |
| Build your own Git | Content-addressable storage, immutable DAG | Immutability (cross-cutting, Ch 06) |
