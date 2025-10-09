<script lang="ts">
	import { tick } from 'svelte';
	import type { Message } from '../types';

	export let message: Message;
	export let level = 0;
	export let path: number[] = [];
	export let selectedPath: number[] | null = null;
	export let selectMessage: (path: number[]) => void = () => {};

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
	let dataUrl: string | null = null;
	let isImage = false;
	let isVideo = false;
	let sizeLabel: string | null = null;
	let downloadName = 'attachment';

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
	$: dataUrl =
		isAttachment && message.encodedContent
			? `data:${normalizedMime || 'application/octet-stream'};base64,${message.encodedContent}`
			: null;
	$: isImage = Boolean(dataUrl && normalizedMime.startsWith('image/'));
	$: isVideo = Boolean(dataUrl && normalizedMime.startsWith('video/'));
	$: sizeLabel = formatBytes(message.sizeBytes ?? undefined);
	$: downloadName = buildDownloadName();

	const openAttachment = async (type: 'image' | 'video') => {
		if (!dataUrl) return;
		if (typeof document !== 'undefined') {
			previousActiveElement = document.activeElement as HTMLElement | null;
		}
		modalAttachment = {
			type,
			url: dataUrl,
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
			{#if isImage && dataUrl}
				<button
					type="button"
					class="attachment-preview attachment-image"
					on:click|stopPropagation={() => openAttachment('image')}
					on:keydown|stopPropagation={createAttachmentKeydownHandler('image')}
					aria-label={`Open image attachment${message.content ? `: ${message.content}` : ''}`}
				>
					<img src={dataUrl} alt={message.content} loading="lazy" />
				</button>
			{:else if isVideo && dataUrl}
				<!-- svelte-ignore a11y-media-has-caption -->
				<button
					type="button"
					class="attachment-preview attachment-video"
					on:click|stopPropagation={() => openAttachment('video')}
					on:keydown|stopPropagation={createAttachmentKeydownHandler('video')}
					aria-label={`Open video attachment${message.content ? `: ${message.content}` : ''}`}
				>
					<video src={dataUrl} preload="metadata" muted playsinline></video>
				</button>
			{:else if dataUrl}
				<a class="attachment-download" href={dataUrl} download={downloadName} rel="noopener">
					Download file
				</a>
			{:else}
				<p class="attachment-placeholder">Attachment unavailable</p>
			{/if}
		</div>
	{:else}
		<p>{message.content}</p>
	{/if}
	<div class={`status ${statusClass}`}>{statusText}</div>
	{#if message.children.length > 0}
		<div class="children">
			{#each message.children as child, index (child.id)}
				<svelte:self
					message={child}
					level={level + 1}
					path={[...path, index]}
					{selectedPath}
					{selectMessage}
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
				<div class="attachment-modal-details">
					<h2 id={`attachment-modal-title-${message.id}`}>{modalAttachment.name}</h2>
					{#if modalAttachment.mime}
						<span class="attachment-modal-mime">{modalAttachment.mime}</span>
					{/if}
				</div>
				<button
					class="attachment-modal-close"
					type="button"
					on:click={closeModal}
					bind:this={modalCloseButton}
					aria-label="Close attachment preview"
				>
					×
				</button>
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
		border-radius: 0.75rem;
		padding: 0.75rem 1rem;
		box-shadow: 0 8px 24px rgba(15, 23, 42, 0.08);
		margin-bottom: 0.75rem;
	}

	.message p {
		margin: 0;
		line-height: 1.5;
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
		display: flex;
		justify-content: space-between;
		align-items: flex-start;
		gap: 1rem;
	}

	.attachment-modal-details h2 {
		margin: 0;
		font-size: 1.15rem;
		font-weight: 600;
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
		border-radius: 999px;
		padding: 0.25rem 0.75rem;
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
		margin-top: 0.35rem;
		font-size: 0.875rem;
		line-height: 1.4;
		font-weight: 500;
		text-overflow: ellipsis;
		overflow: clip;
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
		border-left: 4px solid #3b82f6;
	}

	.level-1 {
		border-left: 4px solid #f97316;
	}

	.level-2 {
		border-left: 4px solid #10b981;
	}

	.level-3,
	.level-4,
	.level-5 {
		border-left: 4px solid #8b5cf6;
	}

	.message:focus {
		outline: none;
		box-shadow: 0 0 0 3px rgba(59, 130, 246, 0.25);
	}

	.message.selected {
		outline: 1px solid #fff;
	}
</style>
