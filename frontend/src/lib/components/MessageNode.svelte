<script lang="ts">
	import type { Message } from '../types';

	export let message: Message;
	export let level = 0;
	export let path: number[] = [];
	export let selectedPath: number[] | null = null;
	export let selectMessage: (path: number[]) => void = () => {};

	const levelClass = `level-${Math.min(level, 5)}`;

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
				<img class="attachment-image" src={dataUrl} alt={message.content} loading="lazy" />
			{:else if isVideo && dataUrl}
				<!-- svelte-ignore a11y-media-has-caption -->
				<video
					class="attachment-video"
					controls
					src={dataUrl}
					aria-label={`Video attachment${message.content ? `: ${message.content}` : ''}`}
				></video>
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

<style>
	.message {
		background: white;
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

	.attachment-image {
		max-width: 100%;
		border-radius: 0.5rem;
		box-shadow: 0 8px 20px rgba(15, 23, 42, 0.1);
	}

	.attachment-video {
		width: 100%;
		border-radius: 0.5rem;
		background: #0f172a;
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
		margin-left: 1.5rem;
		border-left: 2px dashed #e5e7eb;
		padding-left: 1rem;
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
		border-color: #2563eb;
		box-shadow: 0 12px 28px rgba(37, 99, 235, 0.25);
		transform: translateY(-1px);
	}
</style>
