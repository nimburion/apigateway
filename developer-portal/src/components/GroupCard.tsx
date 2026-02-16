import { GroupInfo } from '../types'

interface Props {
  group: GroupInfo;
  isSelected: boolean;
  onClick: () => void;
}

export default function GroupCard({ group, isSelected, onClick }: Props) {
  return (
    <div
      onClick={onClick}
      className={`bg-white rounded-lg p-5 shadow-sm border-2 cursor-pointer transition-all hover:shadow-md ${
        isSelected ? 'border-blue-500 ring-2 ring-blue-200' : 'border-gray-200'
      }`}
    >
      <div className="flex justify-between items-start mb-3">
        <h3 className="font-semibold text-lg text-gray-900">{group.name}</h3>
        <span className="px-2 py-1 bg-blue-100 text-blue-700 text-xs font-medium rounded">
          {group.prefix}
        </span>
      </div>
      
      <div className="flex gap-2 mb-3">
        {group.has_oauth2 && (
          <span className="px-2 py-1 bg-green-100 text-green-700 text-xs font-medium rounded">
            OAuth2
          </span>
        )}
        {group.has_me_api && (
          <span className="px-2 py-1 bg-green-100 text-green-700 text-xs font-medium rounded">
            /me
          </span>
        )}
      </div>
      
      <div className="text-sm text-gray-600 space-y-1">
        <div>Routes: <span className="font-medium">{group.route_count}</span></div>
        <div>WebSockets: <span className="font-medium">{group.websocket_count}</span></div>
      </div>
    </div>
  )
}
