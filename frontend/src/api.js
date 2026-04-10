const BASE = '/api/v1';

export async function fetchPosts({ page = 1, perPage = 20, sourceId, tag, q, before, after } = {}) {
  const params = new URLSearchParams();
  params.set('page', page);
  params.set('per_page', perPage);
  if (sourceId) params.set('source_id', sourceId);
  if (tag) params.set('tag', tag);
  if (q) params.set('q', q);
  if (before) params.set('before', before);
  if (after) params.set('after', after);

  const res = await fetch(`${BASE}/posts?${params}`);
  if (!res.ok) throw new Error(`Failed to fetch posts: ${res.status}`);
  return res.json();
}

export async function fetchPost(id) {
  const res = await fetch(`${BASE}/posts/${id}`);
  if (!res.ok) throw new Error(`Failed to fetch post: ${res.status}`);
  return res.json();
}

export async function fetchSources() {
  const res = await fetch(`${BASE}/sources`);
  if (!res.ok) throw new Error(`Failed to fetch sources: ${res.status}`);
  return res.json();
}
