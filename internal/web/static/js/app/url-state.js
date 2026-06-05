/* ============================================================
   url-state.js — Sync the selected time window (preset range),
   custom time range, and aggregation to/from the URL query
   string so a dashboard view can be bookmarked and shared.

   Recognized parameters:
     range=<seconds>   preset time window (mutually exclusive with from/to)
     from=<ISO>&to=<ISO>  custom time range
     agg=avg|min|max   aggregation mode
   ============================================================ */
'use strict';
import { state } from './state.js';
import { syncTimeRangeUI, syncCustomRangeUI } from './controls.js';

const VALID_AGG = ['avg', 'min', 'max'];

// Preset windows in seconds, mirrors the .time-btn[data-range] buttons
// in index.html. Used to reject arbitrary values from the URL.
const VALID_RANGES = [60, 300, 900, 1800, 3600, 10800, 21600, 43200, 86400, 259200, 604800, 2592000];

// Below this window the data is typically served raw (tier 0) and the dashboard
// hides the aggregation selector, so the agg parameter is meaningless — match
// that in the URL by only emitting agg for windows of 3h or longer.
//
// NOTE: this is a client-side heuristic, not the authoritative gate. The server
// chooses a tier dynamically from retained data (see QueryRangeWithMeta in
// internal/storage/store.go) and the selector's real visibility is driven by the
// returned tier === 0 in updateSamplingInfo() (charts-data.js). We can't consult
// that here because updateUrl() runs before the history fetch returns, so we
// approximate it with the default tier-0 retention window. If that retention is
// reconfigured this constant may drift, but the only effect is a redundant or
// absent agg param in the URL — never wrong data. Keep it in sync with the
// smallest preset window that maps to an aggregated tier.
const AGG_MIN_WINDOW = 10800; // 3h, in seconds

// aggRelevant reports whether the aggregation mode applies to the current
// window (i.e. the aggregation selector would be shown in the UI). See the
// AGG_MIN_WINDOW note above for why this is a heuristic rather than exact.
function aggRelevant() {
    if (state.timeRange !== null) {
        return state.timeRange >= AGG_MIN_WINDOW;
    }
    if (state.customFrom && state.customTo) {
        return (state.customTo - state.customFrom) >= AGG_MIN_WINDOW * 1000;
    }
    return false;
}

// applyUrlState parses the current URL query string and applies any
// recognized parameters into application state, syncing the relevant
// controls. It does NOT trigger a history fetch — the initial fetch is
// performed by the WebSocket onopen handler once connected. Run this
// during init() (after i18n is ready) and before the first fetch.
export function applyUrlState() {
    const params = new URLSearchParams(window.location.search);

    const agg = params.get('agg');
    if (agg && VALID_AGG.includes(agg)) {
        state.currentAggregation = agg;
        state.aggFromUrl = true;
    }

    const from = params.get('from');
    const to = params.get('to');
    if (from && to) {
        const fromDate = new Date(from);
        const toDate = new Date(to);
        if (!isNaN(fromDate.getTime()) && !isNaN(toDate.getTime()) && fromDate < toDate) {
            state.timeRange = null;
            state.customFrom = fromDate;
            state.customTo = toDate;
            syncCustomRangeUI(fromDate, toDate);
            return;
        }
    }

    const range = params.get('range');
    if (range !== null) {
        const seconds = parseInt(range, 10);
        if (VALID_RANGES.includes(seconds)) {
            state.timeRange = seconds;
            state.customFrom = null;
            state.customTo = null;
            syncTimeRangeUI(seconds);
        }
    }
}

// updateUrl rewrites the URL query string to describe the current view
// (range or from/to, plus agg) using replaceState so it does not create
// a new browser history entry. Call after any time-window, custom-range,
// or aggregation change.
export function updateUrl() {
    const params = new URLSearchParams(window.location.search);

    if (state.timeRange !== null) {
        params.set('range', String(state.timeRange));
        params.delete('from');
        params.delete('to');
    } else if (state.customFrom && state.customTo) {
        params.delete('range');
        params.set('from', state.customFrom.toISOString());
        params.set('to', state.customTo.toISOString());
    }

    // Only emit agg when it actually applies to the window and differs from
    // the server default — keeps shared URLs minimal and free of redundancy.
    if (aggRelevant() && state.currentAggregation !== state.defaultAggregation) {
        params.set('agg', state.currentAggregation);
    } else {
        params.delete('agg');
    }

    const qs = params.toString();
    const newUrl = window.location.pathname + (qs ? '?' + qs : '') + window.location.hash;
    window.history.replaceState(null, '', newUrl);
}
