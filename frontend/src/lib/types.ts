export type MessageStatus = 'pending' | 'saved' | 'failed';

export type Message = {
    id: number;
    content: string;
    children: Message[];
    status: MessageStatus;
    key?: string;
    error?: string;
    parentKey?: string;
    encodedContent?: string;
    mimeType?: string;
    isText?: boolean;
    sizeBytes?: number;
    attachmentName?: string;
    createdAt?: string;
    createdAtMs?: number;
    attachmentUrl?: string;
    binaryLoadedBytes?: number;
    binaryTotalBytes?: number;
    depth?: number;
    title?: string;
    computedId?: string;
    // When an edited version exists, resolvedKey/suggestedEdit point to the latest hash.
    resolvedKey?: string;
    suggestedEdit?: string;
    editOf?: string;
};

export type LookupResult = {
    computedId: string;
    keys: string[];
};

export type ComputedIdResult = {
    key: string;
    computedId: string;
};
