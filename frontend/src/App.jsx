import { useState, useEffect, useCallback, useRef } from 'react';
import Header from './components/Header';
import FilterChips from './components/FilterChips';
import PostCard from './components/PostCard';
import SkeletonCard from './components/SkeletonCard';
import DetailModal from './components/DetailModal';
import { fetchPosts, fetchSources } from './api';

function App() {
  const [posts, setPosts] = useState([]);
  const [sources, setSources] = useState([]);
  const [loading, setLoading] = useState(true);
  const [loadingMore, setLoadingMore] = useState(false);
  const [page, setPage] = useState(1);
  const [hasMore, setHasMore] = useState(true);
  const [searchQuery, setSearchQuery] = useState('');
  const [activeSourceId, setActiveSourceId] = useState(null);
  const [activeTag, setActiveTag] = useState(null);
  const [selectedPost, setSelectedPost] = useState(null);
  const sentinelRef = useRef(null);

  // Collect unique tags from current posts
  const allTags = [...new Set(posts.flatMap((p) => p.tags || []))].sort();

  const loadPosts = useCallback(async (pageNum, append = false) => {
    if (append) {
      setLoadingMore(true);
    } else {
      setLoading(true);
    }

    try {
      const data = await fetchPosts({
        page: pageNum,
        perPage: 20,
        sourceId: activeSourceId,
        tag: activeTag,
        q: searchQuery || undefined,
      });

      if (append) {
        setPosts((prev) => [...prev, ...data]);
      } else {
        setPosts(data);
      }

      setHasMore(data.length === 20);
    } catch (err) {
      console.error('Failed to load posts:', err);
    } finally {
      setLoading(false);
      setLoadingMore(false);
    }
  }, [activeSourceId, activeTag, searchQuery]);

  // Load sources on mount
  useEffect(() => {
    fetchSources()
      .then(setSources)
      .catch((err) => console.error('Failed to load sources:', err));
  }, []);

  // Load posts when filters change
  useEffect(() => {
    setPage(1);
    setHasMore(true);
    loadPosts(1, false);
  }, [loadPosts]);

  // Infinite scroll
  useEffect(() => {
    const sentinel = sentinelRef.current;
    if (!sentinel || !hasMore || loading || loadingMore) return;

    const observer = new IntersectionObserver(
      ([entry]) => {
        if (entry.isIntersecting && hasMore && !loadingMore) {
          const nextPage = page + 1;
          setPage(nextPage);
          loadPosts(nextPage, true);
        }
      },
      { threshold: 0.1 }
    );

    observer.observe(sentinel);
    return () => observer.disconnect();
  }, [hasMore, loading, loadingMore, page, loadPosts]);

  const handleSearch = useCallback((q) => {
    setSearchQuery(q);
  }, []);

  return (
    <div className="min-h-screen bg-[#FAFAFA]">
      <Header onSearch={handleSearch} />

      <FilterChips
        sources={sources}
        activeSourceId={activeSourceId}
        onSourceFilter={setActiveSourceId}
        tags={allTags}
        activeTag={activeTag}
        onTagFilter={setActiveTag}
      />

      <main className="max-w-3xl mx-auto px-4 pb-16">
        {loading ? (
          <div className="space-y-4">
            {[...Array(5)].map((_, i) => (
              <SkeletonCard key={i} />
            ))}
          </div>
        ) : posts.length === 0 ? (
          <div className="text-center py-20">
            <div className="text-5xl mb-4">🔭</div>
            <p className="text-gray-500 text-lg">No posts found</p>
            <p className="text-gray-400 text-sm mt-1">
              {searchQuery ? 'Try a different search query' : 'Posts will appear here once feeds are polled'}
            </p>
          </div>
        ) : (
          <div className="space-y-4">
            {posts.map((post) => (
              <PostCard
                key={post.id}
                post={post}
                onClick={setSelectedPost}
              />
            ))}

            {/* Loading more indicator */}
            {loadingMore && (
              <div className="space-y-4">
                <SkeletonCard />
                <SkeletonCard />
              </div>
            )}

            {/* Infinite scroll sentinel */}
            {hasMore && <div ref={sentinelRef} className="h-4" />}
          </div>
        )}
      </main>

      {selectedPost && (
        <DetailModal
          post={selectedPost}
          onClose={() => setSelectedPost(null)}
        />
      )}
    </div>
  );
}

export default App;
