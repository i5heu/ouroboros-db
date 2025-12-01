const DB_NAME = 'ouroboros.v2';
const DB_VERSION = 3;
const THREAD_STORE = 'threadSummaries';
const MESSAGE_STORE = 'messages';
const LEGACY_NODE_STORE = 'threadNodes';

export type ThreadSummaryRecord = {
    key: string;
    preview: string;
    mimeType: string;
    isText: boolean;
    sizeBytes: number;
    childCount: number;
    createdAt?: string;
    cachedAt: number;
};

export type MessageRecord = {
    key: string;
    resolvedKey?: string;
    suggestedEdit?: string;
    editOf?: string;
    mimeType: string;
    isText: boolean;
    sizeBytes: number;
    content?: string;
    encodedContent?: string;
    createdAt?: string;
    attachmentName?: string;
    title?: string;
    cachedAt?: number;
};

const openRequest = (): Promise<IDBDatabase | null> =>
    new Promise((resolve, reject) => {
        if (typeof indexedDB === 'undefined') {
            resolve(null);
            return;
        }
        const request = indexedDB.open(DB_NAME, DB_VERSION);
        request.onupgradeneeded = () => {
            const db = request.result;
            if (!db.objectStoreNames.contains(THREAD_STORE)) {
                db.createObjectStore(THREAD_STORE, { keyPath: 'key' });
            }
            if (db.objectStoreNames.contains(LEGACY_NODE_STORE)) {
                db.deleteObjectStore(LEGACY_NODE_STORE);
            }
            if (!db.objectStoreNames.contains(MESSAGE_STORE)) {
                db.createObjectStore(MESSAGE_STORE, { keyPath: 'key' });
            }
        };
        request.onsuccess = () => {
            resolve(request.result);
        };
        request.onerror = () => {
            reject(request.error ?? new Error('Failed to open IndexedDB.'));
        };
    });

export const openIndexedDb2 = openRequest;

export const writeThreadSummaries = async (
    db: IDBDatabase | null,
    records: ThreadSummaryRecord[]
) => {
    if (!db || records.length === 0) {
        return;
    }
    await new Promise<void>((resolve, reject) => {
        const tx = db.transaction(THREAD_STORE, 'readwrite');
        const store = tx.objectStore(THREAD_STORE);
        records.forEach((record) => {
            store.put({ ...record, cachedAt: record.cachedAt ?? Date.now() });
        });
        tx.oncomplete = () => resolve();
        tx.onerror = () => reject(tx.error ?? new Error('Failed to write thread summaries.'));
    });
};

export const writeMessageRecords = async (db: IDBDatabase | null, records: MessageRecord[]) => {
    if (!db || records.length === 0) {
        return;
    }
    await new Promise<void>((resolve, reject) => {
        const tx = db.transaction(MESSAGE_STORE, 'readwrite');
        const store = tx.objectStore(MESSAGE_STORE);
        records.forEach((record) => {
            store.put({ ...record, cachedAt: record.cachedAt ?? Date.now() });
        });
        tx.oncomplete = () => resolve();
        tx.onerror = () => reject(tx.error ?? new Error('Failed to write messages.'));
    });
};

export const readThreadSummaries = async (db: IDBDatabase | null): Promise<ThreadSummaryRecord[]> => {
    if (!db) {
        return [];
    }
    return await new Promise<ThreadSummaryRecord[]>((resolve, reject) => {
        const tx = db.transaction(THREAD_STORE, 'readonly');
        const store = tx.objectStore(THREAD_STORE);
        const request = store.getAll();
        request.onsuccess = () => {
            resolve(request.result as ThreadSummaryRecord[]);
        };
        request.onerror = () => reject(request.error ?? new Error('Failed to read thread summaries.'));
    });
};

export const readMessageRecord = async (
    db: IDBDatabase | null,
    key: string
): Promise<MessageRecord | null> => {
    if (!db || !key) return null;
    return await new Promise<MessageRecord | null>((resolve, reject) => {
        const tx = db.transaction(MESSAGE_STORE, 'readonly');
        const store = tx.objectStore(MESSAGE_STORE);
        const request = store.get(key);
        request.onsuccess = () => {
            resolve((request.result as MessageRecord) ?? null);
        };
        request.onerror = () => reject(request.error ?? new Error('Failed to read message.'));
    });
};

export const readMessageRecords = async (
    db: IDBDatabase | null,
    keys: string[]
): Promise<Map<string, MessageRecord>> => {
    if (!db || keys.length === 0) return new Map();
    return await new Promise<Map<string, MessageRecord>>((resolve, reject) => {
        const tx = db.transaction(MESSAGE_STORE, 'readonly');
        const store = tx.objectStore(MESSAGE_STORE);
        const result = new Map<string, MessageRecord>();
        let remaining = keys.length;
        const checkDone = () => {
            if (remaining === 0) {
                resolve(result);
            }
        };
        keys.forEach((key) => {
            const request = store.get(key);
            request.onsuccess = () => {
                if (request.result) {
                    result.set(key, request.result as MessageRecord);
                }
                remaining -= 1;
                checkDone();
            };
            request.onerror = () => {
                reject(request.error ?? new Error('Failed to read message.'));
            };
        });
    });
};

export const deleteMessageRecords = async (db: IDBDatabase | null, keys: string[]) => {
    if (!db || keys.length === 0) return;
    await new Promise<void>((resolve, reject) => {
        const tx = db.transaction(MESSAGE_STORE, 'readwrite');
        const store = tx.objectStore(MESSAGE_STORE);
        keys.forEach((key) => store.delete(key));
        tx.oncomplete = () => resolve();
        tx.onerror = () => reject(tx.error ?? new Error('Failed to delete messages.'));
    });
};
