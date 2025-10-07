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
	};

	let messages: Message[] = [];
	let inputValue = '';
	let nextId = 1;
	let selectedPath: number[] | null = null;
	let selectedMessage: Message | null = null;
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

	const buildMessageTree = (records: PersistedRecord[]) => {
		const messageMap = new Map<string, Message>();
		let idCounter = 1;

		for (const record of records) {
			const messageSize = calculateBase64Size(record.content);
			const displayContent = formatPersistedContent(record);
			const message: Message = {
				id: idCounter++,
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
				attachmentName: !record.isText ? record.key : undefined
			};
			messageMap.set(record.key, message);
		}

		const roots: Message[] = [];
		for (const record of records) {
			const node = messageMap.get(record.key);
			if (!node) {
				continue;
			}

			const parentKey = record.parent?.trim();
			if (parentKey && messageMap.has(parentKey)) {
				messageMap.get(parentKey)?.children.push(node);
			} else {
				roots.push(node);
			}
		}

		const sortById = (nodes: Message[]) => {
			nodes.sort((a, b) => a.id - b.id);
			nodes.forEach((child) => sortById(child.children));
		};
		sortById(roots);

		return { roots, nextId: idCounter };
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

	const clearSelection = () => {
		setSelectedPath(null);
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

		const buffer = await response.arrayBuffer();
		const bytes = new Uint8Array(buffer);
		const base64Content = encodeBytesToBase64(bytes);

		return {
			key: headerKey || key,
			content: base64Content,
			isText: isTextHeader === 'true',
			mimeType: mimeHeader || undefined,
			parent: parentHeader.length > 0 ? parentHeader : undefined
		};
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

			const detailResults = await Promise.allSettled(keys.map((key) => fetchMessageRecord(key)));
			const records: PersistedRecord[] = [];
			const errors: string[] = [];
			detailResults.forEach((result) => {
				if (result.status === 'fulfilled') {
					records.push(result.value);
				} else {
					errors.push(
						result.reason instanceof Error ? result.reason.message : String(result.reason)
					);
				}
			});

			if (records.length === 0) {
				throw new Error(errors[0] ?? 'No messages available');
			}

			const orderedRecords = keys
				.map((key) => records.find((record) => record.key === key))
				.filter((record): record is PersistedRecord => Boolean(record));

			const { roots, nextId: computedNextId } = buildMessageTree(orderedRecords);
			messages = roots;
			nextId = Math.max(computedNextId, roots.length + 1);
			keyToPath = buildKeyPathMap(roots);
			const initialPath = resolveInitialPath(roots, keyToPath);
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
	<h1>Threaded Chat</h1>
	<div class="chat-window">
		{#if loading}
			<p class="placeholder">Loading conversation…</p>
		{:else if loadError && messages.length === 0}
			<p class="placeholder error">{loadError}</p>
		{:else if messages.length === 0}
			<p class="placeholder">Type a message to start the conversation.</p>
		{:else}
			{#each messages as message, index (message.id)}
				<MessageNode
					{message}
					level={0}
					path={[index]}
					{selectedPath}
					selectMessage={handleSelectMessage}
				/>
			{/each}
		{/if}
	</div>

	<div class="selection-info">
		{#if selectedMessage}
			<div class="info-text">
				Replying to <span class="snippet">“{truncate(selectedMessage.content)}”</span>
			</div>
			<button type="button" class="clear-selection" on:click={clearSelection}>
				Start new thread
			</button>
		{:else}
			<div class="info-text">Starting a new thread</div>
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
</main>

<style>
	main {
		max-width: 640px;
		margin: 0 auto;
		padding: 2rem 1rem 4rem;
		font-family:
			system-ui,
			-apple-system,
			BlinkMacSystemFont,
			'Segoe UI',
			sans-serif;
		color: #1f2933;
	}

	h1 {
		text-align: center;
		margin-bottom: 1.5rem;
		font-weight: 600;
	}

	.chat-window {
		border: 1px solid #d1d5db;
		border-radius: 0.75rem;
		padding: 1rem;
		height: 420px;
		overflow-y: auto;
		background: #f8fafc;
	}

	.selection-info {
		display: flex;
		align-items: center;
		justify-content: space-between;
		margin-top: 1rem;
		padding: 0.75rem 1rem;
		border-radius: 0.75rem;
		background: #eef2ff;
		color: #3730a3;
		font-weight: 500;
		box-shadow: inset 0 0 0 1px rgba(99, 102, 241, 0.1);
	}

	.selection-info .snippet {
		font-weight: 600;
	}

	.selection-info .clear-selection {
		background: transparent;
		border: none;
		color: #6366f1;
		font-weight: 600;
		cursor: pointer;
		padding: 0.35rem 0.75rem;
		border-radius: 0.5rem;
		transition:
			background-color 0.15s ease,
			color 0.15s ease;
	}

	.selection-info .clear-selection:hover {
		background: rgba(99, 102, 241, 0.12);
	}

	.selection-info .clear-selection:focus {
		outline: none;
		box-shadow: 0 0 0 3px rgba(129, 140, 248, 0.35);
	}

	.placeholder {
		text-align: center;
		color: #6b7280;
		margin-top: 3rem;
	}

	.placeholder.error {
		color: #dc2626;
	}

	.input-area {
		margin-top: 1.25rem;
		display: grid;
		grid-template-columns: 1fr auto auto;
		gap: 0.75rem;
		align-items: start;
	}

	textarea {
		border-radius: 0.75rem;
		border: 1px solid #cbd5f5;
		padding: 0.75rem;
		font-size: 1rem;
		resize: none;
		font-family: inherit;
	}

	textarea:focus {
		outline: none;
		border-color: #3b82f6;
		box-shadow: 0 0 0 3px rgba(59, 130, 246, 0.15);
	}

	button {
		background: linear-gradient(135deg, #3b82f6, #6366f1);
		color: white;
		border: none;
		border-radius: 0.75rem;
		padding: 0.75rem 1.5rem;
		font-size: 1rem;
		cursor: pointer;
		font-weight: 600;
		box-shadow: 0 12px 24px rgba(79, 70, 229, 0.2);
		transition:
			transform 0.15s ease,
			box-shadow 0.15s ease;
	}

	.attach-button {
		background: #ffffff;
		color: #2563eb;
		border: 1px solid #93c5fd;
		box-shadow: none;
		padding: 0.75rem 1.25rem;
	}

	.attach-button:hover:enabled {
		transform: none;
		box-shadow: 0 0 0 2px rgba(147, 197, 253, 0.6);
		background: #eff6ff;
	}

	.attach-button:active:enabled {
		transform: none;
		box-shadow: 0 0 0 2px rgba(59, 130, 246, 0.4);
	}

	button:hover:enabled {
		transform: translateY(-1px);
		box-shadow: 0 16px 26px rgba(79, 70, 229, 0.3);
	}

	button:active:enabled {
		transform: translateY(0);
		box-shadow: 0 8px 16px rgba(79, 70, 229, 0.2);
	}

	button:disabled {
		opacity: 0.6;
		cursor: not-allowed;
	}

	.global-status {
		margin-top: 1rem;
		padding: 0.75rem 1rem;
		border-radius: 0.75rem;
		font-weight: 500;
		font-size: 0.95rem;
		box-shadow: 0 8px 18px rgba(15, 23, 42, 0.08);
	}

	.global-status.sending {
		background: rgba(59, 130, 246, 0.12);
		color: #1d4ed8;
	}

	.global-status.success {
		background: rgba(16, 185, 129, 0.12);
		color: #047857;
	}

	.global-status.error {
		background: rgba(239, 68, 68, 0.12);
		color: #b91c1c;
	}

	@media (max-width: 640px) {
		main {
			padding: 1.5rem 0.75rem 3rem;
		}

		.chat-window {
			height: 320px;
		}

		.selection-info {
			flex-direction: column;
			align-items: flex-start;
			gap: 0.5rem;
		}

		.input-area {
			grid-template-columns: 1fr;
		}

		.input-area textarea {
			grid-column: 1 / -1;
		}

		.input-area button {
			width: 100%;
		}
	}
</style>
