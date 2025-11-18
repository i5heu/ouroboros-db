const DATA_CACHE = 'ouroboros-data-v1';
const META_CACHE = 'ouroboros-meta-v1';
const CACHE_NAMES = [DATA_CACHE, META_CACHE];

self.addEventListener('install', (event) => {
    self.skipWaiting();
    if (!event.waitUntil) {
        return;
    }
    event.waitUntil(Promise.resolve());
});

self.addEventListener('activate', (event) => {
    if (!event.waitUntil) {
        return;
    }
    event.waitUntil(
        caches.keys().then((keys) =>
            Promise.all(
                keys.map((key) => {
                    if (!CACHE_NAMES.includes(key)) {
                        return caches.delete(key);
                    }
                    return Promise.resolve(false);
                })
            ).then(() => self.clients?.claim?.())
        )
    );
});

const cacheFirst = async (cacheName, request) => {
    const cache = await caches.open(cacheName);
    const match = await cache.match(request);
    if (match) {
        return match;
    }
    const response = await fetch(request.clone());
    if (response.ok) {
        await cache.put(request, response.clone());
    }
    return response;
};

const staleWhileRevalidate = async (cacheName, request) => {
    const cache = await caches.open(cacheName);
    const cachedPromise = cache.match(request);
    const fetchPromise = fetch(request.clone())
        .then(async (response) => {
            if (response && response.ok) {
                await cache.put(request, response.clone());
            }
            return response;
        })
        .catch(() => undefined);

    const cached = await cachedPromise;
    return cached ?? (await fetchPromise) ?? fetch(request);
};

self.addEventListener('fetch', (event) => {
    const { request } = event;
    if (!request || request.method !== 'GET') {
        return;
    }
    const url = new URL(request.url);
    if (url.origin !== self.location.origin) {
        return;
    }
    if (request.headers.has('range')) {
        return;
    }
    if (url.pathname.startsWith('/data/')) {
        event.respondWith(cacheFirst(DATA_CACHE, request));
        return;
    }
    if (url.pathname.startsWith('/meta/')) {
        event.respondWith(staleWhileRevalidate(META_CACHE, request));
    }
});
