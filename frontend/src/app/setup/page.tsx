'use client'

import { useEffect, useState, useCallback } from 'react'
import Link from 'next/link'
import { api } from '@/lib/api'

export default function SetupPage() {
  const [configured, setConfigured] = useState(false)
  const [saving, setSaving] = useState(false)
  const [saved, setSaved] = useState(false)
const BASE = "";
  const [error, setError] = useState<string | null>(null)
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')

  const checkConfigured = useCallback(async () => {
    try {
      const result = await api.settings.get('auth_username')
      setConfigured(result.value !== '')
    } catch {
      setConfigured(false)
    }
  }, [])

  useEffect(() => {
    checkConfigured()
  }, [checkConfigured])

  const handleSubmit = useCallback(async (e: React.FormEvent) => {
    e.preventDefault()
    setSaving(true)
    setError(null)

    try {
      // Save credentials to the database via /api/setup
      const res = await fetch(`${BASE}/api/setup`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/x-www-form-urlencoded',
        },
        body: new URLSearchParams({
          username,
          password,
        }),
      })

      if (!res.ok) {
        const text = await res.text()
        throw new Error(`Failed to save: ${res.status} ${text}`)
      }

      const data = await res.json()
      if (data.ok !== true) {
        throw new Error(`Server returned: ${JSON.stringify(data)}`)
      }

      setSaved(true)
      setTimeout(() => {
        setSaved(false)
        // Navigate away after a moment
        window.dispatchEvent(new PopStateEvent('pop'))
      }, 2000)
    } catch (e: any) {
      setError(`Failed to save: ${e.message}`)
    } finally {
      setSaving(false)
    }
  }, [username, password])

  if (configured) {
    // Already configured, redirect to dashboard
    return null // Will redirect via next navigation
  }

  return (
    <div className="min-h-screen bg-gray-900 text-white p-6 max-w-md mx-auto">
      {error && (
        <div className="rounded-lg bg-red-900/30 border border-red-700 px-4 py-3 text-red-400 mb-6 text-sm">
          {error}
        </div>
      )}

      {saved && (
        <div className="rounded-lg bg-emerald-900/30 border border-emerald-700 px-4 py-3 text-emerald-400 mb-6 text-sm">
          ✅ Credentials saved successfully!
        </div>
      )}

      <div className="card bg-gray-800/50 border border-gray-600 rounded-lg p-8">
        <h1 className="text-2xl font-bold mb-6">Groovarr Initial Setup</h1>

        <p className="text-gray-300 mb-8">
          Set up your admin credentials to secure the Groovarr API and UI. Once configured, you'll be prompted to authenticate on future visits.
        </p>

        <form onSubmit={handleSubmit} className="space-y-6">
          <div>
            <label className="block text-sm font-medium mb-2">Username</label>
            <input
              type="text"
              name="username"
              value={username}
              onChange={(e) => setUsername(e.target.value)}
              required
              autoComplete="username"
              className="w-full bg-gray-700 border border-gray-600 rounded px-3 py-2 text-white mb-4"
            />
          </div>

          <div>
            <label className="block text-sm font-medium mb-2">Password</label>
            <input
              type="password"
              name="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              required
              autoComplete="new-password"
              className="w-full bg-gray-700 border border-gray-600 rounded px-3 py-2 text-white mb-4"
            />
          </div>

          <button
            type="submit"
            disabled={saving}
            className={`w-full py-3 px-4 rounded font-medium transition-colors ${saving ? 'opacity-50 cursor-not-allowed' : 'bg-emerald-600 hover:bg-emerald-500 text-white'}`}
          >
            {saving ? 'Saving…' : 'Save Credentials and Continue'}
          </button>
        </form>

        <p className="mt-6 text-xs text-gray-500">
          Credentials are stored in the database. The .env file will be updated on next startup.
        </p>
      </div>
    </div>
  )
}