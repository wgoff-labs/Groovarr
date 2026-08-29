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
  discord_allow_all_users: 'discord_allow_all_users',
  discord_auto_thread: 'discord_auto_thread',
  discord_require_mention: 'discord_require_mention',
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
      setSaved(`${key} saved`);
      setTimeout(() => setSaved(null), 2500);
      setError(null);
      // Reload folders if lidarr settings changed
      if (key.startsWith('lidarr_')) {
        setTimeout(loadFolders, 500);
      }
    } catch (e: any) {
      setError(`Save failed for ${key}: ${e.message}`);
    }
    setSaving(null);
  };

  const updateField = (key: string, value: string) => {
    setSettings((s) => ({ ...s, [key]: value }));
  };

  const toggleSecret = (key: string) => {
    setShowSecrets((s) => ({ ...s, [key]: !s[key] }));
  };

  const Field = ({ label, hint, children }: { label: string; hint?: string; children: React.ReactNode }) => (
    <div className="space-y-1">
      <label className="block text-sm font-medium text-gray-300">{label}</label>
      {children}
      {hint && <p className="text-xs text-gray-500">{hint}</p>}
    </div>
  );

  const TextInput = ({ keyName, placeholder, type = 'text', isSecret = false }: { keyName: string; placeholder?: string; type?: string; isSecret?: boolean }) => {
    const visible = !isSecret || showSecrets[keyName];
    return (
      <div className="flex gap-2">
        <input
          type={isSecret ? (visible ? 'text' : 'password') : type}
          value={settings[keyName] || ''}
          onChange={(e) => updateField(keyName, e.target.value)}
          onBlur={() => save(keyName, settings[keyName] || '')}
          placeholder={placeholder}
          className="flex-1 bg-gray-800 border border-gray-600 rounded-lg px-4 py-2 text-white placeholder-gray-500 focus:outline-none focus:border-emerald-500"
        />
        {isSecret && (
          <button
            type="button"
            onClick={() => toggleSecret(keyName)}
            className="px-3 bg-gray-700 border border-gray-600 rounded-lg text-gray-300 hover:bg-gray-600"
            title={visible ? 'Hide' : 'Show'}
          >
            {visible ? '🙈' : '👁️'}
          </button>
        )}
        <button
          onClick={() => save(keyName, settings[keyName] || '')}
          disabled={saving === keyName}
          className="px-4 py-2 bg-emerald-600 hover:bg-emerald-500 disabled:bg-gray-600 disabled:cursor-not-allowed rounded-lg text-white text-sm font-medium"
        >
          {saving === keyName ? '...' : 'Save'}
        </button>
      </div>
    );
  };

  const Select = ({ keyName, options }: { keyName: string; options: string[] }) => (
    <div className="flex gap-2">
      <select
        value={settings[keyName] || ''}
        onChange={(e) => {
          updateField(keyName, e.target.value);
          save(keyName, e.target.value);
        }}
        className="flex-1 bg-gray-800 border border-gray-600 rounded-lg px-4 py-2 text-white focus:outline-none focus:border-emerald-500"
      >
        <option value="">— select —</option>
        {options.map((o) => (
          <option key={o} value={o}>{o}</option>
        ))}
      </select>
    </div>
  );

  const Toggle = ({ keyName, label, hint }: { keyName: string; label: string; hint?: string }) => (
    <div className="flex items-center justify-between bg-gray-800/50 border border-gray-700 rounded-lg px-4 py-3">
      <div>
        <p className="text-sm font-medium text-white">{label}</p>
        {hint && <p className="text-xs text-gray-500 mt-0.5">{hint}</p>}
      </div>
      <button
        type="button"
        onClick={() => {
          const newVal = settings[keyName] === 'true' ? 'false' : 'true';
          updateField(keyName, newVal);
          save(keyName, newVal);
        }}
        className={`relative w-12 h-6 rounded-full transition-colors ${
          settings[keyName] === 'true' ? 'bg-emerald-500' : 'bg-gray-600'
        }`}
      >
        <span
          className={`absolute top-0.5 left-0.5 w-5 h-5 bg-white rounded-full transition-transform ${
            settings[keyName] === 'true' ? 'translate-x-6' : ''
          }`}
        />
      </button>
    </div>
  );

  const RangeField = ({ keyName, min, max, step }: { keyName: string; min: number; max: number; step: number }) => (
    <div className="flex items-center gap-4">
      <input
        type="range"
        min={min} max={max} step={step}
        value={settings[keyName] || '60'}
        onChange={(e) => updateField(keyName, e.target.value)}
        onMouseUp={() => save(keyName, settings[keyName] || '60')}
        onTouchEnd={() => save(keyName, settings[keyName] || '60')}
        className="flex-1 accent-emerald-500"
      />
      <span className="text-2xl font-bold text-white w-12 text-center">{settings[keyName] || '60'}</span>
    </div>
  );

  return (
    <div className="space-y-6 max-w-3xl">
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

      {/* Lidarr */}
      <div className="card space-y-4">
        <h2 className="text-lg font-semibold flex items-center gap-2">
          🎵 Lidarr
        </h2>
        <p className="text-xs text-gray-400">
          Connect to your Lidarr instance to add artists and track downloads.
        </p>

        <Field label="Lidarr URL" hint="e.g. http://10.0.0.244:8686">
          <TextInput keyName={KEYS.lidarr_url} placeholder="http://localhost:8686" />
        </Field>

        <Field label="Lidarr API Key" hint="Find in Lidarr: Settings → General → API Key">
          <TextInput keyName={KEYS.lidarr_api_key} placeholder="your-lidarr-api-key" isSecret />
        </Field>

        <Field label="Quality Profile" hint="Which Lidarr quality profile to use for new artists">
          <Select keyName={KEYS.lidarr_quality_profile} options={QUALITY_PROFILES} />
        </Field>

        <Field label="Default Root Folder" hint="Where new artists are stored. Folders auto-discovered from Lidarr.">
          {folders.length > 0 ? (
            <Select keyName={KEYS.lidarr_default_root_folder} options={folders.map((f) => f.path)} />
          ) : (
            <TextInput keyName={KEYS.lidarr_default_root_folder} placeholder="/music or similar" />
          )}
        </Field>
      </div>

      {/* Last.fm */}
      <div className="card space-y-4">
        <h2 className="text-lg font-semibold flex items-center gap-2">
          📊 Last.fm
        </h2>
        <p className="text-xs text-gray-400">
          Used to get popularity scores for tracks and albums.
        </p>

        <Field label="Last.fm API Key" hint="Get one at https://www.last.fm/api/account/create">
          <TextInput keyName={KEYS.lastfm_api_key} placeholder="your-lastfm-api-key" isSecret />
        </Field>
      </div>

      {/* General */}
      <div className="card space-y-4">
        <h2 className="text-lg font-semibold flex items-center gap-2">
          🎯 General
        </h2>

        <Field label="Popularity Threshold" hint="Albums/tracks with avg popularity at or above this score are added. Higher = stricter.">
          <RangeField keyName={KEYS.popularity_threshold} min={10} max={100} step={5} />
        </Field>

        <div className="grid grid-cols-2 gap-3">
          {[
            { value: 'tracks', label: '🎶 Tracks Only', desc: 'Only popular tracks downloaded. Rest deleted. (Recommended)' },
            { value: 'album', label: '💿 Full Album', desc: 'Grab the whole album if any track is popular. No pruning.' },
          ].map(({ value, label, desc }) => (
            <button
              key={value}
              onClick={() => save(KEYS.download_mode, value)}
              className={`card text-left transition-all ${
                settings[KEYS.download_mode] === value ? 'border-emerald-500 bg-emerald-900/20' : 'hover:border-gray-500'
              }`}
            >
              <p className="font-semibold text-white mb-1">{label}</p>
              <p className="text-xs text-gray-400">{desc}</p>
            </button>
          ))}
        </div>
      </div>

      {/* Schedule */}
      <div className="card space-y-4">
        <h2 className="text-lg font-semibold flex items-center gap-2">
          ⏰ Schedule
        </h2>

        <Field label="Daily Check Cron" hint="When to run the daily check. Format: '0 9 * * *' = 9 AM daily.">
          <TextInput keyName={KEYS.daily_check_cron} placeholder="0 9 * * *" />
        </Field>

        <Field label="Timezone">
          <Select keyName={KEYS.timezone} options={TIMEZONES} />
        </Field>
      </div>

      {/* Discord */}
      <div className="card space-y-4">
        <h2 className="text-lg font-semibold flex items-center gap-2">
          💬 Discord (optional)
        </h2>
        <p className="text-xs text-gray-400">
          Enable a Discord bot for commands and daily reports. Leave token blank to disable.
        </p>

        <Field label="Bot Token" hint="From https://discord.com/developers/applications">
          <TextInput keyName={KEYS.discord_token} placeholder="your-discord-bot-token" isSecret />
        </Field>

        <Field label="Home Channel ID" hint="Where daily reports are posted. Right-click channel in Discord → Copy ID.">
          <TextInput keyName={KEYS.discord_home_channel} placeholder="123456789012345678" />
        </Field>

        <div className="space-y-2">
          <Toggle
            keyName={KEYS.discord_allow_all_users}
            label="Allow All Users"
            hint="If off, only users in the allow-list can run commands"
          />
          <Toggle
            keyName={KEYS.discord_auto_thread}
            label="Auto-Thread on Mentions"
            hint="Create a thread for each mention of the bot"
          />
          <Toggle
            keyName={KEYS.discord_require_mention}
            label="Require Bot Mention"
            hint="Only respond when the bot is @mentioned"
          />
        </div>
      </div>

      {/* About */}
      <div className="card text-xs text-gray-500">
        <p>Groovarr connects Lidarr with Deezer &amp; Last.fm to automatically filter your music library to only the popular releases.</p>
        <p className="mt-2">All settings are stored in SQLite. Database: <code className="text-gray-400">/data/groovarr.db</code></p>
      </div>
    </div>
  );
}
