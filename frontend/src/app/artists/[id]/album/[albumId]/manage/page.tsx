'use client';

import { useEffect, useState, useCallback, useMemo } from 'react';
import Link from 'next/link';
import { useParams } from 'next/navigation';
import { api, TrackState, ManagedTrack, ApiError } from '@/lib/api';

const STATE_LABELS: Record<TrackState, string> = {
  keep: 'Keep',
  hit: 'Hit',
  not_keep: 'Not Keep',
  '': 'Auto',
};

export default function AlbumManagePage() {
  const params = useParams<{ id: string; albumId: string }>();
  const artistId = Number(params.id);
  const albumLidarrId = Number(params.albumId);

  const [artist, setArtist] = useState<{ id: number; name: string; lidarrId: number | null } | null>(null);
  const [albumName, setAlbumName] = useState<string>('');
  const [tracks, setTracks] = useState<ManagedTrack[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [lidarrError, setLidarrError] = useState(false);
  const [actionMsg, setActionMsg] = useState<string | null>(null);
  const [selected, setSelected] = useState<Set<number>>(new Set());
  type TrackSortKey = 'trackNumber' | 'title' | 'score' | 'downloaded' | 'state';
  const [trackSort, setTrackSort] = useState<{ key: TrackSortKey; dir: 'asc' | 'desc' }>({ key: 'trackNumber', dir: 'asc' });

  const toggleSort = (key: TrackSortKey) => {
    setTrackSort((prev) => prev.key === key ? { key, dir: prev.dir === 'asc' ? 'desc' : 'asc' } : { key, dir: 'asc' });
  };

  const SortIcon = ({ active, dir }: { active: boolean; dir: 'asc' | 'desc' }) => (
    <span className="ml-1 text-xs">{active ? (dir === 'asc' ? '↑' : '↓') : ''}</span>
  );

  const sortedTracks = useMemo(() => {
    const arr = [...tracks];
    const dir = trackSort.dir === 'asc' ? 1 : -1;
    arr.sort((a, b) => {
      const av: any = (a as any)[trackSort.key] ?? '';
      const bv: any = (b as any)[trackSort.key] ?? '';
      if (typeof av === 'boolean' && typeof bv === 'boolean') return (av === bv ? 0 : av ? 1 : -1) * dir;
      if (typeof av === 'number' && typeof bv === 'number') return (av - bv) * dir;
      return (av.toString().toLowerCase() < bv.toString().toLowerCase() ? -1 : av.toString().toLowerCase() > bv.toString().toLowerCase() ? 1 : 0) * dir;
    });
    return arr;
  }, [tracks, trackSort]);

  const loadAlbum = useCallback(async () => {
    setLoading(true);
    setError(null);
    setLidarrError(false);
    try {
      const data = await api.artists.manage(artistId);
      setArtist(data.artist);
      // Filter to just this album's tracks
      const albumTracks = (data.tracks || []).filter((t: ManagedTrack) => t.albumLidarrId === albumLidarrId);
      setTracks(albumTracks);
      if (albumTracks.length > 0) {
        setAlbumName(albumTracks[0].albumTitle);
      }
    } catch (e: any) {
      if (e instanceof ApiError && e.lidarr) {
        setError(e.lidarr.error);
        setLidarrError(true);
      } else {
        setError(`Failed to load: ${e.message}`);
        setLidarrError(false);
      }
      setArtist(null);
      setTracks([]);
    }
    setLoading(false);
  }, [artistId, albumLidarrId]);

  useEffect(() => {
    if (artistId && albumLidarrId) loadAlbum();
  }, [artistId, albumLidarrId, loadAlbum]);

  const setTrackState = async (track: ManagedTrack, newState: TrackState) => {
    try {
      await api.tracks.setState(artistId, track.lidarrId, newState);
      setTracks((prev) =>
        prev.map((t) => (t.lidarrId === track.lidarrId ? { ...t, state: newState } : t))
      );
    } catch (e: any) {
      setActionMsg(`Failed: ${e.message}`);
      setTimeout(() => setActionMsg(null), 4000);
    }
  };

  const bulkSetState = async (state: TrackState) => {
    if (selected.size === 0) return;
    for (const trackId of selected) {
      try {
        await api.tracks.setState(artistId, trackId, state);
      } catch { /* skip */ }
    }
    setTracks((prev) =>
      prev.map((t) => (selected.has(t.lidarrId) ? { ...t, state } : t))
    );
    setActionMsg(`${selected.size} tracks set to "${STATE_LABELS[state]}".`);
    setTimeout(() => setActionMsg(null), 4000);
    setSelected(new Set());
  };

  const toggleTrack = (id: number) => {
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  };

  const toggleAll = () => {
    if (selected.size === sortedTracks.length) {
      setSelected(new Set());
    } else {
      setSelected(new Set(sortedTracks.map((t) => t.lidarrId)));
    }
  };

  const filteredTracks = useMemo(() => {
    return tracks;
  }, [tracks]);

  if (loading) return <p className="text-gray-400">Loading album...</p>;
  if (error) {
    return (
      <div className="space-y-4">
        <div className="rounded-lg bg-red-900/30 border border-red-700 px-4 py-3 text-red-400">
          {error}
          {lidarrError && (
            <div className="mt-2">
              <Link href="/settings" className="text-sm text-blue-400 hover:text-blue-300 underline">
                → Go to Settings
              </Link>
              <span className="text-gray-500 text-xs ml-2">(connect Lidarr, then reload)</span>
            </div>
          )}
        </div>
        <Link href="/artists" className="btn-secondary">← Back to Artists</Link>
      </div>
    );
  }
  if (!artist) {
    return (
      <div className="space-y-4">
        <p className="text-gray-400">Artist not found.</p>
        <Link href="/artists" className="btn-secondary">← Back to Artists</Link>
      </div>
    );
  }

  const downloadedCount = tracks.filter((t) => t.downloaded).length;
  const stateCounts: Record<string, number> = { keep: 0, hit: 0, not_keep: 0, unset: 0 };
  for (const t of tracks) {
    if (t.state === '') stateCounts.unset++;
    else stateCounts[t.state]++;
  }

  return (
    <div className="space-y-6">
      <div>
        <div className="text-sm text-gray-400 mb-1">
          <Link href="/artists" className="hover:text-emerald-400">Artists</Link>
          <span className="mx-2">›</span>
          <Link href={`/artists/${artistId}/manage`} className="hover:text-emerald-400">{artist.name}</Link>
          <span className="mx-2">›</span>
          <span>{albumName || 'Album'}</span>
          <span className="mx-2">›</span>
          <span>Manage</span>
        </div>
        <h1 className="text-2xl font-bold text-white">💿 {albumName || 'Album Tracks'}</h1>
        <p className="text-xs text-gray-500 mt-1">
          {tracks.length} tracks · {downloadedCount} downloaded · by {artist.name}
        </p>
      </div>

      {actionMsg && (
        <div className="rounded-lg bg-emerald-900/30 border border-emerald-700 px-4 py-3 text-emerald-400">
          {actionMsg}
        </div>
      )}

      {/* State summary */}
      <div className="grid grid-cols-2 md:grid-cols-4 gap-3 text-sm">
        <div className="bg-gray-800/50 rounded-lg p-3 border border-gray-700">
          <div className="text-gray-400 text-xs">Keep</div>
          <div className="text-2xl font-bold text-green-400">{stateCounts.keep}</div>
        </div>
        <div className="bg-gray-800/50 rounded-lg p-3 border border-gray-700">
          <div className="text-gray-400 text-xs">Hit</div>
          <div className="text-2xl font-bold text-gray-300">{stateCounts.hit}</div>
        </div>
        <div className="bg-gray-800/50 rounded-lg p-3 border border-gray-700">
          <div className="text-gray-400 text-xs">Not Keep</div>
          <div className="text-2xl font-bold text-red-400">{stateCounts.not_keep}</div>
        </div>
        <div className="bg-gray-800/50 rounded-lg p-3 border border-gray-700">
          <div className="text-gray-400 text-xs">Auto</div>
          <div className="text-2xl font-bold text-gray-400">{stateCounts.unset}</div>
        </div>
      </div>

      {/* Bulk controls */}
      <div className="card">
        <div className="flex items-center justify-between flex-wrap gap-2 mb-4">
          <h2 className="text-lg font-semibold">Track Controls</h2>
          {selected.size > 0 && (
            <div className="flex gap-2">
              <button onClick={() => bulkSetState('keep')} className="px-3 py-1 text-xs rounded bg-green-700 hover:bg-green-600 text-white">Keep All</button>
              <button onClick={() => bulkSetState('hit')} className="px-3 py-1 text-xs rounded bg-gray-600 hover:bg-gray-500 text-white">Hit All</button>
              <button onClick={() => bulkSetState('not_keep')} className="px-3 py-1 text-xs rounded bg-red-700 hover:bg-red-600 text-white">Not Keep All</button>
              <button onClick={() => bulkSetState('')} className="px-3 py-1 text-xs rounded bg-gray-700 hover:bg-gray-600 text-white">Reset All</button>
              <span className="text-gray-400 text-xs self-center">{selected.size} selected</span>
            </div>
          )}
        </div>

        {tracks.length === 0 ? (
          <p className="text-gray-500 text-sm">No tracks found for this album.</p>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="text-left text-gray-400 border-b border-gray-700">
                  <th className="pb-2 font-medium w-8">
                    <input type="checkbox" checked={selected.size === sortedTracks.length && sortedTracks.length > 0} onChange={toggleAll} className="rounded" />
                  </th>
                  <th className="pb-2 font-medium w-12 cursor-pointer select-none hover:text-emerald-400" onClick={() => toggleSort('trackNumber')}>#<SortIcon active={trackSort.key==='trackNumber'} dir={trackSort.dir} /></th>
                  <th className="pb-2 font-medium cursor-pointer select-none hover:text-emerald-400" onClick={() => toggleSort('title')}>Track<SortIcon active={trackSort.key==='title'} dir={trackSort.dir} /></th>
                  <th className="pb-2 font-medium cursor-pointer select-none hover:text-emerald-400" onClick={() => toggleSort('score')}>Score<SortIcon active={trackSort.key==='score'} dir={trackSort.dir} /></th>
                  <th className="pb-2 font-medium cursor-pointer select-none hover:text-emerald-400" onClick={() => toggleSort('downloaded')}>Downloaded<SortIcon active={trackSort.key==='downloaded'} dir={trackSort.dir} /></th>
                  <th className="pb-2 font-medium cursor-pointer select-none hover:text-emerald-400" onClick={() => toggleSort('state')}>State<SortIcon active={trackSort.key==='state'} dir={trackSort.dir} /></th>
                </tr>
              </thead>
              <tbody>
                {sortedTracks.map((t) => (
                    <tr key={t.lidarrId} className="border-b border-gray-700/50 last:border-0 hover:bg-white/5">
                      <td className="py-2">
                        <input type="checkbox" checked={selected.has(t.lidarrId)} onChange={() => toggleTrack(t.lidarrId)} className="rounded" />
                      </td>
                      <td className="py-2 text-gray-500">
                        {t.discNumber > 1 ? `${t.discNumber}-${t.trackNumber}` : t.trackNumber}
                      </td>
                      <td className="py-2 text-white">{t.title}</td>
                      <td className="py-2 text-gray-400">
                        {t.score != null ? (
                          <span className={t.score >= 70 ? 'text-green-400' : t.score >= 50 ? 'text-yellow-400' : 'text-gray-500'}>
                            {t.score}
                          </span>
                        ) : '—'}
                      </td>
                      <td className="py-2">
                        <span className={t.downloaded ? 'text-emerald-400' : 'text-gray-600'}>
                          {t.downloaded ? '⬇️' : '⬜'}
                        </span>
                      </td>
                      <td className="py-2">
                        <div className="flex gap-1">
                          {(['keep', 'hit', 'not_keep'] as TrackState[]).map((state) => (
                            <button
                              key={state}
                              onClick={() => setTrackState(t, state)}
                              className={`px-2 py-0.5 text-xs rounded border transition-colors ${
                                t.state === state
                                  ? state === 'keep'
                                    ? 'bg-green-600 text-white border-green-500'
                                    : state === 'hit'
                                    ? 'bg-gray-500 text-white border-gray-400'
                                    : 'bg-red-600 text-white border-red-500'
                                  : 'bg-gray-800/40 text-gray-300 border-gray-700/50 hover:bg-gray-700/40'
                              }`}
                            >
                              {STATE_LABELS[state]}
                            </button>
                          ))}
                          {t.state !== '' && (
                            <button
                              onClick={() => setTrackState(t, '')}
                              className="px-1 py-0.5 text-xs rounded text-gray-500 hover:text-gray-300"
                              title="Reset to auto"
                            >
                              ↺
                            </button>
                          )}
                        </div>
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
