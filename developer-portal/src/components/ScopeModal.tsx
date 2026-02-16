import { useEffect } from 'react'
import MethodBadge from './MethodBadge'

interface Props {
  method: string;
  scopes: string[];
  path: string;
  onClose: () => void;
}

export default function ScopeModal({ method, scopes, path, onClose }: Props) {
  useEffect(() => {
    const handleEscape = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose()
    }
    window.addEventListener('keydown', handleEscape)
    return () => window.removeEventListener('keydown', handleEscape)
  }, [onClose])

  return (
    <div
      className="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50 p-4"
      onClick={onClose}
    >
      <div
        className="bg-white rounded-lg shadow-xl max-w-md w-full p-6"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex justify-between items-start mb-4">
          <div className="flex items-center gap-2">
            <MethodBadge method={method} onClick={() => {}} />
            <code className="text-sm text-gray-700">{path}</code>
          </div>
          <button
            onClick={onClose}
            className="text-gray-400 hover:text-gray-600 text-2xl leading-none"
          >
            &times;
          </button>
        </div>

        <div className="border-t pt-4">
          <h4 className="text-sm font-semibold text-gray-700 mb-3">Scopes Richiesti</h4>
          {scopes.length > 0 ? (
            <div className="space-y-2">
              {scopes.map((scope, idx) => (
                <div
                  key={idx}
                  className="bg-gray-50 border border-gray-200 rounded px-3 py-2 font-mono text-sm text-gray-800"
                >
                  {scope}
                </div>
              ))}
            </div>
          ) : (
            <p className="text-gray-500 text-sm italic">Nessuno scope richiesto</p>
          )}
        </div>
      </div>
    </div>
  )
}
