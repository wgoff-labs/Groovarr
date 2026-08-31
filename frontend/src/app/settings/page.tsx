'use client';

import { useEffect, useState, useCallback } from 'react';
import Link from 'next/link';
import { api, Folder, Profile, ConnectionStatus } from '@/lib/api';

// Settings keys
const KEYS = {
  // Lidarr
  lidarr_url: 'lidarr_url',
  lidarr_api_key: 'lidarr_api_key',
  lidarr_quality_profile: 'lidarr_quality_profile',
  lidarr_default_root_folder: 'lidarr_default_root_folder',
  // Last.fm
  lastfm_api_key: 'lastfm_api_key',
  // General
  popularity_threshold: 'popularity_threshold',
  download_mode: 'download_mode',
  // Schedule
  daily_check_cron: 'daily_check_cron',
  timezone: 'timezone',
  // Discord
  discord_token: 'discord_token',
  discord_home_channel: 'discord_home_channel',
  discord_allow_users: 'discord_allow_users',
  discord_auto_thread: 'discord_auto_thread',
  discord_allowed_channels: 'discord_allowed_channels',
  discord_allowed_users: 'discord_allowed_users',
} as const;

const TIMEZONES = [
  'America/Detroit', 'America/New_York', 'America/Chicago', 'America/Denver', 'America/Los_Angeles',
  'America/Phoenix', 'America/Anchorage', 'America/Honolulu',
  'Europe/London', 'Europe/Paris', 'Europe/Berlin', 'Europe/Madrid', 'Europe/Rome',
  'Asia/Tokyo', 'Asia/Shanghai', 'Asia/Singapore', 'Asia/Dubai',
  'Australia/Sydney', 'Pacific/Auckland', 'UTC',
];

// Helpers
const connLabel = (svc: ConnectionStatus) => {
  if (svc.status === 'connected') return '✅ Connected';
  if (svc.status === 'connecting') return '⏳ Connecting...';
  if (svc.status === 'error') return `❌ ${svc.error || 'Error'}`;
  return '⭕ Disconnected';
};
const connColor = (svc: ConnectionStatus) => {
  if (svc.status === 'connected') return 'text-emerald-400';
  if (svc.status === 'connecting') return 'text-yellow-400';
  if (svc.status === 'error') return 'text-red-400';
  return 'text-gray-500';
};
const connBtn = (
  svc: ConnectionStatus,
  acting: boolean,
  service: string,
  onAction: (svc: string, action: 'connect' | 'disconnect') => void
) => {
  const isConnected = svc.status === 'connected';
  const isActing = acting || svc.status === 'connecting';
  if (isConnected) {
    return (
      <button
        onClick={() => onAction(service, 'disconnect')}
        disabled={isActing}
        className="px-3 py-1.5 rounded text-sm font-medium bg-red-900/50 border border-red-700 text-red-300 hover:bg-red-800/50 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
      >
        {isActing ? '…' : 'Disconnect'}
      </button>
    );
  }
  return (
    <button
      onClick={() => onAction(service, 'connect')}
      disabled={isActing}
      className="px-3 py-1.5 rounded text-sm font-medium bg-emerald-700 border border-emerald-600 text-emerald-100 hover:bg-emerald-600 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
    >
      {isActing ? '…' : 'Connect'}
    </button>
  );
};

export default function SettingsPage() {
  const [settings, setSettings] = useState<Record<string, string>>({});
  const [saving, setSaving] = useState<string | null>(null);
  const [saved, setSaved] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [folders, setFolders] = useState<Folder[]>([]);
  const [profiles, setProfiles] = useState<Profile[]>([]);
  const [connStatus, setConnStatus] = useState<ConnectionStatus[]>([]);
  const [connActing, setConnActing] = useState<string | null>(null);
  const [connError, setConnError] = useState<string | null>(null);
  const [showSecrets, setShowSecrets] = useState<Record<string, boolean>>({});

  const lidarrStatus = connStatus.find(s => s.service === 'lidarr');
  const discordStatus = connStatus.find(s => s.service === 'discord');
  const lastfmStatus = connStatus.find(s => s.service === 'lastfm');

  const loadAll = useCallback(async () => {
    try {
      const entries = await Promise.all(
        Object.values(KEYS).map(async (key) => {
          try {
            const result = await api.settings.get(key);
            return [key, result.value || ''] as const;
          } catch {
            return [key, ''] as const;
          }
        })
      );
      const map: Record<string, string> = {};
      for (const [k, v] of entries) map[k] = v;
      setSettings(map);
      setError(null);
    } catch (e: any) {
      setError(`Failed to load settings: ${e.message}`);
    }
  }, []);

  const loadFolders = useCallback(async () => {
    try {
      const folderList = await api.folders.list();
      setFolders(Array.isArray(folderList) ? folderList : []);
    } catch {
      setFolders([]);
    }
  }, []);

  const loadProfiles = useCallback(async () => {
    try {
      const profileList = await api.profiles.list();
      setProfiles(Array.isArray(profileList) ? profileList : []);
    } catch {
      setProfiles([]);
    }
  }, []);

  const loadConnections = useCallback(async () => {
    try {
      const data = await api.connections.status();
      setConnStatus(data.statuses || []);
      setConnError(null);
    } catch (e: any) {
      setConnError(e.message);
    }
  }, []);

  const handleConnection = async (service: string, action: 'connect' | 'disconnect') => {
    setConnActing(service);
    try {
      const fn = action === 'connect' ? api.connections.connect : api.connections.disconnect;
      const data = await fn(service);
      setConnStatus(data.statuses || []);
      setConnError(null);
    } catch (e: any) {
      setConnError(e.message);
    }
    setConnActing(null);
  };

  const reloadLidarr = useCallback(() => {
    loadFolders();
    loadProfiles();
  }, [loadFolders, loadProfiles]);

  useEffect(() => {
    loadAll();
    loadConnections();
    loadFolders();
    loadProfiles();
  }, [loadAll, loadConnections, loadFolders, loadProfiles]);

  const save = async (key: string, value: string) => {
    setSaving(key);
    try {
      await api.settings.set(key, value);
      setSettings((s) => ({ ...s, [key]: value }));
      setSaved(`Saved`);
      setTimeout(() => setSaved(null), 2000);
      setError(null);
      if (key === 'lidarr_url' || key === 'lidarr_api_key') {
        setTimeout(loadConnections, 800);
        setTimeout(reloadLidarr, 1200);
      }
    } catch (e: any) {
      setError(`Save failed: ${e.message}`);
    }
    setSaving(null);
  };

  const updateField = (key: string, value: string) => {
    setSettings((s) => ({ ...s, [key]: value }));
  };

  const toggleSecret = (key: string) => {
    setShowSecrets((s) => ({ ...s, [key]: !s[key] }));
  };

  const TextInput = ({ keyName, placeholder, isSecret = false }: { keyName: string; placeholder?: string; isSecret?: boolean }) => {
    const visible = !isSecret || showSecrets[keyName];
    return (
      <div className="flex gap-2">
        <input
          type={isSecret ? (visible ? 'text' : 'password') : 'text'}
          value={settings[keyName] || ''}
          onChange={(e) => updateField(keyName, e.target.value)}
          onBlur={() => save(keyName, settings[keyName] || '')}
          placeholder={placeholder}
          className="flex-1 bg-gray-800 border border-gray-600 rounded px-3 py-2 text-sm text-white placeholder-gray-500 focus:outline-none focus:border-emerald-500"
        />
        {isSecret && (
          <button
            type="button"
            onClick={() => toggleSecret(keyName)}
            className="px-2 bg-gray-700 border border-gray-600 rounded text-gray-300 hover:bg-gray-600 text-sm"
            title={visible ? 'Hide' : 'Show'}
          >
            {visible ? '🙈' : '👁️'}
          </button>
        )}
        <button
          onClick={() => save(keyName, settings[keyName] || '')}
          disabled={saving === keyName}
          className="px-3 py-2 bg-emerald-600 hover:bg-emerald-500 disabled:bg-gray-600 disabled:cursor-not-allowed rounded text-white text-sm font-medium"
        >
          {saving === keyName ? '…' : 'Save'}
        </button>
      </div>
    );
  };

  const SelectInput = ({ keyName, options }: { keyName: string; options: string[] }) => (
    <div className="flex gap-2">
      <select
        value={settings[keyName] || ''}
        onChange={(e) => { updateField(keyName, e.target.value); save(keyName, e.target.value); }}
        className="flex-1 bg-gray-800 border border-gray-600 rounded px-3 py-2 text-sm text-white focus:outline-none focus:border-emerald-500"
      >
        <option value="">— select —</option>
        {options.map((o) => (
          <option key={o} value={o}>{o}</option>
        ))}
      </select>
    </div>
  );

  const ToggleInput = ({ keyName, label }: { keyName: string; label: string }) => (
    <button
      type="button"
      onClick={() => {
        const newVal = settings[keyName] === 'true' ? 'false' : 'true';
        updateField(keyName, newVal);
        save(keyName, newVal);
      }}
      className={`relative w-11 h-6 rounded-full transition-colors ${settings[keyName] === 'true' ? 'bg-emerald-500' : 'bg-gray-600'}`}
    >
      <span className={`absolute top-0.5 left-0.5 w-5 h-5 bg-white rounded-full transition-transform ${settings[keyName] === 'true' ? 'translate-x-5' : ''}`} />
    </button>
  );

  const RangeInput = ({ keyName, min, max, step }: { keyName: string; min: number; max: number; step: number }) => (
    <div className="flex items-center gap-3">
      <input
        type="range" min={min} max={max} step={step}
        value={settings[keyName] || '60'}
        onChange={(e) => updateField(keyName, e.target.value)}
        onMouseUp={() => save(keyName, settings[keyName] || '60')}
        onTouchEnd={() => save(keyName, settings[keyName] || '60')}
        className="flex-1 accent-emerald-500"
      />
      <span className="text-xl font-bold text-white w-10 text-center tabular-nums">{settings[keyName] || '60'}</span>
    </div>
  );

  return (
    <div className="space-y-6 max-w-4xl">
      {error && (
        <div className="rounded-lg bg-red-900/30 border border-red-700 px-4 py-3 text-red-400 text-sm">{error}</div>
      )}
      {saved && (
        <div className="rounded-lg bg-emerald-900/30 border border-emerald-700 px-4 py-3 text-emerald-400 text-sm">✅ {saved}</div>
      )}

      {/* ── Two-column top row ── */}
      <div className="grid grid-cols-2 gap-4">
        {/* Schedule */}
        <div className="card space-y-3">
          <h2 className="text-base font-semibold flex items-center gap-2">⏰ Schedule</h2>
          <div>
            <p className="text-xs font-medium text-gray-300 mb-1">Cron Expression</p>
            <TextInput keyName={KEYS.daily_check_cron} placeholder="0 9 * * *" />
            <p className="text-xs text-gray-500 mt-1">e.g. "0 9 * * *" = 9 AM daily</p>
          </div>
          <div>
            <p className="text-xs font-medium text-gray-300 mb-1">Timezone</p>
            <SelectInput keyName={KEYS.timezone} options={TIMEZONES} />
          </div>
        </div>

        {/* General */}
        <div className="card space-y-3">
          <h2 className="text-base font-semibold flex items-center gap-2">🎯 General</h2>
          <div>
            <p className="text-xs font-medium text-gray-300 mb-1">Popularity Threshold</p>
            <RangeInput keyName={KEYS.popularity_threshold} min={10} max={100} step={5} />
            <p className="text-xs text-gray-500 mt-1">Higher = stricter. Tracks below this score get pruned.</p>
          </div>
          <div>
            <p className="text-xs font-medium text-gray-300 mb-2">Download Mode</p>
            <div className="grid grid-cols-2 gap-2">
              {[
                { value: 'tracks', emoji: '🎶', title: 'Tracks Only', desc: 'Only popular tracks kept' },
                { value: 'album', emoji: '💿', title: 'Full Album', desc: 'Keep full albums' },
              ].map(({ value, emoji, title, desc }) => (
                <button
                  key={value}
                  onClick={() => save(KEYS.download_mode, value)}
                  className={`card text-left transition-all p-2 ${settings[KEYS.download_mode] === value ? 'border-emerald-500 bg-emerald-900/20' : 'hover:border-gray-500'}`}
                >
                  <p className="text-sm font-semibold text-white">{emoji} {title}</p>
                  <p className="text-xs text-gray-400">{desc}</p>
                </button>
              ))}
            </div>
          </div>
        </div>
      </div>

      {/* ── Connections (combined) ── */}
      <div className="card space-y-3">
        <div className="grid grid-cols-2">
          <div className="space-y-2">
            <p className="text-sm font-medium text-white">🎵 Lidarr</p>
            <p className="text-sm font-medium text-white">💬 Discord</p>
            <p className="text-sm font-medium text-white">📊 Last.fm</p>
          </div>
          <div className="space-y-2 text-right">
            <div className="flex items-center justify-end gap-3">
              {lidarrStatus && (
                <>
                  <p className={`text-xs ${connColor(lidarrStatus)}`}>{connLabel(lidarrStatus)}</p>
                  {connBtn(lidarrStatus, connActing === 'lidarr', 'lidarr', handleConnection)}
                </>
              )}
            </div>
            <div className="flex items-center justify-end gap-3">
              {discordStatus && (
                <>
                  <p className={`text-xs ${connColor(discordStatus)}`}>{connLabel(discordStatus)}</p>
                  {connBtn(discordStatus, connActing === 'discord', 'discord', handleConnection)}
                </>
              )}
            </div>
            <div className="flex items-center justify-end gap-3">
              {lastfmStatus && (
                <>
                  <p className={`text-xs ${connColor(lastfmStatus)}`}>{connLabel(lastfmStatus)}</p>
                  {connBtn(lastfmStatus, connActing === 'lastfm', 'lastfm', handleConnection)}
                </>
              )}
            </div>
          </div>
        </div>
        <div className="flex items-center justify-end">
          <Link href="/logs" className="text-xs text-gray-400 hover:text-emerald-400 transition-colors">📋 Connection Logs →</Link>
        </div>
      </div>

      {/* ── Lidarr Config ── */}
      <div className="card space-y-3">
        <h2 className="text-base font-semibold flex items-center gap-2">🎵 Lidarr</h2>
        <p className="text-xs text-gray-400">Connect to your Lidarr instance to add artists and track downloads.</p>
        <div className="grid grid-cols-2 gap-3">
          <div>
            <p className="text-xs font-medium text-gray-300 mb-1">URL</p>
            <TextInput keyName={KEYS.lidarr_url} placeholder="http://10.0.0.244:8686" />
          </div>
          <div>
            <p className="text-xs font-medium text-gray-300 mb-1">API Key</p>
            <TextInput keyName={KEYS.lidarr_api_key} placeholder="your-lidarr-api-key" isSecret />
          </div>
          <div>
            <p className="text-xs font-medium text-gray-300 mb-1">Quality Profile</p>
            {profiles.length > 0
              ? <SelectInput keyName={KEYS.lidarr_quality_profile} options={profiles.map((p) => p.name)} />
              : <p className="text-sm text-gray-500 italic">Connect to Lidarr first</p>
            }
          </div>
          <div>
            <p className="text-xs font-medium text-gray-300 mb-1">Default Root Folder</p>
            {folders.length > 0
              ? <SelectInput keyName={KEYS.lidarr_default_root_folder} options={folders.map((f) => f.path)} />
              : <p className="text-sm text-gray-500 italic">Connect to Lidarr first</p>
            }
          </div>
        </div>
      </div>

      {/* ── Discord Bot Config ── */}
      <div className="card space-y-3">
        <h2 className="text-base font-semibold flex items-center gap-2">💬 Discord Bot</h2>
        <p className="text-xs text-gray-400">Enable a Discord bot for commands and daily reports.</p>
        <div className="grid grid-cols-2 gap-3">
          <div>
            <p className="text-xs font-medium text-gray-300 mb-1">Bot Token</p>
            <TextInput keyName={KEYS.discord_token} placeholder="your-discord-bot-token" isSecret />
          </div>
          <div>
            <p className="text-xs font-medium text-gray-300 mb-1">Home Channel ID</p>
            <TextInput keyName={KEYS.discord_home_channel} placeholder="123456789012345678" />
            <p className="text-xs text-gray-500 mt-1">Daily reports posted here</p>
          </div>
          <div>
            <p className="text-xs font-medium text-gray-300 mb-1">Allowed Channel IDs</p>
            <TextInput keyName={KEYS.discord_allowed_channels} placeholder="123, 456, 789" />
            <p className="text-xs text-gray-500 mt-1">Comma-separated. Empty = all channels</p>
          </div>
          <div>
            <p className="text-xs font-medium text-gray-300 mb-1">Allowed User IDs</p>
            <TextInput keyName={KEYS.discord_allowed_users} placeholder="123, 456, 789" />
            <p className="text-xs text-gray-500 mt-1">Comma-separated. Empty = all users</p>
          </div>
        </div>
        <div className="grid grid-cols-2 gap-3">
          <div className="flex items-center justify-between bg-gray-800/50 border border-gray-700 rounded px-3 py-2">
            <p className="text-sm font-medium text-white">Allow All Users</p>
            <ToggleInput keyName={KEYS.discord_allow_users} label="Allow All Users" />
          </div>
          <div className="flex items-center justify-between bg-gray-800/50 border border-gray-700 rounded px-3 py-2">
            <p className="text-sm font-medium text-white">Auto-Thread</p>
            <ToggleInput keyName={KEYS.discord_auto_thread} label="Auto-Thread" />
          </div>
        </div>
      </div>

      {/* ── Last.fm Config ── */}
      <div className="card space-y-3">
        <h2 className="text-base font-semibold flex items-center gap-2">📊 Last.fm</h2>
        <p className="text-xs text-gray-400">Get popularity scores from Last.fm for better music recommendations.</p>
        <div className="max-w-sm">
          <p className="text-xs font-medium text-gray-300 mb-1">API Key</p>
          <TextInput keyName={KEYS.lastfm_api_key} placeholder="your-lastfm-api-key" isSecret />
          <p className="text-xs text-gray-500 mt-1">
            Get one at <a href="https://www.last.fm/api/account/create" target="_blank" rel="noopener" className="text-emerald-400 hover:underline">last.fm/api/account/create</a>
          </p>
        </div>
      </div>

      {/* About */}
      <div className="card text-xs text-gray-500 space-y-1">
        <p>Groovarr connects Lidarr with Deezer &amp; Last.fm to automatically filter your music library to only the popular releases.</p>
        <p>Settings stored in SQLite · <code className="text-gray-400">/data/groovarr.db</code></p>
        <VersionBadge />
      </div>
    </div>
  );
}

function VersionBadge() {
  const [info, setInfo] = useState<{ version: string; commit: string; build: string } | null>(null);
  useEffect(() => {
    api.version()
      .then(setInfo)
      .catch(() => setInfo(null));
  }, []);
  if (!info) return <p className="text-gray-600">Version: unknown</p>;
  return (
    <p className="text-gray-600">
      Version: <span className="text-gray-400">build #{info.build}</span>{' '}
      <span className="text-gray-500">({info.commit})</span>
    </p>
  );
}
