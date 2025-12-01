<script lang="ts">
	import { onDestroy, onMount, tick } from 'svelte';
	import type { Message } from '../types';
	import { fetchAttachmentBlob, fetchAttachmentRange } from '../apiClient';

	export let message: Message;
	export let level = 0;
	export let path: number[] = [];
	export let selectedPath: number[] | null = null;
	export let selectMessage: (path: number[]) => void = () => {};
	export let apiBaseUrl = '';
	export let getAuthHeaders: () => Promise<Record<string, string>> = async () => ({
		Accept: 'application/json'
	});

	const levelClass = `level-${Math.min(level, 5)}`;

	type ModalAttachment = {
		type: 'image' | 'video';
		url: string;
		name: string;
		description: string;
		mime: string;
	};

	let modalAttachment: ModalAttachment | null = null;
	let modalCloseButton: HTMLButtonElement | null = null;
	let previousActiveElement: HTMLElement | null = null;
	let normalizedMime = '';
	let isAttachment = false;
	let encodedDataUrl: string | null = null;
	let isImage = false;
	let isVideo = false;
	let sizeLabel: string | null = null;
	let downloadName = 'attachment';
	let createdAtLabel: string | null = null;
	let container: HTMLDivElement | null = null;
	let observer: IntersectionObserver | null = null;
	let shouldLoadImage = false;
	let attachmentUrl: string | null = null;
	let attachmentError = '';
	let binaryAbort: AbortController | null = null;
	let videoPreviewUrl: string | null = null;
	let videoPreviewBytes = 0;
	let videoPreviewTotal = 0;
	let videoLoading = false;
	let videoError = '';
	const videoPreviewChunk = 1024 * 1024; // 1MB preview
	let attachmentDisplayUrl: string | null = null;
	let videoPreviewSource: string | null = null;
	let childNodes: Message[] = [];

	const closeModal = () => {
		modalAttachment = null;
		if (previousActiveElement) {
			previousActiveElement.focus();
			previousActiveElement = null;
		}
	};

	const pathsEqual = (a: number[] | null, b: number[] | null) => {
		if (!a || !b) return false;
		if (a.length !== b.length) return false;
		return a.every((value, index) => value === b[index]);
	};

	const formatBytes = (size?: number): string | null => {
		if (!size || size <= 0) return null;
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

	const mime = () => message.mimeType?.trim().toLowerCase() ?? '';

	const buildDownloadName = (): string => {
		if (message.attachmentName && message.attachmentName.trim().length > 0) {
			return message.attachmentName;
		}
		if (message.key) {
			return message.key;
		}
		return 'attachment';
	};

	const formatTimestamp = (value?: string): string | null => {
		if (!value) return null;
		const date = new Date(value);
		if (Number.isNaN(date.getTime())) {
			return null;
		}
		return date.toLocaleString(undefined, {
			dateStyle: 'medium',
			timeStyle: 'medium'
		});
	};

	const cleanupUrls = () => {
		if (attachmentUrl) {
			URL.revokeObjectURL(attachmentUrl);
			attachmentUrl = null;
		}
		if (videoPreviewUrl) {
			URL.revokeObjectURL(videoPreviewUrl);
			videoPreviewUrl = null;
		}
		binaryAbort?.abort();
		binaryAbort = null;
	};

	onMount(() => {
		videoPreviewTotal = message.sizeBytes ?? 0;
		if (!container || typeof window === 'undefined') return;
		if (
			'IntersectionObserver' in window &&
			isAttachment &&
			normalizedMime.startsWith('image/') &&
			!message.encodedContent
		) {
			observer = new IntersectionObserver((entries) => {
				entries.forEach((entry) => {
					if (entry.isIntersecting) {
						shouldLoadImage = true;
						observer?.disconnect();
					}
				});
			});
			observer.observe(container);
		} else if (isAttachment && normalizedMime.startsWith('image/') && !message.encodedContent) {
			shouldLoadImage = true;
		}
	});

	onDestroy(() => {
		observer?.disconnect();
		cleanupUrls();
	});

	$: statusText =
		message.status === 'pending'
			? 'Saving to Ouroboros…'
			: message.status === 'failed'
				? `Save failed${message.error ? `: ${message.error}` : ''}`
				: message.key
					? `Saved • Key ${message.key}`
					: 'Saved locally';

	$: statusClass = `status-${message.status}`;
	$: isSelected = pathsEqual(selectedPath, path);
	$: normalizedMime = mime();
	$: isAttachment =
		message.isText === false || (!!normalizedMime && !normalizedMime.startsWith('text/'));
	$: encodedDataUrl =
		isAttachment && message.encodedContent
			? `data:${normalizedMime || 'application/octet-stream'};base64,${message.encodedContent}`
			: null;
	$: isImage = Boolean((encodedDataUrl || attachmentUrl) && normalizedMime.startsWith('image/'));
	$: isVideo = Boolean(normalizedMime.startsWith('video/'));
	$: sizeLabel = formatBytes(message.sizeBytes ?? undefined);
	$: downloadName = buildDownloadName();
	$: createdAtLabel = formatTimestamp(message.createdAt);
	$: videoPreviewTotal = message.sizeBytes ?? videoPreviewTotal;

	$: if (shouldLoadImage && !encodedDataUrl && normalizedMime.startsWith('image/')) {
		shouldLoadImage = false;
		void loadAttachmentBlob();
	}

	$: attachmentDisplayUrl = encodedDataUrl ?? attachmentUrl ?? null;
	$: videoPreviewSource = videoPreviewUrl ?? attachmentDisplayUrl;
	$: childNodes = (message.children ?? []) as Message[];

	const loadAttachmentBlob = async () => {
		if (!message.key) return;
		attachmentError = '';
		binaryAbort?.abort();
		binaryAbort = new AbortController();
		try {
			const blob = await fetchAttachmentBlob({
				apiBaseUrl,
				key: message.key,
				getHeaders: getAuthHeaders,
				signal: binaryAbort.signal
			});
			cleanupUrls();
			attachmentUrl = URL.createObjectURL(blob);
		} catch (error) {
			attachmentError = error instanceof Error ? error.message : String(error);
		}
	};

	const loadVideoPreview = async () => {
		if (!message.key || videoLoading) return;
		videoError = '';
		videoLoading = true;
		try {
			const chunk = await fetchAttachmentRange({
				apiBaseUrl,
				key: message.key,
				start: 0,
				end: videoPreviewChunk - 1,
				getHeaders: getAuthHeaders
			});
			videoPreviewBytes = chunk.end - chunk.start + 1;
			videoPreviewTotal = chunk.total;
			if (videoPreviewUrl) {
				URL.revokeObjectURL(videoPreviewUrl);
			}
			videoPreviewUrl = URL.createObjectURL(new Blob([chunk.buffer], { type: chunk.mimeType }));
		} catch (error) {
			videoError = error instanceof Error ? error.message : String(error);
		} finally {
			videoLoading = false;
		}
	};

	const downloadFullVideo = async () => {
		await loadAttachmentBlob();
		videoPreviewUrl = attachmentUrl;
		videoPreviewBytes = message.sizeBytes ?? videoPreviewBytes;
	};

	const handleDownloadAttachment = async () => {
		await loadAttachmentBlob();
		if (attachmentUrl) {
			const a = document.createElement('a');
			a.href = attachmentUrl;
			a.download = downloadName;
			a.click();
		}
	};

	const openAttachment = async (type: 'image' | 'video') => {
		const url = type === 'image' ? attachmentDisplayUrl : videoPreviewUrl;
		if (!url) return;
		if (typeof document !== 'undefined') {
			previousActiveElement = document.activeElement as HTMLElement | null;
		}
		modalAttachment = {
			type,
			url,
			name: downloadName,
			description: message.content ?? downloadName,
			mime: normalizedMime
		};
		await tick();
		modalCloseButton?.focus();
	};

	const handleWindowKeydown = (event: KeyboardEvent) => {
		if (!modalAttachment) return;
		if (event.key === 'Escape') {
			event.preventDefault();
			closeModal();
		}
	};

	const createAttachmentKeydownHandler = (type: 'image' | 'video') => (event: KeyboardEvent) => {
		if (event.key === 'Enter' || event.key === ' ') {
			event.preventDefault();
			openAttachment(type);
		}
	};

	const handleBackdropKeydown = (event: KeyboardEvent) => {
		if (event.key === 'Enter' || event.key === ' ') {
			event.preventDefault();
			closeModal();
		}
	};

	const stopPropagationOnKeydown = (event: KeyboardEvent) => {
		if (event.key === 'Enter' || event.key === ' ') {
			event.stopPropagation();
		}
	};

	const handleSelect = (event: MouseEvent) => {
		event.stopPropagation();
		selectMessage(path);
	};

	const handleKeydown = (event: KeyboardEvent) => {
		if (event.key === 'Enter' || event.key === ' ') {
			event.preventDefault();
			selectMessage(path);
		}
	};
</script>

<svelte:window on:keydown={handleWindowKeydown} />

<div
	class={`message ${levelClass} ${isSelected ? 'selected' : ''}`}
	tabindex="0"
	role="button"
	on:click={handleSelect}
	on:keydown={handleKeydown}
	bind:this={container}
>
	{#if isAttachment}
		<div class="attachment">
			<div class="attachment-meta">
				<span class="attachment-name">{message.content}</span>
				{#if sizeLabel}
					<span class="attachment-size">{sizeLabel}</span>
				{/if}
				{#if normalizedMime}
					<span class="attachment-mime">{normalizedMime}</span>
				{/if}
			</div>
			{#if normalizedMime.startsWith('image/')}
				{#if attachmentDisplayUrl}
					<button
						type="button"
						class="attachment-preview attachment-image"
						on:click|stopPropagation={() => openAttachment('image')}
						on:keydown|stopPropagation={createAttachmentKeydownHandler('image')}
						aria-label={`Open image attachment${message.content ? `: ${message.content}` : ''}`}
					>
						<img src={attachmentDisplayUrl} alt={message.content} loading="lazy" />
					</button>
				{:else}
					<button
						type="button"
						class="attachment-download"
						on:click|stopPropagation={loadAttachmentBlob}
					>
						Load image
					</button>
					{#if attachmentError}
						<p class="attachment-error">{attachmentError}</p>
					{/if}
				{/if}
			{:else if isVideo}
				{#if videoPreviewSource}
					<!-- svelte-ignore a11y-media-has-caption -->
					<button
						type="button"
						class="attachment-preview attachment-video"
						on:click|stopPropagation={() => openAttachment('video')}
						on:keydown|stopPropagation={createAttachmentKeydownHandler('video')}
						aria-label={`Open video attachment${message.content ? `: ${message.content}` : ''}`}
					>
						<video src={videoPreviewSource} preload="metadata" muted playsinline></video>
					</button>
				{/if}
				{#if videoPreviewBytes > 0}
					<p class="video-hint">
						Previewing {formatBytes(videoPreviewBytes) ?? '-'} of {formatBytes(videoPreviewTotal) ??
							'-'}
					</p>
				{/if}
				<div class="attachment-actions">
					<button type="button" on:click|stopPropagation={loadVideoPreview} disabled={videoLoading}>
						{videoLoading
							? 'Loading preview…'
							: `Load preview (~${formatBytes(videoPreviewChunk) ?? '1 MB'})`}
					</button>
					<button
						type="button"
						on:click|stopPropagation={downloadFullVideo}
						disabled={videoLoading}
					>
						Download full
					</button>
				</div>
				{#if videoError}
					<p class="attachment-error">{videoError}</p>
				{/if}
			{:else}
				{#if attachmentDisplayUrl}
					<a
						class="attachment-download"
						href={attachmentDisplayUrl}
						download={downloadName}
						rel="noopener"
					>
						Download file
					</a>
				{:else}
					<button
						type="button"
						class="attachment-download"
						on:click|stopPropagation={handleDownloadAttachment}
					>
						Download attachment
					</button>
				{/if}
				{#if attachmentError}
					<p class="attachment-error">{attachmentError}</p>
				{/if}
			{/if}
		</div>
	{:else}
		{#if message.title}
			<div class="message-title">{message.title}</div>
		{/if}
		<p>{message.content}</p>
	{/if}
	{#if createdAtLabel}
		<div class="timestamp">Created {createdAtLabel}</div>
	{/if}
	<div class={`status ${statusClass}`}>{statusText}</div>
	{#if childNodes.length > 0}
		<div class="children">
			{#each childNodes as child, index (`${child.id}-${index}-${child.key ?? ''}`)}
				<svelte:self
					message={child}
					level={level + 1}
					path={[...path, index]}
					{selectedPath}
					{selectMessage}
					{apiBaseUrl}
					{getAuthHeaders}
				/>
			{/each}
		</div>
	{/if}
</div>

{#if modalAttachment}
	<div
		class="attachment-modal-backdrop"
		role="button"
		tabindex="0"
		aria-label="Close attachment preview"
		on:click={closeModal}
		on:keydown={handleBackdropKeydown}
	>
		<div
			class="attachment-modal"
			on:click|stopPropagation
			on:keydown|stopPropagation={stopPropagationOnKeydown}
			role="dialog"
			aria-modal="true"
			aria-labelledby={`attachment-modal-title-${message.id}`}
			aria-describedby={modalAttachment.description
				? `attachment-modal-description-${message.id}`
				: undefined}
			tabindex="-1"
		>
			<header class="attachment-modal-header">
				<h2 id={`attachment-modal-title-${message.id}`}>{modalAttachment.name}</h2>
				<button
					class="attachment-modal-close"
					type="button"
					on:click={closeModal}
					bind:this={modalCloseButton}
					aria-label="Close attachment preview"
				>
					×
				</button>
				{#if modalAttachment.mime}
					<span class="attachment-modal-mime">{modalAttachment.mime}</span>
				{/if}
			</header>
			<div class="attachment-modal-body">
				{#if modalAttachment.type === 'image'}
					<img src={modalAttachment.url} alt={modalAttachment.description} />
				{:else}
					<!-- svelte-ignore a11y-media-has-caption -->
					<video src={modalAttachment.url} controls autoplay playsinline></video>
				{/if}
				{#if modalAttachment.description}
					<p id={`attachment-modal-description-${message.id}`} class="attachment-modal-caption">
						{modalAttachment.description}
					</p>
				{/if}
			</div>
			<footer class="attachment-modal-footer">
				<a
					href={modalAttachment.url}
					download={modalAttachment.name}
					rel="noopener"
					class="attachment-modal-download"
				>
					Download
				</a>
			</footer>
		</div>
	</div>
{/if}

<style>
	.message {
		background: #00000019;
		border-top-right-radius: 0.75rem;
		border-bottom-right-radius: 0.75rem;
		padding: 5px;
		padding-right: 0;
		box-shadow: 0 8px 24px rgba(15, 23, 42, 0.08);
		margin-bottom: 0.75rem;

		/* allow long words to break with hyphenation */
		word-break: break-word;
		hyphens: auto;
	}
	.message.level-0 {
		border-radius: 0.75rem;
		margin-bottom: 0;
	}

	.message p {
		margin: 0;
		line-height: 1.5;
	}

	.message-title {
		margin: 0 0 0.25rem 0;
		font-weight: 700;
		color: var(--text-primary, #e5e7eb);
		font-size: 0.95rem;
	}

	.attachment {
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
	}

	.attachment-meta {
		display: flex;
		flex-wrap: wrap;
		gap: 0.5rem;
		align-items: baseline;
		margin: 0;
	}

	.attachment-name {
		font-weight: 600;
	}

	.attachment-size,
	.attachment-mime {
		font-size: 0.85rem;
		color: #6b7280;
	}

	.attachment-preview {
		display: block;
		padding: 0;
		margin: 0 auto;
		border: none;
		background: transparent;
		cursor: pointer;
		border-radius: 0.5rem;
		box-shadow: 0 8px 20px rgba(15, 23, 42, 0.1);
		overflow: hidden;
		transition:
			transform 0.15s ease,
			box-shadow 0.15s ease;
		font: inherit;
	}

	.attachment-preview:hover,
	.attachment-preview:focus-visible {
		transform: translateY(-1px);
		box-shadow:
			0 14px 32px rgba(15, 23, 42, 0.14),
			0 0 0 3px rgba(59, 130, 246, 0.35);
		outline: none;
	}

	.attachment-preview img,
	.attachment-preview video {
		display: block;
		max-width: 100%;
		max-height: 10em;
		width: auto;
		height: auto;
		object-fit: contain;
		background: #0f172a;
	}

	.attachment-modal-backdrop {
		position: fixed;
		inset: 0;
		display: flex;
		align-items: center;
		justify-content: center;
		padding: 2rem;
		background: rgba(15, 23, 42, 0.75);
		backdrop-filter: blur(4px);
		z-index: 1000;
		cursor: pointer;
	}

	.attachment-modal-backdrop:focus-visible {
		outline: 2px solid rgba(96, 165, 250, 0.75);
		outline-offset: 4px;
	}

	.attachment-modal {
		background: #0b1120;
		color: #e5e7eb;
		border-radius: 0.75rem;
		box-shadow: 0 18px 48px rgba(15, 23, 42, 0.45);
		padding: 1.5rem;
		max-width: min(90vw, 720px);
		max-height: min(90vh, 720px);
		display: flex;
		flex-direction: column;
		gap: 1rem;
		cursor: auto;
	}

	.attachment-modal-header {
		display: grid;
		grid-template-columns: 1fr max-content;
		column-gap: 1rem;
		row-gap: 0.2rem;
		align-items: center;
		width: 100%;
		box-sizing: border-box;
	}

	.attachment-modal-header h2 {
		margin: 0;
		font-size: 1.15rem;
		font-weight: 600;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.attachment-modal-mime {
		display: inline-block;
		margin-top: 0.25rem;
		font-size: 0.9rem;
		color: #94a3b8;
	}

	.attachment-modal-close {
		background: transparent;
		border: none;
		color: #94a3b8;
		font-size: 1.75rem;
		line-height: 1;
		cursor: pointer;
		border-radius: 0.75rem;
	}

	.attachment-modal-close:hover,
	.attachment-modal-close:focus-visible {
		color: #f8fafc;
		background: rgba(148, 163, 184, 0.12);
		outline: none;
	}

	.attachment-modal-body {
		display: flex;
		flex-direction: column;
		gap: 0.75rem;
		align-items: center;
		overflow: hidden;
	}

	.attachment-modal-body img,
	.attachment-modal-body video {
		max-width: 100%;
		max-height: 60vh;
		border-radius: 0.5rem;
		box-shadow: 0 18px 48px rgba(15, 23, 42, 0.45);
		background: #010617;
	}

	.attachment-modal-body video {
		width: 100%;
	}

	.attachment-modal-caption {
		margin: 0;
		font-size: 0.95rem;
		color: #cbd5f5;
		text-align: center;
	}

	.attachment-modal-footer {
		display: flex;
		justify-content: flex-end;
	}

	.attachment-modal-download {
		color: #60a5fa;
		font-weight: 600;
		text-decoration: none;
		border: 1px solid rgba(96, 165, 250, 0.35);
		padding: 0.4rem 0.85rem;
		border-radius: 999px;
		transition:
			background 0.2s ease,
			color 0.2s ease;
	}

	.attachment-modal-download:hover,
	.attachment-modal-download:focus-visible {
		color: #0f172a;
		background: #60a5fa;
		outline: none;
	}

	.attachment-download {
		align-self: flex-start;
		color: #2563eb;
		font-weight: 600;
		text-decoration: none;
	}

	.attachment-download:hover {
		text-decoration: underline;
	}

	.attachment-placeholder {
		margin: 0;
		color: #6b7280;
		font-size: 0.9rem;
		font-style: italic;
	}

	.status {
		margin-top: 0.25rem;
		font-size: 0.75rem;
		line-height: 1.4;
		font-weight: 500;
		text-overflow: ellipsis;
		overflow: clip;
		word-break: normal;
		text-wrap-mode: nowrap;
	}

	.timestamp {
		margin-top: 0.3rem;
		font-size: 0.8rem;
		color: #94a3b8;
	}

	.status-pending {
		color: #2563eb;
	}

	.status-saved {
		color: #059669;
	}

	.status-failed {
		color: #dc2626;
	}

	.children {
		margin-top: 0.5rem;
	}

	.level-0 {
		border: none;
		outline: none;
	}

	.level-1 {
		border-left: 1px solid #f97316;
	}

	.level-2 {
		border-left: 1px solid #10b981;
	}

	.level-3,
	.level-4,
	.level-5 {
		border-left: 1px solid #8b5cf6;
	}

	.message:focus {
		outline: none;
		box-shadow: 0 0 0 3px rgba(59, 130, 246, 0.25);
	}

	.message.selected {
		outline: 1px solid #fff;
	}
</style>
