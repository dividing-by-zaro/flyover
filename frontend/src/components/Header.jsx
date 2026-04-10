import { useState, useEffect, useCallback } from 'react';

export default function Header({ onSearch }) {
  const [query, setQuery] = useState('');

  const debouncedSearch = useCallback(() => {
    onSearch(query);
  }, [query, onSearch]);

  useEffect(() => {
    const timer = setTimeout(debouncedSearch, 300);
    return () => clearTimeout(timer);
  }, [debouncedSearch]);

  return (
    <header className="sticky top-0 z-20 bg-white/80 backdrop-blur-lg border-b border-gray-100">
      <div className="max-w-3xl mx-auto px-4 py-4 flex items-center gap-4">
        <h1 className="text-xl font-semibold bg-gradient-to-r from-teal-500 to-indigo-500 bg-clip-text text-transparent tracking-tight shrink-0">
          Flyover
        </h1>
        <div className="relative flex-1 max-w-md">
          <svg
            className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-gray-400"
            fill="none" stroke="currentColor" viewBox="0 0 24 24"
          >
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2}
              d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
          </svg>
          <input
            type="text"
            placeholder="Search posts..."
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            className="w-full pl-10 pr-4 py-2 text-sm bg-gray-50 border border-gray-200 rounded-xl focus:outline-none focus:ring-2 focus:ring-teal-500/30 focus:border-teal-500 transition-all"
          />
          {query && (
            <button
              onClick={() => setQuery('')}
              className="absolute right-3 top-1/2 -translate-y-1/2 text-gray-400 hover:text-gray-600"
            >
              <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
              </svg>
            </button>
          )}
        </div>
      </div>
    </header>
  );
}
