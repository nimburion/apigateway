package portal

import (
	"embed"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"sort"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/nimburion/apigateway/internal/config"
	"github.com/nimburion/nimburion/pkg/server/router"
)

//go:generate sh -c "cd ../../../developer-portal && npm run build"
//go:generate sh -c "rm -rf dist && mkdir -p dist && cp -R ../../../developer-portal/dist/. dist/"
//go:embed dist
var staticFiles embed.FS

type PortalHandler struct {
	routeConfig *config.Routing
}

type OpenAPIOperation struct {
	Path        string `json:"path"`
	Method      string `json:"method"`
	Summary     string `json:"summary,omitempty"`
	OperationID string `json:"operation_id,omitempty"`
	Deprecated  bool   `json:"deprecated,omitempty"`
}

type OpenAPIInfo struct {
	File        string             `json:"file"`
	Mode        string             `json:"mode"`
	Title       string             `json:"title,omitempty"`
	Version     string             `json:"version,omitempty"`
	Description string             `json:"description,omitempty"`
	Operations  []OpenAPIOperation `json:"operations,omitempty"`
	Error       string             `json:"error,omitempty"`
}

func NewPortalHandler(routeConfig *config.Routing) (*PortalHandler, error) {
	if routeConfig == nil {
		return nil, fmt.Errorf("route config is nil")
	}
	return &PortalHandler{routeConfig: routeConfig}, nil
}

func (h *PortalHandler) GetRoutes(c router.Context) error {
	type MethodInfo struct {
		Method string   `json:"method"`
		Scopes []string `json:"scopes"`
	}

	type RouteInfo struct {
		PathPrefix string       `json:"path_prefix"`
		TargetURL  string       `json:"target_url"`
		Methods    []MethodInfo `json:"methods"`
		OpenAPI    *OpenAPIInfo `json:"openapi,omitempty"`
	}

	type WebSocketInfo struct {
		Path      string   `json:"path"`
		TargetURL string   `json:"target_url"`
		Scopes    []string `json:"scopes"`
	}

	type GroupData struct {
		Name       string          `json:"name"`
		Prefix     string          `json:"prefix"`
		Routes     []RouteInfo     `json:"routes"`
		WebSockets []WebSocketInfo `json:"websockets"`
	}

	groups := []GroupData{}
	openapiCache := make(map[string]*OpenAPIInfo)
	openapiLoader := openapi3.NewLoader()
	openapiLoader.IsExternalRefsAllowed = true

	for groupName, group := range h.routeConfig.Groups {
		groupData := GroupData{
			Name:   groupName,
			Prefix: group.Prefix,
		}

		for _, route := range group.Routes {
			var openapiInfo *OpenAPIInfo
			if route.OpenAPI != nil {
				openapiPath := route.OpenAPI.ResolvedFile
				if openapiPath == "" {
					openapiPath = route.OpenAPI.File
				}
				cacheKey := openapiPath + "|" + route.OpenAPI.Mode
				if cached, ok := openapiCache[cacheKey]; ok {
					openapiInfo = cached
				} else {
					info := &OpenAPIInfo{
						File: openapiPath,
						Mode: route.OpenAPI.Mode,
					}
					if openapiPath == "" {
						info.Error = "openapi.file is empty"
					} else if doc, err := openapiLoader.LoadFromFile(openapiPath); err != nil {
						info.Error = err.Error()
					} else {
						info.Title = doc.Info.Title
						info.Version = doc.Info.Version
						info.Description = doc.Info.Description
						info.Operations = collectOpenAPIOperations(doc)
					}
					openapiCache[cacheKey] = info
					openapiInfo = info
				}
			}
			for _, endpoint := range route.Endpoints {
				methods := []MethodInfo{}
				for method, methodCfg := range endpoint.Methods {
					methods = append(methods, MethodInfo{
						Method: method,
						Scopes: methodCfg.Scopes,
					})
				}
				fullPath := joinRoutePath(route.PathPrefix, endpoint.Path)
				filteredOpenAPI := filterOpenAPIInfo(openapiInfo, fullPath)
				groupData.Routes = append(groupData.Routes, RouteInfo{
					PathPrefix: route.PathPrefix + endpoint.Path,
					TargetURL:  route.TargetURL,
					Methods:    methods,
					OpenAPI:    filteredOpenAPI,
				})
			}
		}

		for _, ws := range group.WebSockets {
			groupData.WebSockets = append(groupData.WebSockets, WebSocketInfo{
				Path:      ws.Path,
				TargetURL: ws.TargetURL,
				Scopes:    ws.Scopes,
			})
		}

		groups = append(groups, groupData)
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"groups": groups,
	})
}

func (h *PortalHandler) GetGroups(c router.Context) error {
	type GroupInfo struct {
		Name        string   `json:"name"`
		Prefix      string   `json:"prefix"`
		Middlewares []string `json:"middlewares"`
		HasOAuth2   bool     `json:"has_oauth2"`
		HasMeAPI    bool     `json:"has_me_api"`
		RouteCount  int      `json:"route_count"`
		WSCount     int      `json:"websocket_count"`
	}

	groups := []GroupInfo{}
	for name, group := range h.routeConfig.Groups {
		info := GroupInfo{
			Name:        name,
			Prefix:      group.Prefix,
			Middlewares: group.Middlewares,
			RouteCount:  len(group.Routes),
			WSCount:     len(group.WebSockets),
		}
		if group.AuthEndpoints != nil {
			info.HasMeAPI = group.AuthEndpoints.Me
			if group.AuthEndpoints.OAuth2 != nil {
				info.HasOAuth2 = group.AuthEndpoints.OAuth2.Enabled
			}
		}
		groups = append(groups, info)
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"groups": groups,
	})
}

func (h *PortalHandler) GetPortalHTML(c router.Context) error {
	// Serve la SPA React dal filesystem embedded
	distFS, err := fs.Sub(staticFiles, "dist")
	if err != nil {
		return err
	}

	// Ottieni il path dalla richiesta
	requestPath := c.Request().URL.Path

	// Rimuovi il prefisso /portal
	filePath := strings.TrimPrefix(requestPath, "/portal")
	// Rimuovi lo slash iniziale se presente
	filePath = strings.TrimPrefix(filePath, "/")

	// Se il path è vuoto, servi index.html
	if filePath == "" {
		filePath = "index.html"
	}

	// Prova ad aprire il file
	file, err := distFS.Open(filePath)
	if err != nil {
		// Se il file non esiste, servi index.html (per SPA routing)
		file, err = distFS.Open("index.html")
		if err != nil {
			return c.JSON(http.StatusNotFound, map[string]string{
				"error": "Portal files not found. Please build the developer-portal first.",
			})
		}
		filePath = "index.html"
	}
	defer file.Close()

	// Leggi il contenuto del file
	content, err := io.ReadAll(file)
	if err != nil {
		return err
	}

	// Determina il content type in base all'estensione
	contentType := "text/html; charset=utf-8"
	if strings.HasSuffix(filePath, ".js") {
		contentType = "application/javascript; charset=utf-8"
	} else if strings.HasSuffix(filePath, ".css") {
		contentType = "text/css; charset=utf-8"
	} else if strings.HasSuffix(filePath, ".json") {
		contentType = "application/json; charset=utf-8"
	} else if strings.HasSuffix(filePath, ".svg") {
		contentType = "image/svg+xml"
	} else if strings.HasSuffix(filePath, ".png") {
		contentType = "image/png"
	} else if strings.HasSuffix(filePath, ".jpg") || strings.HasSuffix(filePath, ".jpeg") {
		contentType = "image/jpeg"
	} else if strings.HasSuffix(filePath, ".ico") {
		contentType = "image/x-icon"
	} else if strings.HasSuffix(filePath, ".woff") || strings.HasSuffix(filePath, ".woff2") {
		contentType = "font/woff2"
	}

	// Imposta gli headers
	c.Response().Header().Set("Content-Type", contentType)

	// Cache solo per assets, non per index.html
	if filePath != "index.html" {
		c.Response().Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		c.Response().Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	}

	c.Response().WriteHeader(http.StatusOK)
	c.Response().Write(content)
	return nil
}

func collectOpenAPIOperations(doc *openapi3.T) []OpenAPIOperation {
	if doc == nil || doc.Paths == nil {
		return nil
	}

	ops := make([]OpenAPIOperation, 0)
	for path, item := range doc.Paths.Map() {
		if item == nil {
			continue
		}
		addOp := func(method string, op *openapi3.Operation) {
			if op == nil {
				return
			}
			ops = append(ops, OpenAPIOperation{
				Path:        path,
				Method:      strings.ToUpper(method),
				Summary:     strings.TrimSpace(op.Summary),
				OperationID: strings.TrimSpace(op.OperationID),
				Deprecated:  op.Deprecated,
			})
		}

		addOp(http.MethodGet, item.Get)
		addOp(http.MethodPost, item.Post)
		addOp(http.MethodPut, item.Put)
		addOp(http.MethodPatch, item.Patch)
		addOp(http.MethodDelete, item.Delete)
		addOp(http.MethodHead, item.Head)
		addOp(http.MethodOptions, item.Options)
		addOp(http.MethodTrace, item.Trace)
	}

	if len(ops) == 0 {
		return nil
	}

	sort.SliceStable(ops, func(i, j int) bool {
		if ops[i].Path == ops[j].Path {
			return ops[i].Method < ops[j].Method
		}
		return ops[i].Path < ops[j].Path
	})

	return ops
}

func filterOpenAPIInfo(info *OpenAPIInfo, fullPath string) *OpenAPIInfo {
	if info == nil {
		return nil
	}

	if len(info.Operations) == 0 || fullPath == "" {
		return info
	}

	normalizedPath := normalizeOpenAPIPath(fullPath)
	if normalizedPath == "" {
		return info
	}

	filtered := make([]OpenAPIOperation, 0, len(info.Operations))
	for _, op := range info.Operations {
		if op.Path == normalizedPath {
			filtered = append(filtered, op)
		}
	}

	if len(filtered) == len(info.Operations) {
		return info
	}

	return &OpenAPIInfo{
		File:        info.File,
		Mode:        info.Mode,
		Title:       info.Title,
		Version:     info.Version,
		Description: info.Description,
		Operations:  filtered,
		Error:       info.Error,
	}
}

func joinRoutePath(prefix, suffix string) string {
	normalizedSuffix := strings.TrimSpace(suffix)
	if normalizedSuffix == "" || normalizedSuffix == "/" {
		return prefix
	}
	if prefix == "/" {
		return normalizedSuffix
	}
	return prefix + normalizedSuffix
}

func normalizeOpenAPIPath(path string) string {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return ""
	}
	if !strings.HasPrefix(trimmed, "/") {
		trimmed = "/" + trimmed
	}
	if trimmed != "/" {
		trimmed = strings.TrimRight(trimmed, "/")
	}

	parts := strings.Split(trimmed, "/")
	for i, part := range parts {
		if strings.HasPrefix(part, ":") && len(part) > 1 {
			parts[i] = "{" + part[1:] + "}"
		}
	}
	return strings.Join(parts, "/")
}

func (h *PortalHandler) AssetCacheMiddleware() router.MiddlewareFunc {
	return func(next router.HandlerFunc) router.HandlerFunc {
		return func(c router.Context) error {
			return next(c)
		}
	}
}

func (h *PortalHandler) StaticMiddleware() router.MiddlewareFunc {
	return func(next router.HandlerFunc) router.HandlerFunc {
		return func(c router.Context) error {
			return next(c)
		}
	}
}
