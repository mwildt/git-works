class Router {
    #listeners = new Set();

    constructor() {
        if (!location.hash) {
            history.replaceState({}, '', '#/'); // URL wird zu https://example.com/#/
        }
        window.addEventListener('hashchange', () => this.#notify());
    }

    get path() {
        return (location.hash.slice(1).split('?')[0]) || '/';
    }

    get query() {
        const qs = location.hash.split('?')[1] || '';
        return Object.fromEntries(new URLSearchParams(qs));
    }

    go(path, query = {}) {
        const qs = new URLSearchParams(query).toString();
        location.hash = qs ? `${path}?${qs}` : path;
        // feuert hashchange automatisch, kein #notify() nötig
    }

    subscribe(fn) {
        this.#listeners.add(fn);
        return () => this.#listeners.delete(fn);
    }

    #notify() {
        const state = { path: this.path, query: this.query };
        for (const fn of this.#listeners) {
            fn(state);
        }
    }
}

export const router = new Router();