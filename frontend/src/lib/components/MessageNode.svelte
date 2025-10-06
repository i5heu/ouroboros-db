<script lang="ts">
	import type { Message } from '../types';

	export let message: Message;
	export let level = 0;

	const levelClass = `level-${Math.min(level, 5)}`;

	$: statusText =
		message.status === 'pending'
			? 'Saving to Ouroboros…'
			: message.status === 'failed'
				? `Save failed${message.error ? `: ${message.error}` : ''}`
				: message.key
					? `Saved • Key ${message.key}`
					: 'Saved locally';

	$: statusClass = `status-${message.status}`;
</script>

<div class={`message ${levelClass}`}>
	<p>{message.content}</p>
	<div class={`status ${statusClass}`}>{statusText}</div>
	{#if message.children.length > 0}
		<div class="children">
			{#each message.children as child (child.id)}
				<svelte:self message={child} level={level + 1} />
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
</style>
