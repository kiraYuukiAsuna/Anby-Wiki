# Collaboration Protocol v1

Endpoint:

`GET /api/v1/pages/{page_id}/collaboration?client_id={uuid}&last_sequence={n}`

The HTTP upgrade uses the normal server-side session. Development actor headers
remain development-only and are stripped by Nginx.

## Server Messages

1. A JSON `hello` message described by `message.schema.json`.
2. An optional binary snapshot frame.
3. Zero or more binary update frames.
4. Live binary update and JSON presence messages.

Server binary frame:

| Bytes | Value |
|---|---|
| `0` | `1` snapshot or `2` update |
| `1..8` | unsigned big-endian durable server sequence |
| `9..` | opaque Yjs payload |

Client update frame:

| Bytes | Value |
|---|---|
| `0..15` | UUID client update idempotency key |
| `16..` | opaque Yjs update |

The server never parses Yjs bytes. A duplicate `(document, client, update)` is
idempotent; reusing the key with different bytes closes the connection.

## Presence

Clients send `{"type":"presence","cursor":{...}}`. The server adds the
authenticated `actor_id`. Presence is limited to 4 KiB, is not persisted, and
is absent after process restart.

## Reconnect

Clients persist the last applied server sequence. If the cursor predates
compaction, the server sends the latest snapshot and subsequent updates.
Otherwise it sends only updates after the cursor. Repeated Yjs updates are safe
and expected around the subscribe/recovery race window.
