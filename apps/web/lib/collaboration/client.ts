import * as Y from "yjs";

import { ResponseError } from "../../../../contracts/generated/typescript";
import { readingApi } from "@/lib/api";
import type { Document } from "@/lib/ast/schema";
import { clientUUID } from "@/lib/client-uuid";
import { OfflineStore, newOfflineRecord, type PendingUpdate } from "@/lib/collaboration/offline-store";
import { encodePresencePosition, resolvePresencePosition, type EditorCursor, type PresenceCursor } from "@/lib/collaboration/presence";
import {
  getYjsAstRoot,
  materializeYjsAst,
  syncYjsAst,
} from "@/lib/collaboration/yjs-ast";

const REMOTE_ORIGIN = Symbol("collaboration-remote");
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
  | "forbidden"
  | "closed";

export interface CollaborationClientOptions {
  pageId: string;
  actorId: string;
  initialAst: Document;
  onAst: (ast: Document, cursor?: EditorCursor) => void;
  onStatus?: (status: CollaborationStatus) => void;
  onPresence?: (presence: CollaborationPresence) => void;
  onStorageError?: () => void;
  socketFactory?: (url: string) => WebSocket;
}

export interface CollaborationPresence {
  actorId: string;
  clientId: string;
  actorUsername?: string;
  actorDisplayName?: string;
  cursor: Record<string, unknown>;
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
  private clientId: string;
  private readonly offline: OfflineStore;
  private readonly restored: Promise<void>;
  private persistenceQueue = Promise.resolve();
  private storageFailed = false;
  private socket: WebSocket | null = null;
  private ready = false;
  private closed = false;
  private connecting = false;
  private latestSequence = 0;
  private reconnectAttempts = 0;
  private reconnectTimer: ReturnType<typeof setTimeout> | null = null;
  private receiveQueue = Promise.resolve();
  private pendingAst: Document | null = null;
  private readonly pendingUpdates: PendingUpdate[] = [];
  private documentId: string | null = null;
  private renderedAstJSON: string;
  private lastPresence: EditorCursor | null = null;
  private readonly presenceHeartbeat: ReturnType<typeof setInterval>;
  private lastSnapshotSequence = 0;
  private snapshotInFlight = false;

  constructor(options: CollaborationClientOptions) {
    this.options = options;
    this.clientId = newOfflineRecord().clientId;
    this.offline = new OfflineStore(options.actorId, options.pageId);
    this.restored = this.restoreOffline();
    this.renderedAstJSON = JSON.stringify(options.initialAst);
    this.ydoc.on("update", this.onDocumentUpdate);
    this.presenceHeartbeat = setInterval(
      () => this.transmitPresence(),
      PRESENCE_HEARTBEAT_MS,
    );
  }

  async connect(): Promise<void> {
    if (this.closed || this.socket || this.connecting) return;
    this.connecting = true;
    this.options.onStatus?.("connecting");
    try {
      await this.restored;
      if (this.closed) return;
      const page = await readingApi().getPageByID({
        id: this.options.pageId,
        contentMode: "sections",
      });
      if (this.closed) return;
      if (!page.canEdit) {
        this.denyAccess();
        return;
      }
    } catch (error) {
      if (this.closed) return;
      if (
        error instanceof ResponseError &&
        [401, 403, 404, 410].includes(error.response.status)
      ) {
        this.denyAccess();
      } else {
        this.options.onStatus?.("offline");
        this.scheduleReconnect();
      }
      return;
    } finally {
      this.connecting = false;
    }
    if (this.closed) return;
    const socket = (this.options.socketFactory ?? defaultSocketFactory)(
      collaborationURL(this.options.pageId, this.clientId, this.latestSequence),
    );
    socket.binaryType = "arraybuffer";
    socket.onopen = () => {
      if (!this.closed && this.socket === socket) this.options.onStatus?.("syncing");
    };
    socket.onmessage = (event) => {
      this.receiveQueue = this.receiveQueue
        .then(() => {
          if (!this.closed && this.socket === socket) return this.receive(event.data);
        })
        .catch(() => {
          if (this.socket === socket) {
            socket.close(1002, "invalid collaboration message");
          }
        });
    };
    socket.onerror = () => {
      if (!this.closed && this.socket === socket) this.options.onStatus?.("offline");
    };
    socket.onclose = (event) => {
      if (this.socket !== socket) return;
      this.socket = null;
      this.ready = false;
      this.snapshotInFlight = false;
      if (this.closed) return;
      if (event.code === 1008) {
        this.denyAccess();
        return;
      }
      this.options.onStatus?.("offline");
      this.scheduleReconnect();
    };
    this.socket = socket;
  }

  syncAst(ast: Document): void {
    if (this.closed) return;
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

  hasWorkingState(): boolean {
    return getYjsAstRoot(this.ydoc).size > 0;
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

  sendPresence(cursor: EditorCursor): void {
    this.lastPresence = cursor;
    this.transmitPresence();
  }

  close(): void {
    if (this.closed) return;
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
    void this.persistenceQueue.finally(() => this.offline.close());
  }

  private denyAccess(): void {
    this.close();
    this.pendingAst = null;
    this.pendingUpdates.length = 0;
    this.options.onStatus?.("forbidden");
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
    // The send follows the durable write, so a lost acknowledgement cannot
    // lose its original idempotency key on browser restart.
    void this.persist(pending, null).then(() => this.sendPendingUpdate(pending));
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
      client_id?: string;
      actor_username?: string;
      actor_display_name?: string;
      cursor?: unknown;
      up_to_sequence?: number;
    };
    if (message.type === "hello") {
      if (!message.document_id || !isUUID(message.document_id)) {
        throw new Error("invalid collaboration hello message");
      }
      if (this.documentId && this.documentId !== message.document_id) {
        this.options.onStorageError?.();
        this.denyAccess();
        return;
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
        clientId: message.client_id ?? message.actor_id,
        actorUsername: cleanPresenceLabel(message.actor_username),
        actorDisplayName: cleanPresenceLabel(message.actor_display_name),
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
    this.latestSequence = Math.max(this.latestSequence, message.latest_sequence!);
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
    } else if (pendingAst && !this.recoveredOffline) {
      syncYjsAst(this.ydoc, pendingAst, "editor");
    }
    this.transmitPresence();
    this.maybeSaveSnapshot();
    if (hasRecoveredState || pendingAst) {
      this.emitAstIfChanged();
    }
    this.options.onStatus?.("online");
    void this.persist(null, null);
  }

  private receiveBinary(buffer: ArrayBuffer): void {
    if (this.closed) return;
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
    if (this.ready && frame[0] === 2 && sequence > this.latestSequence + 1) {
      this.refresh();
      return;
    }
    if (frame[0] === 1) {
      this.lastSnapshotSequence = Math.max(
        this.lastSnapshotSequence,
        sequence,
      );
    } else {
      this.acknowledgePendingUpdate(update);
    }
    const cursor = this.lastPresence ? this.presenceCursor(this.lastPresence) : null;
    Y.applyUpdate(this.ydoc, update, REMOTE_ORIGIN);
    if (cursor) this.lastPresence = resolvePresencePosition(this.ydoc, cursor);
    this.latestSequence = Math.max(this.latestSequence, sequence);
    void this.persist(null, null);
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
    this.options.onAst(ast, this.lastPresence ?? undefined);
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
    void this.persistenceQueue.then(() => {
      for (const pending of this.pendingUpdates) this.sendPendingUpdate(pending);
    });
  }

  private acknowledgePendingUpdate(update: Uint8Array): void {
    const index = this.pendingUpdates.findIndex((pending) =>
      equalBytes(pending.update, update),
    );
    if (index >= 0) {
      const [acknowledged] = this.pendingUpdates.splice(index, 1);
      void this.persist(null, acknowledged.id);
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
        JSON.stringify({
          type: "presence",
          cursor: this.presenceCursor(this.lastPresence),
        }),
      );
    } catch {
      // Presence is ephemeral; the heartbeat retries after reconnect.
    }
  }

  private presenceCursor(cursor: EditorCursor): PresenceCursor {
    return {
      version: 1,
      block_id: cursor.head.blockId,
      selection: "text",
      anchor: encodePresencePosition(this.ydoc, cursor.anchor),
      head: encodePresencePosition(this.ydoc, cursor.head),
    };
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

  resolveCursor(cursor: Record<string, unknown>): EditorCursor | null {
    return resolvePresencePosition(this.ydoc, cursor);
  }

  private recoveredOffline = false;

  private async restoreOffline(): Promise<void> {
    try {
      const record = await this.offline.load();
      if (this.closed || !record) return;
      const validation = new Y.Doc();
      try {
        Y.applyUpdate(validation, record.state, REMOTE_ORIGIN);
        if (getYjsAstRoot(validation).size > 0) materializeYjsAst(validation);
      } finally {
        validation.destroy();
      }
      this.clientId = record.clientId;
      this.documentId = record.documentId;
      this.latestSequence = record.sequence;
      Y.applyUpdate(this.ydoc, record.state, REMOTE_ORIGIN);
      this.pendingUpdates.push(...record.pending);
      this.recoveredOffline = getYjsAstRoot(this.ydoc).size > 0;
      if (this.recoveredOffline) this.emitAstIfChanged();
    } catch {
      this.reportStorageError();
    }
  }

  private persist(added: PendingUpdate | null, acknowledged: Uint8Array | null): Promise<void> {
    const record = {
      clientId: this.clientId,
      documentId: this.documentId,
      sequence: this.latestSequence,
      state: Y.encodeStateAsUpdate(this.ydoc),
    };
    this.persistenceQueue = this.persistenceQueue.then(
      () => this.offline.save(record, added, acknowledged),
    ).catch(() => this.reportStorageError());
    return this.persistenceQueue;
  }

  private reportStorageError(): void {
    if (this.storageFailed) return;
    this.storageFailed = true;
    this.options.onStorageError?.();
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

function cleanPresenceLabel(value: unknown): string | undefined {
  return typeof value === "string" && value.trim() ? value.trim() : undefined;
}

function defaultSocketFactory(url: string): WebSocket {
  return new WebSocket(url);
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
