const AUTH_STORAGE_KEY = 'ouroboros:auth';
const AUTH_DURATION_MS = 15 * 24 * 60 * 60 * 1000; // 15 days
const BASE64_PREFIX = 'b64:';

export type AuthBootstrapResult = {
    hasCredentials: boolean;
    message?: string;
    refreshed: boolean;
};

type StoredAuthContext = {
    keyKvHashHex: string;
    keyKvHashBase64: string;
    authKeyBase64: string;
    expiresAt: number;
};

let authContext: StoredAuthContext | null = null;

const restoreFromStorage = (): StoredAuthContext | null => {
    if (typeof window === 'undefined' || typeof localStorage === 'undefined') {
        return null;
    }
    try {
        const raw = localStorage.getItem(AUTH_STORAGE_KEY);
        if (!raw) {
            return null;
        }
        const parsed = JSON.parse(raw) as StoredAuthContext;
        if (!parsed || typeof parsed !== 'object') {
            return null;
        }
        if (typeof parsed.expiresAt !== 'number' || parsed.expiresAt <= Date.now()) {
            localStorage.removeItem(AUTH_STORAGE_KEY);
            return null;
        }
        return parsed;
    } catch (error) {
        console.error('Failed to restore auth context', error);
        return null;
    }
};

const persistContext = (context: StoredAuthContext) => {
    if (typeof window === 'undefined' || typeof localStorage === 'undefined') {
        return;
    }
    localStorage.setItem(AUTH_STORAGE_KEY, JSON.stringify(context));
};

const setAuthContext = (context: StoredAuthContext | null) => {
    authContext = context;
    if (context) {
        persistContext(context);
    } else if (typeof window !== 'undefined' && typeof localStorage !== 'undefined') {
        localStorage.removeItem(AUTH_STORAGE_KEY);
    }
};

const ensureAuthContext = (): StoredAuthContext | null => {
    if (authContext) {
        return authContext;
    }
    const restored = restoreFromStorage();
    if (restored) {
        authContext = restored;
    }
    return authContext;
};

const decodeBase64 = (value: string): Uint8Array => {
    if (typeof atob !== 'function') {
        throw new Error('Base64 decoding unavailable in this environment');
    }
    const normalized = value.trim();
    let buffer = normalized;
    // Reinstate padding if the string is missing it
    const mod = normalized.length % 4;
    if (mod) {
        buffer = normalized + '='.repeat(4 - mod);
    }
    const binary = atob(buffer);
    const bytes = new Uint8Array(binary.length);
    for (let i = 0; i < binary.length; i += 1) {
        bytes[i] = binary.charCodeAt(i);
    }
    return bytes;
};

const encodeBase64 = (bytes: Uint8Array): string => {
    let binary = '';
    bytes.forEach((byte) => {
        binary += String.fromCharCode(byte);
    });
    if (typeof btoa !== 'function') {
        throw new Error('Base64 encoding unavailable in this environment');
    }
    return btoa(binary);
};

const encodeWithPrefix = (bytes: Uint8Array): string => `${BASE64_PREFIX}${encodeBase64(bytes)}`;

const toArrayBuffer = (bytes: Uint8Array): ArrayBuffer =>
    bytes.buffer.slice(bytes.byteOffset, bytes.byteOffset + bytes.byteLength) as ArrayBuffer;

const hexToBytes = (hex: string): Uint8Array => {
    const normalized = hex.trim().toLowerCase();
    if (normalized.length % 2 !== 0) {
        throw new Error('Invalid hex length in key hash');
    }
    const bytes = new Uint8Array(normalized.length / 2);
    for (let i = 0; i < normalized.length; i += 2) {
        bytes[i / 2] = parseInt(normalized.slice(i, i + 2), 16);
    }
    return bytes;
};

const sha512Hex = async (data: Uint8Array): Promise<string> => {
    if (!globalThis.crypto?.subtle) {
        throw new Error('Web Crypto API is unavailable');
    }
    const digest = await crypto.subtle.digest('SHA-512', toArrayBuffer(data));
    const bytes = new Uint8Array(digest);
    return Array.from(bytes)
        .map((byte) => byte.toString(16).padStart(2, '0'))
        .join('');
};

const encryptAesGcm = async (keyBytes: Uint8Array, iv: Uint8Array, plaintext: Uint8Array) => {
    if (!globalThis.crypto?.subtle) {
        throw new Error('Web Crypto API is unavailable');
    }
    const cryptoKey = await crypto.subtle.importKey(
        'raw',
        toArrayBuffer(keyBytes),
        { name: 'AES-GCM' },
        false,
        ['encrypt']
    );
    const ciphertext = await crypto.subtle.encrypt(
        { name: 'AES-GCM', iv: toArrayBuffer(iv) },
        cryptoKey,
        toArrayBuffer(plaintext)
    );
    return new Uint8Array(ciphertext);
};

const randomBytes = (size: number): Uint8Array => {
    if (!globalThis.crypto?.getRandomValues) {
        throw new Error('Secure random generator unavailable');
    }
    const buffer = new Uint8Array(size);
    crypto.getRandomValues(buffer);
    return buffer;
};

const sanitizeUrlParams = () => {
    if (typeof window === 'undefined' || typeof history === 'undefined') {
        return;
    }
    const params = new URLSearchParams(window.location.search);
    params.delete('base64Key');
    params.delete('base64Nonce');
    const query = params.toString();
    const newUrl = `${window.location.pathname}${query ? `?${query}` : ''}${window.location.hash}`;
    history.replaceState(null, '', newUrl);
};

export const hasAuthContext = (): boolean => ensureAuthContext() != null;

export const describeAuthContext = () => ensureAuthContext();

export const clearAuthContext = () => setAuthContext(null);

export const bootstrapAuthFromParams = async (
    params: URLSearchParams,
    apiBaseUrl: string
): Promise<AuthBootstrapResult> => {
    const base64Key = params.get('base64Key')?.trim();
    const base64Nonce = params.get('base64Nonce')?.trim();
    const context = ensureAuthContext();

    if (!base64Key && !base64Nonce) {
        return {
            hasCredentials: !!context,
            message: context ? 'Using cached authentication key.' : 'Provide ?base64Key=…&base64Nonce=… to authenticate.',
            refreshed: false
        };
    }

    if (!base64Key || !base64Nonce) {
        throw new Error('Both base64Key and base64Nonce query parameters are required.');
    }

    const otkKey = decodeBase64(base64Key);
    const otkNonce = decodeBase64(base64Nonce);

    if (otkKey.length !== 32) {
        throw new Error(`Unexpected one-time key length (expected 32 bytes, got ${otkKey.length}).`);
    }

    if (otkNonce.length !== 12) {
        throw new Error(`Unexpected nonce length (expected 12 bytes, got ${otkNonce.length}).`);
    }

    const sha512 = await sha512Hex(otkKey);
    const newAuthKey = randomBytes(32);
    const ciphertext = await encryptAesGcm(otkKey, otkNonce, newAuthKey);

    const response = await fetch(`${apiBaseUrl}/authProcess`, {
        method: 'POST',
        headers: {
            'Content-Type': 'application/json'
        },
        body: JSON.stringify({
            data: encodeWithPrefix(ciphertext),
            sha512
        })
    });

    if (!response.ok) {
        const message = (await response.text()) || `authProcess failed with status ${response.status}`;
        throw new Error(message.trim());
    }

    const payload: { status?: string; keyKvHash?: string } = await response.json();
    const keyKvHashHex = payload.keyKvHash?.trim();
    if (!keyKvHashHex) {
        throw new Error('authProcess response missing keyKvHash');
    }

    const keyKvHashBase64 = encodeBase64(hexToBytes(keyKvHashHex));
    const stored: StoredAuthContext = {
        keyKvHashHex,
        keyKvHashBase64,
        authKeyBase64: encodeBase64(newAuthKey),
        expiresAt: Date.now() + AUTH_DURATION_MS
    };

    setAuthContext(stored);
    sanitizeUrlParams();

    return {
        hasCredentials: true,
        message: 'Authenticated using the one-time key.',
        refreshed: true
    };
};

export const buildAuthHeaders = async (): Promise<Record<string, string>> => {
    const context = ensureAuthContext();
    if (!context) {
        return {};
    }

    const authKey = decodeBase64(context.authKeyBase64);
    const nonce = randomBytes(12);
    const payloadText = `auth:${Date.now()}:${crypto.randomUUID?.() ?? Math.random().toString(16).slice(2)}`;
    const plaintext = new TextEncoder().encode(payloadText);
    const ciphertext = await encryptAesGcm(authKey, nonce, plaintext);

    return {
        'X-Auth-Token': encodeWithPrefix(ciphertext),
        'X-Auth-Nonce': encodeWithPrefix(nonce),
        'X-Auth-KeyHash-Base64': context.keyKvHashBase64
    };
};
