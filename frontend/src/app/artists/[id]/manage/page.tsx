'use client';

import { useEffect, useState, useCallback, useMemo } from 'react';
import Link from 'next/link';
import { useParams, useRouter } from 'next/navigation';
import { api, TrackState, ManagedAlbum, ManagedTrack } from '@/lib/api';

const STATE_LABELS: Record<TrackState | 'auto', string> = {
  '': 'Auto',
  keep: 'Keep',
  hit: 'Hit',
  not_keep: 'Not Keep',
  auto: 'Auto',
};

function StateBadge({ state }: { state: TrackState }) {
  const cls =
    state === 'keep'
      ? 'bg-green-900/40 text-green-300 border-green-700'
      : state === 'hit'
      ? 'bg-gray-700/60 text-gray-200 border-gray-500'
      : state === 'not_keep'
      ? 'bg-red-900/40 text-red-300 border-red-700'
      : 'bg-gray-800/40 text-gray-400 border-gray-700 border-dashed';
  return (
    <span className={`inline-block text-xs px-2 py-0.5 rounded border ${cls}`}>
      {STATE_LABELS[state] || 'Auto'}
    </span>
  );
}

export default function ArtistManagePage() {
  const params = useParams<{ id: string }>();
  const router = useRouter();
  const artistId = Number(params.id);

  const [artist, setArtist] = useState<{ id: number; name: string; lidarrId: number | null; rootFolder: string | null } | null>(null);
  const [albums, setAlbums] = useState<ManagedAlbum[]>([]);
  const [tracks, setTracks] = useState<ManagedTrack[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [actionMsg, setActionMsg] = useState<string | null>(null);
  const [search, setSearch] = useState('');
  const [stateFilter, setStateFilter] = useState<'' | 'unset' | TrackState>('');
  const [bulkMode, setBulkMode] = useState(false);
  const [selectedTracks, setSelectedTracks] = useState<Set<number>>(new Set());
  type TrackSortKey = 'trackNumber' | 'title' | 'albumTitle' | 'score' | 'downloaded' | 'state';
  const [trackSort, setTrackSort] = useState<{ key: TrackSortKey; dir: 'asc' | 'desc' }>({ key: 'trackNumber', dir: 'asc' });
  type AlbumSortKey = 'title' | 'year' | 'trackCount' | 'monitored';
  const [albumSort, setAlbumSort] = useState<{ key: AlbumSortKey; dir: 'asc' | 'desc' }>({ key: 'title', dir: 'asc' });

  const toggleTrackSort = (key: TrackSortKey) => {
    setTrackSort((prev) => prev.key === key ? { key, dir: prev.dir === 'asc' ? 'desc' : 'asc' } : { key, dir: 'asc' });
  };
  const toggleAlbumSort = (key: AlbumSortKey) => {
    setAlbumSort((prev) => prev.key === key ? { key, dir: prev.dir === 'asc' ? 'desc' : 'asc' } : { key, dir: 'asc' });
  };

  const SortIcon = ({ active, dir }: { active: boolean; dir: 'asc' | 'desc' }) => (
    <span className="ml-1 text-xs">{active ? (dir === 'asc' ? '↑' : '↓') : ''}</span>
  );

  const loadManage = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const data = await api.artists.manage(artistId);
      setArtist(data.artist);
      setAlbums(Array.isArray(data.albums) ? data.albums : []);
      setTracks(Array.isArray(data.tracks) ? data.tracks : []);
    } catch (e: any) {
      setError(`Failed to load: ${e.message}`);
      setArtist(null);
      setAlbums([]);
      setTracks([]);
    }
    setLoading(false);
  }, [artistId]);

  useEffect(() => {
    if (artistId) loadManage();
  }, [artistId, loadManage]);

  const setTrackState = async (track: ManagedTrack, newState: TrackState) => {
    try {
      await api.tracks.setState(track.lidarrId > 0 ? artistId : artistId, track.lidarrId, newState);
      setTracks((prev) =>
        prev.map((t) => (t.lidarrId === track.lidarrId ? { ...t, state: newState } : t))
      );
    } catch (e: any) {
      setActionMsg(`Failed: ${e.message}`);
      setTimeout(() => setActionMsg(null), 4000);
    }
  };

  const bulkSetState = async (state: TrackState) => {
    if (selectedTracks.size === 0) return;
    try {
      // Sequential updates — backend doesn't batch, but tracks are small.
      for (const trackId of selectedTracks) {
        await api.tracks.setState(artistId, trackId, state);
      }
      setTracks((prev) =>
        prev.map((t) => (selectedTracks.has(t.lidarrId) ? { ...t, state } : t))
      );
      setActionMsg(`Set ${selectedTracks.size} tracks to "${STATE_LABELS[state]}".`);
      setTimeout(() => setActionMsg(null), 4000);
      setSelectedTracks(new Set());
      setBulkMode(false);
    } catch (e: any) {
      setActionMsg(`Bulk failed: ${e.message}`);
    }
  };

  const filteredTracks = useMemo(() => {
    return tracks.filter((t) => {
      // State filter
      if (stateFilter === 'unset' && t.state !== '') return false;
      if (stateFilter !== '' && stateFilter !== 'unset' && t.state !== stateFilter) return false;
      // Search filter
      if (search) {
        const q = search.toLowerCase();
        if (
          !t.title.toLowerCase().includes(q) &&
          !t.albumTitle.toLowerCase().includes(q)
        ) {
          return false;
        }
      }
      return true;
    });
  }, [tracks, search, stateFilter]);

  const sortedTracks = useMemo(() => {
    const arr = [...filteredTracks];
    const dir = trackSort.dir === 'asc' ? 1 : -1;
    arr.sort((a, b) => {
      const av: any = (a as any)[trackSort.key] ?? '';
      const bv: any = (b as any)[trackSort.key] ?? '';
      if (typeof av === 'number' && typeof bv === 'number') return (av - bv) * dir;
      return (av.toString().toLowerCase() < bv.toString().toLowerCase() ? -1 : av.toString().toLowerCase() > bv.toString().toLowerCase() ? 1 : 0) * dir;
    });
    return arr;
  }, [filteredTracks, trackSort]);

  const sortedAlbums = useMemo(() => {
    const arr = [...albums];
    const dir = albumSort.dir === 'asc' ? 1 : -1;
    arr.sort((a, b) => {
      const av: any = (a as any)[albumSort.key] ?? '';
      const bv: any = (b as any)[albumSort.key] ?? '';
      if (typeof av === 'number' && typeof bv === 'number') return (av - bv) * dir;
      return (av.toString().toLowerCase() < bv.toString().toLowerCase() ? -1 : av.toString().toLowerCase() > bv.toString().toLowerCase() ? 1 : 0) * dir;
    });
    return arr;
  }, [albums, albumSort]);

  const toggleTrack = (trackId: number) => {
    setSelectedTracks((prev) => {
      const next = new Set(prev);
      if (next.has(trackId)) next.delete(trackId);
      else next.add(trackId);
      return next;
    });
  };

  const toggleAllVisible = () => {
    if (selectedTracks.size === sortedTracks.length) {
      setSelectedTracks(new Set());
    } else {
      setSelectedTracks(new Set(sortedTracks.map((t) => t.lidarrId)));
    }
  };

  if (loading) {
    return <p className="text-gray-400">Loading artist...</p>;
  }
  if (error) {
    return (
      <div className="space-y-4">
        <div className="rounded-lg bg-red-900/30 border border-red-700 px-4 py-3 text-red-400">
          {error}
        </div>
        <Link href="/artists" className="btn-secondary">
          ← Back to Artists
        </Link>
      </div>
    );
  }
  if (!artist) {
    return (
      <div className="space-y-4">
        <p className="text-gray-400">Artist not found.</p>
        <Link href="/artists" className="btn-secondary">
          ← Back to Artists
        </Link>
      </div>
    );
  }

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
          <span>{artist.name}</span>
          <span className="mx-2">›</span>
          <span>Manage</span>
        </div>
        <h1 className="text-2xl font-bold text-white">🎛️ {artist.name}</h1>
        <p className="text-xs text-gray-500 mt-1">
          Lidarr ID: {artist.lidarrId ?? '—'} · Folder: {artist.rootFolder ?? '—'} · {albums.length} albums · {tracks.length} tracks
        </p>
      </div>

      {actionMsg && (
        <div className="rounded-lg bg-emerald-900/30 border border-emerald-700 px-4 py-3 text-emerald-400">
          {actionMsg}
        </div>
      )}

      {/* State summary */}
      <div className="card">
        <h2 className="text-lg font-semibold mb-3">📊 Track State Summary</h2>
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
            <div className="text-gray-400 text-xs">Auto (unset)</div>
            <div className="text-2xl font-bold text-gray-400">{stateCounts.unset}</div>
          </div>
        </div>
      </div>

      {/* Albums list */}
      <div className="card">
        <h2 className="text-lg font-semibold mb-3">💿 Albums ({albums.length})</h2>
        {albums.length === 0 ? (
          <p className="text-gray-500 text-sm">No albums found in Lidarr yet.</p>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="text-left text-gray-400 border-b border-gray-700">
                  <th className="pb-2 font-medium cursor-pointer select-none hover:text-emerald-400" onClick={() => toggleAlbumSort('title')}>Album<SortIcon active={albumSort.key==='title'} dir={albumSort.dir} /></th>
                  <th className="pb-2 font-medium cursor-pointer select-none hover:text-emerald-400" onClick={() => toggleAlbumSort('year')}>Year<SortIcon active={albumSort.key==='year'} dir={albumSort.dir} /></th>
                  <th className="pb-2 font-medium cursor-pointer select-none hover:text-emerald-400" onClick={() => toggleAlbumSort('trackCount')}>Tracks<SortIcon active={albumSort.key==='trackCount'} dir={albumSort.dir} /></th>
                  <th className="pb-2 font-medium cursor-pointer select-none hover:text-emerald-400" onClick={() => toggleAlbumSort('monitored')}>Monitored<SortIcon active={albumSort.key==='monitored'} dir={albumSort.dir} /></th>
                  <th className="pb-2 font-medium text-right">Action</th>
                </tr>
              </thead>
              <tbody>
                {sortedAlbums.map((a) => (
                  <tr key={a.lidarrId} className="border-b border-gray-700/50 last:border-0 hover:bg-white/5">
                    <td className="py-2 text-white">{a.title}</td>
                    <td className="py-2 text-gray-400">{a.year || '—'}</td>
                    <td className="py-2 text-gray-400">{a.trackCount}</td>
                    <td className="py-2">
                      <span className={a.monitored ? 'text-emerald-400' : 'text-gray-500'}>
                        {a.monitored ? '✓' : '—'}
                      </span>
                    </td>
                    <td className="py-2 text-right">
                      <Link
                        href={`/artists/${artistId}/album/${a.lidarrId}/manage`}
                        className="text-xs text-blue-400 hover:text-blue-300"
                      >
                        🎛️ Manage Tracks
                      </Link>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>

      {/* Track-level controls */}
      <div className="card">
        <div className="flex justify-between items-center mb-3 flex-wrap gap-2">
          <h2 className="text-lg font-semibold">🎵 Tracks ({tracks.length})</h2>
          <div className="flex gap-2 items-center text-sm">
            <input
              type="text"
              placeholder="Search track or album..."
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              className="bg-gray-800 border border-gray-600 rounded px-3 py-1.5 text-white placeholder-gray-500 text-sm"
            />
            <select
              value={stateFilter}
              onChange={(e) => setStateFilter(e.target.value as any)}
              className="bg-gray-800 border border-gray-600 rounded px-3 py-1.5 text-white text-sm"
            >
              <option value="">All states</option>
              <option value="unset">Unset (auto)</option>
              <option value="keep">Keep</option>
              <option value="hit">Hit</option>
              <option value="not_keep">Not Keep</option>
            </select>
            <button
              onClick={() => setBulkMode(!bulkMode)}
              className={bulkMode ? 'btn-primary text-sm' : 'btn-secondary text-sm'}
            >
              {bulkMode ? '✓ Bulk Mode' : 'Bulk Edit'}
            </button>
          </div>
        </div>

        {bulkMode && selectedTracks.size > 0 && (
          <div className="rounded-lg bg-blue-900/30 border border-blue-700 px-4 py-2 mb-3 flex items-center gap-3 text-sm">
            <span className="text-blue-200">{selectedTracks.size} selected:</span>
            <button onClick={() => bulkSetState('keep')} className="px-2 py-1 text-xs rounded bg-green-700 hover:bg-green-600 text-white">Keep</button>
            <button onClick={() => bulkSetState('hit')} className="px-2 py-1 text-xs rounded bg-gray-600 hover:bg-gray-500 text-white">Hit</button>
            <button onClick={() => bulkSetState('not_keep')} className="px-2 py-1 text-xs rounded bg-red-700 hover:bg-red-600 text-white">Not Keep</button>
            <button onClick={() => bulkSetState('')} className="px-2 py-1 text-xs rounded bg-gray-700 hover:bg-gray-600 text-white">Reset (Auto)</button>
            <button onClick={() => setSelectedTracks(new Set())} className="ml-auto text-xs text-gray-400 hover:text-gray-200">Clear</button>
          </div>
        )}

        {sortedTracks.length === 0 ? (
          <p className="text-gray-500 text-sm">No tracks match your filters.</p>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="text-left text-gray-400 border-b border-gray-700">
                  {bulkMode && (
                    <th className="pb-2 font-medium w-8">
                      <input
                        type="checkbox"
                        checked={selectedTracks.size === sortedTracks.length && sortedTracks.length > 0}
                        onChange={toggleAllVisible}
                        className="rounded"
                      />
                    </th>
                  )}
                  <th className="pb-2 font-medium cursor-pointer select-none hover:text-emerald-400" onClick={() => toggleTrackSort('trackNumber')}>#<SortIcon active={trackSort.key==='trackNumber'} dir={trackSort.dir} /></th>
                  <th className="pb-2 font-medium cursor-pointer select-none hover:text-emerald-400" onClick={() => toggleTrackSort('title')}>Track<SortIcon active={trackSort.key==='title'} dir={trackSort.dir} /></th>
                  <th className="pb-2 font-medium cursor-pointer select-none hover:text-emerald-400" onClick={() => toggleTrackSort('albumTitle')}>Album<SortIcon active={trackSort.key==='albumTitle'} dir={trackSort.dir} /></th>
                  <th className="pb-2 font-medium cursor-pointer select-none hover:text-emerald-400" onClick={() => toggleTrackSort('score')}>Score<SortIcon active={trackSort.key==='score'} dir={trackSort.dir} /></th>
                  <th className="pb-2 font-medium cursor-pointer select-none hover:text-emerald-400" onClick={() => toggleTrackSort('downloaded')}>Downloaded<SortIcon active={trackSort.key==='downloaded'} dir={trackSort.dir} /></th>
                  <th className="pb-2 font-medium cursor-pointer select-none hover:text-emerald-400" onClick={() => toggleTrackSort('state')}>State<SortIcon active={trackSort.key==='state'} dir={trackSort.dir} /></th>
                </tr>
              </thead>
              <tbody>
                {sortedTracks.map((t) => (
                  <tr key={t.lidarrId} className="border-b border-gray-700/50 last:border-0 hover:bg-white/5">
                    {bulkMode && (
                      <td className="py-2">
                        <input
                          type="checkbox"
                          checked={selectedTracks.has(t.lidarrId)}
                          onChange={() => toggleTrack(t.lidarrId)}
                          className="rounded"
                        />
                      </td>
                    )}
                    <td className="py-2 text-gray-500 w-10">{t.trackNumber}</td>
                    <td className="py-2 text-white">{t.title}</td>
                    <td className="py-2 text-gray-400">
                      <Link
                        href={`/artists/${artistId}/album/${t.albumLidarrId}/manage`}
                        className="hover:text-blue-300"
                      >
                        {t.albumTitle}
                      </Link>
                    </td>
                    <td className="py-2 text-gray-400">
                      {t.score != null ? (
                        <span
                          className={
                            t.score >= 70 ? 'text-green-400' : t.score >= 50 ? 'text-yellow-400' : 'text-gray-500'
                          }
                        >
                          {t.score}
                        </span>
                      ) : (
                        '—'
                      )}
                    </td>
                    <td className="py-2">
                      <span className={t.downloaded ? 'text-emerald-400' : 'text-gray-600'}>
                        {t.downloaded ? '⬇️' : '⬜'}
                      </span>
                    </td>
                    <td className="py-2">
                      <div className="flex gap-1">
                        <button
                          onClick={() => setTrackState(t, 'keep')}
                          className={`px-2 py-0.5 text-xs rounded border ${
                            t.state === 'keep'
                              ? 'bg-green-600 text-white border-green-500'
                              : 'bg-green-900/20 text-green-400 border-green-700/50 hover:bg-green-900/40'
                          }`}
                        >
                          Keep
                        </button>
                        <button
                          onClick={() => setTrackState(t, 'hit')}
                          className={`px-2 py-0.5 text-xs rounded border ${
                            t.state === 'hit'
                              ? 'bg-gray-500 text-white border-gray-400'
                              : 'bg-gray-800/40 text-gray-300 border-gray-600/50 hover:bg-gray-700/50'
                          }`}
                        >
                          Hit
                        </button>
                        <button
                          onClick={() => setTrackState(t, 'not_keep')}
                          className={`px-2 py-0.5 text-xs rounded border ${
                            t.state === 'not_keep'
                              ? 'bg-red-600 text-white border-red-500'
                              : 'bg-red-900/20 text-red-400 border-red-700/50 hover:bg-red-900/40'
                          }`}
                        >
                          Not Keep
                        </button>
                        {t.state !== '' && (
                          <button
                            onClick={() => setTrackState(t, '')}
                            className="px-2 py-0.5 text-xs rounded text-gray-500 hover:text-gray-300"
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

        <p className="text-xs text-gray-500 mt-3">
          💡 <strong>Keep</strong>: always downloaded. <strong>Hit</strong>: stays if score ≥ threshold, else surfaces in Hit-Fallen log. <strong>Not Keep</strong>: unmonitored. <strong>Auto</strong>: scan decides based on popularity threshold.
        </p>
      </div>
    </div>
  );
}
