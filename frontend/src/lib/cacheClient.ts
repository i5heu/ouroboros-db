/**
 * Cache Client - Wrapper for the cache worker
 * Provides a clean API for components to use cache-first data fetching
 */

import type {
    BulkDataRecord,
    ThreadSummaryPayload,
    ThreadNodePayload,
    LookupByComputedIdResult,
    GetComputedIdResult
} from './apiClient';

type CacheWorkerRequest = {
    type: string;
    requestId: number;
    apiBaseUrl: string;
    headers: Record<string, string>;
    [key: string]: unknown;
};

type CacheWorkerResponse = {
    type: string;
    requestId: number;
    fromCache?: boolean;
    [key: string]: unknown;
};

type RequestState<T> = {
    resolve: (value: T) => void;
    reject: (error: Error) => void;
    results: T;
    onItem?: (item: unknown, fromCache: boolean) => void;
    onCacheVerified?: (key: string, changed: boolean, newData?: unknown) => void;
    onCursor?: (cursor: string | null) => void;
};

let worker: Worker | null = null;
let nextRequestId = 1;
const pendingRequests = new Map<number, RequestState<unknown>>();

type HeaderProvider = () => Promise<Record<string, string>>;

const ensureWorker = (): Worker => {
    if (worker) return worker;

    worker = new Worker(
        new URL('./workers/cacheWorker.ts', import.meta.url),
        { type: 'module' }
    );

    worker.addEventListener('message', handleWorkerMessage);
    worker.addEventListener('error', (event) => {
        console.error('Cache worker error:', event);
        resetWorker(new Error('Cache worker crashed'));
    });

    return worker;
};

const resetWorker = (error: Error) => {
    worker?.terminate();
    worker = null;

    pendingRequests.forEach((state) => {
        state.reject(error);
    });
    pendingRequests.clear();
};

const handleWorkerMessage = (event: MessageEvent<CacheWorkerResponse>) => {
    const data = event.data;
    if (!data || !data.requestId) return;

    const state = pendingRequests.get(data.requestId);
    if (!state) return;

    switch (data.type) {
        case 'message':
            if (state.onItem) {
                state.onItem(data.record, data.fromCache ?? false);
            }
            break;

        case 'thread-summary':
            if (state.onItem) {
                state.onItem(data.summary, data.fromCache ?? false);
            }
            break;

        case 'thread-node':
            if (state.onItem) {
                state.onItem(data.node, data.fromCache ?? false);
            }
            break;

        case 'lookup-computed-id-result':
            if (state.onItem) {
                state.onItem(data.result, data.fromCache ?? false);
            }
            break;

        case 'get-computed-id-result':
            if (state.onItem) {
                state.onItem(data.result, data.fromCache ?? false);
            }
            break;

        case 'cursor':
            if (state.onCursor) {
                state.onCursor(data.cursor as string | null);
            }
            break;

        case 'cache-verified':
            if (state.onCacheVerified) {
                state.onCacheVerified(
                    data.key as string,
                    data.changed as boolean,
                    data.newData
                );
            }
            break;

        case 'complete':
            pendingRequests.delete(data.requestId);
            state.resolve(state.results);
            break;

        case 'error':
            pendingRequests.delete(data.requestId);
            state.reject(new Error(data.message as string));
            break;
    }
};

const sendRequest = async <T>(
    request: Omit<CacheWorkerRequest, 'requestId' | 'headers'>,
    getHeaders: HeaderProvider,
    options: {
        onItem?: (item: unknown, fromCache: boolean) => void;
        onCacheVerified?: (key: string, changed: boolean, newData?: unknown) => void;
        onCursor?: (cursor: string | null) => void;
        initialResults?: T;
    } = {}
): Promise<T> => {
    const w = ensureWorker();
    const headers = await getHeaders();
    const requestId = nextRequestId++;

    return new Promise((resolve, reject) => {
        const state: RequestState<T> = {
            resolve: resolve as (value: unknown) => void,
            reject,
            results: options.initialResults ?? (undefined as T),
            onItem: options.onItem,
            onCacheVerified: options.onCacheVerified,
            onCursor: options.onCursor
        };

        pendingRequests.set(requestId, state as RequestState<unknown>);

        w.postMessage({
            ...request,
            requestId,
            headers
        });
    });
};

export type GetMessagesOptions = {
    apiBaseUrl: string;
    keys: string[];
    includeBinary?: boolean;
    getHeaders: HeaderProvider;
    onRecord: (record: BulkDataRecord, fromCache: boolean) => void;
    onCacheVerified?: (key: string, changed: boolean, newRecord?: BulkDataRecord) => void;
};

/**
 * Get messages with cache-first strategy
 * Cached messages are returned immediately via onRecord with fromCache=true
 * Fresh data is fetched in background and onCacheVerified is called if changed
 */
export const getMessagesCached = async (options: GetMessagesOptions): Promise<void> => {
    await sendRequest(
        {
            type: 'get-messages',
            apiBaseUrl: options.apiBaseUrl,
            keys: options.keys,
            includeBinary: options.includeBinary
        },
        options.getHeaders,
        {
            onItem: (item, fromCache) => {
                options.onRecord(item as BulkDataRecord, fromCache);
            },
            onCacheVerified: options.onCacheVerified
                ? (key, changed, newData) => {
                    options.onCacheVerified!(key, changed, newData as BulkDataRecord);
                }
                : undefined
        }
    );
};

export type GetThreadSummariesOptions = {
    apiBaseUrl: string;
    cursor?: string | null;
    limit?: number;
    getHeaders: HeaderProvider;
    onSummary: (summary: ThreadSummaryPayload, fromCache: boolean) => void;
    onCursor?: (cursor: string | null) => void;
};

/**
 * Get thread summaries - these are streamed from the API
 * Cached summaries will be used for subsequent lookups
 */
export const getThreadSummariesCached = async (
    options: GetThreadSummariesOptions
): Promise<void> => {
    await sendRequest(
        {
            type: 'get-thread-summaries',
            apiBaseUrl: options.apiBaseUrl,
            cursor: options.cursor,
            limit: options.limit
        },
        options.getHeaders,
        {
            onItem: (item, fromCache) => {
                options.onSummary(item as ThreadSummaryPayload, fromCache);
            },
            onCursor: options.onCursor
        }
    );
};

export type GetThreadNodesOptions = {
    apiBaseUrl: string;
    rootKey: string;
    getHeaders: HeaderProvider;
    onNode: (node: ThreadNodePayload, fromCache: boolean) => void;
};

/**
 * Get thread nodes - these are streamed from the API
 */
export const getThreadNodesCached = async (options: GetThreadNodesOptions): Promise<void> => {
    await sendRequest(
        {
            type: 'get-thread-nodes',
            apiBaseUrl: options.apiBaseUrl,
            rootKey: options.rootKey
        },
        options.getHeaders,
        {
            onItem: (item, fromCache) => {
                options.onNode(item as ThreadNodePayload, fromCache);
            }
        }
    );
};

export type LookupComputedIdOptions = {
    apiBaseUrl: string;
    computedId: string;
    getHeaders: HeaderProvider;
    onResult?: (result: LookupByComputedIdResult, fromCache: boolean) => void;
    onCacheVerified?: (changed: boolean, newResult?: LookupByComputedIdResult) => void;
};

/**
 * Lookup by computed ID with cache-first strategy
 * Returns cached result immediately, verifies in background
 */
export const lookupComputedIdCached = async (
    options: LookupComputedIdOptions
): Promise<LookupByComputedIdResult | null> => {
    let result: LookupByComputedIdResult | null = null;

    await sendRequest(
        {
            type: 'lookup-computed-id',
            apiBaseUrl: options.apiBaseUrl,
            computedId: options.computedId
        },
        options.getHeaders,
        {
            onItem: (item, fromCache) => {
                result = item as LookupByComputedIdResult;
                if (options.onResult) {
                    options.onResult(result, fromCache);
                }
            },
            onCacheVerified: options.onCacheVerified
                ? (_, changed, newData) => {
                    options.onCacheVerified!(changed, newData as LookupByComputedIdResult);
                }
                : undefined
        }
    );

    return result;
};

export type GetComputedIdOptions = {
    apiBaseUrl: string;
    key: string;
    getHeaders: HeaderProvider;
    onResult?: (result: GetComputedIdResult, fromCache: boolean) => void;
    onCacheVerified?: (changed: boolean, newResult?: GetComputedIdResult) => void;
};

/**
 * Get computed ID for a key with cache-first strategy
 * Returns cached result immediately, verifies in background
 */
export const getComputedIdCached = async (
    options: GetComputedIdOptions
): Promise<GetComputedIdResult | null> => {
    let result: GetComputedIdResult | null = null;

    await sendRequest(
        {
            type: 'get-computed-id',
            apiBaseUrl: options.apiBaseUrl,
            key: options.key
        },
        options.getHeaders,
        {
            onItem: (item, fromCache) => {
                result = item as GetComputedIdResult;
                if (options.onResult) {
                    options.onResult(result, fromCache);
                }
            },
            onCacheVerified: options.onCacheVerified
                ? (_, changed, newData) => {
                    options.onCacheVerified!(changed, newData as GetComputedIdResult);
                }
                : undefined
        }
    );

    return result;
};

/**
 * Check if cache worker is available (browser environment with Worker support)
 */
export const isCacheWorkerAvailable = (): boolean => {
    return typeof window !== 'undefined' && typeof Worker !== 'undefined';
};

/**
 * Terminate the cache worker (cleanup)
 */
export const terminateCacheWorker = () => {
    if (worker) {
        worker.terminate();
        worker = null;
        pendingRequests.clear();
    }
};
