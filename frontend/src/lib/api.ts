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

// In production, frontend and API are served from the same origin (single binary)
// In development, this points to the standalone backend
const BASE = typeof window !== 'undefined' 
  ? ''  // Browser: use same origin (relative URLs)
  : (process.env.NEXT_PUBLIC_API_URL ?? 'http://localhost:8080');

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
};
