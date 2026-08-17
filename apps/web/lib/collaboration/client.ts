import * as Y from "yjs";

import type { Document } from "@/lib/ast/schema";
import { clientUUID } from "@/lib/client-uuid";
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
const PRESENCE_HEARTBEAT_MS = 10_000;
const SNAPSHOT_UPDATE_INTERVAL = 100;
const MAX_SNAPSHOT_BYTES = 16 << 20;

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
  onPresence?: (presence: CollaborationPresence) => void;
  socketFactory?: (url: string) => WebSocket;
}

export interface CollaborationPresence {
  actorId: string;
  cursor: Record<string, unknown>;
}

export interface PreparedAstUpdate {
  expectedSequence: number;
  update: Uint8Array;
}

interface PendingUpdate {
  id: Uint8Array;
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
  private readonly pendingUpdates: PendingUpdate[] = [];
  private documentId: string | null = null;
  private renderedAstJSON: string;
  private lastPresence: Record<string, unknown> | null = null;
  private readonly presenceHeartbeat: ReturnType<typeof setInterval>;
  private lastSnapshotSequence = 0;
  private snapshotInFlight = false;

  constructor(options: CollaborationClientOptions) {
    this.options = options;
    this.clientId = browserClientId();
    this.latestSequence = readSequence(options.pageId);
    this.renderedAstJSON = JSON.stringify(options.initialAst);
    this.ydoc.on("update", this.onDocumentUpdate);
    this.presenceHeartbeat = setInterval(
      () => this.transmitPresence(),
      PRESENCE_HEARTBEAT_MS,
    );
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
      this.snapshotInFlight = false;
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
    // The editor already renders this AST. Remember it before the server echoes
    // the Yjs update so that an acknowledgement cannot remount the editor.
    this.renderedAstJSON = JSON.stringify(ast);
    if (!this.ready && getYjsAstRoot(this.ydoc).size === 0) {
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

  isReady(): boolean {
    return this.ready;
  }

  hasPendingUpdates(): boolean {
    return this.pendingUpdates.length > 0;
  }

  refresh(): void {
    if (this.closed) return;
    this.ready = false;
    if (this.socket) {
      this.socket.close(1012, "refresh collaboration state");
      return;
    }
    this.connect();
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
    this.lastPresence = cursor;
    this.transmitPresence();
  }

  close(): void {
    this.closed = true;
    this.ready = false;
    if (this.reconnectTimer) clearTimeout(this.reconnectTimer);
    this.reconnectTimer = null;
    clearInterval(this.presenceHeartbeat);
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
    if (origin === REMOTE_ORIGIN) return;
    const pending = {
      id: uuidBytes(clientUUID()),
      update: update.slice(),
    };
    this.pendingUpdates.push(pending);
    this.sendPendingUpdate(pending);
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
      actor_id?: string;
      cursor?: unknown;
      up_to_sequence?: number;
    };
    if (message.type === "hello") {
      if (!message.document_id || !isUUID(message.document_id)) {
        throw new Error("invalid collaboration hello message");
      }
      this.documentId = message.document_id;
      return;
    }
    if (message.type === "presence") {
      if (
        !message.actor_id ||
        !isUUID(message.actor_id) ||
        !isRecord(message.cursor)
      ) {
        throw new Error("invalid collaboration presence message");
      }
      this.options.onPresence?.({
        actorId: message.actor_id,
        cursor: message.cursor,
      });
      return;
    }
    if (message.type === "snapshot_saved") {
      if (
        !Number.isSafeInteger(message.up_to_sequence) ||
        (message.up_to_sequence ?? -1) < 0
      ) {
        throw new Error("invalid collaboration snapshot result");
      }
      this.lastSnapshotSequence = Math.max(
        this.lastSnapshotSequence,
        message.up_to_sequence!,
      );
      this.snapshotInFlight = false;
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
    this.flushPendingUpdates();
    if (!hasRecoveredState) {
      syncYjsAst(
        this.ydoc,
        pendingAst ?? this.options.initialAst,
        pendingAst ? "editor" : "initial",
      );
    } else if (pendingAst) {
      syncYjsAst(this.ydoc, pendingAst, "editor");
    }
    this.transmitPresence();
    this.maybeSaveSnapshot();
    if (hasRecoveredState || pendingAst) {
      this.emitAstIfChanged();
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
    const update = frame.slice(9);
    if (frame[0] === 1) {
      this.lastSnapshotSequence = Math.max(
        this.lastSnapshotSequence,
        sequence,
      );
    } else {
      this.acknowledgePendingUpdate(update);
    }
    Y.applyUpdate(this.ydoc, update, REMOTE_ORIGIN);
    this.latestSequence = Math.max(this.latestSequence, sequence);
    writeSequence(this.options.pageId, this.latestSequence);
    if (this.ready) {
      this.emitAstIfChanged();
      this.maybeSaveSnapshot();
    }
  }

  private emitAstIfChanged(): void {
    const ast = materializeYjsAst(this.ydoc);
    const astJSON = JSON.stringify(ast);
    if (astJSON === this.renderedAstJSON) return;
    this.renderedAstJSON = astJSON;
    this.options.onAst(ast);
  }

  private sendPendingUpdate(pending: PendingUpdate): void {
    if (!this.ready || this.socket?.readyState !== WebSocket.OPEN) return;
    const frame = new Uint8Array(pending.id.length + pending.update.length);
    frame.set(pending.id);
    frame.set(pending.update, pending.id.length);
    try {
      this.socket.send(frame);
    } catch {
      // Keep the stable idempotency key queued for the next reconnect.
    }
  }

  private flushPendingUpdates(): void {
    for (const pending of this.pendingUpdates) {
      this.sendPendingUpdate(pending);
    }
  }

  private acknowledgePendingUpdate(update: Uint8Array): void {
    const index = this.pendingUpdates.findIndex((pending) =>
      equalBytes(pending.update, update),
    );
    if (index >= 0) {
      this.pendingUpdates.splice(index, 1);
    }
  }

  private transmitPresence(): void {
    if (
      !this.lastPresence ||
      !this.ready ||
      this.socket?.readyState !== WebSocket.OPEN
    ) {
      return;
    }
    try {
      this.socket.send(
        JSON.stringify({ type: "presence", cursor: this.lastPresence }),
      );
    } catch {
      // Presence is ephemeral; the heartbeat retries after reconnect.
    }
  }

  private maybeSaveSnapshot(): void {
    if (
      !this.ready ||
      this.snapshotInFlight ||
      this.pendingUpdates.length > 0 ||
      this.latestSequence - this.lastSnapshotSequence <
      SNAPSHOT_UPDATE_INTERVAL ||
      this.socket?.readyState !== WebSocket.OPEN
    ) {
      return;
    }
    const state = Y.encodeStateAsUpdate(this.ydoc);
    if (state.length === 0 || state.length > MAX_SNAPSHOT_BYTES) {
      return;
    }
    this.snapshotInFlight = true;
    try {
      this.socket.send(
        JSON.stringify({
          type: "snapshot",
          up_to_sequence: this.latestSequence,
          state: bytesToBase64(state),
          compact: true,
        }),
      );
    } catch {
      this.snapshotInFlight = false;
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
  const id = clientUUID();
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

function isRecord(value: unknown): value is Record<string, unknown> {
  return value !== null && typeof value === "object" && !Array.isArray(value);
}

function equalBytes(left: Uint8Array, right: Uint8Array): boolean {
  if (left.length !== right.length) return false;
  for (let index = 0; index < left.length; index += 1) {
    if (left[index] !== right[index]) return false;
  }
  return true;
}

function bytesToBase64(value: Uint8Array): string {
  const chunks: string[] = [];
  const chunkSize = 0x8000;
  for (let index = 0; index < value.length; index += chunkSize) {
    chunks.push(
      String.fromCharCode(...value.subarray(index, index + chunkSize)),
    );
  }
  return btoa(chunks.join(""));
}
