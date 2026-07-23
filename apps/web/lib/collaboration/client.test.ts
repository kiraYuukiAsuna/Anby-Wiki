import { afterEach, describe, expect, it, vi } from "vitest";
import * as Y from "yjs";

import { parseDocument, type Document } from "@/lib/ast/schema";
import {
  applyYjsState,
  createYjsAstDocument,
  materializeYjsAst,
} from "@/lib/collaboration/yjs-ast";

import {
  CollaborationClient,
  type CollaborationStatus,
} from "./client";

class MockWebSocket {
  readonly url: string;
  binaryType: BinaryType = "blob";
  readyState: number = WebSocket.CONNECTING;
  onopen: ((event: Event) => void) | null = null;
  onmessage: ((event: MessageEvent) => void) | null = null;
  onerror: ((event: Event) => void) | null = null;
  onclose: ((event: CloseEvent) => void) | null = null;
  sent: Array<string | ArrayBufferLike | Blob | ArrayBufferView> = [];
  closedWith: [number, string] | null = null;

  constructor(url: string) {
    this.url = url;
  }

  open(): void {
    this.readyState = WebSocket.OPEN;
    this.onopen?.(new Event("open"));
  }

  message(data: string | ArrayBuffer | Blob): void {
    this.onmessage?.(new MessageEvent("message", { data }));
  }

  disconnect(code = 1006): void {
    this.readyState = WebSocket.CLOSED;
    this.onclose?.(new CloseEvent("close", { code }));
  }

  send(data: string | ArrayBufferLike | Blob | ArrayBufferView): void {
    this.sent.push(data);
  }

  close(code = 1000, reason = ""): void {
    this.closedWith = [code, reason];
    this.readyState = WebSocket.CLOSED;
  }
}

const initialAst = parseDocument({
  type: "document",
  schema_version: 1,
  children: [
    {
      id: "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4d31",
      type: "paragraph",
      content: [{ type: "text", text: "initial" }],
    },
  ],
});

afterEach(() => {
  vi.useRealTimers();
  window.sessionStorage.clear();
});

describe("CollaborationClient", () => {
  it("initializes an empty working document only after ready", async () => {
    const sockets: MockWebSocket[] = [];
    const statuses: CollaborationStatus[] = [];
    const onAst = vi.fn();
    const client = createClient(sockets, onAst, statuses);

    client.connect();
    sockets[0].open();
    expect(sockets[0].sent).toHaveLength(0);

    const documentId = "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4d33";
    sockets[0].message(JSON.stringify({
      type: "hello",
      document_id: documentId,
    }));
    sockets[0].message(JSON.stringify({ type: "ready", latest_sequence: 0 }));
    await flushMessages();

    expect(statuses).toEqual(["connecting", "syncing", "online"]);
    expect(client.getDocumentId()).toBe(documentId);
    expect(onAst).not.toHaveBeenCalled();
    expect(sockets[0].sent).toHaveLength(1);
    expect(sockets[0].sent[0]).toBeInstanceOf(Uint8Array);
    client.close();
  });

  it("preserves and sends edits made while recovery is in progress", async () => {
    const sockets: MockWebSocket[] = [];
    const onAst = vi.fn();
    const client = createClient(sockets, onAst);
    const edited = changeText(initialAst, "edited while syncing");

    client.connect();
    sockets[0].open();
    client.syncAst(edited);
    sockets[0].message(JSON.stringify({ type: "ready", latest_sequence: 0 }));
    await flushMessages();

    expect(onAst).toHaveBeenLastCalledWith(edited);
    expect(sockets[0].sent).toHaveLength(1);
    client.close();
  });

  it("applies recovered binary state before announcing online", async () => {
    const sockets: MockWebSocket[] = [];
    const onAst = vi.fn();
    const client = createClient(sockets, onAst);
    const recovered = changeText(initialAst, "recovered");
    const remote = createYjsAstDocument(recovered);

    client.connect();
    sockets[0].open();
    sockets[0].message(serverFrame(2, 1, Y.encodeStateAsUpdate(remote)));
    sockets[0].message(JSON.stringify({ type: "ready", latest_sequence: 1 }));
    await flushMessages();

    expect(onAst).toHaveBeenLastCalledWith(recovered);
    expect(sockets[0].sent).toHaveLength(0);
    client.close();
  });

  it("reconnects with the last durable sequence after an abnormal close", async () => {
    vi.useFakeTimers();
    const sockets: MockWebSocket[] = [];
    const client = createClient(sockets, vi.fn());
    const remote = createYjsAstDocument(initialAst);

    client.connect();
    sockets[0].open();
    sockets[0].message(serverFrame(2, 7, Y.encodeStateAsUpdate(remote)));
    sockets[0].message(JSON.stringify({ type: "ready", latest_sequence: 7 }));
    await flushMessages();
    sockets[0].disconnect();

    await vi.advanceTimersByTimeAsync(500);
    expect(sockets).toHaveLength(2);
    expect(sockets[1].url).toContain("last_sequence=7");
    client.close();
  });

  it("closes the socket when a protocol message is invalid", async () => {
    const sockets: MockWebSocket[] = [];
    const client = createClient(sockets, vi.fn());

    client.connect();
    sockets[0].open();
    sockets[0].message("{");
    await flushMessages();

    expect(sockets[0].closedWith).toEqual([
      1002,
      "invalid collaboration message",
    ]);
    client.close();
  });

  it("prepares an AST delta against the current durable sequence", async () => {
    const sockets: MockWebSocket[] = [];
    const client = createClient(sockets, vi.fn());
    const remote = createYjsAstDocument(initialAst);
    client.connect();
    sockets[0].open();
    sockets[0].message(serverFrame(2, 4, Y.encodeStateAsUpdate(remote)));
    sockets[0].message(JSON.stringify({ type: "ready", latest_sequence: 4 }));
    await flushMessages();

    const merged = changeText(initialAst, "AI merged");
    const prepared = client.prepareAstUpdate(merged);
    const result = new Y.Doc();
    applyYjsState(result, Y.encodeStateAsUpdate(client.ydoc));
    applyYjsState(result, prepared.update);

    expect(prepared.expectedSequence).toBe(4);
    expect(materializeYjsAst(result)).toEqual(merged);
    expect(materializeYjsAst(client.ydoc)).toEqual(initialAst);
    client.close();
  });
});

function createClient(
  sockets: MockWebSocket[],
  onAst: (ast: Document) => void,
  statuses?: CollaborationStatus[],
): CollaborationClient {
  return new CollaborationClient({
    pageId: "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4d32",
    initialAst,
    onAst,
    onStatus: (status) => statuses?.push(status),
    socketFactory: (url) => {
      const socket = new MockWebSocket(url);
      sockets.push(socket);
      return socket as unknown as WebSocket;
    },
  });
}

function changeText(ast: Document, text: string): Document {
  const changed = structuredClone(ast);
  const block = changed.children[0] as {
    content: Array<{ type: "text"; text: string }>;
  };
  block.content[0].text = text;
  return changed;
}

function serverFrame(
  kind: 1 | 2,
  sequence: number,
  payload: Uint8Array,
): ArrayBuffer {
  const frame = new Uint8Array(9 + payload.length);
  frame[0] = kind;
  new DataView(frame.buffer).setBigUint64(1, BigInt(sequence));
  frame.set(payload, 9);
  return frame.buffer;
}

async function flushMessages(): Promise<void> {
  for (let index = 0; index < 10; index += 1) {
    await Promise.resolve();
  }
}
