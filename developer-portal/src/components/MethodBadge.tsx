interface Props {
  method: string;
  onClick: () => void;
  expanded?: boolean;
}

const methodColors: Record<string, string> = {
  GET: 'bg-blue-100 text-blue-700 hover:bg-blue-200',
  POST: 'bg-green-100 text-green-700 hover:bg-green-200',
  PUT: 'bg-orange-100 text-orange-700 hover:bg-orange-200',
  DELETE: 'bg-red-100 text-red-700 hover:bg-red-200',
  PATCH: 'bg-purple-100 text-purple-700 hover:bg-purple-200',
}

export default function MethodBadge({ method, onClick, expanded = false }: Props) {
  const colorClass = methodColors[method] || 'bg-gray-100 text-gray-700 hover:bg-gray-200'
  const ringClass = expanded ? 'ring-2 ring-offset-1 ring-blue-300' : ''
  
  return (
    <button
      onClick={onClick}
      className={`px-3 py-1 rounded font-semibold text-xs transition-all transform hover:scale-105 ${colorClass} ${ringClass}`}
    >
      {method}
    </button>
  )
}
