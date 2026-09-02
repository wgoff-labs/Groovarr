'use client';

import { useEffect, useState, useCallback } from 'react';
import { api, CheckResult, PruneResult, HitFallenEntry } from '@/lib/api';

export default function DashboardPage() {
  const [status, setStatus] = useState<{ status: string; service: string } | null>(null);
  const [artists, setArtists] = useState<{ id: number; name: string }[]>([]);
  const [checkResult, setCheckResult] = useState<CheckResult[] | null>(null);
  const [pruneResult, setPruneResult] = useState<PruneResult[] | null>(null);
  const [checkLoading, setCheckLoading] = useState(false);
  const [pruneLoading, setPruneLoading] = useState(false);
  const [checkStatus, setCheckStatus] = useState<string>('');
  const [error, setError] = useState<string | null>(null);
  const [hitFallen, setHitFallen] = useState<HitFallenEntry[] | null>(null);
  const [hitFallenOpen, setHitFallenOpen] = useState(false);

  const load = useCallback(async () => {
    try {
      const [s, a] = await Promise.all([api.status(), api.artists.list()]);
      setStatus(s);
      // Defensive: ensure a is always an array (never null/undefined)
      setArtists(Array.isArray(a) ? a : []);
      setCheckResult(null);
      setPruneResult(null);
      setError(null);
      setCheckStatus('');
    } catch (e: any) {
      setError(`Cannot reach Groovarr backend: ${e.message}`);
      setArtists([]); // Ensure artists is never null
      setCheckResult(null);
      setPruneResult(null);
    }
  }, []);

  useEffect(() => { load(); }, [load]);

  const loadHitFallen = async () => {
    if (hitFallen !== null) { setHitFallenOpen(v => !v); return; }
    try {
      const data = await api.hitfallen.list();
      setHitFallen(data.entries ?? []);
      setHitFallenOpen(true);
    } catch (e: any) {
      setHitFallen([]); // empty on error (e.g. Lidarr not connected)
    }
  };

  const runCheck = async () => {
    setCheckLoading(true);
    setCheckStatus('Starting daily check...');
    setError(null);
    const startedAt = Date.now();
    try {
      const results = await api.check();
      const arr = Array.isArray(results) ? results : [];
      setCheckResult(arr);
      // Build a human-readable summary line
      const totalArtists = arr.length;
      const totalAdded = arr.reduce((s, r) => s + (r.albums_added || 0), 0);
      const totalHits = arr.reduce((s, r) => s + (r.hits_kept || 0), 0);
      const totalErrors = arr.reduce((s, r) => s + ((r.errors || []).length), 0);
      const elapsed = ((Date.now() - startedAt) / 1000).toFixed(1);
      setCheckStatus(
        `Done in ${elapsed}s • ${totalArtists} artist${totalArtists !== 1 ? 's' : ''} • ` +
        `${totalAdded} album${totalAdded !== 1 ? 's' : ''} added • ` +
        `${totalHits} hit${totalHits !== 1 ? 's' : ''} kept` +
        (totalErrors > 0 ? ` • ${totalErrors} error${totalErrors !== 1 ? 's' : ''}` : '')
      );
    } catch (e: any) {
      setError(`Check failed: ${e.message}`);
      setCheckResult([]);
      setCheckStatus(`Failed after ${((Date.now() - startedAt) / 1000).toFixed(1)}s`);
    }
    setCheckLoading(false);
  };

  const runPrune = async () => {
    setPruneLoading(true);
    try {
      const results = await api.prune();
      setPruneResult(Array.isArray(results) ? results : []);
    } catch (e: any) {
      setError(`Prune failed: ${e.message}`);
      setPruneResult([]);
    }
    setPruneLoading(false);
  };

  return (
    <div className="space-y-6">
      {error && (
        <div className="rounded-lg bg-red-900/30 border border-red-700 px-4 py-3 text-red-400">
          {error}
        </div>
      )}

      {/* Status card */}
      <div className="grid grid-cols-1 sm:grid-cols-3 gap-4">
        <div className="card">
          <p className="text-xs uppercase tracking-wider text-gray-400 mb-1">Status</p>
          <p className={`text-lg font-bold ${status ? 'text-emerald-400' : 'text-gray-500'}`}>
            {status ? '✅ Online' : '⏳ Connecting...'}
          </p>
        </div>
        <div className="card">
          <p className="text-xs uppercase tracking-wider text-gray-400 mb-1">Watchlist</p>
          <p className="text-lg font-bold">{artists.length} artist{artists.length !== 1 ? 's' : ''}</p>
        </div>
        <div className="card">
          <p className="text-xs uppercase tracking-wider text-gray-400 mb-1">Last Check</p>
          <p className="text-lg font-bold text-gray-400">Not yet run</p>
        </div>
      </div>

      {/* Actions */}
      <div className="flex gap-3 flex-wrap">
        <button onClick={runCheck} disabled={checkLoading} className="btn-primary min-w-0 flex-1 sm:flex-none">
          {checkLoading ? `⏳ ${checkStatus || 'Checking...'}` : '🔍 Run Daily Check'}
        </button>
        <button onClick={runPrune} disabled={pruneLoading} className="btn-ghost">
          {pruneLoading ? '⏳ Pruning...' : '✂️ Run Prune'}
        </button>
        <button onClick={load} className="btn-ghost">
          🔄 Refresh
        </button>
        <button onClick={loadHitFallen} className="btn-ghost">
          {hitFallenOpen ? '🔼 Hide Fallen' : '📉 Show Fallen Hits'}
        </button>
      </div>

      {/* Live check status (visible while running and for 8s after completion) */}
      {checkStatus && (
        <div
          className={`rounded-lg border px-4 py-3 text-sm flex items-center gap-2 ${
            checkLoading
              ? 'border-emerald-600/40 bg-emerald-900/20 text-emerald-300'
              : error
              ? 'border-red-700 bg-red-900/20 text-red-300'
              : 'border-gray-700 bg-gray-800/50 text-gray-300'
          }`}
        >
          {checkLoading ? (
            <span className="inline-block h-3 w-3 rounded-full border-2 border-emerald-400 border-t-transparent animate-spin" />
          ) : error ? (
            <span>❌</span>
          ) : (
            <span>✅</span>
          )}
          <span className="font-mono text-xs sm:text-sm flex-1">{checkStatus}</span>
          {!checkLoading && (
            <button
              onClick={() => setCheckStatus('')}
              className="text-gray-500 hover:text-white text-xs"
              title="Dismiss"
            >
              ✕
            </button>
          )}
        </div>
      )}

      {/* Hit-fallen review widget */}
      {hitFallenOpen && hitFallen !== null && (
        <div className="card">
          <h2 className="text-lg font-semibold mb-3 flex items-center gap-2">
            📉 Fallen Hits
            <span className="text-xs text-gray-500 font-normal">
              ({hitFallen.length} {hitFallen.length === 1 ? 'entry' : 'entries'})
            </span>
          </h2>
          {hitFallen.length === 0 ? (
            <p className="text-gray-400 text-sm">
              No tracks have fallen out of hit status. Tracks marked <span className="text-gray-300">Hit</span> that drop below threshold will appear here for review.
            </p>
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead>
                  <tr className="text-left text-gray-400 border-b border-gray-700">
                    <th className="pb-2 font-medium">Artist</th>
                    <th className="pb-2 font-medium">Track</th>
                    <th className="pb-2 font-medium">Album</th>
                    <th className="pb-2 font-medium">Score @ Fall</th>
                    <th className="pb-2 font-medium">When</th>
                  </tr>
                </thead>
                <tbody>
                  {hitFallen.map((h) => (
                    <tr key={h.id} className="border-b border-gray-700/50 last:border-0 hover:bg-white/5">
                      <td className="py-2 text-white">
                        {h.artistId ? (
                          <a href={`/artists/${h.artistId}/manage`} className="text-emerald-400 hover:underline">
                            {h.artistName}
                          </a>
                        ) : (
                          h.artistName
                        )}
                      </td>
                      <td className="py-2 text-gray-200">{h.trackTitle || <span className="text-gray-500">#{h.trackId}</span>}</td>
                      <td className="py-2 text-gray-400">{h.albumTitle || '—'}</td>
                      <td className="py-2 text-gray-300">
                        <span className={h.scoreAtFall >= 60 ? 'text-yellow-400' : 'text-red-400'}>
                          {h.scoreAtFall}
                        </span>
                      </td>
                      <td className="py-2 text-gray-500 text-xs">
                        {new Date(h.fallenAt).toLocaleString()}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </div>
      )}

      {/* Check results */}
      {checkResult && (
        <div className="card">
          <h2 className="text-lg font-semibold mb-3">📋 Check Results</h2>
          {checkResult.length === 0 ? (
            <p className="text-gray-400">No artists in watchlist. Add some from the Artists page.</p>
          ) : (
            <div className="space-y-3">
              {checkResult.map((r) => (
                <div key={r.artist_name} className="border-b border-gray-700 pb-3 last:border-0">
                  <p className="font-medium text-white">{r.artist_name}</p>
              {(r.errors ?? []).map((e, i) => (
                    <p key={i} className="text-red-400 text-sm">❌ {e}</p>
                  ))}
                  {(r.added_albums ?? []).map((a) => (
                    <p key={a} className="text-emerald-400 text-sm">✅ {a}</p>
                  ))}
                  {r.albums_added === 0 && (r.errors ?? []).length === 0 && (
                    <p className="text-gray-400 text-sm">No new popular releases</p>
                  )}
                </div>
              ))}
            </div>
          )}
        </div>
      )}

      {/* Prune results */}
      {pruneResult && (
        <div className="card">
          <h2 className="text-lg font-semibold mb-3">✂️ Prune Results</h2>
          {pruneResult.length === 0 ? (
            <p className="text-gray-400">Nothing to prune.</p>
          ) : (
            <div className="space-y-2">
              {pruneResult.map((r, i) => (
                <div key={i} className="border-b border-gray-700 pb-2 last:border-0">
                  <p className="text-white">
                    <span className="font-medium">{r.artist_name}</span>
                    {' — '}
                    <span className="text-gray-300">{r.album_name}</span>
                  </p>
                  {r.error ? (
                    <p className="text-red-400 text-sm">❌ {r.error}</p>
                  ) : (
                    <p className="text-gray-400 text-sm">
                      Kept {r.kept_tracks} track{r.kept_tracks !== 1 ? 's' : ''} | Pruned {r.pruned_tracks}
                    </p>
                  )}
                </div>
              ))}
            </div>
          )}
        </div>
      )}
    </div>
  );
}
