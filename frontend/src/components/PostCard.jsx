import { useRef, useEffect, useState } from 'react';

function isRecent(publishedAt) {
  const sevenDaysAgo = Date.now() - 7 * 24 * 60 * 60 * 1000;
  return new Date(publishedAt).getTime() > sevenDaysAgo;
}

function formatDate(dateStr) {
  const date = new Date(dateStr);
  return date.toLocaleDateString('en-US', {
    month: 'short',
    day: 'numeric',
    year: 'numeric',
  });
}

export default function PostCard({ post, onClick }) {
  const ref = useRef(null);
  const [visible, setVisible] = useState(false);

  useEffect(() => {
    const el = ref.current;
    if (!el) return;
    const observer = new IntersectionObserver(
      ([entry]) => {
        if (entry.isIntersecting) {
          setVisible(true);
          observer.disconnect();
        }
      },
      { threshold: 0.1 }
    );
    observer.observe(el);
    return () => observer.disconnect();
  }, []);

  const handleClick = () => {
    if (post.summary_status === 'failed') {
      window.open(post.url, '_blank', 'noopener');
    } else {
      onClick(post);
    }
  };

  const recent = isRecent(post.published_at);
  const summaryText = typeof post.summary_short === 'string' ? post.summary_short : null;
  const tags = post.tags || [];
  const sourceName = post.source_name || 'Unknown';
  const sourceIcon = typeof post.source_icon_url === 'string' ? post.source_icon_url : null;

  return (
    <div
      ref={ref}
      onClick={handleClick}
      className={`
        bg-white rounded-2xl border border-gray-100 p-5 cursor-pointer
        shadow-sm hover:shadow-md hover:-translate-y-0.5
        transition-all duration-200
        ${visible ? 'animate-fade-in-up' : 'opacity-0'}
      `}
    >
      {/* Hero image */}
      {typeof post.image_url === 'string' && post.image_url && (
        <div className="mb-4 -mx-5 -mt-5 rounded-t-2xl overflow-hidden">
          <img
            src={post.image_url.String}
            alt=""
            className="w-full h-40 object-cover"
          />
        </div>
      )}

      {/* Source line */}
      <div className="flex items-center gap-2 text-sm text-gray-500 mb-2">
        {recent && (
          <span className="w-2 h-2 rounded-full bg-gradient-to-r from-teal-500 to-indigo-500 shrink-0" />
        )}
        {sourceIcon ? (
          <img src={sourceIcon} alt="" className="w-4 h-4 rounded-full" />
        ) : (
          <span className="w-4 h-4 rounded-full bg-gradient-to-br from-teal-400 to-indigo-400 shrink-0" />
        )}
        <span className="font-medium text-gray-700">{sourceName}</span>
        <span className="text-gray-300">·</span>
        <span>{formatDate(post.published_at)}</span>
      </div>

      {/* Title */}
      <h3 className="text-base font-semibold text-gray-900 leading-snug mb-2 line-clamp-2">
        {post.title}
      </h3>

      {/* Summary */}
      {summaryText && (
        <p className="text-sm text-gray-500 leading-relaxed mb-3">
          {summaryText}
        </p>
      )}

      {/* Tags */}
      {tags.length > 0 && (
        <div className="flex gap-1.5 flex-wrap">
          {tags.map((tag) => (
            <span
              key={tag}
              className="inline-block px-2 py-0.5 text-xs font-medium text-teal-700 bg-teal-50 rounded-full"
            >
              {tag}
            </span>
          ))}
        </div>
      )}
    </div>
  );
}
