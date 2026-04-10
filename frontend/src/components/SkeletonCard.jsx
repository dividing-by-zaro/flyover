export default function SkeletonCard() {
  return (
    <div className="bg-white rounded-2xl border border-gray-100 p-5 shadow-sm animate-pulse">
      {/* Source line */}
      <div className="flex items-center gap-2 mb-3">
        <div className="w-4 h-4 rounded-full bg-gray-200" />
        <div className="w-24 h-3 rounded bg-gray-200" />
        <div className="w-16 h-3 rounded bg-gray-200" />
      </div>

      {/* Title */}
      <div className="space-y-2 mb-3">
        <div className="w-full h-4 rounded bg-gray-200" />
        <div className="w-3/4 h-4 rounded bg-gray-200" />
      </div>

      {/* Summary */}
      <div className="space-y-1.5 mb-3">
        <div className="w-full h-3 rounded bg-gray-100" />
        <div className="w-5/6 h-3 rounded bg-gray-100" />
      </div>

      {/* Tags */}
      <div className="flex gap-1.5">
        <div className="w-16 h-5 rounded-full bg-gray-100" />
        <div className="w-20 h-5 rounded-full bg-gray-100" />
      </div>
    </div>
  );
}
