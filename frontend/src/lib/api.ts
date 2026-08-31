export interface Artist {
  id: number;
  name: string;
  deezer_id: string | null;
  lidarr_id: number | null;
  root_folder: string | null;
  added_by: string;
  added_at: string;
}

export interface Folder {
  id: number;
  path: string;
}

export interface Profile {
  id: number;
  name: string;
}

export interface ConnectionStatus {
  service: string;
  status: 'disconnected' | 'connecting' | 'connected' | 'error';
  error?: string;
  last_check: string;
}

export interface LogEntry {
  timestamp: string;
  service: string;
  level: 'info' | 'error' | 'warn';
  message: string;
}

export interface CheckResult {
  artist_name: string;
  new_albums_found: number;
  albums_added: number;
  tracks_added: number;
  tracks_skipped: number;
  albums_skipped: number;
  errors: string[];
  added_albums: string[];
  skipped_albums: string[];
  hits_kept?: number;
  hits_fallen?: number;
  tracks_pruned?: number;
}

export interface PruneResult {
  artist_name: string;
  album_name: string;
  total_tracks: number;
  kept_tracks: number;
  pruned_tracks: number;
  already_pruned: boolean;
  error?: string;
}

export interface LidarrImportArtist {
  lidarrId: number;
  name: string;
  sortName: string;
  rootFolder: string;
  qualityProfileId: number;
  qualityProfile: string;
  metadataProfileId: number;
  metadataProfile: string;
  monitor: string; // "none" | "albums" | "all"
  alreadyInGroovarr: boolean;
  genres: string;
}

export interface ImportListResponse {
  artists: LidarrImportArtist[];
  total: number;
  page: number;
  limit: number;
  totalPages: number;
}

export interface BulkImportRequest {
  artistIds: number[];
  rootFolder?: string;
  qualityProfileId?: number;
  monitor?: string;
}

export interface BulkImportResponse {
  imported: number;
  skipped: number;
  errors?: string[];
}

// ── Artist Management Types ──────────────────────────────────────────────────────

export type TrackState = 'keep' | 'hit' | 'not_keep' | '';

export interface ManagedAlbum {
  lidarrId: number;
  title: string;
  year: number;
  trackCount: number;
  monitored: boolean;
  albumType?: string;
}

export interface ManagedTrack {
  lidarrId: number;
  title: string;
  albumTitle: string;
  albumLidarrId: number;
  trackNumber: number;
  discNumber: number;
  duration: number; // seconds
  score: number | null; // popularity score 0-100
  state: TrackState;
  downloaded: boolean; // hasFile
}

export interface ManageArtistResponse {
  artist: {
    id: number;
    name: string;
    lidarrId: number | null;
    rootFolder: string | null;
  };
  albums: ManagedAlbum[];
  tracks: ManagedTrack[];
}

export interface HitFallenEntry {
  id: number;
  artistId: number;
  artistName: string;
  trackId: number;
  trackTitle: string;
  albumTitle: string;
  scoreAtFall: number;
  fallenAt: string;
}

// ── API Client ─────────────────────────────────────────────────────────────────

const BASE = typeof window !== 'undefined'
  ? ''  // Browser: use same origin (relative URLs)
  : (process.env.NEXT_PUBLIC_API_URL ?? 'http://10.0.0.203:8080');

async function fetchJSON<T>(path: string, opts?: RequestInit): Promise<T> {
  const res = await fetch(`${BASE}${path}`, {
    ...opts,
    headers: {
      'Content-Type': 'application/json',
      ...opts?.headers,
    },
  });
  if (!res.ok) {
    throw new Error(`API error ${res.status}: ${await res.text()}`);
  }
  return res.json() as Promise<T>;
}

export const api = {
  status: () => fetchJSON<{ status: string; service: string }>('/api/status'),

  artists: {
    list: () => fetchJSON<Artist[]>('/api/artists'),
    add: (name: string, rootFolder?: string) =>
      fetchJSON<Artist>('/api/artists', {
        method: 'POST',
        body: JSON.stringify({ name, root_folder: rootFolder }),
      }),
    remove: (name: string) =>
      fetchJSON<void>('/api/artists', { method: 'DELETE' }),
    importList: (page: number = 1, limit: number = 20) =>
      fetchJSON<ImportListResponse>(`/api/artists/import?page=${page}&limit=${limit}`),
    importBulk: (req: BulkImportRequest) =>
      fetchJSON<BulkImportResponse>('/api/artists/import/bulk', {
        method: 'POST',
        body: JSON.stringify(req),
      }),
    manage: (artistId: number) =>
      fetchJSON<ManageArtistResponse>(`/api/artist/${artistId}/manage`),
  },

  tracks: {
    setState: (artistId: number, lidarrTrackId: number, state: TrackState) =>
      fetchJSON<{ ok: boolean }>(`/api/artist/${artistId}/track/${lidarrTrackId}/state`, {
        method: 'POST',
        body: JSON.stringify({ state }),
      }),
  },

  hitfallen: {
    list: (limit = 50) =>
      fetchJSON<{ entries: HitFallenEntry[] }>(`/api/hit-fallen?limit=${limit}`),
  },

  check: (artist?: string) =>
    fetchJSON<CheckResult[]>(`/api/check${artist ? `?artist=${encodeURIComponent(artist)}` : ''}`),

  scan: (artist: string) =>
    fetchJSON<CheckResult[]>(`/api/scan?artist=${encodeURIComponent(artist)}`),

  prune: (artist?: string, force = false) =>
    fetchJSON<PruneResult[]>(`/api/prune${artist ? `?artist=${encodeURIComponent(artist)}` : ''}${force ? '&force=true' : ''}`),

  settings: {
    get: (key: string) =>
      fetchJSON<{ key: string; value: string }>(`/api/settings?key=${encodeURIComponent(key)}`),
    set: (key: string, value: string) =>
      fetchJSON<void>('/api/settings', {
        method: 'POST',
        body: JSON.stringify({ key, value }),
      }),
  },

  keep: {
    list: (artist: string, album: string) =>
      fetchJSON<{ artist: string; album: string; tracks: string[] }>(
        `/api/keep?artist=${encodeURIComponent(artist)}&album=${encodeURIComponent(album)}`
      ),
    add: (artist: string, album: string, track: string) =>
      fetchJSON<void>('/api/keep', {
        method: 'POST',
        body: JSON.stringify({ artist, album, track }),
      }),
    remove: (artist: string, album: string, track: string) =>
      fetchJSON<void>('/api/keep', {
        method: 'DELETE',
        body: JSON.stringify({ artist, album, track }),
      }),
  },

  downloads: () => fetchJSON<PruneResult[]>('/api/downloads'),

  folders: {
    list: () => fetchJSON<Folder[]>('/api/folders'),
  },

  profiles: {
    list: () => fetchJSON<Profile[]>('/api/profiles'),
  },

  connections: {
    status: () => fetchJSON<{ statuses: ConnectionStatus[] }>('/api/connections'),
    connect: (service: string) =>
      fetchJSON<{ statuses: ConnectionStatus[] }>('/api/connections', {
        method: 'POST',
        body: JSON.stringify({ service, action: 'connect' }),
      }),
    disconnect: (service: string) =>
      fetchJSON<{ statuses: ConnectionStatus[] }>('/api/connections', {
        method: 'POST',
        body: JSON.stringify({ service, action: 'disconnect' }),
      }),
    logs: () => fetchJSON<{ logs: LogEntry[] }>('/api/connections/logs'),
    clearLogs: () => fetchJSON('/api/connections/logs', { method: 'DELETE' }),
  },

  version: () => fetchJSON<{ version: string; commit: string; build: string }>('/api/version'),
};
