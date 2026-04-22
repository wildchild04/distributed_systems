# Node Architecture

## Difficulty: ★★☆☆☆ (design — read 01-foundations and 07-building-a-node first)

> The layered architecture for a Maelstrom node, designed with clean boundaries, testability, and incremental growth from Echo through Raft.

---

## Design Philosophy

A Maelstrom node is a distributed systems process (see [01-foundations](01-foundations.md), section 2). But it's also a piece of software that needs to be testable, extensible, and understandable.

The key insight: **a node is not one thing — it's five subsystems composed together.** Each subsystem has a single responsibility, a clear interface, and no knowledge of the others' internals.

The design follows two principles:

1. **Dependency Inversion.** The node depends on interfaces (Transport, Codec), not on concrete implementations (STDIN, JSON). This makes every layer independently testable and swappable.

2. **Incremental growth.** You build only what you need for each workload. Echo needs Transport + Codec + Router. Broadcast adds RPC. CRDTs add Scheduler. Raft needs everything. The architecture supports this without refactoring.

---

## Layer Diagram

```
┌─────────────────────────────────────────────────────────┐
│                     main.go                             │
│  Create node, register handlers, run                    │
├─────────────────────────────────────────────────────────┤
│                     Node                                │
│  Identity, lifecycle, public API for handlers           │
│  Handle(), Send(), Reply(), RPC(), SyncRPC()            │
├──────────────┬──────────────────────┬───────────────────┤
│   Router     │      RPC Layer      │    Scheduler      │
│  type→handler│  callbacks, promises │  periodic tasks   │
│   dispatch   │  msg_id generation   │  timers           │
├──────────────┴──────────────────────┴───────────────────┤
│                    Codec                                │
│  Message ↔ []byte                                       │
│  Envelope + body marshaling                             │
├─────────────────────────────────────────────────────────┤
│                  Transport                              │
│  Recv() / Send() over a bus                             │
│  Framing, flushing, write serialization                 │
└─────────────────────────────────────────────────────────┘
```

---

## Subsystem 1: Transport

### Responsibility

Move framed bytes between nodes. Handle bus-specific concerns: framing, flushing, write serialization. Knows nothing about JSON, message types, or handlers.

### Interface

```go
type Transport interface {
    Recv() ([]byte, error)    // block until one complete framed message arrives
    Send(data []byte) error   // send one complete framed message; must be thread-safe
    Close() error
}
```

### Why it's an interface

The transport is the boundary where your distributed system meets the physical world. Maelstrom uses STDIN/STDOUT pipes, but the same node logic could run over:

| Bus | Framing | Failure modes |
|-----|---------|---------------|
| STDIN/STDOUT (pipes) | Newline-delimited | None (unless process dies) |
| TCP socket | Length-prefixed or delimited | Connection reset, half-open |
| UDP datagram | Natural message boundaries | Loss, reorder, duplication |
| Serial/UART | Start/stop markers + checksum | Corruption, buffer overflow |
| Shared memory | Ring buffer + atomic pointers | Requires memory barriers |
| In-memory (test) | Channel of byte slices | None |

By coding to the interface, your node is portable across all of these.

### Implementations

**StdioTransport** — production Maelstrom transport:

```go
type StdioTransport struct {
    scanner *bufio.Scanner    // wraps io.Reader, splits on \n
    mu      sync.Mutex        // serializes writes
    out     io.Writer         // wraps io.Writer
}
```

- `Recv()`: calls `scanner.Scan()`, returns `scanner.Bytes()`
- `Send()`: locks mutex, writes `data + "\n"`, flushes (if `out` implements `Flusher`)
- Constructor takes `io.Reader` and `io.Writer` — NOT `os.Stdin`/`os.Stdout` directly

**MemTransport** — for unit tests:

```go
type MemTransport struct {
    inbox  chan []byte
    outbox chan []byte
}
```

- `Recv()`: reads from `inbox` channel
- `Send()`: writes to `outbox` channel
- Tests push messages into `inbox`, read responses from `outbox`

### Framing subtlety

The transport owns framing — knowing where one message ends and the next begins. For STDIN/STDOUT, framing is newline-delimited (one JSON object per line, no embedded newlines). For TCP, you'd typically use a 4-byte length prefix. The Codec layer above doesn't care how framing works — it just receives complete byte slices.

### Concurrency

`Send()` must be thread-safe. Once you add goroutine-per-message (Broadcast chapter), multiple goroutines call `Send()` concurrently. The mutex inside the transport serializes writes so messages don't interleave on the wire.

`Recv()` is called from a single goroutine (the main loop), so it doesn't need synchronization.

---

## Subsystem 2: Codec

### Responsibility

Translate between wire bytes and structured `Message` objects. Knows about the message envelope shape (`src`, `dest`, `body`). Does NOT know what the body means or how it arrived.

### Interface

```go
type Codec interface {
    Encode(msg Message) ([]byte, error)
    Decode(data []byte) (Message, error)
}
```

### Why it's separate from Transport

Transport deals with bytes and framing. Codec deals with structure and serialization. Separating them means:

- You can swap JSON for Protobuf or MessagePack without touching the transport
- You can test encoding/decoding without any I/O
- You can add compression or encryption as a Codec wrapper

### Implementation: JSONCodec

```go
type JSONCodec struct{}

func (c JSONCodec) Encode(msg Message) ([]byte, error) {
    return json.Marshal(msg)
}

func (c JSONCodec) Decode(data []byte) (Message, error) {
    var msg Message
    err := json.Unmarshal(data, &msg)
    return msg, err
}
```

### The body stays opaque

The `Message.Body` field is `json.RawMessage` — raw bytes that the Codec does NOT parse. Only the handler (or the Router, for extracting `type`) decodes the body. This is deliberate:

- Different message types have different body shapes
- The Codec shouldn't need to know every possible body type
- Handlers decode only the fields they care about

---

## Subsystem 3: Router

### Responsibility

Map incoming messages to the right handler based on `body.type`. Maintains a registry of handler functions. Does NOT know how messages arrive or how replies are sent.

### Structure

```go
type Router struct {
    handlers map[string]HandlerFunc
}

func (r *Router) Register(typ string, fn HandlerFunc) {
    // panic or error if duplicate
    r.handlers[typ] = fn
}

func (r *Router) Dispatch(typ string, msg Message) error {
    h, ok := r.handlers[typ]
    if !ok {
        return fmt.Errorf("no handler for %q", typ)
    }
    return h(msg)
}
```

### Handler signature

```go
type HandlerFunc func(msg Message) error
```

Handlers receive the full `Message` (envelope + raw body). They decode the body themselves, do their work, and call `Node.Reply()` or `Node.Send()` to respond. Returning an error sends an error response to the client.

### Init is special

The `init` message is handled by the Node itself (to set identity), not by user-registered handlers. The Node intercepts it before routing. Optionally, a user can register an `"init"` handler that runs *after* the Node processes identity — useful for one-time setup.

---

## Subsystem 4: RPC Layer

### Responsibility

Manage outgoing requests that expect responses. Owns message ID generation and callback tracking. Bridges async (callback-based) and sync (blocking) RPC styles.

### Structure

```go
type RPCManager struct {
    mu        sync.Mutex
    nextMsgID int
    callbacks map[int]callbackEntry
}

type callbackEntry struct {
    fn      HandlerFunc
    created time.Time
}
```

### Operations

**Register a callback** (called by `Node.RPC()`):

```go
func (r *RPCManager) NextID() int {
    r.mu.Lock()
    defer r.mu.Unlock()
    r.nextMsgID++
    return r.nextMsgID
}

func (r *RPCManager) Register(msgID int, fn HandlerFunc) {
    r.mu.Lock()
    defer r.mu.Unlock()
    r.callbacks[msgID] = callbackEntry{fn: fn, created: time.Now()}
}
```

**Handle a response** (called by the main loop when `in_reply_to` is set):

```go
func (r *RPCManager) HandleCallback(inReplyTo int, msg Message) error {
    r.mu.Lock()
    entry, ok := r.callbacks[inReplyTo]
    if ok {
        delete(r.callbacks, inReplyTo)
    }
    r.mu.Unlock()

    if !ok {
        return nil  // no callback, ignore (e.g. duplicate response)
    }
    return entry.fn(msg)
}
```

### Async RPC (Node.RPC)

```
Node.RPC(dest, body, callback):
  1. RPCManager.NextID() → msgID
  2. RPCManager.Register(msgID, callback)
  3. Inject msg_id into body
  4. Node.Send(dest, body)
```

The callback fires when a response with matching `in_reply_to` arrives.

### Sync RPC (Node.SyncRPC)

Built on top of async RPC using a channel:

```
Node.SyncRPC(ctx, dest, body):
  1. Create ch := make(chan Message, 1)
  2. Node.RPC(dest, body, func(msg) { ch <- msg })
  3. Select on ch or ctx.Done()
  4. Return message or timeout error
```

This bridges the async world (callbacks) with the sync world (blocking calls). Used by the Datomic transactor to talk to `lin-kv`.

### When you need it

- **Echo:** Not needed. You only reply, never initiate requests.
- **Broadcast:** Async RPC for retry-with-acknowledgement.
- **CRDTs:** Not needed. Fire-and-forget replication.
- **Datomic:** Sync RPC for talking to `lin-kv`.
- **Raft:** Both. Async for AppendEntries/RequestVote, sync for proxying to leader.

---

## Subsystem 5: Scheduler

### Responsibility

Run functions periodically in the background. Drives replication (CRDTs), heartbeats (Raft), and election timeouts (Raft).

### Structure

```go
type Scheduler struct {
    tasks []taskEntry
    wg    sync.WaitGroup
}

type taskEntry struct {
    interval time.Duration
    fn       func()
}

func (s *Scheduler) Every(interval time.Duration, fn func()) {
    s.tasks = append(s.tasks, taskEntry{interval: interval, fn: fn})
}

func (s *Scheduler) Start() {
    for _, task := range s.tasks {
        t := task
        s.wg.Add(1)
        go func() {
            defer s.wg.Done()
            for {
                t.fn()
                time.Sleep(t.interval)
            }
        }()
    }
}
```

### When you need it

- **Echo, Broadcast, Datomic:** Not needed.
- **CRDTs:** `Every(5s, replicateState)` — periodic anti-entropy.
- **Raft:** `Every(100ms, checkElectionTimeout)` and `Every(50ms, replicateLog)`.

---

## Subsystem 6: Node (the compositor)

### Responsibility

Wires all subsystems together. Owns identity. Provides the public API that handlers call. Runs the main loop.

### Structure

```go
type Node struct {
    // Identity
    id      string
    nodeIDs []string

    // Subsystems
    transport Transport
    codec     Codec
    router    *Router
    rpc       *RPCManager
    scheduler *Scheduler

    mu sync.Mutex  // protects identity fields during init
}
```

### Public API

| Method | Used by | Does |
|--------|---------|------|
| `Handle(type, fn)` | main.go | Delegates to Router.Register |
| `Send(dest, body)` | Handlers | Encodes via Codec, sends via Transport |
| `Reply(req, body)` | Handlers | Injects `in_reply_to`, calls Send |
| `RPC(dest, body, cb)` | Handlers | Registers callback, injects `msg_id`, calls Send |
| `SyncRPC(ctx, dest, body)` | Handlers | Blocking wrapper around RPC |
| `Run()` | main.go | Main loop: Recv → decode → route/callback → repeat |
| `ID()` | Handlers | Returns this node's ID |
| `NodeIDs()` | Handlers | Returns all node IDs in cluster |

### The main loop (Node.Run)

```
1. Start scheduler (if any tasks registered)
2. Loop:
   a. Transport.Recv() → raw bytes
   b. Codec.Decode(bytes) → Message
   c. Parse body to extract type and in_reply_to
   d. If in_reply_to > 0:
        → RPCManager.HandleCallback(in_reply_to, msg)  [in a goroutine]
   e. Else if type == "init":
        → handleInit(msg)  — store id/nodeIDs, reply init_ok
   f. Else:
        → Router.Dispatch(type, msg)  [in a goroutine, once you need concurrency]
3. Wait for in-flight goroutines to finish
```

### Goroutine-per-message

For Echo, dispatch is synchronous (step 2f runs in the main goroutine). Starting with Broadcast, you wrap dispatch in `go func()` so handlers can block (e.g., retry loops) without stalling the main loop. The Node owns this decision — subsystems below don't know or care.

---

## Message Flow Diagrams

### Receiving a client request (Echo)

```
STDIN
  → Transport.Recv()           → []byte
  → Codec.Decode(bytes)        → Message{Src:"c1", Dest:"n1", Body:{type:"echo",...}}
  → parse body.type            → "echo"
  → Router.Dispatch("echo")    → echoHandler(msg)
  → handler calls Node.Reply() → injects in_reply_to
  → Codec.Encode(response)     → []byte
  → Transport.Send(bytes)      → STDOUT
```

### Sending an async RPC (Broadcast retries)

```
handler calls Node.RPC("n2", body, callback)
  → RPCManager.NextID()        → 7
  → RPCManager.Register(7, cb)
  → inject msg_id:7 into body
  → Codec.Encode(msg)          → []byte
  → Transport.Send(bytes)      → STDOUT

later, response arrives on STDIN:
  → Transport.Recv()           → []byte
  → Codec.Decode(bytes)        → Message{Body:{in_reply_to:7,...}}
  → parse in_reply_to          → 7
  → RPCManager.HandleCallback(7, msg)
  → callback(msg)              → e.g., remove from unacked set
```

### Sync RPC (Datomic talking to lin-kv)

```
handler calls Node.SyncRPC(ctx, "lin-kv", readBody)
  → creates ch
  → Node.RPC("lin-kv", readBody, func(msg) { ch <- msg })
  → blocks on select { case msg := <-ch; case <-ctx.Done() }

response arrives (same as async flow above)
  → callback sends msg to ch
  → SyncRPC unblocks, returns msg
```

---

## Testability

Each subsystem is independently testable because of the interface boundaries.

### Transport tests

```go
// Test StdioTransport with in-memory reader/writer
in := strings.NewReader("{\"src\":\"c1\",...}\n")
out := &bytes.Buffer{}
t := NewStdioTransport(in, out)

data, err := t.Recv()   // returns the JSON line
t.Send([]byte("..."))   // check out.String()
```

### Codec tests

```go
// Pure encode/decode, no I/O
codec := JSONCodec{}
msg := Message{Src: "n1", Dest: "c1", Body: json.RawMessage(`{"type":"echo_ok"}`)}
bytes, _ := codec.Encode(msg)
decoded, _ := codec.Decode(bytes)
// assert decoded == msg
```

### Router tests

```go
// No I/O, no transport
r := NewRouter()
called := false
r.Register("echo", func(msg Message) error { called = true; return nil })
r.Dispatch("echo", someMsg)
// assert called == true
```

### RPC Manager tests

```go
// Register callback, simulate response
rpc := NewRPCManager()
id := rpc.NextID()
var got Message
rpc.Register(id, func(msg Message) error { got = msg; return nil })
rpc.HandleCallback(id, fakeResponse)
// assert got == fakeResponse
```

### Node integration tests

```go
// Full node with MemTransport — no OS, no Maelstrom
in := make(chan []byte, 10)
out := make(chan []byte, 10)
t := &MemTransport{inbox: in, outbox: out}

node := NewNode(t, JSONCodec{})
node.Handle("echo", echoHandler)

// Push init + echo messages
in <- []byte(`{"src":"c1","dest":"n1","body":{"type":"init","msg_id":1,"node_id":"n1","node_ids":["n1"]}}`)
in <- []byte(`{"src":"c1","dest":"n1","body":{"type":"echo","msg_id":2,"echo":"hi"}}`)
close(in)

node.Run()

// Read responses from out channel, assert correctness
```

---

## File Layout

```
node/
├── transport.go          // Transport interface + StdioTransport
├── transport_test.go
├── codec.go              // Codec interface + JSONCodec
├── codec_test.go
├── message.go            // Message, MessageBody, InitBody structs
├── message_test.go
├── router.go             // Router: handler registration + dispatch
├── router_test.go
├── rpc.go                // RPCManager: msg_id gen, callbacks (add for Broadcast)
├── rpc_test.go
├── scheduler.go          // Scheduler: periodic tasks (add for CRDTs)
├── scheduler_test.go
├── node.go               // Node: wires everything, public API, main loop
├── node_test.go
└── errors.go             // RPCError types (add for Datomic)

cmd/
├── echo/
│   └── main.go
├── broadcast/
│   └── main.go
├── g-set/
│   └── main.go
└── raft/
    └── main.go
```

---

## Incremental Build Order

What to implement for each workload. Only build what you need.

### 1. Echo

| File | What to build |
|------|--------------|
| `message.go` | `Message`, `MessageBody`, `InitBody` structs |
| `transport.go` | `Transport` interface, `StdioTransport` (Recv, Send, Close) |
| `codec.go` | `Codec` interface, `JSONCodec` (Encode, Decode) |
| `router.go` | `Router` (Register, Dispatch) |
| `node.go` | `Node` with Handle, Send, Reply, Run (single-goroutine main loop, init handling) |
| `cmd/echo/main.go` | Register echo handler, call Run |

### 2. Broadcast (adds RPC + concurrency)

| File | What to add |
|------|------------|
| `rpc.go` | `RPCManager` (NextID, Register, HandleCallback) |
| `node.go` | `Node.RPC()`, goroutine-per-message in Run loop, callback dispatch |

### 3. CRDTs (adds Scheduler)

| File | What to add |
|------|------------|
| `scheduler.go` | `Scheduler` (Every, Start) |
| `node.go` | Start scheduler in Run |

### 4. Datomic (adds SyncRPC + errors)

| File | What to add |
|------|------------|
| `node.go` | `Node.SyncRPC()` |
| `errors.go` | `RPCError` type with codes (crash, txn-conflict, key-does-not-exist, etc.) |

### 5. Raft (uses everything)

No new subsystems. All the infrastructure is in place. The complexity is in the handler logic (election state machine, log replication, commit tracking), not in the node framework.

---

## Design Principles Summary

| Principle | How it's applied |
|-----------|-----------------|
| **Dependency Inversion** | Node depends on Transport/Codec interfaces, not STDIN/JSON |
| **Single Responsibility** | Each subsystem does one thing: Transport moves bytes, Codec translates, Router dispatches, RPC tracks callbacks, Scheduler runs timers |
| **Interface Segregation** | Transport doesn't know about messages. Codec doesn't know about handlers. Router doesn't know about I/O |
| **Open-Closed** | New message types are added by registering handlers, not by modifying the Node or Router |
| **Incremental Growth** | Each workload adds one subsystem. No refactoring of existing code |
| **Testability** | Every subsystem is testable in isolation via interfaces and dependency injection |
