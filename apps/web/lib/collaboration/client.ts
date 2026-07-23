import * as Y from "yjs";

import type { Document } from "@/lib/ast/schema";
import {
  getYjsAstRoot,
  materializeYjsAst,
  syncYjsAst,
} from "@/lib/collaboration/yjs-ast";

const REMOTE_ORIGIN = Symbol("collaboration-remote");
const CLIENT_ID_KEY = "anbywiki.collaboration.client-id";
const SEQUENCE_KEY_PREFIX = "anbywiki.collaboration.sequence.";
const INITIAL_RECONNECT_DELAY_MS = 500;
const MAX_RECONNECT_DELAY_MS = 10_000;

export type CollaborationStatus =
  | "connecting"
  | "syncing"
  | "online"
  | "offline"
  | "closed";

export interface CollaborationClientOptions {
  pageId: string;
  initialAst: Document;
  onAst: (ast: Document) => void;
  onStatus?: (status: CollaborationStatus) => void;
  socketFactory?: (url: string) => WebSocket;
}

export interface PreparedAstUpdate {
  expectedSequence: number;
  update: Uint8Array;
}

/**
 * Browser adapter for collaboration protocol v1.
 *
 * HTTP APIs still use the generated client. WebSocket binary framing is a
 * separate versioned contract and is contained entirely in this adapter.
 */
export class CollaborationClient {
  readonly ydoc = new Y.Doc();

  private readonly options: CollaborationClientOptions;
  private readonly clientId: string;
  private socket: WebSocket | null = null;
  private ready = false;
  private closed = false;
  private latestSequence = 0;
  private reconnectAttempts = 0;
  private reconnectTimer: ReturnType<typeof setTimeout> | null = null;
  private receiveQueue = Promise.resolve();
  private pendingAst: Document | null = null;
  private documentId: string | null = null;

  constructor(options: CollaborationClientOptions) {
    this.options = options;
    this.clientId = browserClientId();
    this.latestSequence = readSequence(options.pageId);
    this.ydoc.on("update", this.onDocumentUpdate);
  }

  connect(): void {
    if (this.closed || this.socket) return;
    this.options.onStatus?.("connecting");
    const socket = (this.options.socketFactory ?? defaultSocketFactory)(
      collaborationURL(this.options.pageId, this.clientId, this.latestSequence),
    );
    socket.binaryType = "arraybuffer";
    socket.onopen = () => this.options.onStatus?.("syncing");
    socket.onmessage = (event) => {
      this.receiveQueue = this.receiveQueue
        .then(() => this.receive(event.data))
        .catch(() => {
          if (this.socket === socket) {
            socket.close(1002, "invalid collaboration message");
          }
        });
    };
    socket.onerror = () => this.options.onStatus?.("offline");
    socket.onclose = (event) => {
      if (this.socket !== socket) return;
      this.socket = null;
      this.ready = false;
      if (this.closed) return;
      if (event.code === 1008) {
        this.closed = true;
        this.options.onStatus?.("closed");
        return;
      }
      this.options.onStatus?.("offline");
      this.scheduleReconnect();
    };
    this.socket = socket;
  }

  syncAst(ast: Document): void {
    if (!this.ready) {
      this.pendingAst = ast;
      return;
    }
    syncYjsAst(this.ydoc, ast, "editor");
  }

  getDocumentId(): string | null {
    return this.documentId;
  }

  getClientId(): string {
    return this.clientId;
  }

  getLatestSequence(): number {
    return this.latestSequence;
  }

  getAst(): Document {
    return materializeYjsAst(this.ydoc);
  }

  /**
   * Produces a delta without mutating the live document. Governance can submit
   * it with expectedSequence and only apply it after the server CAS succeeds.
   */
  prepareAstUpdate(ast: Document): PreparedAstUpdate {
    if (!this.ready) {
      throw new Error("collaboration document is not ready");
    }
    const stateVector = Y.encodeStateVector(this.ydoc);
    const clone = new Y.Doc();
    try {
      Y.applyUpdate(clone, Y.encodeStateAsUpdate(this.ydoc), REMOTE_ORIGIN);
      syncYjsAst(clone, ast, "governance-merge");
      return {
        expectedSequence: this.latestSequence,
        update: Y.encodeStateAsUpdate(clone, stateVector),
      };
    } finally {
      clone.destroy();
    }
  }

  sendPresence(cursor: Record<string, unknown>): void {
    if (!this.ready || this.socket?.readyState !== WebSocket.OPEN) return;
    this.socket.send(JSON.stringify({ type: "presence", cursor }));
  }

  close(): void {
    this.closed = true;
    this.ready = false;
    if (this.reconnectTimer) clearTimeout(this.reconnectTimer);
    this.reconnectTimer = null;
    this.ydoc.off("update", this.onDocumentUpdate);
    this.socket?.close(1000, "editor closed");
    this.socket = null;
    this.options.onStatus?.("closed");
    this.ydoc.destroy();
  }

  private readonly onDocumentUpdate = (
    update: Uint8Array,
    origin: unknown,
  ): void => {
    if (
      origin === REMOTE_ORIGIN ||
      !this.ready ||
      this.socket?.readyState !== WebSocket.OPEN
    ) {
      return;
    }
    const updateID = uuidBytes(crypto.randomUUID());
    const frame = new Uint8Array(updateID.length + update.length);
    frame.set(updateID);
    frame.set(update, updateID.length);
    this.socket.send(frame);
  };

  private async receive(value: string | ArrayBuffer | Blob): Promise<void> {
    if (typeof value === "string") {
      this.receiveJSON(value);
      return;
    }
    if (value instanceof Blob) {
      this.receiveBinary(await value.arrayBuffer());
      return;
    }
    this.receiveBinary(value);
  }

  private receiveJSON(value: string): void {
    const message = JSON.parse(value) as {
      type?: string;
      document_id?: string;
      latest_sequence?: number;
    };
    if (message.type === "hello") {
      if (!message.document_id || !isUUID(message.document_id)) {
        throw new Error("invalid collaboration hello message");
      }
      this.documentId = message.document_id;
      return;
    }
    if (message.type !== "ready") return;
    if (
      !Number.isSafeInteger(message.latest_sequence) ||
      (message.latest_sequence ?? -1) < 0
    ) {
      throw new Error("invalid collaboration ready message");
    }
    this.latestSequence = message.latest_sequence!;
    writeSequence(this.options.pageId, this.latestSequence);
    this.ready = true;
    this.reconnectAttempts = 0;
    const hasRecoveredState =
      getYjsAstRoot(this.ydoc).size > 0 || this.latestSequence > 0;
    const pendingAst = this.pendingAst;
    this.pendingAst = null;
    if (!hasRecoveredState) {
      syncYjsAst(
        this.ydoc,
        pendingAst ?? this.options.initialAst,
        pendingAst ? "editor" : "initial",
      );
    } else if (pendingAst) {
      syncYjsAst(this.ydoc, pendingAst, "editor");
    }
    if (hasRecoveredState || pendingAst) {
      this.options.onAst(materializeYjsAst(this.ydoc));
    }
    this.options.onStatus?.("online");
  }

  private receiveBinary(buffer: ArrayBuffer): void {
    const frame = new Uint8Array(buffer);
    if (frame.length <= 9 || (frame[0] !== 1 && frame[0] !== 2)) {
      throw new Error("invalid collaboration server frame");
    }
    const rawSequence = new DataView(buffer, 1, 8).getBigUint64(0);
    if (rawSequence > BigInt(Number.MAX_SAFE_INTEGER)) {
      throw new Error("collaboration sequence exceeds safe integer range");
    }
    const sequence = Number(rawSequence);
    Y.applyUpdate(this.ydoc, frame.slice(9), REMOTE_ORIGIN);
    this.latestSequence = Math.max(this.latestSequence, sequence);
    writeSequence(this.options.pageId, this.latestSequence);
    if (this.ready) {
      this.options.onAst(materializeYjsAst(this.ydoc));
    }
  }

  private scheduleReconnect(): void {
    if (this.closed || this.reconnectTimer) return;
    const delay = Math.min(
      INITIAL_RECONNECT_DELAY_MS * 2 ** this.reconnectAttempts,
      MAX_RECONNECT_DELAY_MS,
    );
    this.reconnectAttempts += 1;
    this.reconnectTimer = setTimeout(() => {
      this.reconnectTimer = null;
      this.connect();
    }, delay);
  }
}

function collaborationURL(
  pageId: string,
  clientId: string,
  lastSequence: number,
): string {
  const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
  const url = new URL(
    `/api/v1/pages/${encodeURIComponent(pageId)}/collaboration`,
    `${protocol}//${window.location.host}`,
  );
  url.searchParams.set("client_id", clientId);
  url.searchParams.set("last_sequence", String(lastSequence));
  return url.toString();
}

function defaultSocketFactory(url: string): WebSocket {
  return new WebSocket(url);
}

function browserClientId(): string {
  const existing = window.sessionStorage.getItem(CLIENT_ID_KEY);
  if (existing) return existing;
  const id = crypto.randomUUID();
  window.sessionStorage.setItem(CLIENT_ID_KEY, id);
  return id;
}

function readSequence(pageId: string): number {
  const value = Number(
    window.sessionStorage.getItem(SEQUENCE_KEY_PREFIX + pageId) ?? "0",
  );
  return Number.isSafeInteger(value) && value >= 0 ? value : 0;
}

function writeSequence(pageId: string, sequence: number): void {
  window.sessionStorage.setItem(
    SEQUENCE_KEY_PREFIX + pageId,
    String(sequence),
  );
}

function uuidBytes(value: string): Uint8Array {
  const hex = value.replaceAll("-", "");
  if (!/^[0-9a-f]{32}$/i.test(hex)) throw new Error("invalid UUID");
  return Uint8Array.from(
    Array.from({ length: 16 }, (_, index) =>
      Number.parseInt(hex.slice(index * 2, index * 2 + 2), 16),
    ),
  );
}

function isUUID(value: string): boolean {
  return /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i.test(
    value,
  );
}
