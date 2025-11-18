export type ThreadSummaryPayload = {
    key: string;
    preview: string;
    mimeType: string;
    isText: boolean;
    sizeBytes: number;
    childCount: number;
    createdAt?: string;
};

export type ThreadNodePayload = {
    key: string;
    parent?: string;
    mimeType: string;
    isText: boolean;
    sizeBytes: number;
    createdAt?: string;
    depth: number;
    children: string[];
    content?: string;
    preview?: string;
};

type HeaderProvider = () => Promise<Record<string, string>>;

type ThreadStreamOptions = {
    apiBaseUrl: string;
    cursor?: string | null;
    limit?: number;
    signal?: AbortSignal;
    getHeaders: HeaderProvider;
    onSummary: (payload: ThreadSummaryPayload) => void;
};

type ThreadNodeStreamOptions = {
    apiBaseUrl: string;
    rootKey: string;
    signal?: AbortSignal;
    getHeaders: HeaderProvider;
    onNode: (node: ThreadNodePayload) => void;
};

type NdjsonHandler = (record: unknown) => void;

const consumeNdjson = async (
    response: Response,
    handler: NdjsonHandler,
    signal?: AbortSignal
) => {
    const reader = response.body?.getReader();
    if (!reader) {
        return;
    }
    const decoder = new TextDecoder();
    let buffer = '';
    while (true) {
        const { value, done } = await reader.read();
        if (done) break;
        if (signal?.aborted) return;
        buffer += decoder.decode(value, { stream: true });
        let newlineIndex = buffer.indexOf('\n');
        while (newlineIndex !== -1) {
            const line = buffer.slice(0, newlineIndex).trim();
            buffer = buffer.slice(newlineIndex + 1);
            if (line.length > 0) {
                try {
                    handler(JSON.parse(line));
                } catch (error) {
                    console.error('Failed to parse ndjson line', error, line);
                }
            }
            newlineIndex = buffer.indexOf('\n');
        }
    }
    const tail = buffer.trim();
    if (tail.length > 0) {
        try {
            handler(JSON.parse(tail));
        } catch (error) {
            console.error('Failed to parse trailing ndjson line', error, tail);
        }
    }
};

const withNdjsonAccept = (base: Record<string, string>): Record<string, string> => ({
    ...base,
    Accept: 'application/x-ndjson'
});

export const streamThreadSummaries = async (
    options: ThreadStreamOptions
): Promise<{ nextCursor: string | null }> => {
    const params = new URLSearchParams();
    if (options.cursor) {
        params.set('cursor', options.cursor);
    }
    params.set('limit', String(options.limit ?? 25));

    const headers = withNdjsonAccept(await options.getHeaders());
    const response = await fetch(`${options.apiBaseUrl}/meta/threads?${params.toString()}`, {
        headers,
        signal: options.signal
    });
    if (!response.ok) {
        const message = (await response.text()) || `Failed to stream threads (${response.status}).`;
        throw new Error(message);
    }

    let nextCursor: string | null = null;
    await consumeNdjson(
        response,
        (record) => {
            if (!record || typeof record !== 'object') {
                return;
            }
            const payload = record as { type?: string; thread?: ThreadSummaryPayload; cursor?: string };
            if (payload.type === 'thread' && payload.thread) {
                options.onSummary(payload.thread);
            } else if (payload.type === 'cursor') {
                nextCursor = payload.cursor ?? null;
            }
        },
        options.signal
    );

    return { nextCursor };
};

export const streamThreadNodes = async (options: ThreadNodeStreamOptions): Promise<void> => {
    const headers = withNdjsonAccept(await options.getHeaders());
    const response = await fetch(`${options.apiBaseUrl}/meta/thread/${options.rootKey}/stream`, {
        headers,
        signal: options.signal
    });
    if (!response.ok) {
        const message = (await response.text()) || `Failed to stream thread ${options.rootKey}.`;
        throw new Error(message);
    }

    await consumeNdjson(
        response,
        (record) => {
            if (!record || typeof record !== 'object') {
                return;
            }
            const payload = record as { type?: string; node?: ThreadNodePayload };
            if (payload.type === 'node' && payload.node) {
                options.onNode(payload.node);
            }
        },
        options.signal
    );
};

export const fetchAttachmentBlob = async (params: {
    apiBaseUrl: string;
    key: string;
    getHeaders: HeaderProvider;
    signal?: AbortSignal;
}): Promise<Blob> => {
    const headers = { ...(await params.getHeaders()) };
    const response = await fetch(`${params.apiBaseUrl}/data/${params.key}`, {
        headers,
        signal: params.signal
    });
    if (!response.ok) {
        const message = (await response.text()) || `Attachment download failed (${response.status}).`;
        throw new Error(message);
    }
    return await response.blob();
};

export type AttachmentRangeChunk = {
    buffer: ArrayBuffer;
    start: number;
    end: number;
    total: number;
    mimeType: string;
};

const parseContentRange = (value: string | null): { start: number; end: number; total: number } | null => {
    if (!value || !value.startsWith('bytes')) {
        return null;
    }
    const [, rest] = value.split(' ');
    if (!rest) return null;
    const [range, totalPart] = rest.split('/');
    if (!range || !totalPart) return null;
    const [startStr, endStr] = range.split('-');
    const start = Number(startStr);
    const end = Number(endStr);
    const total = Number(totalPart);
    if (Number.isNaN(start) || Number.isNaN(end) || Number.isNaN(total)) {
        return null;
    }
    return { start, end, total };
};

export const fetchAttachmentRange = async (params: {
    apiBaseUrl: string;
    key: string;
    start: number;
    end?: number;
    getHeaders: HeaderProvider;
    signal?: AbortSignal;
}): Promise<AttachmentRangeChunk> => {
    const headers = { ...(await params.getHeaders()) };
    const upperBound = params.end != null ? params.end : params.start + 1024 * 512 - 1;
    headers.Range = `bytes=${params.start}-${upperBound}`;
    const response = await fetch(`${params.apiBaseUrl}/data/${params.key}`, {
        headers,
        signal: params.signal
    });
    if (!(response.status === 206 || response.status === 200)) {
        const message = (await response.text()) || `Range request failed (${response.status}).`;
        throw new Error(message);
    }
    const contentRange = parseContentRange(response.headers.get('Content-Range'));
    const buffer = await response.arrayBuffer();
    const fallbackTotal = Number(response.headers.get('Content-Length')) || buffer.byteLength;
    return {
        buffer,
        start: contentRange?.start ?? params.start,
        end: contentRange?.end ?? params.start + buffer.byteLength - 1,
        total: contentRange?.total ?? fallbackTotal,
        mimeType: response.headers.get('Content-Type') ?? 'application/octet-stream'
    };
};
