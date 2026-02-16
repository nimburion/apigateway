export interface MethodInfo {
  method: string;
  scopes: string[];
}

export interface RouteInfo {
  path_prefix: string;
  target_url: string;
  methods: MethodInfo[];
  openapi?: OpenAPIInfo | null;
}

export interface OpenAPIInfo {
  file: string;
  mode: string;
  title?: string;
  version?: string;
  description?: string;
  operations?: OpenAPIOperation[];
  error?: string;
}

export interface OpenAPIOperation {
  path: string;
  method: string;
  summary?: string;
  operation_id?: string;
  deprecated?: boolean;
}

export interface WebSocketInfo {
  path: string;
  target_url: string;
  scopes: string[];
}

export interface GroupData {
  name: string;
  prefix: string;
  routes: RouteInfo[];
  websockets: WebSocketInfo[];
}

export interface GroupInfo {
  name: string;
  prefix: string;
  middlewares: string[];
  has_oauth2: boolean;
  has_me_api: boolean;
  route_count: number;
  websocket_count: number;
}
