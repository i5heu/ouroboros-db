<script lang="ts">
	import { onMount } from 'svelte';
	import MessageNode from '../lib/components/MessageNode.svelte';
	import type { Message } from '../lib/types';

	const API_BASE_URL = import.meta.env.VITE_OUROBOROS_API ?? 'http://localhost:8083';
	const LAST_KEY_STORAGE = 'ouroboros:lastKey';

	type PersistedRecord = {
		key: string;
		content: string;
		isText: boolean;
		mimeType?: string;
		parent?: string;
		createdAt?: string;
		createdAtMs?: number;
	};

	let messages: Message[] = [];
	let inputValue = '';
	let nextId = 1;
	let selectedPath: number[] | null = null;
	let selectedMessage: Message | null = null;
	let selectedThreadIndex: number | null = null;
	let fileInput: HTMLInputElement | null = null;
	let activeSaves = 0;
	let statusState: 'idle' | 'sending' | 'success' | 'error' = 'idle';
	let statusText = '';
	let loading = false;
	let loadError = '';
	let keyToPath = new Map<string, number[]>();

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

	const formatPersistedContent = (record: PersistedRecord): string => {
		try {
			const bytes = decodeBase64ToUint8(record.content);
			if (record.isText) {
				return new TextDecoder().decode(bytes);
			}
			const mime = record.mimeType?.trim() || 'binary';
			return `[${mime} • ${formatBytes(bytes.length)}]`;
		} catch (error) {
			if (record.isText && typeof globalThis.atob === 'function') {
				try {
					return globalThis.atob(record.content);
				} catch (innerError) {
					console.error('Failed to decode text payload', innerError);
				}
			}
			const fallbackMime = record.mimeType?.trim() || 'binary';
			return `[${fallbackMime} attachment]`;
		}
	};

	const calculateBase64Size = (value: string): number => {
		if (!value.length) return 0;
		const padding = value.endsWith('==') ? 2 : value.endsWith('=') ? 1 : 0;
		return Math.floor((value.length * 3) / 4) - padding;
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

	const threadTitle = (message: Message): string => {
		const content = message.content?.trim() ?? '';
		if (!content.length) {
			return '[untitled]';
		}
		return truncate(content, 60);
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

	const createMessageFromRecord = (record: PersistedRecord, id: number): Message => {
		const messageSize = calculateBase64Size(record.content);
		const displayContent = formatPersistedContent(record);
		const createdAtMs =
			typeof record.createdAtMs === 'number' && Number.isFinite(record.createdAtMs)
				? record.createdAtMs
				: undefined;
		return {
			id,
			content: displayContent,
			children: [],
			status: 'saved',
			key: record.key,
			error: undefined,
			parentKey: record.parent && record.parent.trim().length > 0 ? record.parent : undefined,
			encodedContent: record.content,
			mimeType: record.mimeType,
			isText: record.isText,
			sizeBytes: messageSize,
			attachmentName: !record.isText ? record.key : undefined,
			createdAt: record.createdAt,
			createdAtMs
		};
	};

	const fetchThreadTree = async (
		key: string,
		state: { nextId: number },
		errors: string[],
		path: Set<string> = new Set<string>()
	): Promise<Message | null> => {
		const normalized = key.trim();
		if (!normalized.length || path.has(normalized)) {
			return null;
		}

		path.add(normalized);
		try {
			const record = await fetchMessageRecord(normalized);
			const message = createMessageFromRecord(record, state.nextId++);

			let childKeys: string[] = [];
			try {
				childKeys = await fetchChildKeys(normalized);
			} catch (error) {
				errors.push(error instanceof Error ? error.message : String(error));
			}

			for (const childKey of childKeys) {
				try {
					const child = await fetchThreadTree(childKey, state, errors, path);
					if (child) {
						message.children.push(child);
					}
				} catch (error) {
					errors.push(error instanceof Error ? error.message : String(error));
				}
			}

			sortMessageTree(message.children);
			return message;
		} finally {
			path.delete(normalized);
		}
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
		return computeLastPath(roots);
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
	};

	$: {
		const message = selectedPath ? getMessageAtPath(messages, selectedPath) : null;
		selectedMessage = message;
		selectedThreadIndex = selectedPath && selectedPath.length > 0 ? selectedPath[0] : null;
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

	const fetchMessageRecord = async (key: string): Promise<PersistedRecord> => {
		const response = await fetch(`${API_BASE_URL}/data/${key}`);
		if (!response.ok) {
			const message = (await response.text()) || `Request failed with status ${response.status}`;
			throw new Error(message);
		}

		const headers = response.headers;
		const headerKey = (headers.get('X-Ouroboros-Key') ?? '').trim();
		const parentHeader = (headers.get('X-Ouroboros-Parent') ?? '').trim();
		const isTextHeader = (headers.get('X-Ouroboros-Is-Text') ?? '').trim().toLowerCase();
		const mimeHeader = (headers.get('X-Ouroboros-Mime') ?? '').trim();
		const createdAtHeader = (headers.get('X-Ouroboros-Created-At') ?? '').trim();
		const parsedCreatedAt = createdAtHeader.length > 0 ? Date.parse(createdAtHeader) : Number.NaN;
		const createdAtMs = Number.isFinite(parsedCreatedAt) ? parsedCreatedAt : undefined;

		const buffer = await response.arrayBuffer();
		const bytes = new Uint8Array(buffer);
		const base64Content = encodeBytesToBase64(bytes);

		return {
			key: headerKey || key,
			content: base64Content,
			isText: isTextHeader === 'true',
			mimeType: mimeHeader || undefined,
			parent: parentHeader.length > 0 ? parentHeader : undefined,
			createdAt: createdAtHeader || undefined,
			createdAtMs
		};
	};

	const fetchChildKeys = async (key: string): Promise<string[]> => {
		const response = await fetch(`${API_BASE_URL}/data/${key}/children`);
		if (!response.ok) {
			const message = (await response.text()) || `Request failed with status ${response.status}`;
			throw new Error(message);
		}

		const body: { keys?: string[] } = await response.json();
		if (!Array.isArray(body.keys)) {
			return [];
		}

		return body.keys.filter(
			(childKey) => typeof childKey === 'string' && childKey.trim().length > 0
		);
	};

	const restoreConversation = async () => {
		loading = true;
		loadError = '';
		try {
			const response = await fetch(`${API_BASE_URL}/data`);
			if (!response.ok) {
				const message = (await response.text()) || `Request failed with status ${response.status}`;
				throw new Error(message);
			}

			const body: { keys?: string[] } = await response.json();
			const keys = Array.isArray(body.keys) ? body.keys : [];
			if (keys.length === 0) {
				messages = [];
				keyToPath = new Map();
				nextId = 1;
				setSelectedPath(null);
				return;
			}

			const errors: string[] = [];
			const threads: Message[] = [];
			const idState = { nextId: 1 };
			for (const key of keys) {
				const normalizedRoot = key.trim();
				if (!normalizedRoot.length) {
					continue;
				}
				try {
					const thread = await fetchThreadTree(normalizedRoot, idState, errors, new Set<string>());
					if (thread) {
						threads.push(thread);
					}
				} catch (error) {
					errors.push(error instanceof Error ? error.message : String(error));
				}
			}

			if (threads.length === 0) {
				throw new Error(errors[0] ?? 'No messages available');
			}

			sortMessageTree(threads);
			messages = threads;
			nextId = Math.max(idState.nextId, threads.length + 1);
			keyToPath = buildKeyPathMap(threads);
			const initialPath = resolveInitialPath(threads, keyToPath);
			setSelectedPath(initialPath.length > 0 ? initialPath : null);
			statusState = 'idle';
			statusText = '';
			if (errors.length > 0) {
				loadError = `Skipped ${errors.length} message${errors.length === 1 ? '' : 's'} due to load errors.`;
			}
		} catch (error) {
			const errorMessage = error instanceof Error ? error.message : 'Unknown error';
			loadError = `Failed to load conversation: ${errorMessage}`;
			messages = [];
			keyToPath = new Map();
			nextId = 1;
			setSelectedPath(null);
		} finally {
			loading = false;
		}
	};

	onMount(() => {
		void restoreConversation();
	});

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
				body: formData
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
	<div class="app-shell">
		<aside class="thread-sidebar">
			<div class="sidebar-header">
				<h2>Threads</h2>
				<button
					type="button"
					class="new-thread"
					on:click={clearSelection}
					disabled={statusState === 'sending'}
				>
					New thread
				</button>
			</div>
			{#if loading && messages.length === 0}
				<p class="thread-placeholder">Loading…</p>
			{:else if messages.length === 0}
				<p class="thread-placeholder">No threads yet. Start one!</p>
			{:else}
				<ul class="thread-list">
					{#each messages as thread, index (thread.id)}
						<li>
							<button
								type="button"
								class="thread-item"
								class:active={selectedThreadIndex === index}
								class:unsaved={thread.status !== 'saved'}
								on:click={() => handleSelectThread(index)}
							>
								<div class="thread-item-title">{threadTitle(thread)}</div>
								<div class="thread-item-meta">
									{threadMeta(thread)}
									{#if thread.status !== 'saved'}
										· {messageStatusLabel(thread.status)}
									{/if}
								</div>
							</button>
						</li>
					{/each}
				</ul>
			{/if}
		</aside>
		<section class="conversation-area">
			<header class="conversation-header">
				<div class="conversation-heading">
					<h1>Threaded Chat</h1>
					<p class="conversation-subtitle">
						{#if selectedMessage}
							Replying to <span class="snippet">“{truncate(selectedMessage.content)}”</span>
						{:else}
							Starting a new conversation
						{/if}
					</p>
				</div>
			</header>

			<div class="chat-window">
				{#if loading && messages.length === 0}
					<p class="placeholder">Loading conversation…</p>
				{:else if loadError && messages.length === 0}
					<p class="placeholder error">{loadError}</p>
				{:else if messages.length === 0}
					<p class="placeholder">Type a message to start the conversation.</p>
				{:else if selectedThreadIndex == null || !messages[selectedThreadIndex]}
					<p class="placeholder">Select a thread to view its messages.</p>
				{:else}
					{#key messages[selectedThreadIndex].id}
						<MessageNode
							message={messages[selectedThreadIndex]}
							level={0}
							path={[selectedThreadIndex]}
							{selectedPath}
							selectMessage={handleSelectMessage}
						/>
					{/key}
				{/if}
			</div>

			<div class="input-area">
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
					disabled={!inputValue.trim().length || statusState === 'sending'}
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
			</div>

			{#if loadError && messages.length > 0}
				<div class="global-status error">
					{loadError}
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
		gap: 1.25rem;
		box-shadow: 0 22px 42px rgba(8, 11, 32, 0.5);
	}

	.conversation-header {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 1rem;
		flex-wrap: wrap;
	}

	.conversation-heading h1 {
		margin: 0;
		font-size: 1.6rem;
		font-weight: 600;
		color: var(--text-primary);
	}

	.conversation-subtitle {
		margin: 0.35rem 0 0;
		font-size: 0.9rem;
		color: var(--text-muted);
	}

	.conversation-subtitle .snippet {
		font-weight: 600;
		color: var(--accent);
	}

	.chat-window {
		border: 1px solid var(--border-strong);
		border-radius: 0.9rem;
		padding: 1.25rem;
		min-height: 380px;
		max-height: 520px;
		overflow-y: auto;
		background: var(--surface-muted);
		box-shadow: inset 0 0 0 1px rgba(15, 23, 42, 0.6);
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
