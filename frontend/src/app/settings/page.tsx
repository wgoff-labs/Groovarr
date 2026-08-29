'use client';

import { useEffect, useState, useCallback } from 'react';
import { api, Folder } from '@/lib/api';

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
  discord_allow_all_users: 'discord_allow_users',
  discord_auto_thread: 'discord_auto_thread',
  discord_require_mention: 'discord_require_mention',
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

const QUALITY_PROFILES = [
  'Standard', 'Lossless', 'High Quality', 'Ultra High Quality',
  'MP3-320', 'MP3-256', 'FLAC', 'Any',
];

export default function SettingsPage() {
  const [settings, setSettings] = useState<Record<string, string>>({});
  const [saving, setSaving] = useState<string | null>(null);
  const [saved, setSaved] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [folders, setFolders] = useState<Folder[]>([]);
  const [showSecrets, setShowSecrets] = useState<Record<string, boolean>>({});

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
      const safeFolders = Array.isArray(folderList) ? folderList : [];
      setFolders(safeFolders);
    } catch {
      setFolders([]);
    }
  }, []);

  useEffect(() => {
    loadAll();
    loadFolders();
  }, [loadAll, loadFolders]);

  const save = async (key: string, value: string) => {
    setSaving(key);
    try {
      await api.settings.set(key, value);
      setSettings((s) => ({ ...s, [key]: value }));
      setSaved(`Saved`);
      setTimeout(() => setSaved(null), 2000);
      setError(null);
      if (key.startsWith('lidarr_')) {
        setTimeout(loadFolders, 500);
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

  // Input components
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
      className={`relative w-11 h-6 rounded-full transition-colors ${
        settings[keyName] === 'true' ? 'bg-emerald-500' : 'bg-gray-600'
      }`}
    >
      <span className={`absolute top-0.5 left-0.5 w-5 h-5 bg-white rounded-full transition-transform ${
        settings[keyName] === 'true' ? 'translate-x-5' : ''
      }`} />
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

  // Section label helper
  const Label = ({ label }: { label: string }) => (
    <p className="text-xs font-semibold text-gray-400 uppercase tracking-wider mb-2">{label}</p>
  );

  return (
    <div className="space-y-6 max-w-4xl">
      {error && (
        <div className="rounded-lg bg-red-900/30 border border-red-700 px-4 py-3 text-red-400 text-sm">
          {error}
        </div>
      )}
      {saved && (
        <div className="rounded-lg bg-emerald-900/30 border border-emerald-700 px-4 py-3 text-emerald-400 text-sm">
          ✅ {saved}
        </div>
      )}

      {/* ── Two-column top row ── */}
      <div className="grid grid-cols-2 gap-4">

        {/* Schedule */}
        <div className="card space-y-3">
          <h2 className="text-base font-semibold flex items-center gap-2">
            ⏰ Schedule
          </h2>
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
          <h2 className="text-base font-semibold flex items-center gap-2">
            🎯 General
          </h2>
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
                  className={`card text-left transition-all p-2 ${
                    settings[KEYS.download_mode] === value
                      ? 'border-emerald-500 bg-emerald-900/20'
                      : 'hover:border-gray-500'
                  }`}
                >
                  <p className="text-sm font-semibold text-white">{emoji} {title}</p>
                  <p className="text-xs text-gray-400">{desc}</p>
                </button>
              ))}
            </div>
          </div>
        </div>
      </div>

      {/* Lidarr */}
      <div className="card space-y-3">
        <h2 className="text-base font-semibold flex items-center gap-2">
          🎵 Lidarr
        </h2>
        <p className="text-xs text-gray-400">
          Connect to your Lidarr instance to add artists and track downloads.
        </p>
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
            <SelectInput keyName={KEYS.lidarr_quality_profile} options={QUALITY_PROFILES} />
          </div>
          <div>
            <p className="text-xs font-medium text-gray-300 mb-1">Default Root Folder</p>
            {folders.length > 0 ? (
              <SelectInput keyName={KEYS.lidarr_default_root_folder} options={folders.map((f) => f.path)} />
            ) : (
              <TextInput keyName={KEYS.lidarr_default_root_folder} placeholder="/music" />
            )}
          </div>
        </div>
      </div>

      {/* Last.fm */}
      <div className="card space-y-3">
        <h2 className="text-base font-semibold flex items-center gap-2">
          📊 Last.fm
        </h2>
        <p className="text-xs text-gray-400">Used to get popularity scores for tracks and albums.</p>
        <div>
          <p className="text-xs font-medium text-gray-300 mb-1">API Key</p>
          <TextInput keyName={KEYS.lastfm_api_key} placeholder="your-lastfm-api-key" isSecret />
          <p className="text-xs text-gray-500 mt-1">Get one at <a href="https://www.last.fm/api/account/create" target="_blank" rel="noopener" className="text-emerald-400 hover:underline">last.fm/api/account/create</a></p>
        </div>
      </div>

      {/* Discord */}
      <div className="card space-y-3">
        <h2 className="text-base font-semibold flex items-center gap-2">
          💬 Discord Bot
        </h2>
        <p className="text-xs text-gray-400">
          Enable a Discord bot for commands and daily reports. Leave token blank to disable.
        </p>

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

        <div className="grid grid-cols-3 gap-3 pt-1">
          <div className="flex items-center justify-between bg-gray-800/50 border border-gray-700 rounded px-3 py-2">
            <p className="text-sm font-medium text-white">Allow All Users</p>
            <ToggleInput keyName={KEYS.discord_allow_all_users} label="Allow All Users" />
          </div>
          <div className="flex items-center justify-between bg-gray-800/50 border border-gray-700 rounded px-3 py-2">
            <p className="text-sm font-medium text-white">Auto-Thread</p>
            <ToggleInput keyName={KEYS.discord_auto_thread} label="Auto-Thread" />
          </div>
          <div className="flex items-center justify-between bg-gray-800/50 border border-gray-700 rounded px-3 py-2">
            <p className="text-sm font-medium text-white">Require Mention</p>
            <ToggleInput keyName={KEYS.discord_require_mention} label="Require Mention" />
          </div>
        </div>
      </div>

      {/* About */}
      <div className="card text-xs text-gray-500 space-y-1">
        <p>Groovarr connects Lidarr with Deezer &amp; Last.fm to automatically filter your music library to only the popular releases.</p>
        <p>Settings stored in SQLite · <code className="text-gray-400">/data/groovarr.db</code></p>
      </div>
    </div>
  );
}
