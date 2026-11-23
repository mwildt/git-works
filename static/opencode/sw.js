self.addEventListener('install', (event) => {
    event.waitUntil(self.skipWaiting());
});

self.addEventListener('activate', (event) => {
    event.waitUntil((async () => {
        // Alte Workbox-Caches löschen
        const names = await caches.keys();
        const scope = self.registration.scope;

        await Promise.all(
            names
                .filter(name =>
                    name.includes('-precache-') &&
                    name.includes(scope)
                )
                .map(name => caches.delete(name))
        );

        await self.clients.claim();
    })());
});
