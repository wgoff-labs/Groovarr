'use client';

import { useEffect, useState, useCallback } from 'react';
import { api, Artist } from '@/lib/api';

export default function ArtistsPage() {
  const [artists, setArtists] = useState<Artist[]>([]);
  const [loading, setLoading] = useState(true);
  const [adding, setAdding] = useState(false);
  const [newName, setNewName] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [actionMsg, setActionMsg] = useState<string | null>(null);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      setArtists(await api.artists.list());
      setError(null);
    } catch (e: any) {
      setError(`Backend unreachable: ${e.message}`);
    }
    setLoading(false);
  }, []);

  useEffect(() => { load(); }, [load]);

  const addArtist = async () => {
    if (!newName.trim()) return;
    setAdding(true);
    try {
      await api.check(newName.trim()); // triggers add via backend
      setNewName('');
      setActionMsg('Artist queued for next check.');
      setTimeout(() => setActionMsg(null), 4000);
    } catch (e: any) {
      setError(`Failed to add artist: ${e.message}`);
    }
    setAdding(false);
  };

  const removeArtist = async (name: string) => {
    if (!confirm(`Remove "${name}" from the watchlist?`)) return;
    try {
      await api.artists.remove(name);
      setArtists((prev) => prev.filter((a) => a.name !== name));
    } catch (e: any) {
      setError(`Remove failed: ${e.message}`);
    }
  };

  const runScan = async (name: string) => {
    setActionMsg(`Scanning ${name}...`);
    try {
      await api.scan(name);
      setActionMsg(`Full scan complete for ${name}.`);
      setTimeout(() => setActionMsg(null), 5000);
    } catch (e: any) {
      setActionMsg(`Scan failed: ${e.message}`);
    }
  };

  return (
    <div className="space-y-6">
      {error && (
        <div className="rounded-lg bg-red-900/30 border border-red-700 px-4 py-3 text-red-400">
          {error}
        </div>
      )}
      {actionMsg && (
        <div className="rounded-lg bg-emerald-900/30 border border-emerald-700 px-4 py-3 text-emerald-400">
          {actionMsg}
        </div>
      )}

      {/* Add artist */}
      <div className="card">
        <h2 className="text-lg font-semibold mb-3">➕ Add Artist</h2>
        <div className="flex gap-3">
          <input
            type="text"
            placeholder="Artist name..."
            value={newName}
            onChange={(e) => setNewName(e.target.value)}
            onKeyDown={(e) => e.key === 'Enter' && addArtist()}
            className="flex-1 bg-gray-800 border border-gray-600 rounded-lg px-4 py-2 text-white placeholder-gray-500 focus:outline-none focus:border-emerald-500"
          />
          <button onClick={addArtist} disabled={adding || !newName.trim()} className="btn-primary">
            {adding ? '⏳...' : 'Add'}
          </button>
        </div>
        <p className="text-xs text-gray-500 mt-2">
          Searches Deezer &amp; Last.fm, adds to Lidarr on next check.
        </p>
      </div>

      {/* Artist list */}
      <div className="card">
        <h2 className="text-lg font-semibold mb-4">🎵 Watchlist ({artists.length})</h2>
        {loading ? (
          <p className="text-gray-400">Loading...</p>
        ) : artists.length === 0 ? (
          <p className="text-gray-400">No artists yet. Add one above.</p>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="text-left text-gray-400 border-b border-gray-700">
                  <th className="pb-2 font-medium">Artist</th>
                  <th className="pb-2 font-medium">Folder</th>
                  <th className="pb-2 font-medium">Added by</th>
                  <th className="pb-2 font-medium">Added</th>
                  <th className="pb-2 font-medium text-right">Actions</th>
                </tr>
              </thead>
              <tbody>
                {artists.map((a) => (
                  <tr key={a.id} className="border-b border-gray-700/50 last:border-0 hover:bg-white/5">
                    <td className="py-2.5 font-medium text-white">{a.name}</td>
                    <td className="py-2.5 text-gray-400">{a.root_folder ?? '—'}</td>
                    <td className="py-2.5 text-gray-400">{a.added_by}</td>
                    <td className="py-2.5 text-gray-500">{a.added_at?.split('T')[0] ?? '—'}</td>
                    <td className="py-2.5 text-right">
                      <button
                        onClick={() => runScan(a.name)}
                        className="text-xs text-emerald-400 hover:text-emerald-300 mr-3"
                      >
                        📡 Scan
                      </button>
                      <button
                        onClick={() => removeArtist(a.name)}
                        className="text-xs text-red-400 hover:text-red-300"
                      >
                        🗑️ Remove
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </div>
  );
}
