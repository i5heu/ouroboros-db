import { streamBulkMessages, type BulkDataRecord } from '../apiClient';

type HydrationRequestMessage = {
    type: 'hydrate-batch';
    requestId: number;
    apiBaseUrl: string;
    keys: string[];
    headers: Record<string, string>;
};

type HydrationWorkerResponse =
    | { type: 'record'; requestId: number; record: BulkDataRecord }
    | { type: 'complete'; requestId: number }
    | { type: 'error'; requestId: number; message: string };

const ctx = self as unknown as Worker;

const postWorkerMessage = (message: HydrationWorkerResponse) => {
    ctx.postMessage(message);
};

const handleHydrationRequest = async (request: HydrationRequestMessage) => {
    if (!request.keys.length) {
        postWorkerMessage({ type: 'complete', requestId: request.requestId });
        return;
    }
    try {
        await streamBulkMessages({
            apiBaseUrl: request.apiBaseUrl,
            keys: request.keys,
            getHeaders: async () => ({ ...request.headers }),
            onRecord: (record) => {
                postWorkerMessage({
                    type: 'record',
                    requestId: request.requestId,
                    record
                });
            }
        });
        postWorkerMessage({ type: 'complete', requestId: request.requestId });
    } catch (error) {
        const message = error instanceof Error ? error.message : String(error);
        postWorkerMessage({ type: 'error', requestId: request.requestId, message });
    }
};

ctx.addEventListener('message', (event: MessageEvent<HydrationRequestMessage>) => {
    const data = event.data;
    if (!data || data.type !== 'hydrate-batch') {
        return;
    }
    void handleHydrationRequest(data);
});

export { };
