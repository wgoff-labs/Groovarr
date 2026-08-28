import type { Metadata } from 'next';
import './globals.css';

export const metadata: Metadata = {
  title: 'Groovarr',
  description: 'Lidarr Hits Monitor & Auto-Pruner',
};

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en">
      <body className="min-h-screen">
        <header className="border-b px-6 py-4 flex items-center justify-between" style={{ borderColor: 'var(--border)' }}>
          <div className="flex items-center gap-3">
            <span className="text-2xl">🎵</span>
            <h1 className="text-xl font-bold tracking-tight">Groovarr</h1>
          </div>
          <nav className="flex gap-4 text-sm text-gray-400">
            <a href="/" className="hover:text-white transition-colors">Dashboard</a>
            <a href="/artists" className="hover:text-white transition-colors">Artists</a>
            <a href="/settings" className="hover:text-white transition-colors">Settings</a>
          </nav>
        </header>
        <main className="p-6 max-w-6xl mx-auto">{children}</main>
      </body>
    </html>
  );
}
