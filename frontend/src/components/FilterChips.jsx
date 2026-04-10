export default function FilterChips({ sources, activeSourceId, onSourceFilter, tags, activeTag, onTagFilter }) {
  return (
    <div className="max-w-3xl mx-auto px-4 py-3">
      {/* Source chips */}
      <div className="flex gap-2 overflow-x-auto pb-2 scrollbar-hide">
        <Chip
          label="All"
          active={!activeSourceId}
          onClick={() => onSourceFilter(null)}
        />
        {sources.map((source) => (
          <Chip
            key={source.id}
            label={source.name}
            icon={typeof source.icon_url === 'string' ? source.icon_url : null}
            active={activeSourceId === source.id}
            onClick={() => onSourceFilter(source.id)}
          />
        ))}
      </div>

      {/* Tag chips */}
      {tags.length > 0 && (
        <div className="flex gap-2 overflow-x-auto pt-1 pb-2 scrollbar-hide">
          {tags.map((tag) => (
            <Chip
              key={tag}
              label={`#${tag}`}
              active={activeTag === tag}
              onClick={() => onTagFilter(activeTag === tag ? null : tag)}
              small
            />
          ))}
        </div>
      )}
    </div>
  );
}

function Chip({ label, icon, active, onClick, small }) {
  return (
    <button
      onClick={onClick}
      className={`
        inline-flex items-center gap-1.5 shrink-0 rounded-full transition-all text-sm
        ${small ? 'px-2.5 py-0.5 text-xs' : 'px-3 py-1.5'}
        ${active
          ? 'bg-gradient-to-r from-teal-500 to-indigo-500 text-white shadow-sm'
          : 'bg-white border border-gray-200 text-gray-600 hover:border-gray-300 hover:bg-gray-50'
        }
      `}
    >
      {icon && (
        <img src={icon} alt="" className="w-4 h-4 rounded-full" />
      )}
      {label}
    </button>
  );
}
