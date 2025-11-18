<script lang="ts">
	import { onDestroy, onMount, tick } from 'svelte';
	import MessageNode from '../lib/components/MessageNode.svelte';
	import type { Message } from '../lib/types';
	import { bootstrapAuthFromParams, buildAuthHeaders } from '../lib/auth';
	import {
		streamThreadNodes,
		streamThreadSummaries,
		streamBulkMessages,
		type ThreadNodePayload,
		type ThreadSummaryPayload,
		type BulkDataRecord
	} from '../lib/apiClient';
	import {
		openIndexedDb2,
		readMessageRecord,
		writeMessageRecords,
		type MessageRecord
	} from '../lib/indexedDb2';

	// Prefer VITE_OUROBOROS_API when set; default to same origin so dev-server proxy reduces CORS
	const API_BASE_URL = import.meta.env.VITE_OUROBOROS_API ?? '';
	const LAST_KEY_STORAGE = 'ouroboros:lastKey';
	const AUTH_BANNER_TIMEOUT_MS = 5_000;

	type AuthStatus = 'idle' | 'authenticating' | 'authenticated' | 'error';

	type ThreadSummary = ThreadSummaryPayload & { cachedAt?: number };

	let threadSummaries: ThreadSummary[] = [];
	let typedThreadSummaries: ThreadSummary[] = [];
	$: typedThreadSummaries = threadSummaries;
	let threadCursor: string | null = null;
	let threadsLoading = false;
	let threadsError = '';
	let threadsComplete = false;
	let nodeLoading = false;
	let nodeError = '';
	let nodeProgressCount = 0;
	let nodeStreamAbort: AbortController | null = null;
	let threadsAbort: AbortController | null = null;
	let db: IDBDatabase | null = null;
	const messageCache = new Map<string, MessageRecord>();
	const pendingMessageWrites = new Map<string, MessageRecord>();
	const messageHydrations = new Map<string, Promise<void>>();
	const pendingHydrationNodes = new Map<string, ThreadNodePayload>();
	const bulkHydrationQueue = new Set<string>();
	const bulkHydrationWaiters = new Map<string, Array<() => void>>();
	let bulkHydrationTimer: ReturnType<typeof setTimeout> | null = null;
	let bulkHydrationInFlight = false;
	type HydrationWorkerRecordMessage = { type: 'record'; requestId: number; record: BulkDataRecord };
	type HydrationWorkerCompleteMessage = { type: 'complete'; requestId: number };
	type HydrationWorkerErrorMessage = { type: 'error'; requestId: number; message: string };
	type HydrationWorkerMessage =
		| HydrationWorkerRecordMessage
		| HydrationWorkerCompleteMessage
		| HydrationWorkerErrorMessage;
	type HydrationRequestState = {
		batch: string[];
		seen: Set<string>;
		resolve: (value: Set<string>) => void;
		reject: (error: Error) => void;
	};
	let hydrationWorker: Worker | null = null;
	const hydrationRequests = new Map<number, HydrationRequestState>();
	let nextHydrationRequestId = 1;
	let sentinel: HTMLElement | null = null;
	let sentinelObserver: IntersectionObserver | null = null;
	let sentinelAttached = false;
	let selectedThreadKey: string | null = null;

	let messages: Message[] = [];
	let inputValue = '';
	let nextId = 1;
	let selectedPath: number[] | null = null;
	let selectedMessage: Message | null = null;
	let fileInput: HTMLInputElement | null = null;
	let activeSaves = 0;
	let statusState: 'idle' | 'sending' | 'success' | 'error' = 'idle';
	let statusText = '';
	let keyToPath = new Map<string, number[]>();
	let newThreadMode = false;
	let newThreadTitle = '';
	let newThreadRootPath: number[] | null = null;
	let titleInputEl: HTMLInputElement | null = null;
	let authStatus: AuthStatus = 'idle';
	let authStatusText = 'Authenticate with the one-time key link to unlock data access.';
	let authBannerVisible = true;
	let authBannerTimer: ReturnType<typeof setTimeout> | null = null;
	let lastAuthStatusText = '';
	let initialSelectionDone = false;

	// Header title: prefer new-thread mode, then current selected thread summary, then root message title, then fallback
	$: headerTitle = (() => {
		if (newThreadMode) return 'Create Thread';
		if (selectedThreadKey) {
			const summary = threadSummaries.find((t) => t.key === selectedThreadKey);
			if (summary) return threadHeaderTitle(summary, messages.length > 0 ? messages[0] : null);
			if (messages.length > 0) return threadHeaderTitle(undefined, messages[0]);
		}
		return 'Threaded Chat';
	})();

	const countAllChildren = (nodeList: Message[] | undefined): number => {
		if (!nodeList || nodeList.length === 0) return 0;
		let count = 0;
		const traverse = (nodes: Message[]) => {
			for (const n of nodes) {
				count += 1;
				traverse(n.children ?? []);
			}
		};
		traverse(nodeList);
		return count;
	};

	const findMostRecentChildCreatedAt = (nodeList: Message[] | undefined): string | null => {
		let maxMs = -1;
		const traverse = (nodes: Message[]) => {
			for (const n of nodes) {
				if (n.createdAt) {
					const ms = Date.parse(n.createdAt);
					if (!Number.isNaN(ms) && ms > maxMs) maxMs = ms;
				}
				traverse(n.children ?? []);
			}
		};
		if (!nodeList) return null;
		traverse(nodeList);
		return maxMs >= 0 ? new Date(maxMs).toISOString() : null;
	};

	// Header meta: creation date, number of items (children), and last child date
	$: headerMeta = (() => {
		if (newThreadMode)
			return { createdAt: null, childrenCount: 0, descendantCount: 0, lastChildAt: null };
		let createdAt: string | null = null;
		let childrenCount = 0; // immediate children
		let descendantCount = 0; // full subtree count
		let lastChildAt: string | null = null;
		if (selectedThreadKey) {
			const summary = threadSummaries.find((t) => t.key === selectedThreadKey);
			if (summary) {
				createdAt = summary.createdAt ?? null;
				childrenCount = summary.childCount ?? 0;
			}
		}
		// if not present from summary, fallback to root message metadata
		if (!createdAt && messages.length > 0) {
			createdAt = messages[0].createdAt ?? null;
		}
		// last child: search across all children (excluding the root)
		if (messages.length > 0) {
			const mostRecent = findMostRecentChildCreatedAt(messages[0].children ?? []);
			lastChildAt = mostRecent;
			// immediate children from the root node
			if (!childrenCount) childrenCount = (messages[0].children ?? []).length;
			// descendant count: total in subtree
			descendantCount = countAllChildren(messages[0].children ?? []);
		}
		return { createdAt, childrenCount, descendantCount, lastChildAt };
	})();

	const deepClone = <T,>(value: T): T => {
		if (typeof structuredClone === 'function') {
			return structuredClone(value);
		}

		return JSON.parse(JSON.stringify(value)) as T;
	};

	const decodeBase64ToUint8 = (value: string): Uint8Array => {
		if (typeof globalThis.atob !== 'function') {
			throw new Error('Base64 decoding unavailable in this environment.');
		}
		const binary = globalThis.atob(value);
		const bytes = new Uint8Array(binary.length);
		for (let i = 0; i < binary.length; i += 1) {
			bytes[i] = binary.charCodeAt(i);
		}
		return bytes;
	};

	const encodeBytesToBase64 = (bytes: Uint8Array): string => {
		let binary = '';
		bytes.forEach((byte) => {
			binary += String.fromCharCode(byte);
		});

		if (typeof globalThis.btoa === 'function') {
			return globalThis.btoa(binary);
		}

		throw new Error('Base64 encoding unavailable in this environment.');
	};

	const withAuthHeaders = async (extras: Record<string, string> = {}) => {
		const authHeaders = await buildAuthHeaders();
		return { ...authHeaders, ...extras };
	};

	const formatBytes = (size: number): string => {
		if (size < 1024) return `${size} B`;
		const units = ['KB', 'MB', 'GB', 'TB'];
		let value = size / 1024;
		let unitIndex = 0;
		while (value >= 1024 && unitIndex < units.length - 1) {
			value /= 1024;
			unitIndex += 1;
		}
		return `${value.toFixed(value >= 10 ? 0 : 1)} ${units[unitIndex]}`;
	};

	const summaryKey = (value: unknown, index: number): string => {
		if (value && typeof value === 'object' && 'key' in value) {
			return (value as ThreadSummary).key;
		}
		return `summary-${index}`;
	};

	const calculateBase64Size = (value: string): number => {
		if (!value.length) return 0;
		const padding = value.endsWith('==') ? 2 : value.endsWith('=') ? 1 : 0;
		return Math.floor((value.length * 3) / 4) - padding;
	};

	const textLoadingPlaceholder = 'Loading message…';

	const nodeParentKey = (value?: string | null): string | undefined => {
		if (!value) return undefined;
		const trimmed = value.trim();
		return trimmed.length > 0 ? trimmed : undefined;
	};

	const attachmentLabel = (mimeType: string, sizeBytes: number): string => {
		const normalizedMime = mimeType?.trim() || 'attachment';
		return sizeBytes > 0
			? `[${normalizedMime} • ${formatBytes(sizeBytes)}]`
			: `[${normalizedMime}]`;
	};

	const readFileAsBase64 = (file: File): Promise<string> =>
		new Promise((resolve, reject) => {
			const reader = new FileReader();
			reader.onerror = () => reject(reader.error ?? new Error('Failed to read file.'));
			reader.onload = () => {
				const result = reader.result;
				if (result instanceof ArrayBuffer) {
					resolve(encodeBytesToBase64(new Uint8Array(result)));
				} else if (typeof result === 'string') {
					const base64Index = result.indexOf('base64,');
					resolve(base64Index >= 0 ? result.slice(base64Index + 7) : result);
				} else {
					reject(new Error('Unsupported file reader result.'));
				}
			};
			reader.readAsArrayBuffer(file);
		});

	const truncate = (value: string, length = 80): string => {
		const trimmed = value.trim();
		return trimmed.length <= length ? trimmed : `${trimmed.slice(0, length)}…`;
	};

	const sortThreadSummaries = (items: ThreadSummary[]): ThreadSummary[] =>
		items.slice().sort((a, b) => {
			const aTime = a.createdAt ?? '';
			const bTime = b.createdAt ?? '';
			return bTime.localeCompare(aTime);
		});

	const upsertThreadSummary = (summary: ThreadSummaryPayload) => {
		const enriched: ThreadSummary = { ...summary, cachedAt: Date.now() };
		const existingIndex = threadSummaries.findIndex((item) => item.key === summary.key);
		if (existingIndex >= 0) {
			const next = [...threadSummaries];
			next[existingIndex] = enriched;
			threadSummaries = sortThreadSummaries(next);
			return;
		}
		threadSummaries = sortThreadSummaries([...threadSummaries, enriched]);
	};

	const threadTitle = (message: Message): string => {
		const content = message.content?.trim() ?? '';
		if (!content.length) {
			return '[untitled]';
		}
		return truncate(content, 60);
	};

	const threadSummaryTitle = (summary: ThreadSummary): string => {
		const content = summary.preview?.trim() ?? '';
		if (!content.length) {
			return '[untitled]';
		}
		return truncate(content, 60);
	};

	// Full header title (do not truncate). Prefer summary preview when available, else root message.
	const threadHeaderTitle = (summary?: ThreadSummary, root?: Message | null): string => {
		const content = summary?.preview?.trim() ?? root?.content?.trim() ?? '';
		if (!content.length) return '[untitled]';
		return content;
	};

	const threadSummaryMeta = (summary: ThreadSummary): string => {
		const parts: string[] = [];
		const createdLabel = formatTimestamp(summary.createdAt);
		if (createdLabel) {
			parts.push(`Created ${createdLabel}`);
		}
		const repliesLabel = summary.childCount
			? `${summary.childCount} repl${summary.childCount === 1 ? 'y' : 'ies'}`
			: 'No replies yet';
		parts.push(repliesLabel);
		return parts.join(' · ');
	};

	const loadCachedThreadSummaries = async () => {
		// keep the db handle initialization for node caching while skipping summaries
		try {
			db = await openIndexedDb2();
			flushPendingMessageWrites();
		} catch (error) {
			console.warn('Failed to open IndexedDB (node cache only)', error);
		}
	};

	const fetchNextThreadBatch = async () => {
		if (threadsLoading || (threadsComplete && threadSummaries.length > 0)) {
			return;
		}
		threadsLoading = true;
		threadsError = '';
		const controller = new AbortController();
		threadsAbort?.abort();
		threadsAbort = controller;
		try {
			const result = await streamThreadSummaries({
				apiBaseUrl: API_BASE_URL,
				cursor: threadCursor ?? undefined,
				limit: 25,
				signal: controller.signal,
				getHeaders: buildAuthHeaders,
				onSummary: (summary) => {
					upsertThreadSummary(summary);
				}
			});
			threadCursor = result.nextCursor ?? null;
			if (!threadCursor) {
				threadsComplete = true;
			}
		} catch (error) {
			if (!controller.signal.aborted) {
				threadsError = error instanceof Error ? error.message : String(error);
			}
		} finally {
			if (!controller.signal.aborted) {
				threadsLoading = false;
			}
		}
	};

	const attemptInitialSelection = () => {
		if (initialSelectionDone) {
			return;
		}
		if (selectedThreadKey) {
			const existing = threadSummaries.find((item) => item.key === selectedThreadKey);
			if (existing) {
				handleSelectThreadSummary(existing);
				initialSelectionDone = true;
				return;
			}
		}
		const storedKey =
			typeof localStorage !== 'undefined' ? localStorage.getItem(LAST_KEY_STORAGE) : null;
		if (storedKey) {
			const cached = threadSummaries.find((item) => item.key === storedKey);
			if (cached) {
				handleSelectThreadSummary(cached);
				initialSelectionDone = true;
				return;
			}
		}
		if (threadSummaries.length > 0) {
			handleSelectThreadSummary(threadSummaries[0]);
			initialSelectionDone = true;
		}
	};

	const createMessageFromNode = (node: ThreadNodePayload): Message => {
		const sizeBytes = node.sizeBytes ?? 0;
		const createdAtMs = node.createdAt ? Date.parse(node.createdAt) : undefined;
		return {
			id: nextId++,
			content: node.isText ? textLoadingPlaceholder : attachmentLabel(node.mimeType, sizeBytes),
			children: [],
			status: 'saved',
			key: node.key,
			parentKey: nodeParentKey(node.parent),
			mimeType: node.mimeType,
			isText: node.isText,
			sizeBytes,
			attachmentName: !node.isText ? node.key : undefined,
			createdAt: node.createdAt,
			createdAtMs
		};
	};

	const upsertNodeFromStream = (node: ThreadNodePayload) => {
		const existingPath = node.key ? keyToPath.get(node.key) : undefined;
		if (existingPath) {
			updateMessageAtPath(existingPath, (message) => {
				message.mimeType = node.mimeType;
				message.isText = node.isText;
				message.sizeBytes = node.sizeBytes;
				message.createdAt = node.createdAt;
				message.createdAtMs = node.createdAt ? Date.parse(node.createdAt) : undefined;
				message.parentKey = nodeParentKey(node.parent);
				if (!node.isText) {
					message.content = attachmentLabel(node.mimeType, node.sizeBytes ?? 0);
				}
			});
			requestMessageHydration(node);
			return;
		}

		const newMessage = createMessageFromNode(node);

		if (!node.parent) {
			messages = [newMessage];
			keyToPath = buildKeyPathMap(messages);
			setSelectedPath([0]);
			nodeLoading = false;
			requestMessageHydration(node);
			return;
		}

		const parentPath = keyToPath.get(node.parent);
		if (!parentPath) {
			return;
		}

		const cloned = deepClone(messages);
		let parentNode: Message | undefined = cloned[parentPath[0]];
		for (let i = 1; i < parentPath.length && parentNode; i += 1) {
			parentNode = parentNode.children[parentPath[i]];
		}
		if (!parentNode) {
			return;
		}

		parentNode.children = [...parentNode.children, newMessage];
		messages = cloned;
		keyToPath = buildKeyPathMap(messages);
		requestMessageHydration(node);
	};

	const applyRecordToMessage = (record: MessageRecord) => {
		const path = record.key ? keyToPath.get(record.key) : undefined;
		if (!path) {
			return;
		}
		updateMessageAtPath(path, (message) => {
			if (record.content !== undefined) {
				message.content = record.content === '' ? '[empty]' : record.content;
			}
			if (record.encodedContent) {
				message.encodedContent = record.encodedContent;
			}
			message.mimeType = record.mimeType;
			message.isText = record.isText;
			message.sizeBytes = record.sizeBytes;
			if (record.createdAt) {
				message.createdAt = record.createdAt;
				const ms = Date.parse(record.createdAt);
				if (!Number.isNaN(ms)) {
					message.createdAtMs = ms;
				}
			}
		});
	};

	const readCachedMessageRecord = async (key: string): Promise<MessageRecord | null> => {
		if (messageCache.has(key)) {
			return messageCache.get(key) ?? null;
		}
		if (!db) {
			return null;
		}
		try {
			const record = await readMessageRecord(db, key);
			if (record) {
				messageCache.set(key, record);
			}
			return record;
		} catch (error) {
			console.warn('Failed to read cached message', error);
			return null;
		}
	};

	const fetchMessageRecord = async (node: ThreadNodePayload): Promise<MessageRecord> => {
		if (!node.key) {
			throw new Error('Message key is required');
		}
		const response = await fetch(`${API_BASE_URL}/data/${node.key}`, {
			headers: await withAuthHeaders()
		});
		if (!response.ok) {
			const message = (await response.text()) || `Failed to load message ${node.key}`;
			throw new Error(message);
		}
		const buffer = await response.arrayBuffer();
		const bytes = new Uint8Array(buffer);
		const resolvedSize = bytes.length > 0 ? bytes.length : (node.sizeBytes ?? 0);
		const encodedContent = encodeBytesToBase64(bytes);
		const headerMime =
			response.headers.get('X-Ouroboros-Mime') ?? response.headers.get('Content-Type');
		const mimeType = headerMime ?? node.mimeType ?? 'application/octet-stream';
		const headerIsText = response.headers.get('X-Ouroboros-Is-Text');
		const isText = headerIsText ? headerIsText.toLowerCase() === 'true' : node.isText;
		const createdAt = response.headers.get('X-Ouroboros-Created-At') ?? node.createdAt;
		let decodedContent: string | undefined;
		if (isText) {
			try {
				decodedContent = new TextDecoder().decode(bytes);
			} catch (error) {
				console.warn('Failed to decode text payload', error);
			}
		}
		return {
			key: node.key,
			mimeType,
			isText,
			sizeBytes: resolvedSize,
			encodedContent,
			content: decodedContent,
			createdAt
		};
	};

	const flushPendingMessageWrites = () => {
		if (!db || pendingMessageWrites.size === 0) {
			return;
		}
		const batch = Array.from(pendingMessageWrites.values());
		pendingMessageWrites.clear();
		void writeMessageRecords(db, batch).catch((error) => {
			console.warn('Failed to write cached messages', error);
			for (const record of batch) {
				if (record.key) {
					pendingMessageWrites.set(record.key, record);
				}
			}
		});
	};

	const persistMessageRecord = (record: MessageRecord) => {
		if (!record.key) {
			return;
		}
		messageCache.set(record.key, record);
		pendingMessageWrites.set(record.key, record);
		flushPendingMessageWrites();
	};

	const resolveBulkWaiters = (key: string) => {
		const waiters = bulkHydrationWaiters.get(key);
		bulkHydrationWaiters.delete(key);
		waiters?.forEach((resolve) => resolve());
	};

	const bulkRecordToMessageRecord = (payload: BulkDataRecord): MessageRecord | null => {
		if (!payload.key || !payload.found) {
			return null;
		}
		const mime =
			payload.mimeType?.trim() ||
			(payload.isText ? 'text/plain; charset=utf-8' : 'application/octet-stream');
		const record: MessageRecord = {
			key: payload.key,
			mimeType: mime,
			isText: Boolean(payload.isText),
			sizeBytes:
				payload.sizeBytes ??
				(payload.content !== undefined ? new TextEncoder().encode(payload.content).length : 0),
			createdAt: payload.createdAt
		};
		if (payload.content !== undefined) {
			record.content = payload.content;
		}
		if (payload.encodedContent) {
			record.encodedContent = payload.encodedContent;
		}
		return record;
	};

	const enqueueBulkHydration = (key: string): Promise<void> => {
		return new Promise((resolve) => {
			if (!bulkHydrationWaiters.has(key)) {
				bulkHydrationWaiters.set(key, []);
			}
			bulkHydrationWaiters.get(key)!.push(resolve);
			bulkHydrationQueue.add(key);
			scheduleBulkHydrationFlush();
		});
	};

	const scheduleBulkHydrationFlush = () => {
		if (bulkHydrationTimer) {
			return;
		}
		bulkHydrationTimer = setTimeout(() => {
			bulkHydrationTimer = null;
			void flushBulkHydrationQueue();
		}, 0);
	};

	const handleBulkPayload = (payload: BulkDataRecord, seen: Set<string>) => {
		if (!payload.key) {
			return;
		}
		seen.add(payload.key);
		const record = bulkRecordToMessageRecord(payload);
		if (record) {
			persistMessageRecord(record);
			applyRecordToMessage(record);
		}
		resolveBulkWaiters(payload.key);
	};

	const handleHydrationWorkerMessage = (event: MessageEvent<HydrationWorkerMessage>) => {
		const data = event.data;
		const state = hydrationRequests.get(data.requestId);
		if (!state) {
			return;
		}
		if (data.type === 'record') {
			handleBulkPayload(data.record, state.seen);
			return;
		}
		hydrationRequests.delete(data.requestId);
		if (data.type === 'complete') {
			state.resolve(state.seen);
		} else if (data.type === 'error') {
			state.reject(new Error(data.message));
		}
	};

	const resetHydrationWorker = (error?: Error) => {
		hydrationWorker?.terminate();
		hydrationWorker = null;
		if (hydrationRequests.size > 0) {
			const failure = error ?? new Error('Hydration worker terminated.');
			hydrationRequests.forEach((state) => state.reject(failure));
			hydrationRequests.clear();
		}
	};

	const ensureHydrationWorker = (): Worker | null => {
		if (typeof window === 'undefined') {
			return null;
		}
		if (hydrationWorker) {
			return hydrationWorker;
		}
		try {
			hydrationWorker = new Worker(
				new URL('../lib/workers/messageHydrationWorker.ts', import.meta.url),
				{ type: 'module' }
			);
			hydrationWorker.addEventListener('message', handleHydrationWorkerMessage);
			hydrationWorker.addEventListener('error', (event) => {
				console.error('Hydration worker runtime error', event);
				resetHydrationWorker(new Error('Hydration worker crashed.'));
			});
			return hydrationWorker;
		} catch (error) {
			const failure = error instanceof Error ? error : new Error(String(error));
			console.warn('Failed to start hydration worker', failure);
			resetHydrationWorker(failure);
			return null;
		}
	};

	const hydrateBatchWithWorker = async (batch: string[]): Promise<Set<string> | null> => {
		const worker = ensureHydrationWorker();
		if (!worker) {
			return null;
		}
		const headers = await buildAuthHeaders();
		const requestId = nextHydrationRequestId++;
		return await new Promise((resolve, reject) => {
			const seen = new Set<string>();
			const state: HydrationRequestState = {
				batch,
				seen,
				resolve: (value) => resolve(value),
				reject
			};
			hydrationRequests.set(requestId, state);
			try {
				worker.postMessage({
					type: 'hydrate-batch',
					requestId,
					apiBaseUrl: API_BASE_URL,
					keys: batch,
					headers
				});
			} catch (error) {
				hydrationRequests.delete(requestId);
				reject(error instanceof Error ? error : new Error(String(error)));
			}
		});
	};

	const hydrateBatchInline = async (batch: string[]): Promise<Set<string>> => {
		const seen = new Set<string>();
		await streamBulkMessages({
			apiBaseUrl: API_BASE_URL,
			keys: batch,
			getHeaders: buildAuthHeaders,
			onRecord: (payload) => handleBulkPayload(payload, seen)
		});
		return seen;
	};

	const fallbackHydrationFetch = async (key: string) => {
		const metadata = pendingHydrationNodes.get(key);
		if (!metadata) {
			resolveBulkWaiters(key);
			return;
		}
		try {
			const record = await fetchMessageRecord(metadata);
			persistMessageRecord(record);
			applyRecordToMessage(record);
		} catch (error) {
			console.warn('Failed to fetch message content (fallback)', error);
		} finally {
			resolveBulkWaiters(key);
		}
	};

	const flushBulkHydrationQueue = async () => {
		if (bulkHydrationInFlight || bulkHydrationQueue.size === 0) {
			return;
		}
		const batch = Array.from(bulkHydrationQueue).slice(0, 500);
		batch.forEach((key) => bulkHydrationQueue.delete(key));
		let seen = new Set<string>();
		let inlineAttempted = false;
		bulkHydrationInFlight = true;
		try {
			const workerSeen = await hydrateBatchWithWorker(batch);
			if (workerSeen) {
				seen = workerSeen;
			} else {
				inlineAttempted = true;
				seen = await hydrateBatchInline(batch);
			}
		} catch (error) {
			console.warn('Hydration worker batch failed, attempting inline fetch', error);
			if (!inlineAttempted) {
				try {
					inlineAttempted = true;
					seen = await hydrateBatchInline(batch);
				} catch (inlineError) {
					console.warn('Bulk hydration request failed', inlineError);
				}
			}
		} finally {
			bulkHydrationInFlight = false;
			const unseen = batch.filter((key) => !seen.has(key));
			for (const key of unseen) {
				await fallbackHydrationFetch(key);
			}
			if (bulkHydrationQueue.size > 0) {
				scheduleBulkHydrationFlush();
			}
		}
	};

	const hydrateMessage = async (node: ThreadNodePayload) => {
		if (!node.key || !node.isText) {
			return;
		}
		const cached = await readCachedMessageRecord(node.key);
		if (cached) {
			applyRecordToMessage(cached);
			return;
		}
		await enqueueBulkHydration(node.key);
	};

	const requestMessageHydration = (node: ThreadNodePayload) => {
		if (!node.key || !node.isText || messageHydrations.has(node.key)) {
			return;
		}
		pendingHydrationNodes.set(node.key, node);
		const promise = hydrateMessage(node)
			.catch((error) => {
				console.warn('Failed to hydrate message', error);
			})
			.finally(() => {
				messageHydrations.delete(node.key as string);
				pendingHydrationNodes.delete(node.key as string);
			});
		messageHydrations.set(node.key, promise);
	};

	const handleSelectThreadSummary = (summary: ThreadSummary) => {
		newThreadMode = false;
		if (selectedThreadKey === summary.key && messages.length > 0) {
			return;
		}
		selectedThreadKey = summary.key;
		nodeLoading = true;
		nodeError = '';
		nodeProgressCount = 0;
		messages = [];
		keyToPath = new Map();
		nextId = 1;
		setSelectedPath(null);
		rememberLastKey(summary.key);
		nodeStreamAbort?.abort();
		const controller = new AbortController();
		nodeStreamAbort = controller;
		void (async () => {
			try {
				await streamThreadNodes({
					apiBaseUrl: API_BASE_URL,
					rootKey: summary.key,
					signal: controller.signal,
					getHeaders: buildAuthHeaders,
					onNode: (node) => {
						nodeProgressCount += 1;
						upsertNodeFromStream(node);
					}
				});
				if (!controller.signal.aborted) {
					nodeLoading = false;
				}
			} catch (error) {
				if (controller.signal.aborted) {
					return;
				}
				nodeLoading = false;
				nodeError = error instanceof Error ? error.message : String(error);
			}
		})();
	};

	const formatTimestamp = (value?: string): string | null => {
		if (!value) {
			return null;
		}

		const date = new Date(value);
		if (Number.isNaN(date.getTime())) {
			return null;
		}

		return date.toLocaleString(undefined, {
			dateStyle: 'medium',
			timeStyle: 'short'
		});
	};

	const threadMeta = (message: Message): string => {
		const replies = message.children.length;
		const parts: string[] = [];
		const createdLabel = formatTimestamp(message.createdAt);
		if (createdLabel) {
			parts.push(`Created ${createdLabel}`);
		}
		parts.push(replies ? `${replies} repl${replies === 1 ? 'y' : 'ies'}` : 'No replies yet');
		return parts.join(' · ');
	};

	const messageStatusLabel = (status: Message['status']): string => {
		switch (status) {
			case 'pending':
				return 'pending';
			case 'failed':
				return 'failed';
			default:
				return '';
		}
	};

	const compareMessages = (a: Message, b: Message) => {
		const aTime = a.createdAtMs ?? Number.POSITIVE_INFINITY;
		const bTime = b.createdAtMs ?? Number.POSITIVE_INFINITY;
		if (aTime !== bTime) {
			return aTime - bTime;
		}
		if (
			a.createdAtMs != null &&
			b.createdAtMs != null &&
			a.createdAt &&
			b.createdAt &&
			a.createdAt !== b.createdAt
		) {
			return a.createdAt < b.createdAt ? -1 : 1;
		}
		return a.id - b.id;
	};

	const sortMessageTree = (nodes: Message[]) => {
		nodes.sort(compareMessages);
		nodes.forEach((child) => sortMessageTree(child.children));
	};

	const buildKeyPathMap = (nodes: Message[]): Map<string, number[]> => {
		const map = new Map<string, number[]>();
		const traverse = (list: Message[], prefix: number[]) => {
			list.forEach((node, index) => {
				const current = [...prefix, index];
				if (node.key) {
					map.set(node.key, current);
				}
				traverse(node.children, current);
			});
		};
		traverse(nodes, []);
		return map;
	};

	const computeLastPath = (nodes: Message[], prefix: number[] = []): number[] => {
		let result: number[] = [];
		nodes.forEach((node, index) => {
			const current = [...prefix, index];
			if (node.children.length > 0) {
				const childResult = computeLastPath(node.children, current);
				result = childResult.length > 0 ? childResult : current;
			} else {
				result = current;
			}
		});
		return result;
	};

	const rememberLastKey = (key: string) => {
		if (typeof localStorage === 'undefined') {
			return;
		}
		localStorage.setItem(LAST_KEY_STORAGE, key);
	};

	const forgetLastKey = () => {
		if (typeof localStorage === 'undefined') {
			return;
		}
		localStorage.removeItem(LAST_KEY_STORAGE);
	};

	const resolveInitialPath = (roots: Message[], map: Map<string, number[]>): number[] => {
		if (typeof localStorage !== 'undefined') {
			const storedKey = localStorage.getItem(LAST_KEY_STORAGE);
			if (storedKey) {
				const storedPath = map.get(storedKey);
				if (storedPath) {
					return [...storedPath];
				}
				forgetLastKey();
			}
		}
		const last = computeLastPath(roots);
		if ((!last || last.length === 0) && roots.length > 0) {
			// fallback to root if there is at least one root node
			return [0];
		}
		return last;
	};

	const getMessageAtPath = (source: Message[], path: number[]): Message | null => {
		if (path.length === 0) {
			return null;
		}

		let current: Message | undefined = source[path[0]];
		for (let i = 1; i < path.length && current; i += 1) {
			current = current.children[path[i]];
		}

		return current ?? null;
	};

	const setSelectedPath = (path: number[] | null) => {
		selectedPath = path ? [...path] : null;
		const message = selectedPath ? getMessageAtPath(messages, selectedPath) : null;
		if (message?.key) {
			rememberLastKey(message.key);
		} else if (!selectedPath) {
			forgetLastKey();
		}
	};

	const handleSelectMessage = (path: number[]) => {
		// If the message clicked is already selected, toggle selection back to the thread root
		const alreadySelected =
			selectedPath !== null &&
			selectedPath.length === path.length &&
			selectedPath.every((v, i) => v === path[i]);
		if (alreadySelected) {
			// set selection back to root
			setSelectedPath([0]);
			return;
		}
		setSelectedPath(path);
	};

	const handleSelectThread = (index: number) => {
		if (index < 0 || index >= messages.length) {
			return;
		}
		setSelectedPath([index]);
	};

	const clearSelection = () => {
		setSelectedPath(null);
		newThreadMode = false;
		newThreadTitle = '';
		newThreadRootPath = null;
	};

	const startNewThread = () => {
		newThreadMode = true;
		newThreadTitle = '';
		messages = [];
		selectedThreadKey = null;
		const newRoot: Message = {
			id: nextId++,
			content: '[untitled]',
			children: [],
			status: 'pending',
			key: '',
			parentKey: undefined,
			mimeType: 'text/plain; charset=utf-8',
			isText: true,
			sizeBytes: 0
		};
		messages = [newRoot];
		keyToPath = buildKeyPathMap(messages);
		setSelectedPath([0]);
		newThreadRootPath = [0];
		// Focus the title input in the next tick (if bound)
		void tick().then(() => titleInputEl?.focus());
	};

	const cancelNewThread = () => {
		// Reset the new thread state and clear the draft
		newThreadMode = false;
		newThreadTitle = '';
		newThreadRootPath = null;
		// revert messages to empty selection (no thread)
		messages = [];
		keyToPath = new Map();
		setSelectedPath(null);
	};

	const applyTitleToRoot = (title: string) => {
		newThreadTitle = title;
		if (newThreadRootPath) {
			updateMessageAtPath(newThreadRootPath, (message) => {
				message.content = title.trim() ? title : '[untitled]';
				message.encodedContent = encodeToBase64(message.content);
				message.sizeBytes = calculateBase64Size(message.encodedContent ?? '');
			});
		}
	};

	const createThreadFromTitle = async () => {
		if (!newThreadRootPath) return;
		// Ensure title is applied
		if (newThreadTitle.trim().length === 0) return;
		await persistMessage(newThreadRootPath);
		const rootMessage = getMessageAtPath(messages, newThreadRootPath);
		if (rootMessage?.key) {
			selectedThreadKey = rootMessage.key;
		}
		newThreadMode = false;
		newThreadTitle = '';
		newThreadRootPath = null;
	};

	$: {
		const message = selectedPath ? getMessageAtPath(messages, selectedPath) : null;
		selectedMessage = message;
		if (selectedPath && !message) {
			selectedPath = null;
			forgetLastKey();
		}
	}

	const updateMessageAtPath = (path: number[], updater: (message: Message) => void) => {
		if (path.length === 0) {
			return;
		}

		const cloned = deepClone(messages);
		let target: Message | undefined = cloned[path[0]];
		for (let i = 1; i < path.length && target; i += 1) {
			target = target.children[path[i]];
		}

		if (!target) {
			return;
		}

		updater(target);
		messages = cloned;
		keyToPath = buildKeyPathMap(cloned);
	};

	const encodeToBase64 = (text: string): string =>
		encodeBytesToBase64(new TextEncoder().encode(text));

	// Convert a runtime Message into a cacheable IndexedDB record
	const messageToRecord = (message: Message): MessageRecord => {
		const normalizedMime =
			message.mimeType ??
			(message.isText ? 'text/plain; charset=utf-8' : 'application/octet-stream');
		const ensuredEncoded =
			message.encodedContent ??
			(message.isText && message.content ? encodeToBase64(message.content) : undefined);
		return {
			key: message.key ?? '',
			mimeType: normalizedMime,
			isText: message.isText ?? true,
			sizeBytes:
				message.sizeBytes ??
				(ensuredEncoded ? calculateBase64Size(ensuredEncoded) : (message.content?.length ?? 0)),
			content: message.isText ? (message.content ?? '') : undefined,
			encodedContent: ensuredEncoded,
			createdAt: message.createdAt,
			attachmentName: message.attachmentName
		};
	};

	onMount(() => {
		const params =
			typeof window !== 'undefined'
				? new URLSearchParams(window.location.search)
				: new URLSearchParams();
		authStatus = 'authenticating';
		void (async () => {
			try {
				const result = await bootstrapAuthFromParams(params, API_BASE_URL);
				authStatus = result.hasCredentials ? 'authenticated' : 'idle';
				authStatusText =
					result.message ??
					(result.hasCredentials
						? 'Authentication ready.'
						: 'Provide ?base64Key=…&base64Nonce=… from the server output to authenticate.');
			} catch (error) {
				authStatus = 'error';
				authStatusText = error instanceof Error ? error.message : String(error);
			} finally {
				await loadCachedThreadSummaries();
				await fetchNextThreadBatch();
				attemptInitialSelection();
			}
		})();
		if (typeof window !== 'undefined' && 'IntersectionObserver' in window) {
			sentinelObserver = new IntersectionObserver(
				(entries) => {
					entries.forEach((entry) => {
						if (entry.isIntersecting) {
							void fetchNextThreadBatch();
						}
					});
				},
				{ rootMargin: '200px' }
			);
		}
	});

	onDestroy(() => {
		if (authBannerTimer) {
			clearTimeout(authBannerTimer);
			authBannerTimer = null;
		}
		sentinelObserver?.disconnect();
		nodeStreamAbort?.abort();
		threadsAbort?.abort();
		resetHydrationWorker();
	});

	$: if (authStatusText !== lastAuthStatusText) {
		lastAuthStatusText = authStatusText;
		if (authBannerTimer) {
			clearTimeout(authBannerTimer);
			authBannerTimer = null;
		}
		const trimmed = authStatusText.trim();
		if (trimmed) {
			authBannerVisible = true;
			authBannerTimer = setTimeout(() => {
				authBannerVisible = false;
				authBannerTimer = null;
			}, AUTH_BANNER_TIMEOUT_MS);
		} else {
			authBannerVisible = false;
		}
	}

	$: if (sentinel && sentinelObserver && !sentinelAttached) {
		sentinelObserver.observe(sentinel);
		sentinelAttached = true;
	}

	const insertMessage = (newMessage: Message, parentPath: number[] | null): number[] => {
		if (parentPath && parentPath.length > 0) {
			const cloned = deepClone(messages);
			let parentNode: Message | undefined = cloned[parentPath[0]];
			for (let i = 1; i < parentPath.length && parentNode; i += 1) {
				parentNode = parentNode.children[parentPath[i]];
			}

			if (!parentNode) {
				const fallback = [...cloned, newMessage];
				messages = fallback;
				keyToPath = buildKeyPathMap(fallback);
				const fallbackPath = [fallback.length - 1];
				setSelectedPath(fallbackPath);
				return fallbackPath;
			}

			parentNode.children = [...parentNode.children, newMessage];
			messages = cloned;
			const newPath = [...parentPath, parentNode.children.length - 1];
			keyToPath = buildKeyPathMap(cloned);
			setSelectedPath(newPath);
			return newPath;
		}

		const cloned = deepClone(messages);
		cloned.push(newMessage);
		messages = cloned;
		const path = [cloned.length - 1];
		keyToPath = buildKeyPathMap(cloned);
		setSelectedPath(path);
		return path;
	};

	const persistMessage = async (path: number[]) => {
		const snapshot = getMessageAtPath(messages, path);
		if (!snapshot) {
			return;
		}

		const parentKey = snapshot.parentKey?.trim() ?? '';
		const base64Content = snapshot.encodedContent ?? encodeToBase64(snapshot.content);
		const inferredIsText =
			snapshot.isText !== undefined
				? snapshot.isText
				: (() => {
						const mime = snapshot.mimeType?.toLowerCase() ?? '';
						return (
							mime.startsWith('text/') ||
							mime.includes('json') ||
							mime.includes('xml') ||
							mime.includes('yaml')
						);
					})();
		const mimeType =
			snapshot.mimeType?.trim() ||
			(inferredIsText ? 'text/plain; charset=utf-8' : 'application/octet-stream');
		const sizeBytes = snapshot.sizeBytes ?? calculateBase64Size(base64Content);

		activeSaves += 1;
		statusState = 'sending';
		statusText = 'Saving message to Ouroboros…';

		updateMessageAtPath(path, (message) => {
			message.status = 'pending';
			delete message.error;
			message.parentKey = parentKey || undefined;
			message.encodedContent = base64Content;
			message.mimeType = mimeType;
			message.isText = inferredIsText;
			message.sizeBytes = sizeBytes;
		});

		const filename = snapshot.attachmentName?.trim()
			? snapshot.attachmentName.trim()
			: inferredIsText
				? 'message.txt'
				: `attachment-${crypto.randomUUID?.() ?? Date.now()}`;

		const metadata: Record<string, unknown> = {
			reed_solomon_shards: 0,
			reed_solomon_parity_shards: 0,
			mime_type: mimeType,
			is_text: inferredIsText,
			filename
		};
		if (parentKey) {
			metadata.parent = parentKey;
		}

		const sourceBytes = decodeBase64ToUint8(base64Content);
		const contentBytes = new Uint8Array(sourceBytes.length);
		contentBytes.set(sourceBytes);
		const blob = new Blob([contentBytes], { type: mimeType });
		const formData = new FormData();
		formData.append('file', blob, filename);
		formData.append('metadata', JSON.stringify(metadata));

		try {
			const response = await fetch(`${API_BASE_URL}/data`, {
				method: 'POST',
				body: formData,
				headers: await withAuthHeaders()
			});

			if (!response.ok) {
				const message = (await response.text()) || `Request failed with status ${response.status}`;
				throw new Error(message);
			}

			const body: { key?: string } = await response.json();
			const key = body.key ?? '';

			updateMessageAtPath(path, (message) => {
				message.status = 'saved';
				if (key) {
					message.key = key;
					rememberLastKey(key);
				}
				delete message.error;
			});

			// Persist newly saved message payload for future hydration
			const updatedMessage = getMessageAtPath(messages, path);
			if (updatedMessage && updatedMessage.key) {
				persistMessageRecord(messageToRecord(updatedMessage));
			}

			statusState = 'success';
			statusText = key ? `Message saved with key ${key}` : 'Message saved successfully.';
		} catch (error) {
			const errorMessage = error instanceof Error ? error.message : 'Unknown error';
			updateMessageAtPath(path, (message) => {
				message.status = 'failed';
				message.error = errorMessage;
			});
			statusState = 'error';
			statusText = `Failed to save message: ${errorMessage}`;
		} finally {
			activeSaves = Math.max(activeSaves - 1, 0);
			if (activeSaves === 0 && statusState === 'sending') {
				statusState = 'idle';
				statusText = '';
			}
		}
	};

	const addMessage = () => {
		if (newThreadMode) return;
		const trimmed = inputValue.trim();
		if (!trimmed) return;

		const parentPath = selectedPath ? [...selectedPath] : null;
		const parentKeyValue = parentPath ? (getMessageAtPath(messages, parentPath)?.key ?? '') : '';
		const textBytes = new TextEncoder().encode(trimmed);
		const base64Content = encodeBytesToBase64(textBytes);
		const newMessage: Message = {
			id: nextId++,
			content: trimmed,
			children: [],
			status: 'pending',
			error: undefined,
			parentKey: parentKeyValue || undefined,
			encodedContent: base64Content,
			mimeType: 'text/plain; charset=utf-8',
			isText: true,
			sizeBytes: textBytes.length
		};

		const newPath = insertMessage(newMessage, parentPath);
		inputValue = '';

		void persistMessage(newPath);
	};

	const openFilePicker = () => {
		fileInput?.click();
	};

	const handleFileSelection = async (event: Event) => {
		const input = event.target as HTMLInputElement | null;
		const files = input?.files;
		if (!files || files.length === 0) {
			return;
		}

		const parentPath = selectedPath ? [...selectedPath] : null;
		const parentKeyValue = parentPath ? (getMessageAtPath(messages, parentPath)?.key ?? '') : '';

		for (const file of Array.from(files)) {
			try {
				const base64 = await readFileAsBase64(file);
				const mimeType = file.type || 'application/octet-stream';
				const labelName = file.name && file.name.trim().length > 0 ? file.name : mimeType;
				const display = `${labelName} (${formatBytes(file.size)})`;

				const newMessage: Message = {
					id: nextId++,
					content: display,
					children: [],
					status: 'pending',
					error: undefined,
					parentKey: parentKeyValue || undefined,
					encodedContent: base64,
					mimeType,
					isText: false,
					sizeBytes: file.size,
					attachmentName: file.name || undefined
				};

				const newPath = insertMessage(newMessage, parentPath);
				await persistMessage(newPath);
			} catch (error) {
				console.error('Failed to attach file', error);
				statusState = 'error';
				statusText = `Failed to attach file: ${error instanceof Error ? error.message : 'Unknown error'}`;
			}
		}

		if (input) {
			input.value = '';
		}
	};

	const handleKeydown = (event: KeyboardEvent) => {
		if (event.key === 'Enter' && !event.shiftKey) {
			event.preventDefault();
			addMessage();
		}
	};
</script>

<main>
	{#if authBannerVisible && authStatusText}
		<div class={`auth-banner ${authStatus}`}>
			{authStatusText}
		</div>
	{/if}
	<div class="app-shell">
		<aside class="thread-sidebar">
			<div class="sidebar-header">
				<h2>Threads</h2>
				<button
					type="button"
					class="new-thread"
					on:click={startNewThread}
					disabled={statusState === 'sending'}
				>
					New thread
				</button>
			</div>
			{#if threadsLoading && threadSummaries.length === 0}
				<p class="thread-placeholder">Loading threads…</p>
			{:else if threadSummaries.length === 0}
				{#if threadsError}
					<p class="thread-placeholder error">{threadsError}</p>
				{:else}
					<p class="thread-placeholder">No threads yet. Start one!</p>
				{/if}
			{:else}
				<ul class="thread-list">
					{#each typedThreadSummaries as summaryValue, index (summaryKey(summaryValue, index))}
						{@const summary = summaryValue as ThreadSummary}
						<li>
							<button
								type="button"
								class="thread-item"
								class:active={selectedThreadKey === summary.key}
								on:click={() => handleSelectThreadSummary(summary)}
							>
								<div class="thread-item-title">{threadSummaryTitle(summary)}</div>
								<div class="thread-item-meta">
									{threadSummaryMeta(summary)}
								</div>
							</button>
						</li>
					{/each}
					<li class="thread-load-more" bind:this={sentinel}>
						{#if threadsLoading}
							<span>Loading more…</span>
						{:else if threadsError}
							<span class="error-text">{threadsError}</span>
						{:else if threadsComplete}
							<span>End of threads</span>
						{:else}
							<button type="button" on:click={() => fetchNextThreadBatch()}> Load more </button>
						{/if}
					</li>
				</ul>
			{/if}
		</aside>
		<section class="conversation-area">
			<div>
				<header class="conversation-header">
					<h1>{headerTitle}</h1>
					{#if headerMeta && (headerMeta.createdAt || headerMeta.childrenCount > 0 || headerMeta.lastChildAt)}
						{#if headerMeta.createdAt}
							<span class="meta-stat">
								<span class="meta-title">Created</span>
								<span class="meta-value">{formatTimestamp(headerMeta.createdAt)}</span>
							</span>
						{/if}
						<span class="meta-stat">
							<span class="meta-title">Children</span>
							<span class="meta-value"
								>{headerMeta.childrenCount} child{headerMeta.childrenCount === 1 ? '' : 'ren'}</span
							>
						</span>
						{#if headerMeta.lastChildAt}
							<span class="meta-stat">
								<span class="meta-title">Last reply</span>
								<span class="meta-value">{formatTimestamp(headerMeta.lastChildAt)}</span>
							</span>
						{/if}
						{#if headerMeta.descendantCount > headerMeta.childrenCount}
							<span class="meta-stat">
								<span class="meta-title">Replies</span>
								<span class="meta-value">{headerMeta.descendantCount} total</span>
							</span>
						{/if}
					{/if}
					{#if newThreadMode}
						<div class="new-thread-title">
							<input
								class="title-input"
								bind:this={titleInputEl}
								type="text"
								placeholder="Enter Title"
								bind:value={newThreadTitle}
								on:input={(e) => applyTitleToRoot((e.target as HTMLInputElement).value)}
								on:keydown={(e) => {
									if (e.key === 'Enter') {
										e.preventDefault();
										void createThreadFromTitle();
									}
								}}
								disabled={statusState === 'sending'}
							/>
							<button
								type="button"
								on:click={createThreadFromTitle}
								disabled={!newThreadTitle.trim() || statusState === 'sending'}
							>
								Create
							</button>
							<button type="button" on:click={cancelNewThread} disabled={statusState === 'sending'}>
								Cancel
							</button>
						</div>
					{:else}
						<p class="conversation-subtitle">
							{#if selectedMessage}
								Replying to <span class="snippet">“{truncate(selectedMessage.content)}”</span>
							{:else}
								Starting a new conversation
							{/if}
						</p>
					{/if}
				</header>

				<div class="chat-window">
					{#if nodeLoading && messages.length === 0}
						<p class="placeholder">Streaming thread…</p>
					{:else if nodeError && messages.length === 0}
						<p class="placeholder error">{nodeError}</p>
					{:else if messages.length === 0}
						<p class="placeholder">Select a thread to view its messages.</p>
					{:else}
						{#key messages[0].id}
							{#if (messages[0].children ?? []).length === 0}
								<p class="placeholder">This thread has no messages yet.</p>
							{:else}
								{#each messages[0].children as child, index (child.id)}
									<svelte:component
										this={MessageNode}
										message={child}
										level={0}
										path={[0, index]}
										{selectedPath}
										selectMessage={handleSelectMessage}
										apiBaseUrl={API_BASE_URL}
										getAuthHeaders={buildAuthHeaders}
									/>
								{/each}
							{/if}
						{/key}
					{/if}
				</div>
			</div>

			<div class="input-area">
				<!-- If newThreadMode, show title input and disable message area until title is created -->
				{#if newThreadMode}
					<div class="new-thread-inputs">
						<input
							type="text"
							placeholder="Thread title"
							bind:value={newThreadTitle}
							on:input={(e) => applyTitleToRoot((e.target as HTMLInputElement).value)}
							on:keydown={(e) => {
								if (e.key === 'Enter') {
									e.preventDefault();
									void createThreadFromTitle();
								}
							}}
							class="title-input-mobile"
							bind:this={titleInputEl}
						/>
						<button
							type="button"
							on:click={createThreadFromTitle}
							disabled={!newThreadTitle.trim() || statusState === 'sending'}
						>
							Create Thread
						</button>
						<button type="button" on:click={cancelNewThread} disabled={statusState === 'sending'}>
							Cancel
						</button>
					</div>
				{:else}
					<textarea
						bind:value={inputValue}
						rows="3"
						placeholder="Type a message and press Enter"
						on:keydown={handleKeydown}
					></textarea>
					<button
						type="button"
						class="attach-button"
						on:click={openFilePicker}
						disabled={statusState === 'sending'}
					>
						Attach
					</button>
					<button
						type="button"
						on:click={addMessage}
						disabled={!inputValue.trim().length || statusState === 'sending' || newThreadMode}
					>
						{statusState === 'sending' ? 'Sending…' : 'Send'}
					</button>
					<input
						type="file"
						class="file-input"
						bind:this={fileInput}
						on:change={handleFileSelection}
						multiple
						hidden
					/>
				{/if}
			</div>

			{#if nodeError && messages.length > 0}
				<div class="global-status error">
					{nodeError}
				</div>
			{/if}

			{#if statusState !== 'idle'}
				<div class={`global-status ${statusState}`}>
					{statusText}
				</div>
			{/if}
		</section>
	</div>
</main>

<style>
	:global(:root) {
		--surface: #111c2f;
		--surface-raised: #15213a;
		--surface-raised-transparent: rgba(21, 33, 58, 0);
		--surface-muted: #0b1528;
		--border: #1f2a3d;
		--border-strong: #2c3b55;
		--accent: #3b82f6;
		--accent-strong: #2563eb;
		--accent-soft: rgba(37, 99, 235, 0.12);
		--text-primary: #e2e8f0;
		--text-muted: #94a3b8;
		--status-success: #22c55e;
		--status-error: #ef4444;
		--status-info: #38bdf8;
		--shadow-strong: 0 18px 38px rgba(2, 6, 23, 0.45);
	}

	:global(body) {
		margin: 0;
		background: #0f172a;
		color: var(--text-primary);
		font-family:
			system-ui,
			-apple-system,
			BlinkMacSystemFont,
			'Segoe UI',
			sans-serif;
	}

	main {
		max-width: 1200px;
		margin: 0 auto;
		padding: 2.5rem 1.5rem 4rem;
	}

	.auth-banner {
		margin-bottom: 1rem;
		padding: 0.85rem 1.1rem;
		border-radius: 0.85rem;
		background: rgba(59, 130, 246, 0.15);
		border: 1px solid rgba(59, 130, 246, 0.4);
		color: #cbd5f5;
		font-weight: 500;
	}

	.auth-banner.authenticated {
		background: rgba(34, 197, 94, 0.18);
		border-color: rgba(34, 197, 94, 0.4);
		color: #bbf7d0;
	}

	.auth-banner.error {
		background: rgba(239, 68, 68, 0.2);
		border-color: rgba(239, 68, 68, 0.45);
		color: #fecaca;
	}

	.auth-banner.idle {
		background: rgba(234, 179, 8, 0.15);
		border-color: rgba(234, 179, 8, 0.4);
		color: #fef3c7;
	}

	.app-shell {
		display: grid;
		grid-template-columns: minmax(220px, 300px) minmax(0, 1fr);
		gap: 1.75rem;
		align-items: stretch;
	}

	.thread-sidebar {
		background: var(--surface);
		border: 1px solid var(--border);
		border-radius: 1rem;
		padding: 1.25rem;
		display: flex;
		flex-direction: column;
		gap: 1rem;
		min-height: 560px;
		box-shadow: var(--shadow-strong);
	}

	.sidebar-header {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 0.75rem;
	}

	.sidebar-header h2 {
		margin: 0;
		font-size: 1.1rem;
		font-weight: 600;
		color: var(--text-primary);
	}

	.new-thread {
		background: var(--accent);
		border: none;
		color: #f8fafc;
		border-radius: 999px;
		padding: 0.5rem 1.1rem;
		font-size: 0.9rem;
		font-weight: 600;
		cursor: pointer;
		transition:
			transform 0.2s ease,
			box-shadow 0.2s ease,
			filter 0.2s ease;
	}

	.new-thread:hover:enabled {
		transform: translateY(-1px);
		box-shadow: 0 10px 25px rgba(59, 130, 246, 0.3);
		filter: brightness(1.06);
	}

	.new-thread:disabled {
		opacity: 0.55;
		cursor: not-allowed;
	}

	.thread-placeholder {
		margin: 0;
		padding: 1.5rem 0.75rem;
		text-align: center;
		color: var(--text-muted);
		background: rgba(15, 23, 42, 0.65);
		border-radius: 0.85rem;
	}

	.thread-list {
		margin: 0;
		padding: 0;
		list-style: none;
		display: flex;
		flex-direction: column;
		gap: 0.6rem;
		overflow-y: auto;
	}

	.thread-list li {
		margin: 0;
	}

	.thread-item {
		background: var(--surface-muted);
		border: 1px solid transparent;
		border-radius: 0.85rem;
		padding: 0.75rem 0.9rem;
		cursor: pointer;
		display: flex;
		flex-direction: column;
		gap: 0.3rem;
		width: 100%;
		text-align: left;
		color: inherit;
		transition:
			transform 0.15s ease,
			border-color 0.15s ease,
			background-color 0.15s ease;
	}

	.thread-item:hover {
		transform: translateX(4px);
		border-color: var(--accent);
		background: var(--accent-soft);
	}

	.thread-item.active {
		border-color: rgba(59, 130, 246, 0.8);
		background: rgba(37, 99, 235, 0.2);
		box-shadow: inset 0 0 0 1px rgba(37, 99, 235, 0.35);
	}

	.thread-item.unsaved {
		border-color: rgba(234, 179, 8, 0.5);
		background: rgba(234, 179, 8, 0.16);
	}

	.thread-item-title {
		font-size: 0.95rem;
		font-weight: 600;
		color: var(--text-primary);
	}

	.thread-item-meta {
		font-size: 0.8rem;
		color: var(--text-muted);
	}

	.conversation-area {
		background: var(--surface-raised);
		border: 1px solid var(--border);
		border-radius: 1rem;
		padding: 1.5rem;
		display: flex;
		flex-direction: column;
		box-shadow: 0 22px 42px rgba(8, 11, 32, 0.5);
	}

	.conversation-header {
		display: grid;
		grid-template-columns: min-content min-content min-content auto;
		grid-auto-rows: min-content;
		grid-auto-flow: row;
		gap: 0.75rem;
		align-items: center;
		position: sticky;
		top: 0;
		background: var(--surface-raised);
		padding: 0.75rem;
	}

	.conversation-header h1 {
		margin: 0;
		font-size: 1.6rem;
		font-weight: 600;
		color: var(--text-primary);
		overflow-wrap: anywhere;
		white-space: normal;
		text-overflow: initial;
		grid-column: 1 / -1;
	}

	.new-thread-title {
		display: flex;
		align-items: center;
		gap: 0.6rem;
	}

	.title-input,
	.title-input-mobile {
		border-radius: 0.5rem;
		padding: 0.5rem 0.7rem;
		border: 1px solid var(--border);
		background: var(--surface);
		color: var(--text-primary);
		min-width: 240px;
	}

	.new-thread-inputs {
		display: flex;
		align-items: center;
		gap: 0.6rem;
	}

	.conversation-subtitle {
		margin: 0.35rem 0 0;
		font-size: 0.9rem;
		color: var(--text-muted);
	}

	.conversation-header .meta-stat {
		display: inline-flex;
		flex-direction: row;
		gap: 0.4rem;
		align-items: center;
		white-space: nowrap;
		font-size: 0.8rem;
	}

	.conversation-header .meta-title {
		color: var(--text-muted);
	}

	.conversation-header .conversation-subtitle,
	.conversation-header .new-thread-title {
		grid-column: 1 / -1;
		margin-top: 0.25rem;
	}

	.conversation-subtitle .snippet {
		font-weight: 600;
		color: var(--accent);
	}

	.chat-window {
		border-radius: 0.9rem;
		padding: 0.25rem;
		min-height: 380px;
		overflow-y: auto;
		background: var(--surface-muted);
		box-shadow: inset 0 0 0 1px rgba(15, 23, 42, 0.6);
		margin: 1rem 0;
	}

	.placeholder {
		text-align: center;
		color: var(--text-muted);
		margin-top: 3rem;
	}

	.placeholder.error {
		color: var(--status-error);
	}

	.input-area {
		display: grid;
		grid-template-columns: minmax(0, 1fr) auto auto;
		gap: 0.9rem;
		align-items: start;
		position: sticky;
		bottom: 0;
		background: var(--surface-raised);
		padding: 1rem 0;
		z-index: 5;
	}

	:global(:root) {
		--gradient-size: 18px;
	}

	.input-area::before {
		content: '';
		position: absolute;
		left: 0;
		right: 0;
		top: calc(var(--gradient-size) * -1);
		height: var(--gradient-size);
		pointer-events: none;
		background: linear-gradient(
			180deg,
			var(--surface-raised-transparent) 0%,
			var(--surface-raised) 100%
		);
	}

	.conversation-header::after {
		content: '';
		position: absolute;
		left: 0;
		right: 0;
		bottom: calc(var(--gradient-size) * -1);
		height: var(--gradient-size);
		pointer-events: none;
		background: linear-gradient(
			0deg,
			var(--surface-raised-transparent) 0%,
			var(--surface-raised) 100%
		);
	}

	textarea {
		border-radius: 0.85rem;
		border: 1px solid var(--border);
		padding: 0.85rem;
		font-size: 1rem;
		resize: none;
		font-family: inherit;
		background: var(--surface);
		color: var(--text-primary);
		box-shadow: inset 0 0 0 1px rgba(15, 23, 42, 0.6);
		caret-color: var(--accent);
	}

	textarea::placeholder {
		color: rgba(148, 163, 184, 0.8);
	}

	textarea:focus {
		outline: none;
		border-color: var(--accent);
		box-shadow: 0 0 0 3px rgba(59, 130, 246, 0.25);
	}

	button {
		border-radius: 0.85rem;
		padding: 0.75rem 1.5rem;
		font-size: 1rem;
		font-weight: 600;
		cursor: pointer;
		transition:
			transform 0.2s ease,
			box-shadow 0.2s ease,
			filter 0.2s ease;
		border: none;
	}

	button:hover:enabled {
		transform: translateY(-1px);
		filter: brightness(1.04);
	}

	button:active:enabled {
		transform: translateY(0);
		filter: brightness(0.98);
	}

	button:disabled {
		opacity: 0.55;
		cursor: not-allowed;
	}

	.attach-button {
		background: var(--surface);
		border: 1px solid var(--border-strong);
		color: var(--text-muted);
	}

	.attach-button:hover:enabled {
		box-shadow: 0 12px 24px rgba(15, 23, 42, 0.35);
		filter: brightness(1.05);
	}

	.input-area button:last-of-type {
		background: linear-gradient(135deg, var(--accent-strong), var(--accent));
		color: #f8fafc;
		box-shadow: 0 16px 32px rgba(37, 99, 235, 0.35);
	}

	.global-status {
		margin-top: 0.5rem;
		padding: 0.75rem 1rem;
		border-radius: 0.85rem;
		font-weight: 500;
		font-size: 0.95rem;
		box-shadow: 0 18px 30px rgba(15, 23, 42, 0.45);
		text-overflow: ellipsis;
		overflow: clip;
	}

	.global-status.sending {
		background: rgba(59, 130, 246, 0.18);
		color: #cbd5f5;
	}

	.global-status.success {
		background: rgba(34, 197, 94, 0.18);
		color: #bbf7d0;
	}

	.global-status.error {
		background: rgba(239, 68, 68, 0.22);
		color: #fecaca;
	}

	@media (max-width: 960px) {
		.app-shell {
			grid-template-columns: 1fr;
		}

		.thread-sidebar {
			flex-direction: column;
			min-height: auto;
		}

		.conversation-area {
			min-height: auto;
		}

		.conversation-header {
			grid-template-columns: 1fr;
		}
	}

	@media (max-width: 640px) {
		main {
			padding: 1.75rem 1rem 3rem;
		}

		.conversation-header {
			align-items: flex-start;
		}

		.input-area {
			grid-template-columns: 1fr;
		}

		.input-area button {
			width: 100%;
		}

		.chat-window {
			max-height: none;
			min-height: 320px;
		}
	}
</style>
