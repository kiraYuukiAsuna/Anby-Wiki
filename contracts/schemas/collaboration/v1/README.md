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
authenticated `actor_id`. Presence is limited to 4 KiB, is not persisted, and
is absent after process restart. The browser sends a heartbeat for its latest
stable Block ID; the server does not echo presence to the sender. Remote entries
expire in the editor after 30 seconds.

## Reconnect

Clients persist the last applied server sequence. If the cursor predates
compaction, the server sends the latest snapshot and subsequent updates.
Otherwise it sends only updates after the cursor. Repeated Yjs updates are safe
and expected around the subscribe/recovery race window.

Each local Yjs update keeps the same client update UUID until the server echoes
the persisted payload. Unacknowledged updates remain queued across transient
WebSocket reconnects, merge with recovered remote updates in the local Y.Doc,
and are then resent idempotently.

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
