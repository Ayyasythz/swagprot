# swagprot — Swagger for gRPC

`swagprot` is a Go library (with a CLI) that does for gRPC what Swagger UI does
for REST: it introspects an API and serves an interactive web UI to **browse
services and methods and invoke RPCs straight from the browser** ("try it out"),
including streaming methods.

It needs **no generated stubs** for the target API. Descriptors are resolved at
runtime from any of:

- **gRPC Server Reflection** — point it at a running server.
- **`.proto` files** — compiled in-process (standard imports included automatically).
- **A compiled `FileDescriptorSet`** — e.g. from `protoc --include_imports`.

Requests and responses are carried as dynamic messages and converted to/from
proto3-JSON, so every proto feature (enums, maps, oneofs, well-known types,
nesting) is supported.

## Install

```sh
go install github.com/Ayyasythz/swagprot/cmd/swagprot@latest
```

## CLI usage

```sh
# Reflection-based, plaintext server:
swagprot --addr localhost:50051 --plaintext

# Browse from .proto files, invoke against a live server:
swagprot --proto api/greeter.proto --import-path api --addr localhost:50051 --plaintext

# From a precompiled descriptor set (browse-only without --addr):
swagprot --descriptor api.binpb --addr localhost:50051 --plaintext
```

Then open <http://localhost:8080>.

Key flags: `--addr`, `--proto`/`--import-path`, `--descriptor`, `--plaintext`,
`--cacert`/`--cert`/`--key`/`--insecure`/`--authority` (TLS), `-H "key: value"`
(default request metadata), `--port`, `--open`.

Invocation requires `--addr`. With only `--proto`/`--descriptor` the UI is
browse-only.

## Library usage

`swagprot.NewServer` returns an `http.Handler` you can mount anywhere:

```go
srv, err := swagprot.NewServer(ctx, swagprot.Config{
    Dial: swagprot.DialOptions{Address: "localhost:50051", Plaintext: true},
})
if err != nil { log.Fatal(err) }
defer srv.Close()
http.ListenAndServe(":8080", srv)
```

`Config` highlights:

- `Dial` — how to reach the gRPC server (address, plaintext/TLS/mTLS).
- `Conn` — reuse an existing `*grpc.ClientConn` instead of dialing (keeps your
  credentials/interceptors; swagprot won't close it).
- `ProtoFiles` / `ImportPaths` / `DescriptorSet` — browse from static sources.
- `BasePath` — mount the UI/API under a sub-path (e.g. `/swagger`).
- `DefaultMetadata` — pre-fill the request metadata box.

## Embedding in a service (`/swagger` endpoint)

To expose a Swagger-style UI for your gRPC API from inside an existing service
(the shape of [`nocturna-ta/ums`](https://github.com/nocturna-ta/ums): a Fiber
REST API plus a gRPC server), build the handler with a `BasePath` and mount it.

### Fiber (fasthttp)

```go
swag, err := swagprot.NewServer(ctx, swagprot.Config{
    Dial:     swagprot.DialOptions{Address: "localhost:35000", Plaintext: true}, // ums gRPC port
    BasePath: "/swagger",
})
if err != nil { log.Fatal(err) }
defer swag.Close()

// Fiber passes the full path to the net/http handler; BasePath strips it.
app.Use("/swagger", adaptor.HTTPHandler(swag)) // middleware/adaptor
```

Open `http://localhost:8900/swagger/`. Browse + unary "try it out" work fully.

> **Streaming note.** Fiber is built on fasthttp, whose net/http adaptor cannot
> upgrade WebSockets. swagprot detects this and the UI **automatically falls back
> to a buffered HTTP endpoint** (`POST /api/invoke-stream`) — streaming RPCs still
> work, but responses arrive together at the end instead of live. For live
> streaming, use one of the options below.

Runnable example: [`examples/embed-fiber`](examples/embed-fiber) (its own module).

### net/http, chi, gin, … (full live streaming)

With any `net/http`-based router, WebSocket streaming works with full fidelity:

```go
mux := http.NewServeMux()
mux.Handle("/swagger/", swag) // swag built with BasePath: "/swagger"
mux.Handle("/swagger", swag)
```

Runnable example: [`examples/embed-http`](examples/embed-http).

### Sidecar (no code changes)

Or run swagprot as its own process pointing at the gRPC server — full streaming,
nothing to wire in:

```sh
swagprot --addr localhost:35000 --plaintext --port 8901
```

## Try the example

```sh
# terminal 1 — a tiny gRPC server with reflection enabled
go run ./examples/greeter

# terminal 2 — serve the UI against it
go run ./cmd/swagprot --addr localhost:50051 --plaintext --open
```

Invoke `greet.Greeter.SayHello` (unary) and `greet.Greeter.SayHelloStream`
(server streaming) from the browser.

## HTTP API

The UI is built on a small JSON API you can also use directly:

| Method & path            | Purpose                                            |
| ------------------------ | -------------------------------------------------- |
| `GET /api/config`        | `{canInvoke, defaultMetadata}`                     |
| `GET /api/services`      | Service/method navigation tree                     |
| `GET /api/method?name=…` | Method metadata + expanded request form schema     |
| `POST /api/invoke`       | Unary call: `{method, request, metadata}`          |
| `GET /api/stream` (WS)   | Live streaming calls: send a start frame, receive frames |
| `POST /api/invoke-stream`| Buffered streaming fallback: all messages returned at once |

All paths are relative to `BasePath` when one is configured (e.g.
`/swagger/api/services`).

## Architecture

```
DescriptorSource ──▶ Schema (form schema) ──▶ UI
        │                                       │
        └──────────────▶ Invocation engine ◀────┘
                         (dynamicpb + protojson)
```

- `internal/descsource` — reflection / `.proto` / descriptor-set sources behind one interface.
- `internal/schema`    — walks message descriptors into a JSON form schema.
- `internal/invoke`    — dials the target and runs unary/streaming RPCs dynamically.
- `internal/web`       — JSON API, WebSocket streaming, and the embedded vanilla-JS UI.

## Status / roadmap

- v1 renders an editable proto3-JSON request (pre-filled from the schema) plus a
  schema view. Structured per-field form inputs are a planned enhancement.
- Bidirectional streaming uses a non-interactive "send all, then receive" model.
- Static HTML docs export is not yet implemented.
