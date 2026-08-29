import type { Metadata } from 'next';
import Link from 'next/link';
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
            <Link href="/" className="hover:text-white transition-colors">Dashboard</Link>
            <Link href="/artists" className="hover:text-white transition-colors">Artists</Link>
            <Link href="/settings" className="hover:text-white transition-colors">Settings</Link>
          </nav>
        </header>
        <main className="p-6 max-w-6xl mx-auto">{children}</main>
      </body>
    </html>
  );
}
