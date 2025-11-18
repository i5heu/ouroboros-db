const DB_NAME = 'ouroboros.v2';
const DB_VERSION = 2;
const THREAD_STORE = 'threadSummaries';
const NODE_STORE = 'threadNodes';

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

export type ThreadNodeRecord = {
    key: string; // server key or generated local key
    rootKey: string; // the thread's root summary key
    parent?: string; // parent key
    mimeType: string;
    isText: boolean;
    sizeBytes: number;
    preview?: string;
    content?: string; // plain text (non-encoded)
    encodedContent?: string; // base64 for attachment or original data
    createdAt?: string;
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
            if (!db.objectStoreNames.contains(NODE_STORE)) {
                const store = db.createObjectStore(NODE_STORE, { keyPath: 'key' });
                store.createIndex('rootKey', 'rootKey', { unique: false });
                store.createIndex('parent', 'parent', { unique: false });
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

export const writeThreadNodes = async (
    db: IDBDatabase | null,
    records: ThreadNodeRecord[]
) => {
    if (!db || records.length === 0) {
        return;
    }
    await new Promise<void>((resolve, reject) => {
        const tx = db.transaction(NODE_STORE, 'readwrite');
        const store = tx.objectStore(NODE_STORE);
        records.forEach((record) => {
            store.put({ ...record, cachedAt: record.cachedAt ?? Date.now() });
        });
        tx.oncomplete = () => resolve();
        tx.onerror = () => reject(tx.error ?? new Error('Failed to write thread nodes.'));
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

export const readThreadNodes = async (
    db: IDBDatabase | null,
    rootKey: string
): Promise<ThreadNodeRecord[]> => {
    if (!db || !rootKey) return [];
    return await new Promise<ThreadNodeRecord[]>((resolve, reject) => {
        try {
            const tx = db.transaction(NODE_STORE, 'readonly');
            const store = tx.objectStore(NODE_STORE);
            const index = store.index('rootKey');
            const request = index.getAll(IDBKeyRange.only(rootKey));
            request.onsuccess = () => {
                resolve(request.result as ThreadNodeRecord[]);
            };
            request.onerror = () => reject(request.error ?? new Error('Failed to read thread nodes.'));
        } catch (error) {
            reject(error);
        }
    });
};

export const deleteThreadNodes = async (db: IDBDatabase | null, keys: string[]) => {
    if (!db || keys.length === 0) return;
    await new Promise<void>((resolve, reject) => {
        const tx = db.transaction(NODE_STORE, 'readwrite');
        const store = tx.objectStore(NODE_STORE);
        keys.forEach((k) => {
            store.delete(k);
        });
        tx.oncomplete = () => resolve();
        tx.onerror = () => reject(tx.error ?? new Error('Failed to delete thread nodes.'));
    });
};
