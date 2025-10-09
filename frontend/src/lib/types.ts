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
};
