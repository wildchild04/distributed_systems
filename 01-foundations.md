# Foundations

## Difficulty: ★☆☆☆☆

> Maelstrom Chapters: 1 (Getting Ready) + 2 (Echo)

## What This Chapter Builds

A single-node echo server that receives JSON messages on STDIN, parses them, and replies on STDOUT. No inter-node communication yet — this is purely about understanding the execution model.

---

## Concepts & Theory

### 1. The Asynchronous Message-Passing Model

**What it is.** Distributed nodes are independent processes with private state. They cannot read each other's memory. The only way to share information is by sending and receiving messages over an unreliable network.

**How Maelstrom uses it.** Each node is a separate OS process. Maelstrom routes JSON messages between them via a simulated network. Nodes read from STDIN, write to STDOUT, and log to STDERR. There is no shared filesystem, no shared database, no shared anything.

**Theory.** This is the *asynchronous distributed model* formalized by Fischer, Lynch, and Paterson (1985). Its key properties:

- **No shared clock.** Nodes cannot agree on "what time it is." There is no global ordering of events without explicit coordination.
- **No shared memory.** The only way to learn about another node's state is to ask it.
- **Unreliable delivery.** Messages may be delayed, reordered, duplicated, or lost entirely.
- **Independent failure.** Any node can crash at any time without the others knowing immediately.

This model is pessimistic on purpose — if your algorithm works here, it works in the real world too.

**Real-world examples.** Microservices communicating over HTTP, database replicas syncing over TCP, mobile apps talking to a backend. All of these are instances of the asynchronous message-passing model, even if the network is usually reliable.

### 2. The Node (Process) as a Theoretical Primitive

**What it is.** A *node* (or *process*) is the fundamental unit of computation in distributed systems theory. It is a sequential state machine that:
- Holds private state that no other node can observe directly
- Receives messages from an input buffer
- Transitions between states based on its current state and the message received
- Produces output messages as a result of transitions

This isn't just an implementation convenience — it's the formal building block that every distributed systems result is built on.

**How Maelstrom uses it.** Each Maelstrom node is a literal OS process. Its state is whatever variables it holds in memory. Its input buffer is STDIN. Its output channel is STDOUT. Its transition function is the main loop that reads a message, dispatches to a handler, and potentially sends replies. This is a direct, concrete realization of the theoretical model.

**Theory.** The node/process appears as a first-class primitive across the foundational literature:

- **Lamport (1978), "Time, Clocks, and the Ordering of Events in a Distributed System"** — defines a distributed system as a collection of *processes*, where each process consists of a sequence of events. The entire theory of logical clocks, happens-before ordering, and causal consistency is built on reasoning about events *within* and *between* processes.

- **Fischer, Lynch, Paterson (1985), the FLP impossibility proof** — models each process as having:
  - An *internal state* (from a possibly infinite set of states)
  - An *input buffer* (an unordered set of messages)
  - A *transition function* that maps (state, message) → (new state, output messages)
  
  The FLP result proves that no deterministic protocol can guarantee consensus if even *one* process can crash. The proof works by showing that there's always a reachable state where the system hasn't decided yet — and a single crash at that moment prevents progress forever.

- **Lynch (1996), *Distributed Algorithms*** — formalizes processes as *I/O automata*: state machines with input actions (receiving messages), output actions (sending messages), and internal actions (local computation). This framework lets you compose processes into systems and reason about their combined behavior.

- **Schneider (1990), "Implementing Fault-Tolerant Services Using the State Machine Approach"** — defines the *state machine replication* paradigm: if you model your service as a deterministic state machine (a process), and you get all replicas to apply the same inputs in the same order, they'll all reach the same state. This is the theoretical foundation of Raft, Paxos, and every replicated database.

**The five constraints that define a node.** What makes a node a *distributed systems* concept (rather than just "a computer") is the set of constraints it operates under:

| Constraint | Meaning | Consequence |
|-----------|---------|-------------|
| **Private state** | No shared memory with other nodes | You can't just read another node's variable |
| **Local knowledge only** | A node cannot observe another node's state directly | Everything you know about others comes from messages, which may be stale |
| **Communication only via messages** | Messages may be delayed, lost, reordered, or duplicated | You can never be sure a message arrived unless you get an acknowledgement (which itself might be lost) |
| **Independent failure** | Any node can crash at any time | You can't distinguish "crashed" from "slow" — this is the core of the *failure detection* problem |
| **No global clock** | Nodes cannot agree on time without a protocol | There is no "now" that all nodes share; ordering events requires explicit mechanisms (logical clocks, consensus) |

Every algorithm in the field — Paxos, Raft, gossip, CRDTs, two-phase commit — is fundamentally about what a node can *infer* and *decide* given these limitations.

**The node is where uncertainty lives.** The reason distributed systems are hard isn't the network — it's that each node has an *incomplete, possibly outdated* view of the world. A node that received a message 5 seconds ago doesn't know if the sender is still alive. A node that didn't receive a message doesn't know if it was never sent, is still in transit, or was lost. This *epistemic uncertainty* — what a node knows vs. what is actually true — is the central challenge of distributed computing.

**Crash models.** The theory distinguishes several failure models for nodes, from weakest to strongest:

| Model | What can fail | Examples |
|-------|--------------|---------|
| **Crash-stop** | Node stops executing and never recovers | Process killed, hardware failure |
| **Crash-recovery** | Node stops, then restarts with durable state | Server reboot, container restart |
| **Omission** | Node fails to send or receive some messages | Network interface drops packets |
| **Byzantine** | Node can behave arbitrarily (including maliciously) | Compromised server, bit flips, bugs |

Maelstrom uses the **crash-stop** model with **network partitions** (omission faults on the network). Most practical systems (etcd, Cassandra, Kafka) are designed for crash-recovery. Byzantine fault tolerance (BFT) is used in blockchains and some safety-critical systems, but is much more expensive.

**Why this matters for Maelstrom.** When you write a Maelstrom node, you're not just writing a program — you're implementing a process in the formal sense. Your main loop is the transition function. Your instance variables are the process state. Your STDIN is the input buffer. Understanding this connection means you can apply results from the theory directly: if a theorem says "no process can distinguish a crashed node from a slow one," that applies to your Maelstrom node too.

### 3. Node Identity and Initialization

**What it is.** Before a node can participate in a cluster, it needs to know who it is (its node ID) and who else exists (the set of all node IDs). Maelstrom sends an `init` message at startup with this information.

**How Maelstrom uses it.** The very first message every node receives is:

```json
{"type": "init", "node_id": "n1", "node_ids": ["n1", "n2", "n3"]}
```

The node stores its ID, replies `init_ok`, and is ready to participate.

**Theory.** This is the *membership problem* in its simplest form: a static, known-at-startup cluster. In production systems, membership is often dynamic — nodes join and leave — which requires protocols like SWIM, gossip-based membership, or a coordination service (ZooKeeper, etcd). Maelstrom sidesteps this complexity by providing membership upfront.

**Why it matters.** Many distributed algorithms (voting, quorums, sharding) need to know the cluster size. If you don't know how many nodes exist, you can't compute a majority.

### 4. Request-Response and RPC

**What it is.** A client sends a request with a `msg_id`, and the server replies with `in_reply_to` set to that `msg_id`. This pairs requests with responses, enabling the client to match which reply goes with which request.

**How Maelstrom uses it.** Every RPC in Maelstrom follows this pattern:

```
Client → Server:  {type: "echo", echo: "hello", msg_id: 1}
Server → Client:  {type: "echo_ok", echo: "hello", in_reply_to: 1}
```

The `msg_id` / `in_reply_to` pair is the correlation mechanism.

**Theory.** This is *Remote Procedure Call (RPC)*, first described by Birrell and Nelson (1984). The key insight is that network communication can be made to *look like* a local function call, but it has fundamentally different failure modes:

- **The call may never arrive** (network loss)
- **The response may never arrive** (server crash after processing)
- **The call may arrive twice** (retry after timeout)

These failure modes lead to the classic RPC semantics:
- *At-most-once:* The server processes the request 0 or 1 times (use deduplication)
- *At-least-once:* The server processes the request 1 or more times (use retries)
- *Exactly-once:* The server processes the request exactly 1 time (requires idempotency or transaction logs)

Maelstrom's echo workload uses at-most-once semantics — if the server doesn't reply in time, the client records a timeout.

### 5. Message Framing and Serialization

**What it is.** Nodes need a shared language for messages. Maelstrom uses newline-delimited JSON: one JSON object per line.

**How Maelstrom uses it.** Every message has an envelope (`src`, `dest`) and a `body` containing the payload. The envelope handles routing; the body handles semantics.

```json
{"src": "c1", "dest": "n1", "body": {"type": "echo", "echo": "hi", "msg_id": 1}}
```

**Theory.** Serialization format choice affects performance, debuggability, and schema evolution. JSON is human-readable but verbose. Production systems often use Protocol Buffers, Avro, or MessagePack. The tradeoff is always readability vs. efficiency vs. schema enforcement.

The newline-delimited framing is a simple *length-independent framing protocol* — you don't need to know the message size upfront, you just read until `\n`. This is the same approach used by NDJSON, Redis's RESP protocol, and many log formats.

### 6. Deterministic Testing via Simulation

**What it is.** Instead of running nodes on separate machines with a real network, Maelstrom runs them as local processes and routes messages through a simulated network it fully controls.

**How Maelstrom uses it.** Maelstrom can:
- Add latency to messages
- Drop messages (simulate partitions)
- Record every message for post-hoc analysis
- Generate Lamport diagrams of message flow
- Check correctness properties against the recorded history

**Theory.** This is *simulation-based testing* (also called *deterministic simulation testing*). The idea: if you control the network, you control the nondeterminism, and you can reproduce bugs. This approach was pioneered by FoundationDB, which found that simulation testing caught more bugs than any other technique. Jepsen (which Maelstrom is built on) takes a similar philosophy but operates at a higher level — testing real or simulated systems against formal correctness properties.

**Real-world parallel.** FoundationDB's simulation tester, AWS's Zelkova (for IAM policy analysis), and TLA+ model checking all share this philosophy: make the system's behavior exhaustively explorable.

### 7. History-Based Correctness Checking

**What it is.** After a test, Maelstrom has a *history*: a sequence of invocations and responses from all clients. It checks whether this history is consistent with the expected behavior of the system.

**How Maelstrom uses it.** For the echo workload, the check is simple: did every echo request get back the same payload? For later workloads, the checks become much more sophisticated — linearizability, serializability, convergence.

**Theory.** This is *linearizability checking*, formalized by Herlihy and Wing (1990). Given a history of concurrent operations, is there *some* sequential ordering of those operations that:
1. Is consistent with the operation's semantics (reads see the latest write), and
2. Respects real-time ordering (if op A finished before op B started, A must appear before B)?

Checking linearizability is NP-complete in general, but practical algorithms exist for small histories (Knossos, used by Jepsen) and for specific data structures (Elle, used for transactional workloads).

### 8. The Node Abstraction

**What it is.** Maelstrom progressively builds a reusable `Node` class that encapsulates everything a distributed systems node needs: message parsing, sending, replying, handler registration, asynchronous RPC with callbacks, synchronous RPC with promises, and periodic task scheduling. This abstraction is built once and reused in every subsequent chapter.

**How Maelstrom builds it.** The Node evolves across Chapters 2 and 3:

| Capability | Added in | Purpose |
|-----------|----------|---------|
| STDIN read loop + JSON parsing | Echo (Ch2) | Receive messages |
| `send!(dest, body)` | Echo (Ch2) | Send a message to any node |
| `reply!(req, body)` | Echo (Ch2) | Reply to a request (fills in `in_reply_to`) |
| `on(type, &handler)` | Broadcast (Ch3) | Register a handler for a message type |
| `@handlers` dispatch in main loop | Broadcast (Ch3) | Route incoming messages to the right handler |
| `rpc!(dest, body, &callback)` | Broadcast (Ch3) | Async RPC — send a request, invoke callback when response arrives |
| `@callbacks` map (`msg_id` → block) | Broadcast (Ch3) | Match responses to outstanding requests |
| `sync_rpc!(dest, body)` | Datomic (Ch5) | Blocking RPC — send a request, block until response (uses Promises) |
| `Promise` class (deliver/await) | Datomic (Ch5) | Thread synchronization for sync RPC |
| `every(dt, &block)` | CRDTs (Ch4) | Schedule periodic background tasks (replication, elections) |
| `brpc!(body, &handler)` | Raft (Ch6) | Broadcast RPC — send to all other nodes, invoke callback per response |
| Thread-per-message in main loop | Broadcast (Ch3) | Concurrent message handling (prevents blocking on retries) |

**Why it matters.** The Node abstraction is a *network server framework* in miniature. Every real distributed system has an equivalent layer:

- **gRPC / Thrift:** Handler registration, request routing, RPC with callbacks
- **Akka / Erlang OTP:** Message dispatch, actor mailboxes, periodic timers
- **Raft libraries (etcd/raft, hashicorp/raft):** RPC transport, callback-based replication

By building it from scratch, Maelstrom makes explicit what production frameworks hide: how message correlation works, why you need concurrency in the message loop, and how synchronous RPC is built on top of asynchronous primitives.

**Theory.** The Node abstraction implements several classic patterns:

- **Reactor pattern.** The main loop reads messages and dispatches them to registered handlers. This is the same pattern used by Node.js's event loop, Nginx, and Redis. The key property: a single thread handles I/O, and handlers are invoked synchronously (until the thread-per-message change).

- **Callback-based async RPC.** The `rpc!` method sends a message with a `msg_id` and stores a callback keyed by that ID. When a response arrives with a matching `in_reply_to`, the callback is invoked and removed. This is the *correlation ID pattern*, used in JMS, AMQP, and HTTP/2 stream multiplexing.

- **Promise / Future.** The `sync_rpc!` method wraps the callback-based `rpc!` in a `Promise` that blocks the calling thread until a value is delivered. This bridges async and sync worlds — the same pattern as Java's `CompletableFuture`, JavaScript's `Promise`, and Rust's `Future`. The implementation uses a mutex + condition variable, which is the standard approach for thread synchronization.

- **Periodic task scheduler.** The `every(dt)` method spawns a background thread that runs a block at a fixed interval. This drives replication in CRDTs, heartbeats in Raft, and election timeouts. Production systems use more sophisticated schedulers (timer wheels, cron-like systems), but the concept is identical.

**Key design decisions in the abstraction:**

1. **Handler registration vs. switch statement.** The `on(type)` method decouples message routing from message handling. Each workload registers its own handlers without modifying the Node class. This is the *open-closed principle* applied to network servers.

2. **Thread-per-message.** The initial single-threaded main loop deadlocks when a handler blocks (e.g., retry loops waiting for acknowledgements that arrive on the same thread). Spawning a thread per message fixes this but introduces concurrency concerns — hence the `Monitor` lock throughout the codebase. Production systems use thread pools or async runtimes instead, but the lesson is the same: **blocking in a message handler blocks the entire node.**

3. **Sync RPC from async primitives.** `sync_rpc!` is built on `rpc!` + `Promise`. This layering is important: the async primitive is more general (supports concurrent outstanding requests), and the sync wrapper is a convenience for sequential code. The timeout in `Promise#await` prevents indefinite blocking when a response never arrives — a critical concern in distributed systems where any message can be lost.

4. **Error handling in handlers.** The main loop catches `RPCError` exceptions and serializes them as error responses. Unexpected exceptions are caught and returned as `crash` errors with stack traces. This ensures that a bug in one handler doesn't kill the entire node — the same principle behind Erlang's "let it crash" philosophy, applied at the request level.

---

## Key Tradeoffs

This chapter doesn't involve distributed tradeoffs yet — it's establishing the model. But the Node abstraction itself embodies several engineering tradeoffs:

| Dimension | Less ← → More |
|-----------|---------------|
| **Single-threaded vs. thread-per-message** | Simpler, no locks needed, but blocks on slow handlers ← → Concurrent, needs synchronization, but never blocks |
| **Async RPC vs. sync RPC** | More flexible, harder to reason about ← → Easier sequential code, blocks a thread |
| **Generic handler dispatch vs. hardcoded switch** | Extensible, slight overhead ← → Faster, but every new message type requires modifying the loop |

---

## Prerequisites for Next Chapter

Before moving to Gossip & Broadcast, you should be comfortable with:

- Nodes as independent processes with private state
- Message passing as the only communication mechanism
- Request-response RPC with `msg_id` / `in_reply_to` correlation
- The idea that messages can be lost, delayed, or reordered
- How the Node abstraction dispatches messages to handlers
- The difference between async RPC (callbacks) and sync RPC (promises)
- Why concurrent message handling is necessary (blocking handlers deadlock the node)
