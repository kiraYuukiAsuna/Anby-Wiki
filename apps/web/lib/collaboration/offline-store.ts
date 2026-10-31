import * as Y from "yjs";
import { z } from "zod";

import { clientUUID } from "@/lib/client-uuid";

const DATABASE = "anbywiki.collaboration";
const STORE = "documents";
const bytesSchema = z.custom<Uint8Array>((value) => value instanceof Uint8Array);
const recordSchema = z.object({
  clientId: z.string().uuid(),
  documentId: z.string().uuid().nullable(),
  sequence: z.number().int().nonnegative().max(Number.MAX_SAFE_INTEGER),
  state: bytesSchema,
  pending: z.array(z.object({
    id: bytesSchema.refine((value) => value.length === 16),
    update: bytesSchema,
  })),
});

export type OfflineRecord = z.infer<typeof recordSchema>;
export type PendingUpdate = OfflineRecord["pending"][number];

/**
 * State, contiguous sequence and unacknowledged updates commit together.
 * Read/merge/write transactions also preserve edits from other local tabs.
 * The actor/page key prevents recovery under a different logged-in account.
 */
export class OfflineStore {
  private readonly database: Promise<IDBDatabase>;
  private readonly key: string;

  constructor(actorId: string, pageId: string) {
    this.key = `${actorId}.${pageId}`;
    this.database = new Promise((resolve, reject) => {
      const request = indexedDB.open(DATABASE, 1);
      request.onupgradeneeded = () => request.result.createObjectStore(STORE);
      request.onsuccess = () => resolve(request.result);
      request.onerror = () => reject(request.error);
      request.onblocked = () => reject(new Error("offline database blocked"));
    });
  }

  async load(): Promise<OfflineRecord | null> {
    const database = await this.database;
    return new Promise((resolve, reject) => {
      const transaction = database.transaction(STORE, "readwrite", { durability: "strict" });
      const store = transaction.objectStore(STORE);
      const request = store.get(this.key);
      let record: OfflineRecord;
      request.onsuccess = () => {
        try {
          record = request.result ? recordSchema.parse(request.result) : newOfflineRecord();
          if (!request.result) store.put(record, this.key);
        } catch (error) {
          reject(error);
          transaction.abort();
        }
      };
      transaction.oncomplete = () => resolve(record);
      transaction.onabort = () => reject(transaction.error);
      request.onerror = () => reject(request.error);
    });
  }

  async save(
    record: Omit<OfflineRecord, "pending">,
    added: PendingUpdate | null,
    acknowledged: Uint8Array | null,
  ): Promise<void> {
    const database = await this.database;
    return new Promise((resolve, reject) => {
      const transaction = database.transaction(STORE, "readwrite", { durability: "strict" });
      const store = transaction.objectStore(STORE);
      const request = store.get(this.key);
      request.onsuccess = () => {
        try {
          const previous = request.result ? recordSchema.parse(request.result) : null;
          if (previous?.documentId && previous.documentId !== record.documentId) {
            throw new Error("offline working document changed");
          }
          const pending = previous?.pending ?? [];
          if (added && !pending.some((item) => sameId(item.id, added.id))) pending.push(added);
          store.put({
            ...record,
            clientId: previous?.clientId ?? record.clientId,
            sequence: Math.max(previous?.sequence ?? 0, record.sequence),
            state: previous ? Y.mergeUpdates([previous.state, record.state]) : record.state,
            pending: acknowledged
              ? pending.filter((item) => !sameId(item.id, acknowledged))
              : pending,
          }, this.key);
        } catch (error) {
          reject(error);
          transaction.abort();
        }
      };
      transaction.oncomplete = () => resolve();
      transaction.onabort = () => reject(transaction.error);
      transaction.onerror = () => reject(transaction.error);
    });
  }

  close(): void {
    void this.database.then((database) => database.close(), () => {});
  }
}

export function newOfflineRecord(): OfflineRecord {
  const document = new Y.Doc();
  const state = Y.encodeStateAsUpdate(document);
  document.destroy();
  return {
    clientId: clientUUID(),
    documentId: null,
    sequence: 0,
    state,
    pending: [],
  };
}

function sameId(left: Uint8Array, right: Uint8Array): boolean {
  return left.length === right.length && left.every((value, index) => value === right[index]);
}
