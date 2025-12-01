/**
 * Cache Worker - All API requests go through this worker
 * Uses IndexedDB as a cache layer with cache-first strategy
 * Returns cached data immediately, then verifies and updates in background
 */

import {
    streamBulkMessages,
    streamThreadSummaries,
    streamThreadNodes,
    lookupByComputedId,
    getComputedId,
    type BulkDataRecord,
    type ThreadSummaryPayload,
    type ThreadNodePayload,
    type LookupByComputedIdResult,
    type GetComputedIdResult
} from '../apiClient';

const DB_NAME = 'ouroboros.cache.v1';
const DB_VERSION = 1;

const STORES = {
    MESSAGES: 'messages',
    THREAD_SUMMARIES: 'threadSummaries',
    THREAD_NODES: 'threadNodes',
    COMPUTED_ID_LOOKUP: 'computedIdLookup',
    KEY_TO_COMPUTED_ID: 'keyToComputedId'
} as const;

// Cache TTL in milliseconds (5 minutes for regular data, 1 hour for computed IDs)
const CACHE_TTL = 5 * 60 * 1000;
const COMPUTED_ID_CACHE_TTL = 60 * 60 * 1000;

type CachedRecord<T> = {
    data: T;
    cachedAt: number;
};

type MessageCacheRecord = CachedRecord<BulkDataRecord> & { key: string };
type ThreadSummaryCacheRecord = CachedRecord<ThreadSummaryPayload> & { key: string };
type ThreadNodeCacheRecord = CachedRecord<ThreadNodePayload> & { key: string };
type ComputedIdLookupCacheRecord = CachedRecord<LookupByComputedIdResult> & { computedId: string };
type KeyToComputedIdCacheRecord = CachedRecord<GetComputedIdResult> & { key: string };

// Request types
type BaseRequest = {
    requestId: number;
    apiBaseUrl: string;
    headers: Record<string, string>;
};

type GetMessagesRequest = BaseRequest & {
    type: 'get-messages';
    keys: string[];
    includeBinary?: boolean;
};

type GetThreadSummariesRequest = BaseRequest & {
    type: 'get-thread-summaries';
    cursor?: string | null;
    limit?: number;
};

type GetThreadNodesRequest = BaseRequest & {
    type: 'get-thread-nodes';
    rootKey: string;
};

type LookupComputedIdRequest = BaseRequest & {
    type: 'lookup-computed-id';
    computedId: string;
};

type GetComputedIdRequest = BaseRequest & {
    type: 'get-computed-id';
    key: string;
};

type CacheRequest =
    | GetMessagesRequest
    | GetThreadSummariesRequest
    | GetThreadNodesRequest
    | LookupComputedIdRequest
    | GetComputedIdRequest;

// Response types
type MessageResponse = {
    type: 'message';
    requestId: number;
    record: BulkDataRecord;
    fromCache: boolean;
};

type ThreadSummaryResponse = {
    type: 'thread-summary';
    requestId: number;
    summary: ThreadSummaryPayload;
    fromCache: boolean;
};

type ThreadNodeResponse = {
    type: 'thread-node';
    requestId: number;
    node: ThreadNodePayload;
    fromCache: boolean;
};

type LookupComputedIdResponse = {
    type: 'lookup-computed-id-result';
    requestId: number;
    result: LookupByComputedIdResult;
    fromCache: boolean;
};

type GetComputedIdResponse = {
    type: 'get-computed-id-result';
    requestId: number;
    result: GetComputedIdResult;
    fromCache: boolean;
};

type CursorResponse = {
    type: 'cursor';
    requestId: number;
    cursor: string | null;
};

type CompleteResponse = {
    type: 'complete';
    requestId: number;
};

type ErrorResponse = {
    type: 'error';
    requestId: number;
    message: string;
};

type CacheVerifiedResponse = {
    type: 'cache-verified';
    requestId: number;
    key: string;
    changed: boolean;
    newData?: unknown;
};

type CacheResponse =
    | MessageResponse
    | ThreadSummaryResponse
    | ThreadNodeResponse
    | LookupComputedIdResponse
    | GetComputedIdResponse
    | CursorResponse
    | CompleteResponse
    | ErrorResponse
    | CacheVerifiedResponse;

const ctx = self as unknown as Worker;

const postMessage = (message: CacheResponse) => {
    ctx.postMessage(message);
};

let db: IDBDatabase | null = null;

const openDatabase = (): Promise<IDBDatabase> => {
    return new Promise((resolve, reject) => {
        if (db) {
            resolve(db);
            return;
        }

        const request = indexedDB.open(DB_NAME, DB_VERSION);

        request.onupgradeneeded = () => {
            const database = request.result;

            if (!database.objectStoreNames.contains(STORES.MESSAGES)) {
                database.createObjectStore(STORES.MESSAGES, { keyPath: 'key' });
            }
            if (!database.objectStoreNames.contains(STORES.THREAD_SUMMARIES)) {
                database.createObjectStore(STORES.THREAD_SUMMARIES, { keyPath: 'key' });
            }
            if (!database.objectStoreNames.contains(STORES.THREAD_NODES)) {
                database.createObjectStore(STORES.THREAD_NODES, { keyPath: 'key' });
            }
            if (!database.objectStoreNames.contains(STORES.COMPUTED_ID_LOOKUP)) {
                database.createObjectStore(STORES.COMPUTED_ID_LOOKUP, { keyPath: 'computedId' });
            }
            if (!database.objectStoreNames.contains(STORES.KEY_TO_COMPUTED_ID)) {
                database.createObjectStore(STORES.KEY_TO_COMPUTED_ID, { keyPath: 'key' });
            }
        };

        request.onsuccess = () => {
            db = request.result;
            resolve(db);
        };

        request.onerror = () => {
            reject(request.error ?? new Error('Failed to open cache database'));
        };
    });
};

const getCached = async <T>(
    storeName: string,
    key: string
): Promise<CachedRecord<T> | null> => {
    const database = await openDatabase();
    return new Promise((resolve, reject) => {
        const tx = database.transaction(storeName, 'readonly');
        const store = tx.objectStore(storeName);
        const request = store.get(key);

        request.onsuccess = () => {
            resolve(request.result ?? null);
        };
        request.onerror = () => {
            reject(request.error);
        };
    });
};

const getMultipleCached = async <T>(
    storeName: string,
    keys: string[]
): Promise<Map<string, CachedRecord<T>>> => {
    if (keys.length === 0) return new Map();

    const database = await openDatabase();
    return new Promise((resolve, reject) => {
        const tx = database.transaction(storeName, 'readonly');
        const store = tx.objectStore(storeName);
        const results = new Map<string, CachedRecord<T>>();
        let remaining = keys.length;

        keys.forEach((key) => {
            const request = store.get(key);
            request.onsuccess = () => {
                if (request.result) {
                    results.set(key, request.result);
                }
                remaining--;
                if (remaining === 0) resolve(results);
            };
            request.onerror = () => {
                reject(request.error);
            };
        });
    });
};

const setCached = async <T>(
    storeName: string,
    keyField: string,
    key: string,
    data: T
): Promise<void> => {
    const database = await openDatabase();
    return new Promise((resolve, reject) => {
        const tx = database.transaction(storeName, 'readwrite');
        const store = tx.objectStore(storeName);
        const record = {
            [keyField]: key,
            data,
            cachedAt: Date.now()
        };
        const request = store.put(record);

        request.onsuccess = () => resolve();
        request.onerror = () => reject(request.error);
    });
};

const setMultipleCached = async <T>(
    storeName: string,
    keyField: string,
    items: Array<{ key: string; data: T }>
): Promise<void> => {
    if (items.length === 0) return;

    const database = await openDatabase();
    return new Promise((resolve, reject) => {
        const tx = database.transaction(storeName, 'readwrite');
        const store = tx.objectStore(storeName);
        const now = Date.now();

        items.forEach(({ key, data }) => {
            const record = {
                [keyField]: key,
                data,
                cachedAt: now
            };
            store.put(record);
        });

        tx.oncomplete = () => resolve();
        tx.onerror = () => reject(tx.error);
    });
};

const isCacheValid = (cachedAt: number, ttl: number = CACHE_TTL): boolean => {
    return Date.now() - cachedAt < ttl;
};

// Handle get-messages request with cache-first strategy
const handleGetMessages = async (request: GetMessagesRequest) => {
    const { requestId, apiBaseUrl, headers, keys, includeBinary } = request;

    try {
        // 1. Check cache first
        const cached = await getMultipleCached<BulkDataRecord>(STORES.MESSAGES, keys);
        const uncachedKeys: string[] = [];
        const staleKeys: string[] = [];

        for (const key of keys) {
            const cachedRecord = cached.get(key);
            if (cachedRecord) {
                // Return cached data immediately
                postMessage({
                    type: 'message',
                    requestId,
                    record: cachedRecord.data,
                    fromCache: true
                });

                // Check if stale and needs background refresh
                if (!isCacheValid(cachedRecord.cachedAt)) {
                    staleKeys.push(key);
                }
            } else {
                uncachedKeys.push(key);
            }
        }

        // 2. Fetch uncached keys from API
        const keysToFetch = [...uncachedKeys, ...staleKeys];
        if (keysToFetch.length > 0) {
            const newRecords: Array<{ key: string; data: BulkDataRecord }> = [];

            await streamBulkMessages({
                apiBaseUrl,
                keys: keysToFetch,
                includeBinary,
                getHeaders: async () => headers,
                onRecord: (record) => {
                    if (record.found) {
                        newRecords.push({ key: record.key, data: record });

                        // If it was uncached, send to client
                        if (uncachedKeys.includes(record.key)) {
                            postMessage({
                                type: 'message',
                                requestId,
                                record,
                                fromCache: false
                            });
                        } else {
                            // It was stale - notify client of potential update
                            const oldCached = cached.get(record.key);
                            const changed = JSON.stringify(oldCached?.data) !== JSON.stringify(record);
                            if (changed) {
                                postMessage({
                                    type: 'cache-verified',
                                    requestId,
                                    key: record.key,
                                    changed: true,
                                    newData: record
                                });
                            }
                        }
                    }
                }
            });

            // Update cache
            await setMultipleCached(STORES.MESSAGES, 'key', newRecords);
        }

        postMessage({ type: 'complete', requestId });
    } catch (error) {
        postMessage({
            type: 'error',
            requestId,
            message: error instanceof Error ? error.message : String(error)
        });
    }
};

// Handle get-thread-summaries request
const handleGetThreadSummaries = async (request: GetThreadSummariesRequest) => {
    const { requestId, apiBaseUrl, headers, cursor, limit } = request;

    try {
        // For thread summaries, we don't cache the paginated results, just individual summaries
        // This is because pagination state is complex to cache
        const summariesToCache: Array<{ key: string; data: ThreadSummaryPayload }> = [];

        const result = await streamThreadSummaries({
            apiBaseUrl,
            cursor,
            limit,
            getHeaders: async () => headers,
            onSummary: (summary) => {
                summariesToCache.push({ key: summary.key, data: summary });
                postMessage({
                    type: 'thread-summary',
                    requestId,
                    summary,
                    fromCache: false
                });
            }
        });

        // Cache all summaries
        await setMultipleCached(STORES.THREAD_SUMMARIES, 'key', summariesToCache);

        postMessage({ type: 'cursor', requestId, cursor: result.nextCursor });
        postMessage({ type: 'complete', requestId });
    } catch (error) {
        postMessage({
            type: 'error',
            requestId,
            message: error instanceof Error ? error.message : String(error)
        });
    }
};

// Handle get-thread-nodes with cache-first for individual nodes
const handleGetThreadNodes = async (request: GetThreadNodesRequest) => {
    const { requestId, apiBaseUrl, headers, rootKey } = request;

    try {
        const nodesToCache: Array<{ key: string; data: ThreadNodePayload }> = [];

        await streamThreadNodes({
            apiBaseUrl,
            rootKey,
            getHeaders: async () => headers,
            onNode: (node) => {
                nodesToCache.push({ key: node.key, data: node });
                postMessage({
                    type: 'thread-node',
                    requestId,
                    node,
                    fromCache: false
                });
            }
        });

        // Cache all nodes
        await setMultipleCached(STORES.THREAD_NODES, 'key', nodesToCache);

        postMessage({ type: 'complete', requestId });
    } catch (error) {
        postMessage({
            type: 'error',
            requestId,
            message: error instanceof Error ? error.message : String(error)
        });
    }
};

// Handle lookup-computed-id with cache-first
const handleLookupComputedId = async (request: LookupComputedIdRequest) => {
    const { requestId, apiBaseUrl, headers, computedId } = request;

    try {
        // Check cache first
        const cached = await getCached<LookupByComputedIdResult>(
            STORES.COMPUTED_ID_LOOKUP,
            computedId
        );

        if (cached && isCacheValid(cached.cachedAt, COMPUTED_ID_CACHE_TTL)) {
            // Return cached result immediately
            postMessage({
                type: 'lookup-computed-id-result',
                requestId,
                result: cached.data,
                fromCache: true
            });

            // Still verify in background
            void verifyComputedIdLookup(requestId, apiBaseUrl, headers, computedId, cached.data);
        } else {
            // Fetch from API
            const result = await lookupByComputedId({
                apiBaseUrl,
                computedId,
                getHeaders: async () => headers
            });

            await setCached(STORES.COMPUTED_ID_LOOKUP, 'computedId', computedId, result);

            postMessage({
                type: 'lookup-computed-id-result',
                requestId,
                result,
                fromCache: false
            });
        }

        postMessage({ type: 'complete', requestId });
    } catch (error) {
        postMessage({
            type: 'error',
            requestId,
            message: error instanceof Error ? error.message : String(error)
        });
    }
};

// Background verification for computed ID lookup
const verifyComputedIdLookup = async (
    requestId: number,
    apiBaseUrl: string,
    headers: Record<string, string>,
    computedId: string,
    cachedResult: LookupByComputedIdResult
) => {
    try {
        const freshResult = await lookupByComputedId({
            apiBaseUrl,
            computedId,
            getHeaders: async () => headers
        });

        const changed = JSON.stringify(cachedResult) !== JSON.stringify(freshResult);

        if (changed) {
            await setCached(STORES.COMPUTED_ID_LOOKUP, 'computedId', computedId, freshResult);
            postMessage({
                type: 'cache-verified',
                requestId,
                key: computedId,
                changed: true,
                newData: freshResult
            });
        }
    } catch (error) {
        console.warn('Background verification failed for computed ID lookup:', error);
    }
};

// Handle get-computed-id with cache-first
const handleGetComputedId = async (request: GetComputedIdRequest) => {
    const { requestId, apiBaseUrl, headers, key } = request;

    try {
        // Check cache first
        const cached = await getCached<GetComputedIdResult>(STORES.KEY_TO_COMPUTED_ID, key);

        if (cached && isCacheValid(cached.cachedAt, COMPUTED_ID_CACHE_TTL)) {
            // Return cached result immediately
            postMessage({
                type: 'get-computed-id-result',
                requestId,
                result: cached.data,
                fromCache: true
            });

            // Still verify in background
            void verifyGetComputedId(requestId, apiBaseUrl, headers, key, cached.data);
        } else {
            // Fetch from API
            const result = await getComputedId({
                apiBaseUrl,
                key,
                getHeaders: async () => headers
            });

            await setCached(STORES.KEY_TO_COMPUTED_ID, 'key', key, result);

            postMessage({
                type: 'get-computed-id-result',
                requestId,
                result,
                fromCache: false
            });
        }

        postMessage({ type: 'complete', requestId });
    } catch (error) {
        postMessage({
            type: 'error',
            requestId,
            message: error instanceof Error ? error.message : String(error)
        });
    }
};

// Background verification for get computed ID
const verifyGetComputedId = async (
    requestId: number,
    apiBaseUrl: string,
    headers: Record<string, string>,
    key: string,
    cachedResult: GetComputedIdResult
) => {
    try {
        const freshResult = await getComputedId({
            apiBaseUrl,
            key,
            getHeaders: async () => headers
        });

        const changed = cachedResult.computed_id !== freshResult.computed_id;

        if (changed) {
            await setCached(STORES.KEY_TO_COMPUTED_ID, 'key', key, freshResult);
            postMessage({
                type: 'cache-verified',
                requestId,
                key,
                changed: true,
                newData: freshResult
            });
        }
    } catch (error) {
        console.warn('Background verification failed for computed ID:', error);
    }
};

// Main message handler
ctx.addEventListener('message', (event: MessageEvent<CacheRequest>) => {
    const request = event.data;
    if (!request || !request.type) return;

    switch (request.type) {
        case 'get-messages':
            void handleGetMessages(request);
            break;
        case 'get-thread-summaries':
            void handleGetThreadSummaries(request);
            break;
        case 'get-thread-nodes':
            void handleGetThreadNodes(request);
            break;
        case 'lookup-computed-id':
            void handleLookupComputedId(request);
            break;
        case 'get-computed-id':
            void handleGetComputedId(request);
            break;
    }
});

export { };
