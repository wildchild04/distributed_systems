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
