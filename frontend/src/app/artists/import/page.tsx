'use client';

import { useEffect, useState, useCallback } from 'react';
import Link from 'next/link';
import { api, LidarrImportArtist, ImportListResponse, BulkImportRequest, BulkImportResponse } from '@/lib/api';

export default function ArtistImportPage() {
  const [artists, setArtists] = useState<LidarrImportArtist[]>([]);
  const [selectedIds, setSelectedIds] = useState<Set<number>>(new Set());
  const [currentPage, setCurrentPage] = useState(1);
  const [limitOptions] = useState([10, 20, 50]);
  const [itemsPerPage, setItemsPerPage] = useState(20);
  const [total, setTotal] = useState(0);
  const [totalPages, setTotalPages] = useState(1);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [importing, setImporting] = useState(false);
  const [importError, setImportError] = useState<string | null>(null);
  const [importSuccess, setImportSuccess] = useState<string | null>(null);

  const loadArtists = useCallback(async () => {
    setLoading(true);
    try {
      const data = await api.artists.importList(currentPage, itemsPerPage);
      setArtists(data.artists);
      setTotal(data.total);
      setTotalPages(data.totalPages);
      setError(null);
    } catch (e: any) {
      setError(`Failed to load artists: ${e.message}`);
      setArtists([]);
      setTotal(0);
      setTotalPages(1);
    } finally {
      setLoading(false);
    }
  }, [currentPage, itemsPerPage]);

  useEffect(() => {
    loadArtists();
  }, [loadArtists]);

  const handlePageChange = (page: number) => {
    if (page >= 1 && page <= totalPages) {
      setCurrentPage(page);
      // Reset selection when changing pages to avoid confusion
      setSelectedIds(new Set());
    }
  };

  const handleItemsPerPageChange = (limit: number) => {
    setItemsPerPage(limit);
    setCurrentPage(1);
    setSelectedIds(new Set());
  };

  const toggleSelect = (id: number) => {
    const newSet = new Set(selectedIds);
    if (newSet.has(id)) {
      newSet.delete(id);
    } else {
      newSet.add(id);
    }
    setSelectedIds(newSet);
  };

  const selectAllOnPage = () => {
    const newSet = new Set(selectedIds);
    artists.forEach(a => newSet.add(a.lidarrId));
    setSelectedIds(newSet);
  };

  const clearSelection = () => {
    setSelectedIds(new Set());
  };

  const handleImport = async () => {
    if (selectedIds.size === 0) {
      setImportError('Please select at least one artist to import');
      return;
    }

    setImporting(true);
    setImportError(null);
    setImportSuccess(null);

    try {
      const result = await api.artists.importBulk({
        artistIds: Array.from(selectedIds),
      });

      if (result.imported > 0) {
        setImportSuccess(`Successfully imported ${result.imported} artist(s)`);
        // Refresh the artist list after import
        setTimeout(() => {
          loadArtists();
        }, 1000);
      } else {
        setImportError('No artists were imported');
      }

      if (result.skipped > 0) {
        // Show skipped count in success message or separate info
        const msg = result.imported > 0 
          ? `Imported ${result.imported}, skipped ${result.skipped} (already in Groovarr)`
          : `Skipped ${result.skipped} (already in Groovarr)`;
        setImportSuccess(msg);
      }

      if (result.errors && result.errors.length > 0) {
        setImportError(`Import completed with ${result.errors.length} error(s): ${result.errors.join(', ')}`);
      }

      // Clear selection after import
      setSelectedIds(new Set());
    } catch (e: any) {
      setImportError(`Import failed: ${e.message}`);
    } finally {
      setImporting(false);
    }
  };

  if (loading) {
    return (
      <div className="space-y-6">
        <div className="card">
          <h2 className="text-lg font-semibold mb-4">📥 Import Artists from Lidarr</h2>
          <p className="text-gray-400">Loading artists...</p>
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex justify-between items-wrap">
        <div>
          <h2 className="text-xl font-semibold">📥 Import Artists from Lidarr</h2>
          <p className="text-sm text-gray-500">
            Bulk import artists from your Lidarr instance. Shows artists not yet in Groovarr.
          </p>
        </div>
        <Link href="/artists" className="btn-secondary">
          ← Back to Artist List
        </Link>
      </div>

      {/* Import controls */}
      <div className="card">
        <div className="space-y-4">
          <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-3">
            <div className="flex flex-wrap items-center gap-4">
              <span className="text-sm text-gray-400">Page size:</span>
              <div className="flex gap-2">
                {limitOptions.map(limit => (
                  <button
                    key={limit}
                    onClick={() => handleItemsPerPageChange(limit)}
                    className={`px-3 py-1 text-xs rounded 
                      ${itemsPerPage === limit 
                        ? 'bg-emerald-500 text-white' 
                        : 'bg-gray-700 text-gray-300 hover:bg-gray-600'}`}
                  >
                    {limit}
                  </button>
                ))}
              </div>
            </div>

            <div className="flex-1 flex justify-between">
              <div className="text-sm text-gray-400">
                Page {currentPage} of {totalPages} • {total} total artists
              </div>
              <div className="flex items-center gap-3">
                <button
                  onClick={() => handlePageChange(currentPage - 1)}
                  disabled={currentPage === 1}
                  className="text-xs text-gray-400 hover:text-white"
                >
                  ‹ Prev
                </button>
                <button
                  onClick={() => handlePageChange(currentPage + 1)}
                  disabled={currentPage === totalPages}
                  className="text-xs text-gray-400 hover:text-white"
                >
                  Next ›
                </button>
              </div>
            </div>
          </div>

          <div className="flex justify-end">
            <button
              onClick={clearSelection}
              disabled={selectedIds.size === 0}
              className="text-xs text-gray-400 hover:text-gray-300"
            >
              Clear Selection
            </button>
            <button
              onClick={selectAllOnPage}
              disabled={artists.length === 0}
              className="text-xs text-gray-400 hover:text-gray-300"
            >
              Select All on Page
            </button>
          </div>
        </div>
      </div>

      {/* Artists table */}
      {artists.length > 0 ? (
        <div className="card">
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-gray-700">
                  <th className="p-2 text-left text-xs font-medium text-gray-400">
                    Select
                  </th>
                  <th className="p-2 text-left text-xs font-medium text-gray-400">
                    Artist Name
                  </th>
                  <th className="p-2 text-left text-xs font-medium text-gray-400">
                    Root Folder
                  </th>
                  <th className="p-2 text-left text-xs font-medium text-gray-400">
                    Quality Profile
                  </th>
                  <th className="p-2 text-left text-xs font-medium text-gray-400">
                    Monitor
                  </th>
                  <th className="p-2 text-left text-xs font-medium text-gray-400">
                    Status
                  </th>
                </tr>
              </thead>
              <tbody>
                {artists.map(artist => (
                  <tr
                    key={artist.lidarrId}
                    className={`border-b border-gray-700/50 last:border-0 
                      ${selectedIds.has(artist.lidarrId) ? 'bg-gray-800/50' : ''}`}
                  >
                    <td className="p-2 text-center">
                      <input
                        type="checkbox"
                        checked={selectedIds.has(artist.lidarrId)}
                        onChange={() => toggleSelect(artist.lidarrId)}
                        className="h-4 w-4 text-emerald-600"
                      />
                    </td>
                    <td className="p-2 text-left font-medium text-white">
                      {artist.name}
                    </td>
                    <td className="p-2 text-left text-gray-300 truncate max-w-xs">
                      {artist.rootFolder}
                    </td>
                    <td className="p-2 text-left text-gray-300">
                      {artist.qualityProfile}
                    </td>
                    <td className="p-2 text-left text-gray-300">
                      {artist.monitor === 'none' ? '⭕ None' : 
                       artist.monitor === 'albums' ? '💿 Albums' : '💿💿 All'}
                    </td>
                    <td className="p-2 text-left">
                      {artist.alreadyInGroovarr ? (
                        <span className="text-yellow-400">⚠️ Already in Groovarr</span>
                      ) : (
                        <span className="text-emerald-400">✅ Ready to import</span>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>

          <div className="flex items-center justify-between pt-4">
            <div className="text-sm text-gray-400">
              {selectedIds.size} artist(s) selected
            </div>
            <div className="flex items-center gap-3">
              {importError ? (
                <span className="text-red-400">
                  ⚠️ {importError}
                </span>
              ) : importSuccess ? (
                <span className="text-emerald-400">
                  ✅ {importSuccess}
                </span>
              ) : null}
              <button
                onClick={handleImport}
                disabled={importing || selectedIds.size === 0}
                className="btn-primary"
              >
                {importing ? '⏳ Importing...' : '📥 Import Selected'}
              </button>
            </div>
          </div>
        </div>
      ) : (
        <div className="card">
          {error ? (
            <div className="rounded-lg bg-red-900/30 border border-red-700 px-4 py-3 text-red-400">
              {error}
            </div>
          ) : (
            <p className="text-gray-400 text-center py-8">
              No artists available to import. All Lidarr artists are already in Groovarr.
            </p>
          )}
        </div>
      )}
    </div>
  );
}