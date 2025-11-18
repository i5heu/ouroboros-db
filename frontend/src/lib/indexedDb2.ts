const DB_NAME = 'ouroboros.v2';
const DB_VERSION = 1;
const THREAD_STORE = 'threadSummaries';

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
