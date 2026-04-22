# Building a Node

## Difficulty: ★★☆☆☆ (practical — read 01-foundations first)

> Reference guide for implementing your own Maelstrom node from scratch, in any language.

---

## The Protocol

Every Maelstrom node is a binary that reads JSON from STDIN, writes JSON to STDOUT, and logs to STDERR. That's it. No sockets, no HTTP, no gRPC. If your language can read lines and parse JSON, you can build a node.

### Message Envelope

Every message has this shape:

```json
{
  "src":  "c1",
  "dest": "n1",
  "body": { "type": "echo", "msg_id": 1, "echo": "hello" }
}
```

- `src` / `dest`: Node IDs. Clients are `c1`, `c2`, etc. Servers are `n1`, `n2`, etc. Services are `lin-kv`, `seq-kv`, etc.
- `body.type`: Determines what kind of message this is.
- `body.msg_id`: Optional. Unique per-node integer. Include it when you expect a response.
- `body.in_reply_to`: Optional. Set to the `msg_id` of the request you're responding to.

### Initialization

The first message every node receives:

```json
{"src": "c1", "dest": "n1", "body": {"type": "init", "msg_id": 1, "node_id": "n1", "node_ids": ["n1", "n2", "n3"]}}
```

You must reply:

```json
{"src": "n1", "dest": "c1", "body": {"type": "init_ok", "in_reply_to": 1}}
```

Store `node_id` (who you are) and `node_ids` (who's in the cluster).

### Error Responses

Instead of a normal response, you can reply with an error:

```json
{"type": "error", "in_reply_to": 5, "code": 11, "text": "not ready yet"}
```

Key error codes:

| Code | Name | Definite? | Use when |
|------|------|-----------|----------|
| 0 | timeout | No | Operation couldn't complete in time |
| 10 | not-supported | Yes | You haven't implemented this operation |
| 11 | temporarily-unavailable | Yes | Node can't serve requests right now (not leader, initializing) |
| 13 | crash | No | Catch-all for unexpected errors |
| 14 | abort | Yes | Operation definitely did not happen |
| 20 | key-does-not-exist | Yes | Read/CAS on a missing key |
| 22 | precondition-failed | Yes | CAS `from` value doesn't match |
| 30 | txn-conflict | Yes | Transaction aborted due to conflict |

**Definite** = the operation definitely did not happen. **Indefinite** = it might have. When in doubt, use indefinite (code 13). Maelstrom uses this to interpret the history correctly.

---

## Skeleton Node (Pseudocode)

This is the minimal structure every node needs. Adapt to your language.

```
node_id = null
node_ids = []
next_msg_id = 0
handlers = {}

function send(dest, body):
    body.src = node_id
    body.dest = dest
    print(json_encode(body))    # to STDOUT
    flush(STDOUT)

function reply(request, body):
    body.in_reply_to = request.body.msg_id
    send(request.src, body)

function handle(type, callback):
    handlers[type] = callback

# Register init handler
handle("init", (msg) => {
    node_id = msg.body.node_id
    node_ids = msg.body.node_ids
    reply(msg, {type: "init_ok"})
})

# Main loop
while line = read_line(STDIN):
    msg = json_decode(line)
    handler = handlers[msg.body.type]
    if handler:
        handler(msg)
    else:
        reply(msg, {type: "error", code: 10, text: "not supported"})
```

That's ~25 lines. Everything else builds on top.

---

## What to Add for Each Workload

### Echo
Just the skeleton above, plus:

```
handle("echo", (msg) => {
    reply(msg, {type: "echo_ok", echo: msg.body.echo})
})
```

Test: `./maelstrom test -w echo --bin YOUR_BINARY --node-count 1 --time-limit 10`

### Unique IDs
Generate a globally unique ID per request. Simplest approach: `"{node_id}-{counter}"`.

```
handle("generate", (msg) => {
    id = node_id + "-" + (next_msg_id++)
    reply(msg, {type: "generate_ok", id: id})
})
```

Test: `./maelstrom test -w unique-ids --bin YOUR_BINARY --time-limit 30 --rate 1000 --node-count 3`

### Broadcast
You need: a message set, a neighbor list, and inter-node gossip.

```
messages = Set()
neighbors = []

handle("topology", (msg) => {
    neighbors = msg.body.topology[node_id]
    reply(msg, {type: "topology_ok"})
})

handle("read", (msg) => {
    reply(msg, {type: "read_ok", messages: messages.to_array()})
})

handle("broadcast", (msg) => {
    if msg.body.message not in messages:
        messages.add(msg.body.message)
        for neighbor in neighbors:
            if neighbor != msg.src:
                send(neighbor, {type: "broadcast", message: msg.body.message})
    if msg.body.msg_id:    # client request, not inter-node
        reply(msg, {type: "broadcast_ok"})
})
```

Test: `./maelstrom test -w broadcast --bin YOUR_BINARY --node-count 5 --time-limit 20 --rate 10`

With partitions: add `--nemesis partition` (requires retry logic — see 02-gossip-and-broadcast.md).

### G-Set / G-Counter / PN-Counter
You need: a CRDT data structure + periodic replication. See 03-crdts.md for the theory. The pattern is always:

1. Handle `add` → update local CRDT
2. Handle `read` → return CRDT value
3. Handle `replicate` → merge remote state into local CRDT
4. Periodic task → send full CRDT state to all other nodes

Test: `./maelstrom test -w g-set --bin YOUR_BINARY --time-limit 20 --rate 10`

### Transactions (Datomic-style)
You need: sync RPC to `lin-kv`, a `Map` data structure, CAS-based commit. See 04-transactions.md.

Test: `./maelstrom test -w txn-list-append --bin YOUR_BINARY --node-count 2 --time-limit 20 --rate 100`

### Linearizable KV (Raft)
You need: the full Raft protocol. See 05-consensus.md.

Test: `./maelstrom test -w lin-kv --bin YOUR_BINARY --node-count 3 --time-limit 20 --rate 10 --concurrency 2n`

---

## Using Maelstrom Services

Services are built-in nodes you can send RPCs to. They act as infrastructure primitives.

### lin-kv — Linearizable Key-Value Store

Send to node ID `"lin-kv"`. Supports `read`, `write`, and `cas`.

```json
// Read
{"dest": "lin-kv", "body": {"type": "read", "key": "mykey", "msg_id": 1}}
// Response
{"body": {"type": "read_ok", "value": 42, "in_reply_to": 1}}

// Write
{"dest": "lin-kv", "body": {"type": "write", "key": "mykey", "value": 42, "msg_id": 2}}
// Response
{"body": {"type": "write_ok", "in_reply_to": 2}}

// Compare-and-set
{"dest": "lin-kv", "body": {"type": "cas", "key": "mykey", "from": 42, "to": 43, "msg_id": 3}}
// Response (success)
{"body": {"type": "cas_ok", "in_reply_to": 3}}
// Response (failure)
{"body": {"type": "error", "code": 22, "text": "expected 42 but had 99", "in_reply_to": 3}}
```

CAS supports `"create_if_not_exists": true` to create missing keys instead of returning error 20.

### seq-kv — Sequentially Consistent Key-Value Store

Same API as `lin-kv`, but with relaxed consistency. Operations appear in a total order, but clients may interact with past states as long as per-client ordering is preserved.

### lww-kv — Eventually Consistent Key-Value Store

Same API as `lin-kv`, but intentionally pathological. Simulates multiple independent replicas with last-write-wins gossip. Reads may return stale data or `key-does-not-exist` for recently written keys. Use retries.

### lin-tso — Linearizable Timestamp Oracle

Produces monotonically increasing integers.

```json
// Request
{"dest": "lin-tso", "body": {"type": "ts", "msg_id": 1}}
// Response
{"body": {"type": "ts_ok", "ts": 123, "in_reply_to": 1}}
```

---

## Capabilities You'll Need to Build

Not every workload needs every capability. Here's what to implement and when.

| Capability | Needed for | Complexity |
|-----------|-----------|------------|
| STDIN/STDOUT JSON loop | Everything | Trivial |
| `send(dest, body)` | Everything | Trivial |
| `reply(request, body)` | Everything | Trivial |
| `handle(type, callback)` | Everything | Easy |
| Message deduplication (seen-set) | Broadcast | Easy |
| Async RPC (`rpc!` with callbacks) | Broadcast retries, Raft | Medium |
| Thread-per-message / async handlers | Broadcast retries, Raft | Medium |
| Sync RPC (promises/futures) | Datomic (talking to lin-kv) | Medium |
| Periodic tasks (timers) | CRDTs, Raft heartbeats/elections | Medium |
| Broadcast RPC (send to all, collect) | Raft vote requests | Medium |
| Locks / synchronization | Anything concurrent | Medium |
| Error types (RPCError class) | Datomic, Raft | Easy |

### Suggested build order

1. **STDIN loop + send + reply + init handler** → test with echo
2. **Handler dispatch** (`on`/`handle`) → cleaner echo
3. **Seen-set + neighbor gossip** → broadcast
4. **Async RPC + callbacks + thread-per-message** → broadcast with retries
5. **Periodic tasks** → CRDTs
6. **Sync RPC + promises** → Datomic
7. **All of the above + Raft state machine** → Raft

---

## Debugging Tips

### Log to STDERR
STDOUT is for messages only. Use STDERR for all debugging output. Maelstrom saves node logs to `store/latest/node-logs/n1.log`, etc.

### Use `--log-stderr`
Pass `--log-stderr` to see your node's STDERR output in Maelstrom's main log in real time.

### Check `messages.svg`
Lamport diagrams in `store/latest/` show every message between every node. Invaluable for understanding message flow.

### Check `results.edn`
The full analysis output. Look for `:valid? false` and the `:anomalies` section to understand what went wrong.

### Start with `--node-count 1`
Get single-node behavior correct before adding more nodes. Many bugs are visible with just one node.

### Start with low rates
Use `--rate 1` or `--rate 10` first. High rates amplify bugs and make logs unreadable.

### Common failure modes

| Symptom | Likely cause |
|---------|-------------|
| `timed out` on init | You're not replying `init_ok`, or not flushing STDOUT |
| `No handler for ...` | Missing handler for a message type |
| All requests timeout | Main loop is blocked (handler is doing a blocking retry loop) |
| `lost` messages in broadcast | Not gossiping to neighbors, or not retrying during partitions |
| Linearizability violation | Applying operations before they're committed (Raft), or interleaving reads across keys (Datomic) |
| `broadcast storm` (millions of messages) | Not deduplicating — forwarding messages you've already seen |

---

## Reference Implementations

Maelstrom ships with demos in multiple languages at `~/repos/maelstrom/demo/`:

| Language | Node library | Workloads implemented |
|----------|-------------|----------------------|
| **Ruby** | `node.rb` | Echo, broadcast, G-Set, PN-Counter, Datomic, Raft |
| **Python** | `maelstrom.py` | Echo, broadcast, Raft |
| **Go** | `node.go` | Echo (via `cmd/`) |
| **JavaScript** | `node.js` | Echo, gossip, G-Set, PN-Counter, transactions |
| **Clojure** | `node.clj` | Echo, gossip, G-Set, G-Counter, Kafka, transactions |
| **Java** | `src/` | Echo, broadcast, G-Set, G-Counter, transactions |
| **Rust** | `src/` | Echo |
| **C++** | `maelstrom.h` | Echo |

The Ruby implementations are the most complete and match the Maelstrom guide 1:1. Start there if you want to read a reference before writing your own.

---

## Workload Quick Reference

All the `./maelstrom test` commands in one place.

```bash
# Echo
./maelstrom test -w echo --bin YOUR_BIN --node-count 1 --time-limit 10

# Unique IDs
./maelstrom test -w unique-ids --bin YOUR_BIN --time-limit 30 --rate 1000 --node-count 3

# Broadcast (basic)
./maelstrom test -w broadcast --bin YOUR_BIN --node-count 5 --time-limit 20 --rate 10

# Broadcast (with partitions)
./maelstrom test -w broadcast --bin YOUR_BIN --node-count 5 --time-limit 20 --rate 10 --nemesis partition

# Broadcast (performance tuning)
./maelstrom test -w broadcast --bin YOUR_BIN --node-count 25 --time-limit 20 --rate 100 --latency 100

# G-Set
./maelstrom test -w g-set --bin YOUR_BIN --time-limit 20 --rate 10

# G-Counter
./maelstrom test -w g-counter --bin YOUR_BIN --time-limit 20 --rate 10

# PN-Counter
./maelstrom test -w pn-counter --bin YOUR_BIN --time-limit 20 --rate 10

# PN-Counter (with partitions)
./maelstrom test -w pn-counter --bin YOUR_BIN --time-limit 30 --rate 10 --nemesis partition

# Transactions (strict serializable)
./maelstrom test -w txn-list-append --bin YOUR_BIN --node-count 2 --time-limit 20 --rate 100

# Linearizable KV (Raft)
./maelstrom test -w lin-kv --bin YOUR_BIN --node-count 3 --time-limit 20 --rate 10 --concurrency 2n

# Raft (with partitions)
./maelstrom test -w lin-kv --bin YOUR_BIN --node-count 3 --time-limit 60 --rate 100 --concurrency 10n --nemesis partition --nemesis-interval 10

# Kafka
./maelstrom test -w kafka --bin YOUR_BIN --node-count 2 --time-limit 20 --rate 10
```
