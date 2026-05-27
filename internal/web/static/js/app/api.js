/* ============================================================
   api.js — URL helpers that prepend the server-configured base
   path. The base path is injected by the HTML template into
   window.KULA_BASE_PATH. When unset, both helpers behave
   identically to using the bare path.
   ============================================================ */
'use strict';

const BASE = (typeof window !== 'undefined' && window.KULA_BASE_PATH) || '';

export function apiUrl(path) {
    return BASE + path;
}

export function wsUrl(path) {
    const proto = location.protocol === 'https:' ? 'wss:' : 'ws:';
    return `${proto}//${location.host}${BASE}${path}`;
}
