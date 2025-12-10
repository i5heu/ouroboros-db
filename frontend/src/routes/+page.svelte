<script lang="ts">
	import { onDestroy, onMount, tick } from 'svelte';
	import MessageNode from '../lib/components/MessageNode.svelte';
	import type { Message } from '../lib/types';
	import { bootstrapAuthFromParams, buildAuthHeaders } from '../lib/auth';
	import {
		streamThreadNodes,
		streamThreadSummaries,
		searchThreadSummaries,
		streamBulkMessages,
		lookupByComputedId,
		lookupWithData,
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
	import { terminateCacheWorker } from '../lib/cacheClient';

	// Prefer VITE_OUROBOROS_API when set; default to same origin so dev-server proxy reduces CORS
	const API_BASE_URL = import.meta.env.VITE_OUROBOROS_API ?? '';
	const LAST_KEY_STORAGE = 'ouroboros:lastKey';
	const AUTH_BANNER_TIMEOUT_MS = 1_500;

	type AuthStatus = 'idle' | 'authenticating' | 'authenticated' | 'error';

	type ThreadSummary = ThreadSummaryPayload & { cachedAt?: number };

	let threadSummaries: ThreadSummary[] = [];
	let typedThreadSummaries: ThreadSummary[] = [];
	$: typedThreadSummaries = threadSummaries;
	let searchQuery = '';
	$: filteredThreadSummaries = searchQuery
		? searchMode
			? typedThreadSummaries // In search mode, results are already filtered by the API
			: typedThreadSummaries.filter((s) => {
					const key = s.key?.toLowerCase() ?? '';
					const preview = s.preview?.toLowerCase() ?? '';
					const needle = searchQuery.trim().toLowerCase();
					return key.includes(needle) || preview.includes(needle);
				})
		: typedThreadSummaries;
	let threadCursor: string | null = null;
	let threadsLoading = false;
	let threadsError = '';
	let threadsComplete = false;
	let nodeLoading = false;
	let nodeError = '';
	let nodeProgressCount = 0;
	let nodeStreamAbort: AbortController | null = null;
	let threadsAbort: AbortController | null = null;
	let searchAbort: AbortController | null = null;
	let searchMode = false;
	let searchTimer: ReturnType<typeof setTimeout> | null = null;
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
	// Optional title for message being composed
	let inputTitle = '';
	let nextId = 1;
	let selectedPath: number[] | null = null;
	let selectedMessage: Message | null = null;
	let selectedComputedId: string | null = null;
	let computedIdCopied = false;
	let fileInput: HTMLInputElement | null = null;
	let activeSaves = 0;
	let statusState: 'idle' | 'sending' | 'success' | 'error' = 'idle';
	let statusText = '';
	let keyToPath = new Map<string, number[]>();
	let newThreadMode = false;
	let newThreadTitle = '';
	let newThreadRootPath: number[] | null = null;
	let titleInputEl: HTMLInputElement | null = null;
	let titleEditing = false;
	let editMode = false;
	let editTargetPath: number[] | null = null;

	const applyTitleToSelected = (title: string) => {
		if (!selectedPath) return;
		const selMsg = getMessageAtPath(messages, selectedPath);
		// Only apply title edits to selected messages that are pending (local/unpersisted) or have no key
		if (!selMsg || (selMsg.key && selMsg.status === 'saved')) {
			return;
		}
		const trimmed = title.trim();
		updateMessageAtPath(selectedPath, (message) => {
			message.title = trimmed ? trimmed : undefined;
			if (message.isText) {
				if ((message.content ?? '').trim().length === 0 && message.title) {
					// if text message blank but a title exists, reflect it as content for visibility
					message.content = message.title;
					message.encodedContent = message.encodedContent ?? encodeToBase64(message.content);
					message.sizeBytes =
						message.sizeBytes ?? calculateBase64Size(message.encodedContent ?? '');
				}
			} else {
				// For attachments, update the attachmentName and the displayed content
				message.attachmentName = trimmed ? trimmed : undefined;
				const nameForDisplay = message.attachmentName ?? message.attachmentName ?? '';
				const size = message.sizeBytes ?? 0;
				message.content = nameForDisplay
					? `${nameForDisplay} (${formatBytes(size)})`
					: attachmentLabel(message.mimeType ?? '', size);
			}
		});
		// if message has a key (saved) persist the record locally so the title is stored in cache
		const message = selectedPath ? getMessageAtPath(messages, selectedPath) : null;
		if (message && message.key) {
			persistMessageRecord(messageToRecord(message));
		}
	};
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

	// reactive helper: detect selected pending attachment and whether we can send
	$: selectedPendingAttachment = selectedPath ? getMessageAtPath(messages, selectedPath) : null;
	$: canSend =
		statusState !== 'sending' &&
		!newThreadMode &&
		(editMode
			? inputValue.trim().length > 0
			: inputValue.trim().length > 0 ||
				(selectedPendingAttachment &&
					selectedPendingAttachment.isText === false &&
					selectedPendingAttachment.status === 'pending'));

	// When we select a pending attachment, ensure the title input shows the title or filename
	$: if (
		selectedPendingAttachment &&
		!selectedPendingAttachment.isText &&
		selectedPendingAttachment.status === 'pending' &&
		!titleEditing
	) {
		const desired =
			selectedPendingAttachment.title ?? selectedPendingAttachment.attachmentName ?? '';
		if (inputTitle !== desired) inputTitle = desired;
	}

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
		const content = summary.title?.trim() ?? summary.preview?.trim() ?? '';
		if (!content.length) {
			return '[untitled]';
		}
		return truncate(content, 60);
	};

	// Full header title (do not truncate). Prefer summary preview when available, else root message.
	const threadHeaderTitle = (summary?: ThreadSummary, root?: Message | null): string => {
		const content =
			summary?.title?.trim() ??
			summary?.preview?.trim() ??
			root?.title?.trim() ??
			root?.content?.trim() ??
			'';
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

	// Check if query looks like a computed_id (contains colon separator)
	const isComputedIdQuery = (query: string): boolean => {
		return query.includes(':') && !query.startsWith('http');
	};

	// Simple in-memory cache for computed_id lookups
	const computedIdCache = new Map<string, { keys: string[]; timestamp: number }>();
	const COMPUTED_ID_CACHE_TTL = 60000; // 1 minute

	// Cache for message summaries (for search results)
	const messageSummaryCache = new Map<string, ThreadSummaryPayload>();

	const performSearch = async (query: string) => {
		// Abort previous search
		searchAbort?.abort();
		if (!query || query.trim().length === 0) {
			searchMode = false;
			threadCursor = null;
			threadSummaries = [];
			threadsComplete = false;
			await fetchNextThreadBatch();
			return;
		}
		searchMode = true;
		threadsComplete = true; // disable paging for search results
		threadsError = '';
		threadsLoading = true;
		const controller = new AbortController();
		searchAbort = controller;

		try {
			const trimmedQuery = query.trim();
			let results: ThreadSummaryPayload[] = [];

			// If query looks like a computed_id, try lookup first
			if (isComputedIdQuery(trimmedQuery)) {
				try {
					// Check in-memory cache first (instant)
					const cached = computedIdCache.get(trimmedQuery);
					if (cached && Date.now() - cached.timestamp < COMPUTED_ID_CACHE_TTL) {
						// Use cached keys - check message cache
						for (const key of cached.keys) {
							const cachedSummary = messageSummaryCache.get(key);
							if (cachedSummary) {
								results.push(cachedSummary);
							}
						}
						// If we have all cached, use them
						if (results.length === cached.keys.length && results.length > 0) {
							threadSummaries = results;
							threadsLoading = false;
							return;
						}
					}

					// Use the combined endpoint - single request for lookup + data
					const fetchedKeys: string[] = [];
					await lookupWithData({
						apiBaseUrl: API_BASE_URL,
						computedId: trimmedQuery,
						signal: controller.signal,
						getHeaders: buildAuthHeaders,
						onRecord: (record) => {
							if (!record || !record.key || !record.found) return;
							fetchedKeys.push(record.key);
							const preview = record.content ?? '';
							const summary: ThreadSummaryPayload = {
								key: record.key,
								preview: preview.slice(0, 240),
								title: record.title,
								mimeType: record.mimeType ?? 'application/octet-stream',
								isText: Boolean(record.isText),
								sizeBytes: record.sizeBytes ?? 0,
								childCount: 0,
								createdAt: record.createdAt,
								computedId: record.computedId
							};
							// Cache the summary
							messageSummaryCache.set(record.key, summary);
							results.push(summary);
						}
					});

					// Cache the computed_id -> keys mapping
					if (fetchedKeys.length > 0) {
						computedIdCache.set(trimmedQuery, { keys: fetchedKeys, timestamp: Date.now() });
					}
				} catch (lookupErr) {
					// If computed_id lookup fails, fall back to regular search
					console.error('Lookup error:', lookupErr);
				}
			}

			// If no results from computed_id lookup, do regular text search
			if (results.length === 0) {
				results = await searchThreadSummaries({
					apiBaseUrl: API_BASE_URL,
					query: trimmedQuery,
					limit: 50,
					getHeaders: buildAuthHeaders
				});
			}

			if (controller.signal.aborted) return;
			threadSummaries = results;
		} catch (err) {
			if (!controller.signal.aborted) {
				threadsError = err instanceof Error ? err.message : String(err);
			}
		} finally {
			if (!controller.signal.aborted) {
				threadsLoading = false;
			}
		}
	};

	const onSearchInputChanged = (value: string) => {
		if (searchTimer) clearTimeout(searchTimer);
		// Shorter debounce for faster response
		searchTimer = setTimeout(() => {
			void performSearch(value);
		}, 150);
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
			// Prefer a server-provided title on stream for attachments. If not
			// present, leave attachmentName undefined so it can be hydrated via
			// metadata (without overriding the key, which is used as a fallback).
			attachmentName: !node.isText ? (node.title ?? undefined) : undefined,
			createdAt: node.createdAt,
			createdAtMs,
			title: node.title,
			computedId: node.computedId,
			resolvedKey: node.key,
			suggestedEdit: node.suggestedEdit,
			editOf: node.editOf
		};
	};

	const upsertNodeFromStream = (node: ThreadNodePayload) => {
		// If this node is an edit of another message, do NOT display it separately.
		// Instead, update the original message to point to this edit, and register
		// the edit key so subsequent edits in the chain can find the original.
		// EXCEPTION: If messages is empty, this is the first/root node being streamed
		// (e.g., from a search selection), so we should display it even if it's an edit.
		if (node.editOf && messages.length > 0) {
			// Walk the edit chain to find the original (non-edit) message
			let originalKey = node.editOf;
			let visited = new Set<string>();
			while (originalKey && !visited.has(originalKey)) {
				visited.add(originalKey);
				const path = keyToPath.get(originalKey);
				if (path) {
					const msg = getMessageAtPath(messages, path);
					if (msg && msg.editOf) {
						// This message is itself an edit, keep walking back
						originalKey = msg.editOf;
					} else {
						// Found the original message
						break;
					}
				} else {
					break;
				}
			}

			const originalPath = keyToPath.get(originalKey);
			if (originalPath) {
				updateMessageAtPath(originalPath, (message) => {
					// Update the original message with the edit's content metadata
					message.mimeType = node.mimeType;
					message.isText = node.isText;
					message.sizeBytes = node.sizeBytes;
					// Keep original createdAt to preserve position in sort order
					if (node.computedId) {
						message.computedId = node.computedId;
					}
					// Track that this message has been edited and store the edit key
					message.suggestedEdit = node.key;
					message.resolvedKey = node.key;
					if (node.title) {
						message.title = node.title;
						if (!message.attachmentName) {
							message.attachmentName = node.title;
						}
					}
				});
				// Register the edit key to point to the original's path
				if (node.key) {
					keyToPath.set(node.key, originalPath);
				}
				requestMessageHydration(node);
				return;
			}
			// If original not found, skip this edit entirely - don't create a duplicate
			return;
		}

		const existingPath = node.key ? keyToPath.get(node.key) : undefined;
		if (existingPath) {
			updateMessageAtPath(existingPath, (message) => {
				message.mimeType = node.mimeType;
				message.isText = node.isText;
				message.sizeBytes = node.sizeBytes;
				message.createdAt = node.createdAt;
				message.createdAtMs = node.createdAt ? Date.parse(node.createdAt) : undefined;
				message.parentKey = nodeParentKey(node.parent);
				if (node.computedId) {
					message.computedId = node.computedId;
				}
				if (node.suggestedEdit) {
					message.suggestedEdit = node.suggestedEdit;
				}
				if (node.editOf) {
					message.editOf = node.editOf;
				}
				if (!node.isText) {
					message.content = attachmentLabel(node.mimeType, node.sizeBytes ?? 0);
				}
				if (node.title) {
					message.title = node.title;
					if (!message.attachmentName) {
						message.attachmentName = node.title;
					}
				}
			});
			requestMessageHydration(node);
			return;
		}

		const newMessage = createMessageFromNode(node);

		// If our messages list is empty, treat the first streamed node as the
		// root in the client view even if it reports a parent (this happens when
		// selecting a child from search results). This ensures the node is visible
		// and that replies reference it as the parent.
		if (messages.length === 0 || !node.parent) {
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
		// sort parent's children and recalc mapping
		sortMessageTree(parentNode.children);
		messages = cloned;
		keyToPath = buildKeyPathMap(messages);
		// Do not auto-select streamed child nodes — keep selection stable unless
		// the UI or user explicitly chooses a message. This prevents streamed
		// nodes from hijacking selection (and causing new attachments to be
		// posted as replies to a nested node).
		requestMessageHydration(node);
	};

	const applyRecordToMessage = (record: MessageRecord) => {
		const path = record.key ? keyToPath.get(record.key) : undefined;
		if (!path) {
			return;
		}
		updateMessageAtPath(path, (message) => {
			if (record.resolvedKey) {
				message.resolvedKey = record.resolvedKey;
				if (record.resolvedKey !== record.key) {
					keyToPath.set(record.resolvedKey, path);
				}
			}
			if (record.suggestedEdit) {
				message.suggestedEdit = record.suggestedEdit;
			}
			if (record.editOf) {
				message.editOf = record.editOf;
			}
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
			if (record.title) {
				message.title = record.title;
				// For attachments, also reflect the title into attachmentName so
				// the UI shows the friendly name after hydration.
				if (!message.isText) {
					// Don't override an existing attachmentName (e.g. user-edit), only set
					// when none is present yet.
					if (!message.attachmentName) {
						message.attachmentName = record.title;
					}
					const size = message.sizeBytes ?? 0;
					message.content = `${message.attachmentName} (${formatBytes(size)})`;
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
		const headerTitle = response.headers.get('X-Ouroboros-Title');
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
			createdAt,
			title: headerTitle || undefined
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
			resolvedKey: payload.resolvedKey ?? payload.key,
			suggestedEdit: payload.suggestedEdit,
			editOf: payload.editOf,
			mimeType: mime,
			isText: Boolean(payload.isText),
			sizeBytes:
				payload.sizeBytes ??
				(payload.content !== undefined ? new TextEncoder().encode(payload.content).length : 0),
			createdAt: payload.createdAt
		};
		if (payload.title) record.title = payload.title;
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
		// Apply computedId directly to the message (not stored in IndexedDB cache)
		if (payload.computedId) {
			const path = keyToPath.get(payload.key);
			if (path) {
				updateMessageAtPath(path, (message) => {
					message.computedId = payload.computedId;
				});
			}
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
			const record = metadata.isText
				? await fetchMessageRecord(metadata)
				: await fetchMessageMetadata(metadata);
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
		if (!node.key) {
			return;
		}
		// Use suggestedEdit key if available - this points to the latest edit content
		const hydrateKey = node.suggestedEdit || node.key;

		// Map the hydration key to the original's path BEFORE hydrating,
		// so when the result comes back, applyRecordToMessage can find it
		if (hydrateKey !== node.key) {
			const path = keyToPath.get(node.key);
			if (path) {
				keyToPath.set(hydrateKey, path);
			}
		}

		const cached = await readCachedMessageRecord(hydrateKey);
		if (cached) {
			applyRecordToMessage({ ...cached, key: node.key });
			return;
		}
		// For text nodes, we hydrate via the bulk worker (it will provide content
		// and metadata). For non-text nodes we avoid downloading the full payload
		// (could be large) and instead perform a small Range fetch to get headers
		// such as X-Ouroboros-Title.
		if (node.isText) {
			await enqueueBulkHydration(hydrateKey);
			return;
		}
		try {
			const record = await fetchMessageMetadata({ ...node, key: hydrateKey });
			if (record) {
				// Store with the hydrate key but apply to the original message
				persistMessageRecord(record);
				applyRecordToMessage({ ...record, key: node.key });
			}
		} catch (error) {
			console.warn('Failed to fetch message metadata', error);
		}
	};

	const fetchMessageMetadata = async (node: ThreadNodePayload): Promise<MessageRecord> => {
		if (!node.key) {
			throw new Error('Message key is required');
		}
		const headers = await withAuthHeaders();
		// Request just the first byte to get headers without fetching full payload
		headers['Range'] = 'bytes=0-0';
		const response = await fetch(`${API_BASE_URL}/data/${node.key}`, { headers });
		if (!response.ok) {
			const message = (await response.text()) || `Failed to load message ${node.key}`;
			throw new Error(message);
		}
		const headerMime =
			response.headers.get('X-Ouroboros-Mime') ?? response.headers.get('Content-Type');
		const mimeType = headerMime ?? node.mimeType ?? 'application/octet-stream';
		const headerIsText = response.headers.get('X-Ouroboros-Is-Text');
		const isText = headerIsText ? headerIsText.toLowerCase() === 'true' : node.isText;
		const createdAt = response.headers.get('X-Ouroboros-Created-At') ?? node.createdAt;
		const headerTitle = response.headers.get('X-Ouroboros-Title');
		const resolvedKey = response.headers.get('X-Ouroboros-Key') ?? node.key;
		const suggestedEdit = response.headers.get('X-Ouroboros-Suggested-Edit') ?? undefined;
		const editOf = response.headers.get('X-Ouroboros-Edit-Of') ?? undefined;
		const resolvedSize = node.sizeBytes ?? 0;
		return {
			key: node.key,
			resolvedKey,
			suggestedEdit: suggestedEdit || undefined,
			editOf: editOf || undefined,
			mimeType,
			isText,
			sizeBytes: resolvedSize,
			createdAt,
			title: headerTitle || undefined
		};
	};

	const requestMessageHydration = (node: ThreadNodePayload) => {
		if (!node.key || messageHydrations.has(node.key)) {
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
			timeStyle: 'medium'
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
				// Also map suggestedEdit and resolvedKey to the same path,
				// so hydration results from edit keys can find the original message
				if (node.suggestedEdit && node.suggestedEdit !== node.key) {
					map.set(node.suggestedEdit, current);
				}
				if (node.resolvedKey && node.resolvedKey !== node.key) {
					map.set(node.resolvedKey, current);
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

	const findPathById = (nodes: Message[], id: number, prefix: number[] = []): number[] | null => {
		for (let i = 0; i < nodes.length; i += 1) {
			const node = nodes[i];
			const current = [...prefix, i];
			if (node.id === id) return current;
			const childResult = findPathById(node.children, id, current);
			if (childResult) return childResult;
		}
		return null;
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

	const copyComputedId = async () => {
		if (!selectedComputedId) return;
		try {
			await navigator.clipboard.writeText(selectedComputedId);
			computedIdCopied = true;
			setTimeout(() => {
				computedIdCopied = false;
			}, 2000);
		} catch {
			// Fallback: create a temporary input element
			const input = document.createElement('input');
			input.value = selectedComputedId;
			document.body.appendChild(input);
			input.select();
			document.execCommand('copy');
			document.body.removeChild(input);
			computedIdCopied = true;
			setTimeout(() => {
				computedIdCopied = false;
			}, 2000);
		}
	};

	const setSelectedPath = (path: number[] | null) => {
		selectedPath = path ? [...path] : null;
		if (
			editMode &&
			(!editTargetPath ||
				!path ||
				editTargetPath.length !== path.length ||
				editTargetPath.some((v, idx) => path![idx] !== v))
		) {
			cancelEdit();
		}
		const message = selectedPath ? getMessageAtPath(messages, selectedPath) : null;
		if (message?.key) {
			rememberLastKey(message.key);
			// Use computedId from the message data (populated from stream/bulk endpoints)
			selectedComputedId = message.computedId || null;
		} else if (!selectedPath) {
			forgetLastKey();
			selectedComputedId = null;
		} else {
			selectedComputedId = null;
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
				message.title = title.trim() ? title : undefined;
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
		// After updating a message, re-sort affected lists to maintain chronological order
		sortMessageTree(cloned);
		messages = cloned;
		keyToPath = buildKeyPathMap(cloned);
		// Keep the selected path pointing to the same message (by id) if still selected
		if (selectedPath) {
			const selectedMsg = getMessageAtPath(messages, selectedPath);
			if (selectedMsg) {
				const newSelectedPath = findPathById(messages, selectedMsg.id);
				if (newSelectedPath) setSelectedPath(newSelectedPath);
			} else {
				// If message isn't found, clear selection
				setSelectedPath(null);
			}
		}
		// If message has been saved previously, persist updated record into local cache
		const updatedMessage = getMessageAtPath(messages, path);
		if (updatedMessage && updatedMessage.key) {
			persistMessageRecord(messageToRecord(updatedMessage));
		}
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
			resolvedKey: message.resolvedKey ?? message.key,
			suggestedEdit: message.suggestedEdit,
			editOf: message.editOf,
			mimeType: normalizedMime,
			isText: message.isText ?? true,
			sizeBytes:
				message.sizeBytes ??
				(ensuredEncoded ? calculateBase64Size(ensuredEncoded) : (message.content?.length ?? 0)),
			content: message.isText ? (message.content ?? '') : undefined,
			encodedContent: ensuredEncoded,
			createdAt: message.createdAt,
			attachmentName: message.attachmentName,
			title: message.title
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
		searchAbort?.abort();
		if (searchTimer) clearTimeout(searchTimer);
		resetHydrationWorker();
		terminateCacheWorker();
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
			// sort children and re-calc mapping and selection
			sortMessageTree(parentNode.children);
			messages = cloned;
			keyToPath = buildKeyPathMap(cloned);
			const newPath = findPathById(messages, newMessage.id);
			if (newPath) setSelectedPath(newPath);
			return newPath ?? [...parentPath, parentNode.children.length - 1];
		}

		const cloned = deepClone(messages);
		cloned.push(newMessage);
		sortMessageTree(cloned);
		messages = cloned;
		keyToPath = buildKeyPathMap(cloned);
		const path = findPathById(messages, newMessage.id) ?? [cloned.length - 1];
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
		if (snapshot.title) {
			metadata.title = snapshot.title;
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

	const canEditMessage = (message: Message | null): boolean => {
		return Boolean(message && message.key && message.isText && message.status === 'saved');
	};

	const addMessage = async () => {
		if (newThreadMode || editMode) return;
		const trimmed = inputValue.trim();
		if (!trimmed) return;

		const parentPath = selectedPath ? [...selectedPath] : messages.length > 0 ? [0] : null;
		// If replying to a local-only parent without a key, persist the parent first
		if (parentPath && parentPath.length > 0) {
			const parentNode = getMessageAtPath(messages, parentPath);
			if (parentNode && !parentNode.key) {
				// Persist parent so it has a key to attach as parent for this reply
				await persistMessage(parentPath);
			}
		}
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
			sizeBytes: textBytes.length,
			title: inputTitle.trim() ? inputTitle.trim() : undefined
		};

		// will be declared at top-level after variable declarations

		const newPath = insertMessage(newMessage, parentPath);
		inputValue = '';

		void persistMessage(newPath);
		// reset title input for next message
		inputTitle = '';
	};

	const openFilePicker = () => {
		if (editMode) return;
		fileInput?.click();
	};

	const handleFileSelection = async (event: Event) => {
		if (editMode) return;
		const input = event.target as HTMLInputElement | null;
		const files = input?.files;
		if (!files || files.length === 0) {
			return;
		}

		const parentPath = selectedPath ? [...selectedPath] : messages.length > 0 ? [0] : null;
		// If replying to a local-only parent without a key, persist the parent first
		if (parentPath && parentPath.length > 0) {
			const parentNode = getMessageAtPath(messages, parentPath);
			if (parentNode && !parentNode.key) {
				await persistMessage(parentPath);
			}
		}
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
				// Do not persist automatically; let the user add a title before sending.
				inputTitle = file.name || '';
				setSelectedPath(newPath);
				void tick().then(() => titleInputEl?.focus());
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

	const startEditingSelected = async (pathArg?: number[]) => {
		// If a path is provided, ensure the message is selected first.
		if (pathArg) setSelectedPath(pathArg);
		if (!selectedPath) return;
		const target = getMessageAtPath(messages, selectedPath);
		if (!canEditMessage(target)) return;

		editMode = true;
		editTargetPath = [...selectedPath];
		statusState = 'idle';
		statusText = '';

		// Ensure the message is hydrated so the textarea shows the latest text.
		if (target?.key && (!target.content || target.content === textLoadingPlaceholder)) {
			await enqueueBulkHydration(target.key);
			const refreshed = getMessageAtPath(messages, selectedPath);
			if (refreshed?.content) {
				inputValue = refreshed.content;
			}
		} else {
			inputValue = target?.content ?? '';
		}
		inputTitle = target?.title ?? '';
	};

	const cancelEdit = () => {
		editMode = false;
		editTargetPath = null;
		inputValue = '';
		inputTitle = '';
		if (statusState === 'success') {
			statusState = 'idle';
			statusText = '';
		}
	};

	const submitEdit = async () => {
		if (!editMode || !editTargetPath) return;
		const target = getMessageAtPath(messages, editTargetPath);
		if (!target || !target.key || !target.isText) {
			cancelEdit();
			return;
		}

		const trimmedContent = inputValue.trim();
		if (!trimmedContent) {
			statusState = 'error';
			statusText = 'Cannot save an empty edit.';
			return;
		}

		const mimeType =
			target.mimeType?.trim() ||
			(target.isText ? 'text/plain; charset=utf-8' : 'application/octet-stream');
		const title = inputTitle.trim() || target.title || '';
		const base64Content = encodeToBase64(trimmedContent);
		const sizeBytes = calculateBase64Size(base64Content);

		activeSaves += 1;
		statusState = 'sending';
		statusText = 'Saving edit…';

		const metadata: Record<string, unknown> = {
			reed_solomon_shards: 0,
			reed_solomon_parity_shards: 0,
			mime_type: mimeType,
			is_text: true,
			filename: 'message.txt',
			// Use resolvedKey or suggestedEdit if available - this ensures edits form
			// a chain (Test1 → Test2 → Test3 → Test4) rather than all pointing to the
			// original (Test1 ← Test2, Test1 ← Test3, Test1 ← Test4).
			edit_of: target.resolvedKey || target.suggestedEdit || target.key
		};
		if (target.parentKey) {
			metadata.parent = target.parentKey;
		}
		if (title) {
			metadata.title = title;
		}

		const contentBytes = decodeBase64ToUint8(base64Content);
		const blob = new Blob([contentBytes], { type: mimeType });
		const formData = new FormData();
		formData.append('file', blob, 'message.txt');
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

			updateMessageAtPath(editTargetPath, (message) => {
				message.content = trimmedContent;
				message.encodedContent = base64Content;
				message.sizeBytes = sizeBytes;
				message.title = title || undefined;
				message.suggestedEdit = key || message.suggestedEdit;
				message.resolvedKey = key || message.resolvedKey || message.key;
				message.status = 'saved';
				delete message.error;
			});

			const updated = getMessageAtPath(messages, editTargetPath);
			if (updated && updated.key) {
				// Cache with both the original key AND the new edit key.
				// On reload, we'll hydrate using suggestedEdit (the new key),
				// so it needs to be in the cache.
				const record = messageToRecord(updated);
				persistMessageRecord(record);
				if (key && key !== updated.key) {
					// Also cache under the new edit key so hydration can find it
					persistMessageRecord({ ...record, key: key });
				}
			}

			statusState = 'success';
			statusText = key ? `Edit saved with key ${key}` : 'Edit saved.';
			editMode = false;
			editTargetPath = null;
			inputValue = '';
			inputTitle = '';
		} catch (error) {
			const errorMessage = error instanceof Error ? error.message : 'Unknown error';
			statusState = 'error';
			statusText = `Failed to save edit: ${errorMessage}`;
		} finally {
			activeSaves = Math.max(activeSaves - 1, 0);
			if (activeSaves === 0 && statusState === 'sending') {
				statusState = 'idle';
				statusText = '';
			}
		}
	};

	const handleKeydown = (event: KeyboardEvent) => {
		if (event.key === 'Enter' && !event.shiftKey) {
			event.preventDefault();
			if (editMode) {
				void submitEdit();
			} else {
				addMessage();
			}
		}
	};
</script>

<main>
	{#if authBannerVisible && authStatusText}
		<div class={`auth-banner ${authStatus}`}>
			{authStatusText}
		</div>
	{/if}
	<aside class="thread-sidebar">
		<div class="global-search">
			<div class="search-input">
				<svg class="search-icon" viewBox="0 0 24 24" aria-hidden="true"
					><path
						fill="currentColor"
						d="M15.5 14h-.79l-.28-.27A6.471 6.471 0 0 0 16 9.5 6.5 6.5 0 1 0 9.5 16c1.61 0 3.09-.59 4.23-1.57l.27.28v.79l5 4.99L20.49 19l-4.99-5zM4 9.5C4 6.46 6.46 4 9.5 4S15 6.46 15 9.5 12.54 15 9.5 15 4 12.54 4 9.5z"
					></path></svg
				>
				<input
					type="search"
					aria-label="Search messages and threads by text or path"
					placeholder="Search or enter path (e.g. root:child:topic)…"
					bind:value={searchQuery}
					on:input={(e) => onSearchInputChanged((e.target as HTMLInputElement).value)}
				/>
				{#if searchQuery}
					<button
						type="button"
						class="search-clear"
						on:click={() => {
							searchQuery = '';
							onSearchInputChanged('');
						}}
						aria-label="Clear search">×</button
					>
				{/if}
			</div>
		</div>
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
				{#each filteredThreadSummaries as summaryValue, index (summaryKey(summaryValue, index))}
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
				{#if selectedComputedId}
					<div class="computed-id-display">
						<span class="computed-id-label">Path:</span>
						<code class="computed-id-value">{selectedComputedId}</code>
						<button
							type="button"
							class="copy-computed-id"
							on:click={copyComputedId}
							title="Copy path to clipboard"
							aria-label="Copy path to clipboard"
						>
							{#if computedIdCopied}
								✓
							{:else}
								📋
							{/if}
						</button>
					</div>
				{/if}
				{#if editMode}
					<div class="edit-controls">
						<button type="button" on:click={cancelEdit} disabled={statusState === 'sending'}>
							Cancel edit
						</button>
					</div>
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
						<!-- Render the root message at the very top of the chat window -->
						{@html ''}
						<svelte:component
							this={MessageNode}
							message={messages[0]}
							level={0}
							path={[0]}
							{selectedPath}
							selectMessage={handleSelectMessage}
							apiBaseUrl={API_BASE_URL}
							getAuthHeaders={buildAuthHeaders}
							startEditing={startEditingSelected}
							forceShowEdit={true}
						/>

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
									startEditing={startEditingSelected}
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
				<div class="message-input-wrapper">
					<input
						type="text"
						placeholder={selectedPendingAttachment &&
						!selectedPendingAttachment.isText &&
						selectedPendingAttachment.status === 'pending'
							? 'Filename or title'
							: selectedMessage && !selectedMessage.isText && selectedMessage.key
								? 'Reply title (optional)'
								: 'Optional title'}
						aria-label={selectedPendingAttachment &&
						!selectedPendingAttachment.isText &&
						selectedPendingAttachment.status === 'pending'
							? 'Edit attached filename or title'
							: 'Optional message title'}
						bind:value={inputTitle}
						class="title-input-mobile"
						on:input={(e) => {
							const value = (e.target as HTMLInputElement).value;
							// always update the composer title string
							inputTitle = value;
							// but only update the selected message's title for pending attachments
							if (
								selectedPendingAttachment &&
								!selectedPendingAttachment.isText &&
								selectedPendingAttachment.status === 'pending'
							) {
								applyTitleToSelected(value);
							}
						}}
						on:focus={() => (titleEditing = true)}
						on:blur={() => (titleEditing = false)}
						on:keydown={(e) => {
							if (e.key === 'Enter') {
								e.preventDefault();
								const selectedMsg = selectedPath ? getMessageAtPath(messages, selectedPath) : null;
								if (selectedMsg && !selectedMsg.isText && selectedMsg.status === 'pending') {
									void persistMessage(selectedPath!);
									inputTitle = '';
								} else {
									const ta = document.querySelector(
										'.input-area textarea'
									) as HTMLTextAreaElement | null;
									ta?.focus();
								}
							}
						}}
					/>
					{#if !(selectedPendingAttachment && !selectedPendingAttachment.isText && selectedPendingAttachment.status === 'pending')}
						{#if editMode}
							<div class="edit-banner">Editing message</div>
						{/if}
						<textarea
							bind:value={inputValue}
							rows="3"
							placeholder={editMode
								? 'Edit message and press Enter'
								: 'Type a message and press Enter'}
							on:keydown={handleKeydown}
						></textarea>
					{:else}
						<div class="attachment-pending">
							<div class="attachment-meta">
								<span class="attachment-name"
									>{selectedPendingAttachment.attachmentName ??
										selectedPendingAttachment.content}</span
								>
								{#if selectedPendingAttachment.sizeBytes}
									<span class="attachment-size"
										>{formatBytes(selectedPendingAttachment.sizeBytes)}</span
									>
								{/if}
							</div>
						</div>
					{/if}
				</div>
				<button
					type="button"
					class="attach-button"
					class:attached={selectedPendingAttachment &&
						selectedPendingAttachment.status === 'pending'}
					on:click={openFilePicker}
					disabled={statusState === 'sending' || editMode}
				>
					{#if selectedPendingAttachment && selectedPendingAttachment.status === 'pending' && !selectedPendingAttachment.isText}
						Attached
					{:else}
						Attach
					{/if}
				</button>
				<button
					type="button"
					on:click={() => {
						if (editMode) {
							void submitEdit();
							return;
						}
						const selectedMsg = selectedPath ? getMessageAtPath(messages, selectedPath) : null;
						if (selectedMsg && !selectedMsg.isText && selectedMsg.status === 'pending') {
							void persistMessage(selectedPath!);
							inputTitle = '';
							// Clear selected pending attachment after sending
							setSelectedPath(null);
						} else {
							void addMessage();
						}
					}}
					disabled={!canSend}
				>
					{#if editMode}
						{statusState === 'sending' ? 'Saving…' : 'Save edit'}
					{:else}
						{statusState === 'sending' ? 'Sending…' : 'Send'}
					{/if}
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
		padding: 0;
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
		display: grid;
		height: 100vh;
		min-height: 100vh;
		width: 100%;
		gap: 1rem;
		padding: 1.5rem;
		box-sizing: border-box;
		align-items: stretch;
		overflow: hidden;

		grid-template-columns: 1fr 5fr;
		grid-template-rows: auto;
	}

	.auth-banner {
		position: absolute;
		top: 2.5rem;
		left: 2.5rem;
		right: 2.5rem;
		z-index: 500;
		padding: 0.85rem 1.1rem;
		border-radius: 0.85rem;
		background: rgb(59, 131, 246);
		border: 1px solid rgba(59, 130, 246, 0.4);
		color: #cbd5f5;
		font-weight: 500;
	}

	.auth-banner.authenticated {
		background: rgb(34, 197, 94);
		border-color: rgba(34, 197, 94, 0.686);
		color: #bbf7d0;
	}

	.auth-banner.error {
		background: rgb(239, 68, 68);
		border-color: rgba(239, 68, 68, 0.45);
		color: #fecaca;
	}

	.auth-banner.idle {
		background: rgba(234, 179, 8);
		border-color: rgba(234, 179, 8, 0.4);
		color: #fef3c7;
	}

	.thread-sidebar {
		background: var(--surface);
		border: 1px solid var(--border);
		border-radius: 1rem;
		padding: 1.25rem;
		display: flex;
		flex-direction: column;
		gap: 1rem;
		overflow: hidden;
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

	.global-search {
		display: block;
		margin-bottom: 0.6rem;
	}

	.global-search .search-input {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		background: var(--surface-muted);
		border-radius: 0.75rem;
		padding: 0.4rem 0.6rem;
		border: 1px solid var(--border);
	}

	.global-search input[type='search'] {
		flex: 1 1 auto;
		min-width: 0; /* allow shrinking inside flex */
		background: transparent;
		border: none;
		color: var(--text-primary);
		padding: 0.35rem 0.25rem;
		font-size: 0.95rem;
		outline: none;
	}

	/* Remove browser-native search clear button so we can use our own clear UI */
	.global-search input[type='search']::-webkit-search-decoration,
	.global-search input[type='search']::-webkit-search-cancel-button,
	.global-search input[type='search']::-webkit-search-results-button,
	.global-search input[type='search']::-webkit-search-results-decoration {
		-webkit-appearance: none;
		appearance: none;
		display: none;
	}

	.global-search input[type='search']::-ms-clear,
	.global-search input[type='search']::-ms-expand {
		display: none;
		width: 0;
		height: 0;
	}

	.global-search .search-icon {
		width: 1rem;
		height: 1rem;
		color: var(--text-muted);
		flex: 0 0 auto;
	}

	.global-search .search-clear {
		background: transparent;
		border: none;
		color: var(--text-muted);
		font-size: 1.05rem;
		cursor: pointer;
		padding: 0 0.25rem;
		border-radius: 0.5rem;
		flex: 0 0 auto; /* constrain within flex */
		width: 1.6rem;
		height: 1.6rem;
		line-height: 1.6rem;
		text-align: center;
	}

	.global-search .search-clear:hover,
	.global-search .search-clear:focus {
		color: var(--text-primary);
		outline: none;
		background: rgba(255, 255, 255, 0.02);
	}

	.thread-list {
		margin: 0;
		padding: 5px;
		list-style: none;
		display: flex;
		flex-direction: column;
		gap: 0.6rem;
		flex: 1 1 auto;
		overflow-y: auto;
		min-height: 0;
		scrollbar-width: thin;
		scrollbar-color: rgba(59, 130, 246, 0.75) transparent;
	}

	.thread-list::-webkit-scrollbar {
		width: 0.45rem;
	}

	.thread-list::-webkit-scrollbar-track {
		background: linear-gradient(
			180deg,
			transparent 0%,
			rgba(15, 23, 42, 0.35) 40%,
			transparent 100%
		);
		border-radius: 999px;
	}

	.thread-list::-webkit-scrollbar-thumb {
		background: linear-gradient(180deg, rgba(96, 165, 250, 0.85), rgba(59, 130, 246, 0.85));
		border-radius: 999px;
		box-shadow: 0 0 6px rgba(15, 23, 42, 0.35);
	}

	.thread-list::-webkit-scrollbar-thumb:hover {
		background: linear-gradient(180deg, rgba(96, 165, 250, 1), rgba(37, 99, 235, 1));
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
		/* Preserve whitespace and show line breaks from message previews */
		white-space: pre-wrap;
		/* Keep long words from overflowing */
		word-break: break-word;
	}

	.thread-item-meta {
		font-size: 0.8rem;
		color: var(--text-muted);
	}

	.conversation-area {
		background: var(--surface-raised);
		border: 1px solid var(--border);
		border-radius: 1rem;
		padding: 0 1.5rem;
		display: flex;
		flex-direction: column;
		box-shadow: 0 22px 42px rgba(8, 11, 32, 0.5);
		overflow-y: auto;
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
		z-index: 100;
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

	.computed-id-display {
		grid-column: 1 / -1;
		display: flex;
		align-items: center;
		gap: 0.5rem;
		margin-top: 0.5rem;
		padding: 0.4rem 0.6rem;
		background: var(--surface-muted);
		border-radius: 0.4rem;
		border: 1px solid var(--border);
		font-size: 0.8rem;
	}

	.edit-controls {
		grid-column: 1 / -1;
		display: flex;
		align-items: center;
		gap: 0.5rem;
	}

	.edit-banner {
		margin-bottom: 0.25rem;
		font-weight: 600;
		color: var(--accent);
	}

	.computed-id-label {
		color: var(--text-muted);
		font-weight: 500;
	}

	.computed-id-value {
		color: var(--accent);
		font-family: 'Menlo', 'Monaco', 'Courier New', monospace;
		font-size: 0.75rem;
		background: var(--surface);
		padding: 0.2rem 0.4rem;
		border-radius: 0.25rem;
		word-break: break-all;
		max-width: 100%;
		overflow: hidden;
		text-overflow: ellipsis;
	}

	.copy-computed-id {
		background: none;
		border: none;
		cursor: pointer;
		padding: 0.2rem 0.4rem;
		border-radius: 0.25rem;
		font-size: 0.9rem;
		transition: background 0.15s ease;
	}

	.copy-computed-id:hover {
		background: var(--accent-soft);
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
		overflow-y: auto;
		background: var(--surface-muted);
		box-shadow: inset 0 0 0 1px rgba(15, 23, 42, 0.6);
		margin: 1rem 0;
	}

	.placeholder {
		text-align: center;
		color: var(--text-muted);
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

	.message-input-wrapper {
		display: flex;
		flex-direction: column;
		gap: 0.45rem;
		min-width: 0;
	}

	/* Attach button attached state */
	.attach-button.attached {
		background: var(--accent-soft);
		border: 1px solid var(--accent);
		color: var(--text-primary);
	}

	.input-area .title-input-mobile {
		min-width: 0; /* allow shrinking inside the input-area */
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
		.thread-sidebar {
			flex-direction: column;
			min-height: auto;
			max-height: none;
			position: static;
			align-self: stretch;
			overflow: visible;
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
		}
	}
</style>
