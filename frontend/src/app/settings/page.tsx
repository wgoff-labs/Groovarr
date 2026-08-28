'use client';

import { useEffect, useState, useCallback } from 'react';
import { api } from '@/lib/api';

const THRESHOLD_KEY = 'popularity_threshold';
const MODE_KEY = 'download_mode';

export default function SettingsPage() {
  const [threshold, setThreshold] = useState('60');
  const [mode, setMode] = useState('tracks');
  const [saving, setSaving] = useState(false);
  const [saved, setSaved] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(async () => {
    try {
      const [t, m] = await Promise.all([
        api.settings.get(THRESHOLD_KEY),
        api.settings.get(MODE_KEY),
      ]);
      setThreshold(t.value || '60');
      setMode(m.value || 'tracks');
    } catch (_) {
      // Settings may not exist yet — use defaults
    }
  }, []);

  useEffect(() => { load(); }, [load]);

  const save = async (key: string, value: string) => {
    setSaving(true);
    try {
      await api.settings.set(key, value);
      setSaved(`${key} saved`);
      setTimeout(() => setSaved(null), 3000);
      setError(null);
    } catch (e: any) {
      setError(`Save failed: ${e.message}`);
    }
    setSaving(false);
  };

  return (
    <div className="space-y-6 max-w-xl">
      {error && (
        <div className="rounded-lg bg-red-900/30 border border-red-700 px-4 py-3 text-red-400">
          {error}
        </div>
      )}
      {saved && (
        <div className="rounded-lg bg-emerald-900/30 border border-emerald-700 px-4 py-3 text-emerald-400">
          ✅ {saved}
        </div>
      )}

      {/* Popularity Threshold */}
      <div className="card">
        <h2 className="text-lg font-semibold mb-1">🎯 Popularity Threshold</h2>
        <p className="text-xs text-gray-400 mb-4">
          Albums/tracks with avg popularity at or above this score are added. Higher = stricter.
        </p>

        <div className="flex items-center gap-4">
          <input
            type="range"
            min="10" max="100" step="5"
            value={threshold}
            onChange={(e) => setThreshold(e.target.value)}
            className="flex-1 accent-emerald-500"
          />
          <span className="text-2xl font-bold text-white w-12 text-center">{threshold}</span>
        </div>
        <div className="flex justify-between text-xs text-gray-500 mt-1">
          <span>10 — Everything</span>
          <span>50 — Deep cuts + hits</span>
          <span>80 — Big hits only</span>
        </div>
        <button onClick={() => save(THRESHOLD_KEY, threshold)} disabled={saving} className="btn-primary mt-4">
          {saving ? 'Saving...' : 'Save Threshold'}
        </button>
      </div>

      {/* Download Mode */}
      <div className="card">
        <h2 className="text-lg font-semibold mb-1">💾 Download Mode</h2>
        <p className="text-xs text-gray-400 mb-4">
          How Groovarr handles albums that pass the threshold.
        </p>

        <div className="grid grid-cols-2 gap-3">
          {[
            { value: 'tracks', label: '🎶 Tracks Only', desc: 'Only popular tracks downloaded. Rest deleted after download. (Recommended)' },
            { value: 'album', label: '💿 Full Album', desc: 'Grab the whole album if any track is popular. No pruning.' },
          ].map(({ value, label, desc }) => (
            <button
              key={value}
              onClick={() => { setMode(value); save(MODE_KEY, value); }}
              className={`card text-left cursor-pointer transition-all ${mode === value ? 'border-emerald-500 bg-emerald-900/20' : 'hover:border-gray-500'}`}
            >
              <p className="font-semibold text-white mb-1">{label}</p>
              <p className="text-xs text-gray-400">{desc}</p>
            </button>
          ))}
        </div>
      </div>

      {/* About */}
      <div className="card text-xs text-gray-500">
        <p>Groovarr connects Lidarr with Deezer &amp; Last.fm to automatically filter your music library to only the popular releases.</p>
        <p className="mt-2">Schedule: Daily at 9 AM (America/Detroit) | Database: SQLite | Discord: Optional</p>
      </div>
    </div>
  );
}
