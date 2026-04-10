import { useEffect, useCallback } from 'react';

function formatDate(dateStr) {
  return new Date(dateStr).toLocaleDateString('en-US', {
    weekday: 'long',
    month: 'long',
    day: 'numeric',
    year: 'numeric',
  });
}

export default function DetailModal({ post, onClose }) {
  const handleKeyDown = useCallback((e) => {
    if (e.key === 'Escape') onClose();
  }, [onClose]);

  useEffect(() => {
    document.addEventListener('keydown', handleKeyDown);
    document.body.style.overflow = 'hidden';
    return () => {
      document.removeEventListener('keydown', handleKeyDown);
      document.body.style.overflow = '';
    };
  }, [handleKeyDown]);

  if (!post) return null;

  const summaryLong = typeof post.summary_long === 'string' ? post.summary_long : null;
  const author = typeof post.author === 'string' ? post.author : null;
  const sourceName = post.source_name || 'Unknown';
  const tags = post.tags || [];

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center p-4"
      onClick={onClose}
    >
      {/* Backdrop */}
      <div className="absolute inset-0 bg-black/40 backdrop-blur-sm" />

      {/* Modal */}
      <div
        className="relative bg-white rounded-3xl shadow-2xl max-w-2xl w-full max-h-[85vh] overflow-y-auto animate-modal-in"
        onClick={(e) => e.stopPropagation()}
      >
        {/* Close button */}
        <button
          onClick={onClose}
          className="absolute top-4 right-4 p-2 rounded-full hover:bg-gray-100 transition-colors z-10"
        >
          <svg className="w-5 h-5 text-gray-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
          </svg>
        </button>

        <div className="p-8">
          {/* Title */}
          <h2 className="text-2xl font-bold text-gray-900 leading-tight mb-4 pr-8">
            {post.title}
          </h2>

          {/* Meta */}
          <div className="flex items-center gap-2 text-sm text-gray-500 mb-6">
            <span className="font-medium text-gray-700">{sourceName}</span>
            {author && (
              <>
                <span className="text-gray-300">·</span>
                <span>{author}</span>
              </>
            )}
            <span className="text-gray-300">·</span>
            <span>{formatDate(post.published_at)}</span>
          </div>

          {/* Long summary */}
          {summaryLong && (
            <div className="text-gray-600 leading-relaxed text-[15px] mb-6 whitespace-pre-line">
              {summaryLong}
            </div>
          )}

          {/* Tags */}
          {tags.length > 0 && (
            <div className="flex gap-2 flex-wrap mb-8">
              {tags.map((tag) => (
                <span
                  key={tag}
                  className="inline-block px-3 py-1 text-sm font-medium text-teal-700 bg-teal-50 rounded-full"
                >
                  {tag}
                </span>
              ))}
            </div>
          )}

          {/* CTA */}
          <a
            href={post.url}
            target="_blank"
            rel="noopener noreferrer"
            className="inline-flex items-center gap-2 px-6 py-3 bg-gradient-to-r from-teal-500 to-indigo-500 text-white font-medium rounded-xl hover:shadow-lg hover:shadow-teal-500/25 transition-all"
          >
            Read Original
            <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M14 5l7 7m0 0l-7 7m7-7H3" />
            </svg>
          </a>
        </div>
      </div>
    </div>
  );
}
