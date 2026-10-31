# Collaboration Protocol v1

Endpoint:

`GET /api/v1/pages/{page_id}/collaboration?client_id={uuid}&last_sequence={n}`

The HTTP upgrade uses the normal server-side session. Development actor headers
remain development-only; production configuration rejects the feature and the
Go API strips spoofed identity headers.

## Server Messages

1. A JSON `hello` message described by `message.schema.json`.
2. An optional binary snapshot frame.
3. Zero or more binary update frames.
4. Live binary update, JSON presence, and `snapshot_saved` messages.

Server binary frame:

| Bytes | Value |
| --- | --- |
| `0` | `1` snapshot or `2` update |
| `1..8` | unsigned big-endian durable server sequence |
| `9..` | opaque Yjs payload |

Client update frame:

| Bytes | Value |
| --- | --- |
| `0..15` | UUID client update idempotency key |
| `16..` | opaque Yjs update |

The server never parses Yjs bytes. A duplicate `(document, client, update)` is
idempotent; reusing the key with different bytes closes the connection.

## Presence

Clients send `{"type":"presence","cursor":{...}}`. The server adds the
authenticated `actor_id`, `actor_username`, `actor_display_name` and `client_id`.
These identity fields are never taken from the client payload.
The cursor uses version 1 with `anchor` and `head`; each has a stable AST
`block_id`, a UTF-16 `offset` (inline atoms occupy one position), and an optional
base64 Yjs `relative_position` limited to 256 decoded bytes. Legacy block-only
cursors remain accepted. The editor resolves relative positions after concurrent
edits and renders named caret/selection decorations; deleted positions disappear.
Presence is limited to 4 KiB and is not persisted. A 10-second heartbeat repeats
the latest cursor; the server excludes the sending connection and remote entries
expire after 30 seconds.

## Reconnect

Clients atomically persist Yjs state together with the last contiguous applied
server sequence, WorkingDocument ID, stable client ID and unacknowledged updates
in IndexedDB, keyed by authenticated Actor and Page. Concurrent local tabs merge
their state and pending update sets rather than overwriting each other.
If the cursor predates
compaction, the server sends the latest snapshot and subsequent updates.
Otherwise it sends only updates after the cursor. Repeated Yjs updates are safe
and expected around the subscribe/recovery race window.

Each local Yjs update keeps the same client update UUID until the server echoes
the persisted payload. Unacknowledged updates remain queued across transient
WebSocket reconnects and browser closure/reload/restart, merge with recovered
remote updates in the local Y.Doc, and are then resent idempotently. Recovery
uses the saved CRDT rather than replaying an older AST draft. Storage failures
are reported; only committed local transactions are durable. Loading the editor
still requires the Web application and authentication to be reachable.

## Multiple API Instances

API instances sharing PostgreSQL and the same Redis database broadcast on
`anbywiki:collaboration:v1:<redis-db>`. The explicit DB suffix is necessary because
Redis Pub/Sub itself ignores logical database boundaries. An instance UUID
prevents its own Redis broadcasts from being echoed locally. Slow subscribers
and Redis receive/publish failures disconnect editors so they recover durable
updates instead of retaining a state with missing messages.

Every 10 seconds the connection also checks PostgreSQL for updates after its
contiguous sequence, covering a process crash after commit but before publish.
Recovery reads document metadata, snapshot and updates in one repeatable-read,
read-only transaction so concurrent snapshot compaction cannot hide an update.
Live sequence gaps cause clients to reconnect. Connection stickiness is not
required; ephemeral presence is restored by heartbeat after reconnect.

## Snapshot and Compaction

After 100 durable updates and only when no local update is awaiting an echo,
the browser sends:

`{"type":"snapshot","up_to_sequence":100,"state":"<base64 Yjs state>","compact":true}`

The authenticated service stores at most 16 MiB of opaque Yjs state and removes
covered updates in the same transaction. It then broadcasts
`{"type":"snapshot_saved","up_to_sequence":100}`. A reconnect cursor older than
that sequence receives the snapshot followed by later updates. Snapshot bytes
and document content are never logged.

## Publish CAS

An HTTP publish that includes `working_document_id` must also include the
durable `expected_sequence` from which the browser materialized `ast`. The API
locks the Page and WorkingDocument in one transaction and rejects a sequence
mismatch with HTTP 409 before creating a Revision or rebasing the document.
