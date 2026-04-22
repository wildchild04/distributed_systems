# Gossip & Broadcast

## Difficulty: ★★☆☆☆

> Maelstrom Chapter: 3 (Broadcast)

## What This Chapter Builds

A multi-node broadcast system where clients send messages to any node, and every node eventually has every message. Starts with naive flooding, evolves through topology optimization, and ends with fault-tolerant reliable delivery via retries.

---

## Concepts & Theory

### 1. The Broadcast Problem

**What it is.** Given a cluster of nodes, when one node receives a new piece of data, ensure that *every* node eventually has that data. This is one of the most fundamental problems in distributed systems.

**How Maelstrom uses it.** Clients send `broadcast` messages (containing an integer) to arbitrary nodes. Clients also send `read` requests to any node. At the end of the test, Maelstrom checks that every broadcast message is present on every node.

**Theory.** This is the *reliable broadcast* problem. There are several variants with increasing strength:

- **Best-effort broadcast:** If the sender doesn't crash, all correct nodes eventually receive the message.
- **Reliable broadcast:** If *any* correct node receives the message, *all* correct nodes eventually receive it.
- **Totally ordered broadcast:** All nodes receive all messages in the same order. (This is equivalent to consensus — see Chapter 5.)

Maelstrom's broadcast workload tests reliable broadcast: messages must eventually appear everywhere, even if the network is partitioned temporarily.

### 2. Epidemic (Gossip) Protocols

**What it is.** Each node that learns a new message forwards it to its neighbors. Those neighbors forward it to *their* neighbors. The message spreads through the network like a disease through a population — hence "epidemic."

**How Maelstrom uses it.** When a node receives a `broadcast` message, it adds the value to its local set and sends `broadcast` messages to all its neighbors. Nodes deduplicate messages they've already seen to prevent infinite loops.

**Theory.** Epidemic protocols were formalized by Demers et al. (1987) in "Epidemic Algorithms for Replicated Database Maintenance." There are two main variants:

- **Anti-entropy (state-based):** Nodes periodically exchange their *entire* state and merge. Simple but bandwidth-heavy. Used in Chapter 4 (CRDTs).
- **Rumor mongering (operation-based):** Nodes forward individual updates as they arrive. Lower bandwidth per message, but requires deduplication. Used here.

Key properties of epidemic protocols:
- **Distributed:** No central coordinator
- **Robust:** Tolerant of individual node failures
- **Eventually consistent:** All nodes converge, but there's no bound on when
- **Redundant:** The same message may arrive via multiple paths

**Real-world examples.** Cassandra uses gossip for cluster membership and failure detection. Redis Cluster uses gossip for node state propagation. Bitcoin uses gossip to propagate transactions and blocks.

### 3. Broadcast Storm and Deduplication

**What it is.** If a node blindly forwards every message it receives, including ones it's already seen, messages ricochet infinitely between neighbors, amplifying exponentially. This is a *broadcast storm*.

**How Maelstrom uses it.** The first naive implementation in Maelstrom causes exactly this: 894,000 messages for 11 operations. The fix is a set-membership check — only forward a message if you haven't seen it before.

**Theory.** Deduplication is the fundamental mechanism that turns unbounded flooding into bounded epidemic spread. It requires:
- **Unique message identifiers** so you can tell if you've seen something before
- **A seen-set** (or bloom filter, or similar structure) to track what's been processed

The seen-set grows monotonically — you never "unsee" a message. This is fine for bounded workloads but in production, you'd need expiration or compaction strategies.

**Tradeoff:** Memory (storing the seen-set) vs. network (redundant messages). Bloom filters trade a small false-positive rate for dramatically lower memory usage.

### 4. Network Topology

**What it is.** The *topology* defines which nodes are neighbors — who talks to whom. Different topologies produce radically different performance characteristics.

**How Maelstrom uses it.** Maelstrom sends a `topology` message at test start, providing each node with its neighbor list. You can override this with `--topology` flags: `grid`, `line`, `tree4`, `total`.

**Theory.** Topology is a graph theory problem. Key metrics:

| Topology | Messages per broadcast | Propagation latency | Fault tolerance |
|----------|----------------------|--------------------|-----------------| 
| **Line** | O(n) — optimal | O(n) — worst | Fragile — one cut splits the network |
| **Grid** | O(n) — near optimal | O(√n) | Moderate — multiple paths exist |
| **Tree** | O(n) — optimal | O(log n) | Fragile — losing the root is catastrophic |
| **Total** (fully connected) | O(n²) — worst | O(1) — best | Maximum — every node reaches every other |

The fundamental tradeoff: **more connections = lower latency but more messages.** The tree topology hits the sweet spot for many workloads: optimal message count with logarithmic latency.

**Key metric: free path length.** The *diameter* of the topology graph determines worst-case propagation latency. A line has diameter n-1. A grid has diameter ~2√n. A balanced tree has diameter ~2 log n.

**Real-world examples.** CDN networks use tree-like topologies for content distribution. Blockchain networks use random graph topologies (each node connects to ~8 random peers) for a balance of redundancy and efficiency.

### 5. Propagation Latency and Stable Latency

**What it is.** *Propagation latency* is how long it takes for a broadcast message to reach all nodes. *Stable latency* is when a message becomes visible in reads on every node. *Stale* messages are ones that haven't propagated everywhere yet.

**How Maelstrom uses it.** Maelstrom measures stable latencies at quantiles (p50, p95, p99, p100). With `--latency 100` on a line topology of 25 nodes, the worst case is ~2400ms (24 hops × 100ms). On a grid, it drops to ~800ms (8 hops).

**Theory.** This connects to the concept of *convergence time* in eventually consistent systems. The time to converge depends on:
- Network diameter (topology)
- Per-hop latency
- Message loss rate
- Retry interval

An interesting result from Maelstrom: when latency is *exponentially distributed* rather than constant, multi-path topologies (like grids) actually get *faster*. This is because some messages take shortcuts via low-latency paths. This is the probabilistic advantage of redundant paths.

### 6. Reliable Delivery via Retries

**What it is.** When the network can lose messages (partitions), fire-and-forget gossip loses data. To guarantee delivery, nodes must track which neighbors have acknowledged each message and keep retrying until they do.

**How Maelstrom uses it.** The broadcast handler maintains an `unacked` set per message per neighbor. It sends the message, and when the neighbor replies `broadcast_ok`, removes them from the set. A background loop retries unacked messages every second.

**Theory.** This is the transition from *best-effort broadcast* to *reliable broadcast*. The mechanism is:

1. Send message to neighbor
2. Wait for acknowledgement
3. If no ack within timeout, retry
4. Repeat until ack received

This guarantees *at-least-once delivery* — the neighbor will eventually get the message, but may get it multiple times. Combined with the deduplication from concept #3, the system achieves *exactly-once processing* (even though delivery is at-least-once).

**Tradeoff:** Reliability vs. resource usage. During a partition, retry loops accumulate — each undelivered message spawns a thread that keeps retrying. With thousands of pending messages, you get thousands of threads spamming the network. Production systems use:
- **Exponential backoff** to reduce retry frequency over time
- **Bounded retry queues** to limit memory usage
- **Batch retries** to amortize overhead

### 7. Concurrency in Message Handlers

**What it is.** If a message handler blocks (e.g., waiting for retries in a loop), the node's main loop can't process other messages — including the acknowledgements it's waiting for. This is a deadlock.

**How Maelstrom uses it.** The first retry implementation blocks the main loop, causing all requests to time out. The fix is spawning a thread per message handler: `Thread.new(handler, msg) { |h, m| h.call(m) }`.

**Theory.** This is the classic *concurrency model* choice in network servers:

- **Single-threaded event loop:** One thread processes all messages sequentially. Simple but can't block. (Node.js, Redis)
- **Thread-per-request:** Each message gets its own thread. Simple to reason about but expensive at scale. (Maelstrom's approach)
- **Thread pool:** Fixed number of worker threads. Bounds resource usage. (Most production servers)
- **Async/await (CPS):** Cooperative multitasking without OS threads. Efficient but harder to debug. (Go goroutines, Rust async)
- **Actor model:** Each actor has a mailbox and processes messages sequentially. Natural fit for distributed systems. (Erlang/OTP, Akka)

Maelstrom uses thread-per-request for simplicity. In production, you'd use a thread pool or async runtime.

---

## Key Tradeoffs

| Dimension | Less ← → More |
|-----------|---------------|
| **Connections per node** | Fewer messages, higher latency ← → Lower latency, more messages |
| **Retry frequency** | Less network load, slower recovery ← → Faster recovery, more network load |
| **Dedup set size** | Less memory, risk of reprocessing ← → More memory, guaranteed dedup |
| **Concurrency model** | Simpler code, risk of blocking ← → More complex, better throughput |

The overarching lesson: **topology and retry strategy are the two biggest levers** for tuning a gossip-based broadcast system.

---

## Prerequisites for Next Chapter

Before moving to CRDTs, you should be comfortable with:

- How epidemic protocols spread data through a network
- Why topology affects both latency and message count
- The difference between best-effort and reliable broadcast
- Why concurrent message handling is necessary for retry loops
- The concept of eventual consistency (all nodes converge, but not instantly)
