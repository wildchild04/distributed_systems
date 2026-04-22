# Transactions

## Difficulty: ★★★★☆

> Maelstrom Chapter: 5 (Datomic)

## What This Chapter Builds

A strict-serializable transactional key-value store. Starts with a single-node state machine, then shares state via a linearizable KV service using compare-and-set. Evolves through immutable persistent data structures (thunks, lazy-loaded trees) and optimizations like caching and speculative execution.

---

## Concepts & Theory

### 1. Transactions and Serializability

**What it is.** A *transaction* is a group of operations that execute atomically — either all succeed or none do. *Serializability* means that the result of executing transactions concurrently is equivalent to some sequential execution of those same transactions.

**How Maelstrom uses it.** Clients submit `txn` messages containing a list of micro-operations (reads and appends on keys). The server must execute the entire transaction atomically and return completed results. Maelstrom uses the Elle checker to verify that the resulting history is consistent with a serial order.

**Theory.** Serializability was defined by Papadimitriou (1979). There's a hierarchy of isolation levels:

| Level | Guarantees | Allows |
|-------|-----------|--------|
| **Read uncommitted** | Nothing useful | Dirty reads, lost updates, everything |
| **Read committed** | No dirty reads | Non-repeatable reads, phantom reads |
| **Snapshot isolation** | Consistent snapshot per txn | Write skew |
| **Serializable** | Equivalent to some serial order | Anomalies that respect real-time order |
| **Strict serializable** | Serial order respects real time | Nothing — strongest guarantee |

Maelstrom targets *strict serializability*, the strongest level. This means: if transaction T1 completes before T2 starts, T1 must appear before T2 in the serial order.

### 2. Strict Serializability = Linearizability of Transactions

**What it is.** Strict serializability is linearizability where the "object" being linearized is a transactional database. Every transaction appears to take effect at a single instant between its invocation and response.

**How Maelstrom uses it.** By storing the entire database state under a single key in a linearizable KV store and updating it via compare-and-set, each transaction is effectively a single linearizable operation on that key.

**Theory.** Herlihy and Wing (1990) defined linearizability for individual objects. Strict serializability extends this to multi-object transactions. The key insight in Maelstrom's Datomic chapter: if you can reduce your entire database to a single linearizable object (a pointer), you get strict serializability for free.

This is exactly what Datomic does in production: a single *transactor* process serializes all writes through a single reference that points to an immutable database value.

### 3. Compare-and-Set (CAS)

**What it is.** An atomic operation: "if the current value of key K is X, set it to Y; otherwise fail." This is the fundamental building block of optimistic concurrency control.

**How Maelstrom uses it.** The transactor reads the current database state from `lin-kv`, applies the transaction locally to produce a new state, then CAS-es the root pointer from old to new. If another transactor modified the state in between, the CAS fails and the transaction retries.

**Theory.** CAS is a *consensus primitive* — it's equivalent in power to consensus for a single register. Hardware provides CAS as a CPU instruction (`CMPXCHG` on x86). Distributed systems provide it via linearizable stores (etcd, ZooKeeper, DynamoDB conditional writes).

The pattern of read → modify → CAS is called *optimistic concurrency control* (OCC):
1. Read the current state (optimistically assuming no conflict)
2. Compute the new state locally
3. Attempt to write it back, conditioned on the state not having changed
4. If it changed, retry from step 1

**Tradeoff:** OCC works well under low contention (most CAS attempts succeed). Under high contention, transactions abort and retry repeatedly, wasting work. Pessimistic concurrency control (locks) avoids wasted work but reduces parallelism.

### 4. Database as a Single Value

**What it is.** Instead of storing each key separately in the linearizable store (which allows interleaving between keys), store the *entire database* as a single value. This eliminates cross-key anomalies because every transaction reads and writes a consistent snapshot.

**How Maelstrom uses it.** The first multi-node attempt stores each key separately in `lin-kv`. This fails — two reads of the same key within a transaction can see different values because another transaction snuck in between. The fix: store the entire `Map` under a single `root` key and CAS the whole thing.

**Theory.** This is the *single-writer principle*: by funneling all mutations through a single atomic operation (CAS on root), you get a total order on all transactions. It's the same principle behind:
- **Datomic's transactor:** Single process serializes all writes
- **Calvin:** Deterministic transaction ordering
- **Event sourcing:** Single event log defines the total order

The downside is obvious: the single CAS point is a throughput bottleneck. Every transaction must read and write the root, even if they touch completely different keys.

### 5. Immutable Persistent Data Structures

**What it is.** Data structures where "modification" produces a *new version* that shares most of its structure with the old version. The old version remains valid and unchanged.

**How Maelstrom uses it.** The `Map` class is immutable — `assoc(k, v)` returns a new Map. The `transact` method takes one Map and returns a new Map plus the completed transaction. This eliminates a whole class of bugs (the mutable-list bug in the chapter, where appending to a list mutated it in all previous reads).

**Theory.** Persistent data structures (Driscoll et al., 1986) are foundational in functional programming. Key properties:

- **Structural sharing:** New versions reuse unchanged parts of the old structure. A tree with 1 million nodes where you change 1 leaf only allocates O(log n) new nodes.
- **Safe concurrency:** Since old versions are never modified, readers never need locks.
- **Time travel:** You can always refer to any previous version of the data.

In the Maelstrom context, immutability enables a critical optimization: values that haven't changed don't need to be re-written to storage.

### 6. Thunks and Lazy Loading

**What it is.** A *thunk* is a deferred computation — a placeholder that loads its value from storage only when accessed. By storing thunk IDs (pointers) in the root map instead of actual values, you avoid loading data you don't need.

**How Maelstrom uses it.** Instead of storing `{key: [1,2,3]}` in the root, the root stores `{key: "n1-42"}` where `"n1-42"` is the ID of a thunk in the KV store. When a transaction reads key, the thunk fetches `[1,2,3]` from storage on demand. Unaccessed keys are never loaded.

**Theory.** This is *lazy evaluation* applied to distributed storage. It's the same principle behind:
- **Virtual memory:** Pages are loaded from disk only when accessed
- **Database buffer pools:** Pages are fetched on demand
- **Git's object store:** Tree objects contain hashes (pointers) to blob objects

The key insight: since thunks are *immutable* (their value never changes once written), they can be:
- **Cached indefinitely** — no invalidation needed
- **Stored in eventually consistent storage** — if a read fails, retry until the write propagates
- **Shared across transactions** — multiple transactions can reference the same thunk

### 7. Linearizable vs. Eventually Consistent Storage

**What it is.** The root pointer must be in a linearizable store (to ensure CAS atomicity), but the immutable thunks can live in an eventually consistent store (since their values never change).

**How Maelstrom uses it.** Moving thunks from `lin-kv` to `lww-kv` (last-write-wins, eventually consistent) works — but requires retry logic for reads that hit stale replicas. The root pointer stays in `lin-kv`.

**Theory.** This is a *tiered consistency* architecture:
- **Hot path (root pointer):** Linearizable, low-volume, high-contention
- **Cold path (immutable data):** Eventually consistent, high-volume, zero-contention

This pattern appears everywhere in production:
- **Datomic:** Transactor writes to a log (linearizable), storage is DynamoDB/S3 (eventually consistent)
- **Git:** Refs (branch pointers) are linearizable, objects are content-addressed and immutable
- **CDNs:** Origin server is authoritative, edge caches are eventually consistent

The insight: *immutability eliminates the need for strong consistency on data*. You only need strong consistency on the *pointers* to that data.

### 8. Speculative Execution and Caching

**What it is.** Instead of reading the root pointer from storage before every transaction, assume the locally cached version is still current. Apply the transaction speculatively, then CAS. If the CAS fails (someone else changed the root), re-read and retry.

**How Maelstrom uses it.** The `State` class caches the current `@map`. Transactions run against the cached map. On CAS success, the cache is updated. On failure, the cache is refreshed from storage and the transaction retries with random backoff.

**Theory.** This is *speculative execution* — doing work before you know if it's valid, then validating cheaply. It's the same principle behind:
- **CPU branch prediction:** Execute instructions speculatively, roll back on misprediction
- **Optimistic locking in databases:** Execute the transaction, check for conflicts at commit time
- **Speculative Paxos:** Execute requests before consensus completes

The random backoff on retry is critical: without it, multiple transactors retry simultaneously and keep conflicting. This is the *thundering herd* problem, mitigated by *jittered exponential backoff*.

### 9. Stale Reads and the Consistency Boundary

**What it is.** Read-only transactions that skip the CAS check can return stale data — they may not see the effects of recently committed transactions. This drops the system from strict serializable to merely serializable.

**How Maelstrom uses it.** If `map1.id == map2.id` (no writes), the optimization skips the CAS. This allows a node to read from a cached state that's behind the latest committed state. Maelstrom's Elle checker detects the resulting G-single-realtime anomalies.

**Theory.** The CAS on the root pointer isn't just for writes — it's a *fence* that ensures the reader sees the latest state. Without it, you lose the real-time ordering guarantee. The system is still serializable (there exists *some* valid ordering) but not *strict* serializable (that ordering may not respect wall-clock time).

This is exactly the tradeoff many production databases make:
- **Serializable reads:** Must check the latest state (higher latency)
- **Snapshot reads:** Can read from a potentially stale snapshot (lower latency)
- **Stale reads:** Can read from any past state (lowest latency)

---

## Key Tradeoffs

| Dimension | Less ← → More |
|-----------|---------------|
| **Single root vs. per-key storage** | Strict serializability, bottleneck ← → Higher throughput, weaker isolation |
| **Eager vs. lazy loading** | Simpler, loads everything ← → Faster, more complex pointer management |
| **Linearizable vs. EC storage for data** | Simpler, higher latency ← → Lower latency, needs retry logic |
| **Speculative vs. pessimistic execution** | No wasted work, lower throughput ← → Higher throughput, wasted work on conflict |
| **Strict serializable vs. serializable reads** | Strongest guarantee, extra round-trip ← → Faster reads, allows stale data |

---

## Prerequisites for Next Chapter

Before moving to Consensus, you should be comfortable with:

- The serializability hierarchy (read committed → snapshot isolation → serializable → strict serializable)
- How CAS enables optimistic concurrency control
- Why storing the database as a single value gives strict serializability
- How immutable data structures enable tiered consistency (linearizable pointers, EC data)
- The tradeoff between speculative execution and wasted work under contention
