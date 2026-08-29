'use client';

import { useEffect, useState, useCallback } from 'react';
import { api, LogEntry } from '@/lib/api';
import Link from 'next/link';

export default function LogsPage() {
  const [logs, setLogs] = useState<LogEntry[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const loadLogs = useCallback(async () => {
    setLoading(true);
    try {
      const data = await api.connections.logs();
      setLogs(data.logs || []);
      setError(null);
    } catch (e: any) {
      setError(e.message);
    }
    setLoading(false);
  }, []);

  useEffect(() => {
    loadLogs();
    const interval = setInterval(loadLogs, 5000);
    return () => clearInterval(interval);
  }, [loadLogs]);

  const handleClear = async () => {
    if (!confirm('Clear all logs?')) return;
    try {
      await api.connections.clearLogs();
      setLogs([]);
    } catch (e: any) {
      setError(e.message);
    }
  };

  const levelColor = (level: string) => {
    switch (level) {
      case 'error': return 'text-red-400';
      case 'warn': return 'text-yellow-400';
      default: return 'text-gray-400';
    }
  };

  const formatTime = (ts: string) => {
    const d = new Date(ts);
    return d.toLocaleString();
  };

  return (
    <div className="space-y-4 max-w-4xl">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-3">
          <Link href="/settings" className="text-gray-400 hover:text-white text-sm">← Settings</Link>
          <h1 className="text-xl font-semibold">Connection Logs</h1>
        </div>
        <div className="flex gap-2">
          <button
            onClick={loadLogs}
            className="px-3 py-1.5 bg-gray-700 hover:bg-gray-600 rounded text-sm text-white"
          >
            🔄 Refresh
          </button>
          <button
            onClick={handleClear}
            className="px-3 py-1.5 bg-red-900/50 hover:bg-red-800/50 border border-red-700 rounded text-sm text-red-300"
          >
            🗑️ Clear
          </button>
        </div>
      </div>

      {error && (
        <div className="rounded-lg bg-red-900/30 border border-red-700 px-4 py-3 text-red-400 text-sm">
          {error}
        </div>
      )}

      {loading && logs.length === 0 ? (
        <div className="card text-center text-gray-400 py-8">
          Loading...
        </div>
      ) : logs.length === 0 ? (
        <div className="card text-center text-gray-500 py-8">
          No connection logs yet.
        </div>
      ) : (
        <div className="space-y-1">
          {logs.map((log, i) => (
            <div key={i} className={`card py-2 px-3 text-sm font-mono ${levelColor(log.level)}`}>
              <span className="text-gray-600 mr-3">{formatTime(log.timestamp)}</span>
              <span className="text-gray-500 mr-3">[{log.service}]</span>
              <span className="text-gray-400">[{log.level.toUpperCase()}]</span>{' '}
              <span className="text-gray-300">{log.message}</span>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
