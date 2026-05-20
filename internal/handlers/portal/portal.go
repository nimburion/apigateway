package portal

import (
	"embed"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"path"
	"sort"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/nimburion/apigateway/internal/config"
	"github.com/nimburion/apigateway/internal/routing"
	httpopenapi "github.com/nimburion/nimburion/pkg/http/openapi"
	"github.com/nimburion/nimburion/pkg/http/router"
)

//go:generate sh -c "cd ../../../developer-portal && npm run build"
//go:generate sh -c "rm -rf dist && mkdir -p dist && cp -R ../../../developer-portal/dist/. dist/"
//go:embed dist
var staticFiles embed.FS

type PortalHandler struct {
	routeConfig         *config.Routing
	routeConfigProvider func() config.Routing
	portalConfig        config.PortalConfig
	runtimeInfo         RuntimeInfo
	runtimeRoutes       []RuntimeRoute
	metricsHistoryStore MetricsHistoryStore
}

type RuntimeRoute struct {
	Route          httpopenapi.Route
	Metadata       config.ResourceMetadata
	AuthRequired   bool
	Scopes         []string
	HasRateLimit   bool
	GroupName      string
	GroupPrefix    string
	SurfaceContext string
}

type RuntimeInfo struct {
	AuthEnabled           bool     `json:"auth_enabled"`
	ManagementEnabled     bool     `json:"management_enabled"`
	ManagementAuthEnabled bool     `json:"management_auth_enabled"`
	PortalMode            string   `json:"portal_mode"`
	FrameworkMiddlewares  []string `json:"framework_middlewares"`
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

type RateLimitInfo struct {
	RequestsPerSecond int    `json:"requests_per_second"`
	Burst             int    `json:"burst"`
	Source            string `json:"source"`
}

type MethodInfo struct {
	Method              string         `json:"method"`
	Scopes              []string       `json:"scopes"`
	Middlewares         []string       `json:"middlewares"`
	DeclaredMiddlewares []string       `json:"declared_middlewares"`
	DisabledMiddlewares []string       `json:"disabled_middlewares"`
	AuthRequired        bool           `json:"auth_required"`
	HasRateLimit        bool           `json:"has_rate_limit"`
	RateLimit           *RateLimitInfo `json:"rate_limit,omitempty"`
}

type RouteInfo struct {
	PathPrefix          string                  `json:"path_prefix"`
	TargetURL           string                  `json:"target_url,omitempty"`
	Methods             []MethodInfo            `json:"methods"`
	OpenAPI             *OpenAPIInfo            `json:"openapi,omitempty"`
	Metadata            config.ResourceMetadata `json:"metadata"`
	Middlewares         []string                `json:"middlewares"`
	DeclaredMiddlewares []string                `json:"declared_middlewares"`
	DisabledMiddlewares []string                `json:"disabled_middlewares"`
	EndpointMiddlewares []string                `json:"endpoint_middlewares"`
	EndpointDisabledMws []string                `json:"endpoint_disabled_middlewares"`
	AuthRequired        bool                    `json:"auth_required"`
	HasOpenAPI          bool                    `json:"has_openapi"`
	HasRateLimit        bool                    `json:"has_rate_limit"`
	RateLimit           *RateLimitInfo          `json:"rate_limit,omitempty"`
	Deprecated          bool                    `json:"deprecated"`
	HasOpenAPIErrors    bool                    `json:"has_openapi_errors"`
	ExposesTargetURL    bool                    `json:"exposes_target_url"`
	ExposesOpenAPIErrs  bool                    `json:"exposes_openapi_errors"`
	RuntimeOnly         bool                    `json:"runtime_only"`
	SurfaceContext      string                  `json:"surface_context"`
}

type WebSocketInfo struct {
	Path                string                  `json:"path"`
	TargetURL           string                  `json:"target_url,omitempty"`
	Scopes              []string                `json:"scopes"`
	Metadata            config.ResourceMetadata `json:"metadata"`
	Middlewares         []string                `json:"middlewares"`
	DeclaredMiddlewares []string                `json:"declared_middlewares"`
	DisabledMiddlewares []string                `json:"disabled_middlewares"`
	AuthRequired        bool                    `json:"auth_required"`
	HasRateLimit        bool                    `json:"has_rate_limit"`
	RateLimit           *RateLimitInfo          `json:"rate_limit,omitempty"`
	Deprecated          bool                    `json:"deprecated"`
	ExposesTarget       bool                    `json:"exposes_target_url"`
}

type GroupData struct {
	Name                   string                  `json:"name"`
	Prefix                 string                  `json:"prefix"`
	Metadata               config.ResourceMetadata `json:"metadata"`
	Middlewares            []string                `json:"middlewares"`
	AuthRequired           bool                    `json:"auth_required"`
	HasRateLimit           bool                    `json:"has_rate_limit"`
	HasRateLimitedSurfaces bool                    `json:"has_rate_limited_surfaces"`
	Deprecated             bool                    `json:"deprecated"`
	Routes                 []RouteInfo             `json:"routes"`
	WebSockets             []WebSocketInfo         `json:"websockets"`
	RateLimit              *RateLimitInfo          `json:"rate_limit,omitempty"`
}

type GroupInfo struct {
	Name         string                  `json:"name"`
	Prefix       string                  `json:"prefix"`
	Metadata     config.ResourceMetadata `json:"metadata"`
	Middlewares  []string                `json:"middlewares"`
	HasOAuth2    bool                    `json:"has_oauth2"`
	HasMeAPI     bool                    `json:"has_me_api"`
	RouteCount   int                     `json:"route_count"`
	WSCount      int                     `json:"websocket_count"`
	AuthRequired bool                    `json:"auth_required"`
	HasOpenAPI   bool                    `json:"has_openapi"`
	HasRateLimit bool                    `json:"has_rate_limit"`
	RateLimit    *RateLimitInfo          `json:"rate_limit,omitempty"`
	Deprecated   bool                    `json:"deprecated"`
	RuntimeInfo  RuntimeInfo             `json:"runtime_info"`
}

func newRateLimitInfo(rl *config.RateLimit, source string) *RateLimitInfo {
	if rl == nil {
		return nil
	}
	return &RateLimitInfo{
		RequestsPerSecond: rl.RequestsPerSecond,
		Burst:             rl.Burst,
		Source:            source,
	}
}

func NewPortalHandler(routeConfig *config.Routing, portalConfig *config.PortalConfig, runtimeInfo *RuntimeInfo, metricsHistoryStore MetricsHistoryStore, runtimeRoutes ...RuntimeRoute) (*PortalHandler, error) {
	if routeConfig == nil {
		return nil, fmt.Errorf("route config is nil")
	}
	cfg := config.NewDefaultConfig().Portal
	if portalConfig != nil {
		cfg = *portalConfig
	}
	info := RuntimeInfo{PortalMode: cfg.Mode}
	if runtimeInfo != nil {
		info = *runtimeInfo
		if info.PortalMode == "" {
			info.PortalMode = cfg.Mode
		}
	}
	return &PortalHandler{
		routeConfig:         routeConfig,
		portalConfig:        cfg,
		runtimeInfo:         info,
		runtimeRoutes:       append([]RuntimeRoute(nil), runtimeRoutes...),
		metricsHistoryStore: metricsHistoryStore,
	}, nil
}

func (h *PortalHandler) SetRouteConfigProvider(provider func() config.Routing) {
	h.routeConfigProvider = provider
}

func (h *PortalHandler) currentRouteConfig() config.Routing {
	if h.routeConfigProvider != nil {
		return h.routeConfigProvider()
	}
	if h.routeConfig == nil {
		return config.Routing{}
	}
	return *h.routeConfig
}

func (h *PortalHandler) GetMetricsHistory(c router.Context) error {
	if h.metricsHistoryStore == nil {
		return c.JSON(http.StatusOK, PortalMetricsHistoryResponse{
			Source:             "disabled",
			SnapshotCount:      0,
			SnapshotIntervalMs: h.portalConfig.MetricsHistory.SnapshotInterval.Milliseconds(),
			RetentionMs:        h.portalConfig.MetricsHistory.MaxAge.Milliseconds(),
			Snapshots:          []PortalMetricsSnapshot{},
		})
	}
	return c.JSON(http.StatusOK, h.metricsHistoryStore.Read())
}

func (h *PortalHandler) GetSummary(c router.Context) error {
	routeConfig := h.currentRouteConfig()
	groupCount := len(routeConfig.Groups)
	routeCount := 0
	websocketCount := 0
	for _, group := range routeConfig.Groups {
		routeCount += len(group.Routes)
		websocketCount += len(group.WebSockets)
	}
	return c.JSON(http.StatusOK, map[string]any{
		"groups":       groupCount,
		"routes":       routeCount,
		"websockets":   websocketCount,
		"runtime_info": h.runtimeInfo,
	})
}

func (h *PortalHandler) GetRoutes(c router.Context) error {
	groups := []GroupData{}
	seenRuntimeRoutes := make(map[string]struct{})
	configuredMethodsByPath := make(map[string]map[string]struct{})
	openapiCache := make(map[string]*OpenAPIInfo)
	openapiLoader := openapi3.NewLoader()
	openapiLoader.IsExternalRefsAllowed = true
	exposeTargetURLs := h.portalConfig.Catalog.ExposeTargetURLs
	exposeOpenAPIErrors := h.portalConfig.Catalog.ExposeOpenAPIErrors

	routeConfig := h.currentRouteConfig()
	for groupName, group := range routeConfig.Groups {
		groupData := GroupData{
			Name:                   groupName,
			Prefix:                 group.Prefix,
			Metadata:               group.Metadata,
			Middlewares:            append([]string(nil), group.Middlewares...),
			AuthRequired:           requiresAuth(group.Middlewares, nil),
			HasRateLimit:           group.RateLimit != nil,
			HasRateLimitedSurfaces: false,
			RateLimit:              newRateLimitInfo(group.RateLimit, "group"),
			Deprecated:             isDeprecated(group.Metadata),
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
			routeMiddlewares := routing.ApplyMiddlewareDirectives(group.Middlewares, route.Middlewares, route.DisableMiddlewares)
			for _, endpoint := range route.Endpoints {
				methods := []MethodInfo{}
				routeAuthRequired := requiresAuth(routeMiddlewares, nil)
				routeHasRateLimit := group.RateLimit != nil || route.RateLimit != nil
				routeDeprecated := isDeprecated(route.Metadata)
				effectiveRouteRateLimit := group.RateLimit
				routeRateLimitSource := "group"
				if route.RateLimit != nil {
					effectiveRouteRateLimit = route.RateLimit
					routeRateLimitSource = "route"
				}
				for method, methodCfg := range endpoint.Methods {
					methodMiddlewares := routing.ApplyMiddlewareDirectives(routeMiddlewares, methodCfg.Middlewares, methodCfg.DisableMiddlewares)
					methodAuthRequired := requiresAuth(methodMiddlewares, methodCfg.Scopes)
					if methodAuthRequired {
						routeAuthRequired = true
					}
					if methodCfg.RateLimit != nil {
						routeHasRateLimit = true
					}
					methods = append(methods, MethodInfo{
						Method:              method,
						Scopes:              append([]string(nil), methodCfg.Scopes...),
						Middlewares:         methodMiddlewares,
						DeclaredMiddlewares: append([]string(nil), methodCfg.Middlewares...),
						DisabledMiddlewares: append([]string(nil), methodCfg.DisableMiddlewares...),
						AuthRequired:        methodAuthRequired,
						HasRateLimit:        methodCfg.RateLimit != nil || route.RateLimit != nil || group.RateLimit != nil,
						RateLimit: func() *RateLimitInfo {
							if methodCfg.RateLimit != nil {
								return newRateLimitInfo(methodCfg.RateLimit, "method")
							}
							return newRateLimitInfo(effectiveRouteRateLimit, routeRateLimitSource)
						}(),
					})
				}
				fullPath := joinRoutePath(route.PathPrefix, endpoint.Path)
				effectivePath := joinRoutePath(group.Prefix, fullPath)
				filteredOpenAPI := sanitizeOpenAPIInfo(filterOpenAPIInfo(openapiInfo, fullPath), exposeOpenAPIErrors)
				hasOpenAPIErrors := filteredOpenAPI != nil && strings.TrimSpace(filteredOpenAPI.Error) != ""
				if filteredOpenAPI != nil {
					for _, op := range filteredOpenAPI.Operations {
						if op.Deprecated {
							routeDeprecated = true
							break
						}
					}
				}
				groupData.Routes = append(groupData.Routes, RouteInfo{
					PathPrefix:          route.PathPrefix + endpoint.Path,
					TargetURL:           conditionalString(exposeTargetURLs, route.TargetURL),
					Methods:             methods,
					OpenAPI:             filteredOpenAPI,
					Metadata:            route.Metadata,
					Middlewares:         routeMiddlewares,
					DeclaredMiddlewares: append([]string(nil), route.Middlewares...),
					DisabledMiddlewares: append([]string(nil), route.DisableMiddlewares...),
					EndpointMiddlewares: append([]string(nil), endpoint.Middlewares...),
					EndpointDisabledMws: append([]string(nil), endpoint.DisableMiddlewares...),
					AuthRequired:        routeAuthRequired,
					HasOpenAPI:          route.OpenAPI != nil,
					HasRateLimit:        routeHasRateLimit,
					RateLimit:           newRateLimitInfo(effectiveRouteRateLimit, routeRateLimitSource),
					Deprecated:          routeDeprecated,
					HasOpenAPIErrors:    hasOpenAPIErrors,
					ExposesTargetURL:    exposeTargetURLs,
					ExposesOpenAPIErrs:  exposeOpenAPIErrors,
					SurfaceContext:      "public",
				})
				for _, methodInfo := range methods {
					seenRuntimeRoutes[runtimeRouteKey(methodInfo.Method, effectivePath)] = struct{}{}
					configuredForPath := configuredMethodsByPath[normalizeRuntimePath(effectivePath)]
					if configuredForPath == nil {
						configuredForPath = make(map[string]struct{})
						configuredMethodsByPath[normalizeRuntimePath(effectivePath)] = configuredForPath
					}
					configuredForPath[strings.ToUpper(strings.TrimSpace(methodInfo.Method))] = struct{}{}
				}
			}
		}

		for _, ws := range group.WebSockets {
			wsMiddlewares := routing.ApplyMiddlewareDirectives(group.Middlewares, ws.Middlewares, ws.DisableMiddlewares)
			authRequired := requiresAuth(wsMiddlewares, ws.Scopes)
			groupData.WebSockets = append(groupData.WebSockets, WebSocketInfo{
				Path:                ws.Path,
				TargetURL:           conditionalString(exposeTargetURLs, ws.TargetURL),
				Scopes:              append([]string(nil), ws.Scopes...),
				Metadata:            ws.Metadata,
				Middlewares:         wsMiddlewares,
				DeclaredMiddlewares: append([]string(nil), ws.Middlewares...),
				DisabledMiddlewares: append([]string(nil), ws.DisableMiddlewares...),
				AuthRequired:        authRequired,
				HasRateLimit:        ws.RateLimit != nil || group.RateLimit != nil,
				RateLimit: func() *RateLimitInfo {
					if ws.RateLimit != nil {
						return newRateLimitInfo(ws.RateLimit, "websocket")
					}
					return newRateLimitInfo(group.RateLimit, "group")
				}(),
				Deprecated:    isDeprecated(ws.Metadata),
				ExposesTarget: exposeTargetURLs,
			})
			if authRequired {
				groupData.AuthRequired = true
			}
			if ws.RateLimit != nil {
				groupData.HasRateLimitedSurfaces = true
			}
			if isDeprecated(ws.Metadata) {
				groupData.Deprecated = true
			}
		}

		for _, routeInfo := range groupData.Routes {
			if routeInfo.AuthRequired {
				groupData.AuthRequired = true
			}
			if routeInfo.HasRateLimit {
				groupData.HasRateLimitedSurfaces = true
			}
			if routeInfo.Deprecated {
				groupData.Deprecated = true
			}
		}

		groups = append(groups, groupData)
	}

	groups = h.appendRuntimeOnlyRoutes(groups, seenRuntimeRoutes, configuredMethodsByPath)
	sort.SliceStable(groups, func(i, j int) bool {
		if strings.EqualFold(groups[i].Name, "default") {
			return true
		}
		if strings.EqualFold(groups[j].Name, "default") {
			return false
		}
		return groups[i].Name < groups[j].Name
	})

	return c.JSON(http.StatusOK, map[string]interface{}{
		"groups": groups,
	})
}

func (h *PortalHandler) appendRuntimeOnlyRoutes(groups []GroupData, seen map[string]struct{}, configuredMethodsByPath map[string]map[string]struct{}) []GroupData {
	if len(h.runtimeRoutes) == 0 {
		return groups
	}

	groupIndex := make(map[string]int, len(groups))
	for i, group := range groups {
		groupIndex[group.Name] = i
	}

	for _, runtimeRoute := range h.runtimeRoutes {
		route := runtimeRoute.Route
		key := runtimeRouteKey(route.Method, route.Path)
		if _, ok := seen[key]; ok {
			continue
		}
		if isSyntheticMethodNotAllowedRoute(runtimeRoute, configuredMethodsByPath) {
			continue
		}

		groupName := strings.TrimSpace(runtimeRoute.GroupName)
		groupPrefix := strings.TrimSpace(runtimeRoute.GroupPrefix)
		if groupName == "" {
			groupName, groupPrefix = h.matchRuntimeRouteGroup(route.Path)
		}
		if groupName == "" {
			groupName = "runtime"
			groupPrefix = "/"
		}

		index, ok := groupIndex[groupName]
		if !ok {
			groups = append(groups, GroupData{
				Name:         groupName,
				Prefix:       groupPrefix,
				Metadata:     config.ResourceMetadata{Domain: "runtime", Status: "active"},
				Middlewares:  nil,
				AuthRequired: false,
				HasRateLimit: false,
				Deprecated:   false,
				Routes:       nil,
				WebSockets:   nil,
			})
			index = len(groups) - 1
			groupIndex[groupName] = index
		}

		methodSummary := route.Annotations.Summary
		openapiInfo := &OpenAPIInfo{
			Title:       "Runtime route",
			Description: strings.TrimSpace(route.Annotations.Description),
			Operations: []OpenAPIOperation{
				{
					Path:        route.Path,
					Method:      route.Method,
					Summary:     methodSummary,
					OperationID: strings.TrimSpace(route.Annotations.OperationID),
				},
			},
		}
		if openapiInfo.Description == "" && methodSummary == "" {
			openapiInfo = nil
		}
		metadata := runtimeRoute.Metadata
		if metadata.Domain == "" {
			metadata.Domain = "runtime"
		}
		if metadata.Status == "" {
			metadata.Status = "active"
		}

		groups[index].Routes = append(groups[index].Routes, RouteInfo{
			PathPrefix:          route.Path,
			TargetURL:           "",
			Methods:             []MethodInfo{{Method: route.Method, Scopes: append([]string(nil), runtimeRoute.Scopes...), AuthRequired: runtimeRoute.AuthRequired, HasRateLimit: runtimeRoute.HasRateLimit}},
			OpenAPI:             openapiInfo,
			Metadata:            metadata,
			Middlewares:         nil,
			DeclaredMiddlewares: nil,
			DisabledMiddlewares: nil,
			EndpointMiddlewares: nil,
			EndpointDisabledMws: nil,
			AuthRequired:        runtimeRoute.AuthRequired,
			HasOpenAPI:          openapiInfo != nil,
			HasRateLimit:        runtimeRoute.HasRateLimit,
			Deprecated:          false,
			HasOpenAPIErrors:    false,
			ExposesTargetURL:    false,
			ExposesOpenAPIErrs:  false,
			RuntimeOnly:         true,
			SurfaceContext:      runtimeSurfaceContext(runtimeRoute),
		})
	}

	for i := range groups {
		sort.SliceStable(groups[i].Routes, func(left, right int) bool {
			if groups[i].Routes[left].PathPrefix == groups[i].Routes[right].PathPrefix {
				if len(groups[i].Routes[left].Methods) == 0 || len(groups[i].Routes[right].Methods) == 0 {
					return groups[i].Routes[left].PathPrefix < groups[i].Routes[right].PathPrefix
				}
				return groups[i].Routes[left].Methods[0].Method < groups[i].Routes[right].Methods[0].Method
			}
			return groups[i].Routes[left].PathPrefix < groups[i].Routes[right].PathPrefix
		})
	}

	return groups
}

func runtimeSurfaceContext(route RuntimeRoute) string {
	if strings.TrimSpace(route.SurfaceContext) != "" {
		return strings.TrimSpace(route.SurfaceContext)
	}
	return "public"
}

func normalizeRuntimePath(path string) string {
	normalized := strings.TrimSpace(path)
	if normalized == "" {
		return "/"
	}
	if len(normalized) > 1 {
		normalized = strings.TrimSuffix(normalized, "/")
	}
	if normalized == "" {
		return "/"
	}
	return normalized
}

func isSyntheticMethodNotAllowedRoute(route RuntimeRoute, configuredMethodsByPath map[string]map[string]struct{}) bool {
	if strings.TrimSpace(route.Route.Annotations.Summary) != "" ||
		strings.TrimSpace(route.Route.Annotations.Description) != "" ||
		strings.TrimSpace(route.Route.Annotations.OperationID) != "" ||
		len(route.Route.Annotations.Tags) > 0 ||
		route.Metadata != (config.ResourceMetadata{}) ||
		route.AuthRequired ||
		len(route.Scopes) > 0 ||
		route.HasRateLimit {
		return false
	}

	allowedMethods := configuredMethodsByPath[normalizeRuntimePath(route.Route.Path)]
	if len(allowedMethods) == 0 {
		return false
	}
	_, ok := allowedMethods[strings.ToUpper(strings.TrimSpace(route.Route.Method))]
	return !ok
}

func (h *PortalHandler) matchRuntimeRouteGroup(path string) (string, string) {
	bestName := ""
	bestPrefix := ""
	bestLen := -1
	routeConfig := h.currentRouteConfig()
	for groupName, group := range routeConfig.Groups {
		prefix := strings.TrimSpace(group.Prefix)
		if prefix == "" {
			prefix = "/"
		}
		if prefix == "/" || strings.HasPrefix(path, prefix) {
			if len(prefix) > bestLen {
				bestName = groupName
				bestPrefix = prefix
				bestLen = len(prefix)
			}
		}
	}
	return bestName, bestPrefix
}

func runtimeRouteKey(method, path string) string {
	normalizedPath := strings.TrimSpace(path)
	if normalizedPath == "" {
		normalizedPath = "/"
	}
	if len(normalizedPath) > 1 {
		normalizedPath = strings.TrimSuffix(normalizedPath, "/")
	}
	return strings.ToUpper(strings.TrimSpace(method)) + " " + normalizedPath
}

func (h *PortalHandler) GetGroups(c router.Context) error {
	groups := []GroupInfo{}
	routeConfig := h.currentRouteConfig()
	for name, group := range routeConfig.Groups {
		info := GroupInfo{
			Name:         name,
			Prefix:       group.Prefix,
			Metadata:     group.Metadata,
			Middlewares:  append([]string(nil), group.Middlewares...),
			RouteCount:   len(group.Routes),
			WSCount:      len(group.WebSockets),
			AuthRequired: requiresAuth(group.Middlewares, nil),
			HasRateLimit: group.RateLimit != nil,
			Deprecated:   isDeprecated(group.Metadata),
			RuntimeInfo:  h.runtimeInfo,
		}
		if group.AuthEndpoints != nil {
			info.HasMeAPI = group.AuthEndpoints.Me
			if group.AuthEndpoints.OAuth2 != nil {
				info.HasOAuth2 = group.AuthEndpoints.OAuth2.Enabled
			}
		}
		for _, route := range group.Routes {
			info.HasOpenAPI = info.HasOpenAPI || route.OpenAPI != nil
			info.Deprecated = info.Deprecated || isDeprecated(route.Metadata)
			info.HasRateLimit = info.HasRateLimit || route.RateLimit != nil
			routeMiddlewares := routing.ApplyMiddlewareDirectives(group.Middlewares, route.Middlewares, route.DisableMiddlewares)
			info.AuthRequired = info.AuthRequired || requiresAuth(routeMiddlewares, nil)
			for _, endpoint := range route.Endpoints {
				for _, methodCfg := range endpoint.Methods {
					methodMiddlewares := routing.ApplyMiddlewareDirectives(routeMiddlewares, methodCfg.Middlewares, methodCfg.DisableMiddlewares)
					info.AuthRequired = info.AuthRequired || requiresAuth(methodMiddlewares, methodCfg.Scopes)
					info.HasRateLimit = info.HasRateLimit || methodCfg.RateLimit != nil
				}
			}
		}
		for _, ws := range group.WebSockets {
			wsMiddlewares := routing.ApplyMiddlewareDirectives(group.Middlewares, ws.Middlewares, ws.DisableMiddlewares)
			info.AuthRequired = info.AuthRequired || requiresAuth(wsMiddlewares, ws.Scopes)
			info.HasRateLimit = info.HasRateLimit || ws.RateLimit != nil
			info.Deprecated = info.Deprecated || isDeprecated(ws.Metadata)
		}
		groups = append(groups, info)
	}
	groups = h.appendRuntimeOnlyGroupInfo(groups)

	return c.JSON(http.StatusOK, map[string]interface{}{
		"groups": groups,
	})
}

func (h *PortalHandler) appendRuntimeOnlyGroupInfo(groups []GroupInfo) []GroupInfo {
	if len(h.runtimeRoutes) == 0 {
		return groups
	}

	groupIndex := make(map[string]int, len(groups))
	for i := range groups {
		groupIndex[groups[i].Name] = i
	}

	for _, runtimeRoute := range h.runtimeRoutes {
		groupName := strings.TrimSpace(runtimeRoute.GroupName)
		groupPrefix := strings.TrimSpace(runtimeRoute.GroupPrefix)
		if groupName == "" {
			groupName, groupPrefix = h.matchRuntimeRouteGroup(runtimeRoute.Route.Path)
		}
		if groupName == "" {
			groupName = "runtime"
			groupPrefix = "/"
		}

		index, ok := groupIndex[groupName]
		if !ok {
			metadata := runtimeRoute.Metadata
			if metadata.Domain == "" {
				metadata.Domain = runtimeSurfaceContext(runtimeRoute)
			}
			if metadata.Status == "" {
				metadata.Status = "active"
			}
			groups = append(groups, GroupInfo{
				Name:         groupName,
				Prefix:       groupPrefix,
				Metadata:     metadata,
				RouteCount:   0,
				WSCount:      0,
				AuthRequired: false,
				HasOpenAPI:   false,
				HasRateLimit: false,
				Deprecated:   false,
				RuntimeInfo:  h.runtimeInfo,
			})
			index = len(groups) - 1
			groupIndex[groupName] = index
		}

		groups[index].RouteCount++
		groups[index].AuthRequired = groups[index].AuthRequired || runtimeRoute.AuthRequired
		groups[index].HasOpenAPI = groups[index].HasOpenAPI || strings.TrimSpace(runtimeRoute.Route.Annotations.Summary) != "" || strings.TrimSpace(runtimeRoute.Route.Annotations.OperationID) != ""
		groups[index].HasRateLimit = groups[index].HasRateLimit || runtimeRoute.HasRateLimit
	}

	sort.SliceStable(groups, func(i, j int) bool {
		if strings.EqualFold(groups[i].Name, "default") {
			return true
		}
		if strings.EqualFold(groups[j].Name, "default") {
			return false
		}
		return groups[i].Name < groups[j].Name
	})

	return groups
}

func (h *PortalHandler) GetPortalHTML(c router.Context) error {
	// Serve la SPA React dal filesystem embedded
	distFS, err := fs.Sub(staticFiles, "dist")
	if err != nil {
		return err
	}

	// Ottieni il path dalla richiesta. EscapedPath preserva segmenti encoded come %2e%2e.
	requestPath := c.Request().URL.EscapedPath()
	if requestPath == "" {
		requestPath = c.Request().URL.Path
	}

	// Rimuovi il prefisso /portal
	filePath := strings.TrimPrefix(requestPath, "/portal")
	// Rimuovi lo slash iniziale se presente
	filePath = strings.TrimPrefix(filePath, "/")
	filePath, valid := sanitizePortalFilePath(filePath)
	if !valid {
		return c.JSON(http.StatusNotFound, map[string]string{
			"error": "invalid portal asset path",
		})
	}

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

func sanitizePortalFilePath(filePath string) (string, bool) {
	trimmed := strings.TrimSpace(filePath)
	if trimmed == "" {
		return "", true
	}
	for _, segment := range strings.Split(trimmed, "/") {
		if segment == ".." || strings.EqualFold(segment, "%2e%2e") {
			return "", false
		}
	}
	cleaned := path.Clean("/" + trimmed)
	if strings.HasPrefix(cleaned, "/../") || cleaned == "/.." {
		return "", false
	}
	cleaned = strings.TrimPrefix(cleaned, "/")
	if cleaned == "." {
		return "", true
	}
	if strings.HasPrefix(cleaned, "../") || cleaned == ".." {
		return "", false
	}
	return cleaned, true
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

func sanitizeOpenAPIInfo(info *OpenAPIInfo, exposeErrors bool) *OpenAPIInfo {
	if info == nil {
		return nil
	}
	clone := *info
	clone.Operations = append([]OpenAPIOperation(nil), info.Operations...)
	if !exposeErrors {
		clone.Error = ""
	}
	return &clone
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

func conditionalString(expose bool, value string) string {
	if !expose {
		return ""
	}
	return value
}

func requiresAuth(middlewares, scopes []string) bool {
	if len(scopes) > 0 {
		return true
	}
	for _, middlewareName := range middlewares {
		switch middlewareName {
		case "Authenticate", "ClaimsGuardFromConfig":
			return true
		}
	}
	return false
}

func isDeprecated(metadata config.ResourceMetadata) bool {
	return metadata.Status == config.MetadataStatusDeprecated
}
