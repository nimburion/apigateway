package config

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/nimburion/nimburion/pkg/auth"
	"github.com/nimburion/nimburion/pkg/http/router"
	"gopkg.in/yaml.v3"
)

type Routing struct {
	Groups map[string]Group `yaml:"groups" mapstructure:"groups"`
}

func (r Routing) routingGroups() map[string]Group {
	if r.Groups == nil {
		return map[string]Group{}
	}
	return r.Groups
}

type rawRoutingFile struct {
	Routing Routing `yaml:"routes"`
}

const (
	MetadataVisibilityPublic   = "public"
	MetadataVisibilityPartner  = "partner"
	MetadataVisibilityInternal = "internal"

	MetadataStatusActive       = "active"
	MetadataStatusDeprecated   = "deprecated"
	MetadataStatusExperimental = "experimental"
)

type ResourceMetadata struct {
	OwnerTeam      string `yaml:"owner_team" mapstructure:"owner_team"`
	Domain         string `yaml:"domain" mapstructure:"domain"`
	Visibility     string `yaml:"visibility" mapstructure:"visibility"`
	Status         string `yaml:"status" mapstructure:"status"`
	DocsURL        string `yaml:"docs_url" mapstructure:"docs_url"`
	RunbookURL     string `yaml:"runbook_url" mapstructure:"runbook_url"`
	SupportChannel string `yaml:"support_channel" mapstructure:"support_channel"`
}

type Group struct {
	Prefix        string           `yaml:"prefix" mapstructure:"prefix"`
	Metadata      ResourceMetadata `yaml:"metadata" mapstructure:"metadata"`
	Middlewares   []string         `yaml:"middlewares" mapstructure:"middlewares"`
	RateLimit     *RateLimit       `yaml:"rate_limit" mapstructure:"rate_limit"`
	AuthEndpoints *AuthEndpoints   `yaml:"auth_endpoints" mapstructure:"auth_endpoints"`
	Routes        []Route          `yaml:"routes" mapstructure:"routes"`
	WebSockets    []WebSocket      `yaml:"websockets" mapstructure:"websockets"`
}

type AuthEndpoints struct {
	Me     bool          `yaml:"me" mapstructure:"me"`
	OAuth2 *OAuth2Config `yaml:"oauth2" mapstructure:"oauth2"`
}

type OAuth2Config struct {
	Enabled              bool     `yaml:"enabled" mapstructure:"enabled"`
	AuthorizeURL         string   `yaml:"authorize_url" mapstructure:"authorize_url"`
	TokenURL             string   `yaml:"token_url" mapstructure:"token_url"`
	Audience             string   `yaml:"audience" mapstructure:"audience"`
	ClientID             string   `yaml:"client_id" mapstructure:"client_id"`
	ClientSecret         string   `yaml:"client_secret" mapstructure:"client_secret"`
	RedirectURL          string   `yaml:"redirect_url" mapstructure:"redirect_url"`
	Scopes               []string `yaml:"scopes" mapstructure:"scopes"`
	PostLoginRedirectURL string   `yaml:"post_login_redirect_url" mapstructure:"post_login_redirect_url"`
	StateCookieName      string   `yaml:"state_cookie_name" mapstructure:"state_cookie_name"`
	CookieSecure         bool     `yaml:"cookie_secure" mapstructure:"cookie_secure"`
	CookieDomain         string   `yaml:"cookie_domain" mapstructure:"cookie_domain"`
	CookieSameSite       string   `yaml:"cookie_same_site" mapstructure:"cookie_same_site"`
}

func (c *OAuth2Config) SetDefaults() {
	if c.StateCookieName == "" {
		c.StateCookieName = "auth_state"
	}
	if c.CookieSameSite == "" {
		c.CookieSameSite = "lax"
	}
}

func (c OAuth2Config) IsEnabled() bool                 { return c.Enabled }
func (c OAuth2Config) GetAuthorizeURL() string         { return c.AuthorizeURL }
func (c OAuth2Config) GetTokenURL() string             { return c.TokenURL }
func (c OAuth2Config) GetAudience() string             { return c.Audience }
func (c OAuth2Config) GetClientID() string             { return c.ClientID }
func (c OAuth2Config) GetClientSecret() string         { return c.ClientSecret }
func (c OAuth2Config) GetRedirectURL() string          { return c.RedirectURL }
func (c OAuth2Config) GetScopes() []string             { return c.Scopes }
func (c OAuth2Config) GetPostLoginRedirectURL() string { return c.PostLoginRedirectURL }
func (c OAuth2Config) GetStateCookieName() string      { return c.StateCookieName }
func (c OAuth2Config) IsCookieSecure() bool            { return c.CookieSecure }
func (c OAuth2Config) GetCookieDomain() string         { return c.CookieDomain }
func (c OAuth2Config) GetCookieSameSite() string       { return c.CookieSameSite }

func (c OAuth2Config) ToAuthConfig() auth.OAuth2Config {
	return auth.OAuth2Config{
		AuthorizeURL: c.AuthorizeURL,
		TokenURL:     c.TokenURL,
		ClientID:     c.ClientID,
		ClientSecret: c.ClientSecret,
		RedirectURL:  c.RedirectURL,
		Audience:     c.Audience,
		Scopes:       c.Scopes,
	}
}

type Route struct {
	PathPrefix         string           `yaml:"path_prefix" mapstructure:"path_prefix"`
	TargetURL          string           `yaml:"target_url" mapstructure:"target_url"`
	StripPrefix        string           `yaml:"strip_prefix" mapstructure:"strip_prefix"`
	OpenAPI            *OpenAPI         `yaml:"openapi" mapstructure:"openapi"`
	Metadata           ResourceMetadata `yaml:"metadata" mapstructure:"metadata"`
	Middlewares        []string         `yaml:"middlewares" mapstructure:"middlewares"`
	DisableMiddlewares []string         `yaml:"disable_middlewares" mapstructure:"disable_middlewares"`
	RateLimit          *RateLimit       `yaml:"rate_limit" mapstructure:"rate_limit"`
	Endpoints          []Endpoint       `yaml:"endpoints" mapstructure:"endpoints"`
	Group              string           `yaml:"-"`
}

type WebSocket struct {
	Path               string           `yaml:"path"`
	TargetURL          string           `yaml:"target_url"`
	StripPrefix        string           `yaml:"strip_prefix"`
	Metadata           ResourceMetadata `yaml:"metadata" mapstructure:"metadata"`
	Scopes             []string         `yaml:"scopes"`
	Middlewares        []string         `yaml:"middlewares"`
	DisableMiddlewares []string         `yaml:"disable_middlewares"`
	RateLimit          *RateLimit       `yaml:"rate_limit"`
	Group              string           `yaml:"-"`
}

type Endpoint struct {
	Path               string             `yaml:"path" mapstructure:"path"`
	Middlewares        []string           `yaml:"middlewares" mapstructure:"middlewares"`
	DisableMiddlewares []string           `yaml:"disable_middlewares" mapstructure:"disable_middlewares"`
	Methods            map[string]*Method `yaml:"methods" mapstructure:"methods"`
}

type Method struct {
	Scopes             []string   `yaml:"scopes" mapstructure:"scopes"`
	Middlewares        []string   `yaml:"middlewares" mapstructure:"middlewares"`
	DisableMiddlewares []string   `yaml:"disable_middlewares" mapstructure:"disable_middlewares"`
	RateLimit          *RateLimit `yaml:"rate_limit" mapstructure:"rate_limit"`
}

type RateLimit struct {
	RequestsPerSecond int `yaml:"requests_per_second" mapstructure:"requests_per_second"`
	Burst             int `yaml:"burst" mapstructure:"burst"`
}

type OpenAPI struct {
	File         string `yaml:"file" mapstructure:"file"`
	Mode         string `yaml:"mode" mapstructure:"mode"`
	ResolvedFile string `yaml:"-" mapstructure:"-"`
}

const (
	OpenAPIValidationModeStrict   = "strict"
	OpenAPIValidationModeWarnOnly = "warn-only"
)

var supportedMethods = map[string]struct{}{
	http.MethodGet:    {},
	http.MethodPost:   {},
	http.MethodPut:    {},
	http.MethodPatch:  {},
	http.MethodDelete: {},
}

func ResolvePathWithBaseDir(path, baseDir string) (string, error) {
	if filepath.IsAbs(path) {
		return path, nil
	}
	dir := baseDir
	if dir == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("determine routes base directory for %s: %w", path, err)
		}
		dir = cwd
	}
	routeFilePath := filepath.Join(dir, path)
	return routeFilePath, nil
}

// LoadMultipleWithBaseDir loads and merges multiple route files.
// If baseDir is provided, relative paths are resolved relative to baseDir.
// If baseDir is empty, relative paths are resolved from current working directory.
func LoadMultipleWithBaseDir(paths []string, baseDir string, middlewareRegistry map[string]func() router.MiddlewareFunc) (Routing, error) {
	if len(paths) == 0 {
		return Routing{}, fmt.Errorf("no routes files provided")
	}

	supportedMiddlewares := make(map[string]struct{}, len(middlewareRegistry))
	for name := range middlewareRegistry {
		supportedMiddlewares[name] = struct{}{}
	}

	merged := Routing{Groups: make(map[string]Group)}

	for _, path := range paths {
		resolvedPath, err := ResolvePathWithBaseDir(path, baseDir)
		if err != nil {
			return Routing{}, err
		}

		content, err := os.ReadFile(resolvedPath)
		if err != nil {
			return Routing{}, fmt.Errorf("read routes file %s: %w", path, err)
		}

		var raw rawRoutingFile
		if err := yaml.Unmarshal(content, &raw); err != nil {
			return Routing{}, fmt.Errorf("parse routes yaml %s: %w", path, err)
		}

		groups := raw.Routing.routingGroups()
		for groupName, group := range groups {
			if _, exists := merged.Groups[groupName]; exists {
				return Routing{}, fmt.Errorf("duplicate group '%s' found in %s", groupName, path)
			}
			merged.Groups[groupName] = group
		}
	}

	if len(merged.Groups) == 0 {
		return Routing{}, fmt.Errorf("no groups found in routes files")
	}

	return validateAndNormalize(merged, supportedMiddlewares)
}

func validateRateLimit(rl *RateLimit, path string) error {
	if rl.RequestsPerSecond <= 0 {
		return fmt.Errorf("%s.rate_limit.requests_per_second must be > 0", path)
	}
	if rl.Burst <= 0 {
		return fmt.Errorf("%s.rate_limit.burst must be > 0", path)
	}
	return nil
}

func validateMiddlewares(middlewares []string, path string, supportedMiddlewares map[string]struct{}) error {
	for _, mw := range middlewares {
		if _, ok := supportedMiddlewares[mw]; !ok {
			return fmt.Errorf("%s contains unsupported middleware: %s", path, mw)
		}
	}
	return nil
}

func validateResourceMetadata(metadata ResourceMetadata, path string) (ResourceMetadata, error) {
	normalized := ResourceMetadata{
		OwnerTeam:      strings.TrimSpace(metadata.OwnerTeam),
		Domain:         strings.TrimSpace(metadata.Domain),
		Visibility:     strings.ToLower(strings.TrimSpace(metadata.Visibility)),
		Status:         strings.ToLower(strings.TrimSpace(metadata.Status)),
		DocsURL:        strings.TrimSpace(metadata.DocsURL),
		RunbookURL:     strings.TrimSpace(metadata.RunbookURL),
		SupportChannel: strings.TrimSpace(metadata.SupportChannel),
	}

	if normalized.Visibility != "" {
		switch normalized.Visibility {
		case MetadataVisibilityPublic, MetadataVisibilityPartner, MetadataVisibilityInternal:
		default:
			return ResourceMetadata{}, fmt.Errorf("%s.metadata.visibility must be %q, %q, or %q", path, MetadataVisibilityPublic, MetadataVisibilityPartner, MetadataVisibilityInternal)
		}
	}

	if normalized.Status != "" {
		switch normalized.Status {
		case MetadataStatusActive, MetadataStatusDeprecated, MetadataStatusExperimental:
		default:
			return ResourceMetadata{}, fmt.Errorf("%s.metadata.status must be %q, %q, or %q", path, MetadataStatusActive, MetadataStatusDeprecated, MetadataStatusExperimental)
		}
	}

	if normalized.DocsURL != "" {
		if err := validateAbsoluteURL(normalized.DocsURL); err != nil {
			return ResourceMetadata{}, fmt.Errorf("%s.metadata.docs_url must be a valid absolute URL: %w", path, err)
		}
	}

	if normalized.RunbookURL != "" {
		if err := validateAbsoluteURL(normalized.RunbookURL); err != nil {
			return ResourceMetadata{}, fmt.Errorf("%s.metadata.runbook_url must be a valid absolute URL: %w", path, err)
		}
	}

	return normalized, nil
}

func validateAbsoluteURL(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil {
		return err
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return fmt.Errorf("missing scheme or host")
	}
	return nil
}

func validateOpenAPI(openapiCfg *OpenAPI, path string) (*OpenAPI, error) {
	if openapiCfg == nil {
		return nil, nil
	}

	file := strings.TrimSpace(openapiCfg.File)
	if file == "" {
		return nil, fmt.Errorf("%s.openapi.file is required when openapi is configured", path)
	}

	mode := strings.ToLower(strings.TrimSpace(openapiCfg.Mode))
	if mode == "" {
		mode = OpenAPIValidationModeStrict
	}
	if mode != OpenAPIValidationModeStrict && mode != OpenAPIValidationModeWarnOnly {
		return nil, fmt.Errorf("%s.openapi.mode must be '%s' or '%s'", path, OpenAPIValidationModeStrict, OpenAPIValidationModeWarnOnly)
	}

	return &OpenAPI{
		File:         file,
		Mode:         mode,
		ResolvedFile: openapiCfg.ResolvedFile,
	}, nil
}

func validateWebSocket(ws WebSocket, groupName, groupPrefix string, wsIndex int, seenPaths map[string]struct{}, supportedMiddlewares map[string]struct{}) (WebSocket, error) {
	metadata, err := validateResourceMetadata(ws.Metadata, fmt.Sprintf("groups.%s.websockets[%d]", groupName, wsIndex))
	if err != nil {
		return WebSocket{}, err
	}

	path := strings.TrimSpace(ws.Path)
	if path == "" {
		return WebSocket{}, fmt.Errorf("groups.%s.websockets[%d].path is required", groupName, wsIndex)
	}
	if !strings.HasPrefix(path, "/") {
		return WebSocket{}, fmt.Errorf("groups.%s.websockets[%d].path must start with '/'", groupName, wsIndex)
	}
	if path != "/" {
		path = strings.TrimRight(path, "/")
	}
	effectivePath := joinScopedPath(groupPrefix, path)
	if _, exists := seenPaths[effectivePath]; exists {
		return WebSocket{}, fmt.Errorf("duplicate path: %s", effectivePath)
	}
	seenPaths[effectivePath] = struct{}{}

	targetURL := strings.TrimSpace(ws.TargetURL)
	target, err := url.Parse(targetURL)
	if err != nil || target.Scheme == "" || target.Host == "" {
		return WebSocket{}, fmt.Errorf("groups.%s.websockets[%d].target_url must be a valid absolute URL", groupName, wsIndex)
	}
	if target.Scheme != "ws" && target.Scheme != "wss" {
		return WebSocket{}, fmt.Errorf("groups.%s.websockets[%d].target_url must use ws:// or wss:// scheme", groupName, wsIndex)
	}

	stripPrefix := strings.TrimSpace(ws.StripPrefix)
	if stripPrefix != "" {
		if !strings.HasPrefix(stripPrefix, "/") {
			return WebSocket{}, fmt.Errorf("groups.%s.websockets[%d].strip_prefix must start with '/' when set", groupName, wsIndex)
		}
		stripPrefix = strings.TrimRight(stripPrefix, "/")
		if stripPrefix == "" {
			stripPrefix = "/"
		}
	}

	if err := validateMiddlewares(ws.Middlewares, fmt.Sprintf("groups.%s.websockets[%d].middlewares", groupName, wsIndex), supportedMiddlewares); err != nil {
		return WebSocket{}, err
	}
	if err := validateMiddlewares(ws.DisableMiddlewares, fmt.Sprintf("groups.%s.websockets[%d].disable_middlewares", groupName, wsIndex), supportedMiddlewares); err != nil {
		return WebSocket{}, err
	}
	if ws.RateLimit != nil {
		if err := validateRateLimit(ws.RateLimit, fmt.Sprintf("groups.%s.websockets[%d]", groupName, wsIndex)); err != nil {
			return WebSocket{}, err
		}
	}

	return WebSocket{
		Path:               path,
		TargetURL:          targetURL,
		StripPrefix:        stripPrefix,
		Metadata:           metadata,
		Scopes:             ws.Scopes,
		Middlewares:        ws.Middlewares,
		DisableMiddlewares: ws.DisableMiddlewares,
		RateLimit:          ws.RateLimit,
		Group:              groupName,
	}, nil
}

func validateRoute(route Route, groupName, groupPrefix string, routeIndex int, seenPaths map[string]struct{}, supportedMiddlewares map[string]struct{}) (Route, error) {
	metadata, err := validateResourceMetadata(route.Metadata, fmt.Sprintf("groups.%s.routes[%d]", groupName, routeIndex))
	if err != nil {
		return Route{}, err
	}

	pathPrefix := strings.TrimSpace(route.PathPrefix)
	if pathPrefix == "" {
		return Route{}, fmt.Errorf("groups.%s.routes[%d].path_prefix is required", groupName, routeIndex)
	}
	if !strings.HasPrefix(pathPrefix, "/") {
		return Route{}, fmt.Errorf("groups.%s.routes[%d].path_prefix must start with '/'", groupName, routeIndex)
	}
	if pathPrefix != "/" {
		pathPrefix = strings.TrimRight(pathPrefix, "/")
	}
	effectivePath := joinScopedPath(groupPrefix, pathPrefix)
	if _, exists := seenPaths[effectivePath]; exists {
		return Route{}, fmt.Errorf("duplicate path_prefix: %s", effectivePath)
	}
	seenPaths[effectivePath] = struct{}{}

	targetURL := strings.TrimSpace(route.TargetURL)
	target, err := url.Parse(targetURL)
	if err != nil || target.Scheme == "" || target.Host == "" {
		return Route{}, fmt.Errorf("groups.%s.routes[%d].target_url must be a valid absolute URL", groupName, routeIndex)
	}
	if target.Scheme != "http" && target.Scheme != "https" {
		return Route{}, fmt.Errorf("groups.%s.routes[%d].target_url must use http:// or https:// scheme", groupName, routeIndex)
	}

	stripPrefix := strings.TrimSpace(route.StripPrefix)
	if stripPrefix != "" {
		if !strings.HasPrefix(stripPrefix, "/") {
			return Route{}, fmt.Errorf("groups.%s.routes[%d].strip_prefix must start with '/' when set", groupName, routeIndex)
		}
		stripPrefix = strings.TrimRight(stripPrefix, "/")
		if stripPrefix == "" {
			stripPrefix = "/"
		}
	}

	if err := validateMiddlewares(route.Middlewares, fmt.Sprintf("groups.%s.routes[%d].middlewares", groupName, routeIndex), supportedMiddlewares); err != nil {
		return Route{}, err
	}
	if err := validateMiddlewares(route.DisableMiddlewares, fmt.Sprintf("groups.%s.routes[%d].disable_middlewares", groupName, routeIndex), supportedMiddlewares); err != nil {
		return Route{}, err
	}
	if route.RateLimit != nil {
		if err := validateRateLimit(route.RateLimit, fmt.Sprintf("groups.%s.routes[%d]", groupName, routeIndex)); err != nil {
			return Route{}, err
		}
	}

	openapiCfg, err := validateOpenAPI(route.OpenAPI, fmt.Sprintf("groups.%s.routes[%d]", groupName, routeIndex))
	if err != nil {
		return Route{}, err
	}

	if len(route.Endpoints) == 0 {
		return Route{}, fmt.Errorf("groups.%s.routes[%d].endpoints must contain at least one endpoint", groupName, routeIndex)
	}

	normalizedEndpoints := make([]Endpoint, 0, len(route.Endpoints))
	seenEndpointMethod := make(map[string]struct{})

	for endpointIndex, endpoint := range route.Endpoints {
		endpointPath := strings.TrimSpace(endpoint.Path)
		if endpointPath == "" {
			endpointPath = "/"
		}
		if !strings.HasPrefix(endpointPath, "/") {
			return Route{}, fmt.Errorf("groups.%s.routes[%d].endpoints[%d].path must start with '/'", groupName, routeIndex, endpointIndex)
		}

		if err := validateMiddlewares(endpoint.Middlewares, fmt.Sprintf("groups.%s.routes[%d].endpoints[%d].middlewares", groupName, routeIndex, endpointIndex), supportedMiddlewares); err != nil {
			return Route{}, err
		}
		if err := validateMiddlewares(endpoint.DisableMiddlewares, fmt.Sprintf("groups.%s.routes[%d].endpoints[%d].disable_middlewares", groupName, routeIndex, endpointIndex), supportedMiddlewares); err != nil {
			return Route{}, err
		}

		if len(endpoint.Methods) == 0 {
			return Route{}, fmt.Errorf("groups.%s.routes[%d].endpoints[%d].methods must define at least one method", groupName, routeIndex, endpointIndex)
		}

		normalizedMethods := make(map[string]*Method, len(endpoint.Methods))
		for methodName, method := range endpoint.Methods {
			normalizedMethod := strings.ToUpper(strings.TrimSpace(methodName))
			if _, ok := supportedMethods[normalizedMethod]; !ok {
				return Route{}, fmt.Errorf("groups.%s.routes[%d].endpoints[%d].methods.%s is not supported", groupName, routeIndex, endpointIndex, methodName)
			}

			key := endpointPath + "#" + normalizedMethod
			if _, exists := seenEndpointMethod[key]; exists {
				return Route{}, fmt.Errorf("duplicate endpoint method for path %s and method %s", endpointPath, normalizedMethod)
			}
			seenEndpointMethod[key] = struct{}{}

			if method == nil {
				method = &Method{}
			}

			if err := validateMiddlewares(method.Middlewares, fmt.Sprintf("groups.%s.routes[%d].endpoints[%d].methods.%s.middlewares", groupName, routeIndex, endpointIndex, normalizedMethod), supportedMiddlewares); err != nil {
				return Route{}, err
			}
			if err := validateMiddlewares(method.DisableMiddlewares, fmt.Sprintf("groups.%s.routes[%d].endpoints[%d].methods.%s.disable_middlewares", groupName, routeIndex, endpointIndex, normalizedMethod), supportedMiddlewares); err != nil {
				return Route{}, err
			}
			if method.RateLimit != nil {
				if err := validateRateLimit(method.RateLimit, fmt.Sprintf("groups.%s.routes[%d].endpoints[%d].methods.%s", groupName, routeIndex, endpointIndex, normalizedMethod)); err != nil {
					return Route{}, err
				}
			}

			normalizedMethods[normalizedMethod] = method
		}

		normalizedEndpoints = append(normalizedEndpoints, Endpoint{
			Path:               endpointPath,
			Middlewares:        endpoint.Middlewares,
			DisableMiddlewares: endpoint.DisableMiddlewares,
			Methods:            normalizedMethods,
		})
	}

	return Route{
		PathPrefix:         pathPrefix,
		TargetURL:          targetURL,
		StripPrefix:        stripPrefix,
		OpenAPI:            openapiCfg,
		Metadata:           metadata,
		Middlewares:        route.Middlewares,
		DisableMiddlewares: route.DisableMiddlewares,
		RateLimit:          route.RateLimit,
		Endpoints:          normalizedEndpoints,
		Group:              groupName,
	}, nil
}

func validateAndNormalize(r Routing, supportedMiddlewares map[string]struct{}) (Routing, error) {
	groups := r.routingGroups()
	if len(groups) == 0 {
		return Routing{}, fmt.Errorf("routes file must define at least one group")
	}

	normalized := Routing{Groups: make(map[string]Group)}
	seenPaths := make(map[string]struct{})

	for groupName, group := range groups {
		metadata, err := validateResourceMetadata(group.Metadata, fmt.Sprintf("groups.%s", groupName))
		if err != nil {
			return Routing{}, err
		}

		prefix := strings.TrimSpace(group.Prefix)
		if prefix == "" {
			return Routing{}, fmt.Errorf("groups.%s.prefix is required", groupName)
		}
		if !strings.HasPrefix(prefix, "/") {
			return Routing{}, fmt.Errorf("groups.%s.prefix must start with '/'", groupName)
		}
		if prefix != "/" {
			prefix = strings.TrimRight(prefix, "/")
		}

		if err := validateMiddlewares(group.Middlewares, fmt.Sprintf("groups.%s.middlewares", groupName), supportedMiddlewares); err != nil {
			return Routing{}, err
		}
		if group.RateLimit != nil {
			if err := validateRateLimit(group.RateLimit, fmt.Sprintf("groups.%s", groupName)); err != nil {
				return Routing{}, err
			}
		}

		if group.AuthEndpoints != nil && group.AuthEndpoints.OAuth2 != nil {
			oauth2 := group.AuthEndpoints.OAuth2
			if oauth2.Enabled {
				if strings.TrimSpace(oauth2.AuthorizeURL) == "" {
					return Routing{}, fmt.Errorf("groups.%s.auth_endpoints.oauth2.authorize_url is required when enabled", groupName)
				}
				if strings.TrimSpace(oauth2.TokenURL) == "" {
					return Routing{}, fmt.Errorf("groups.%s.auth_endpoints.oauth2.token_url is required when enabled", groupName)
				}
				if strings.TrimSpace(oauth2.ClientID) == "" {
					return Routing{}, fmt.Errorf("groups.%s.auth_endpoints.oauth2.client_id is required when enabled", groupName)
				}
				if strings.TrimSpace(oauth2.RedirectURL) == "" {
					return Routing{}, fmt.Errorf("groups.%s.auth_endpoints.oauth2.redirect_url is required when enabled", groupName)
				}
			}
		}

		if len(group.Routes) == 0 && len(group.WebSockets) == 0 {
			return Routing{}, fmt.Errorf("groups.%s must define at least one route or websocket", groupName)
		}

		normalizedRoutes := make([]Route, 0, len(group.Routes))
		for routeIndex, route := range group.Routes {
			normalizedRoute, err := validateRoute(route, groupName, prefix, routeIndex, seenPaths, supportedMiddlewares)
			if err != nil {
				return Routing{}, err
			}
			normalizedRoutes = append(normalizedRoutes, normalizedRoute)
		}

		normalizedWebSockets := make([]WebSocket, 0, len(group.WebSockets))
		for wsIndex, ws := range group.WebSockets {
			normalizedWS, err := validateWebSocket(ws, groupName, prefix, wsIndex, seenPaths, supportedMiddlewares)
			if err != nil {
				return Routing{}, err
			}
			normalizedWebSockets = append(normalizedWebSockets, normalizedWS)
		}

		normalized.Groups[groupName] = Group{
			Prefix:        prefix,
			Metadata:      metadata,
			Middlewares:   group.Middlewares,
			RateLimit:     group.RateLimit,
			AuthEndpoints: group.AuthEndpoints,
			Routes:        normalizedRoutes,
			WebSockets:    normalizedWebSockets,
		}
	}

	return normalized, nil
}

func joinScopedPath(groupPrefix, localPath string) string {
	group := strings.TrimSpace(groupPrefix)
	if group == "" {
		group = "/"
	}
	if !strings.HasPrefix(group, "/") {
		group = "/" + group
	}
	if group != "/" {
		group = strings.TrimRight(group, "/")
	}

	local := strings.TrimSpace(localPath)
	if local == "" || local == "/" {
		return group
	}
	if !strings.HasPrefix(local, "/") {
		local = "/" + local
	}
	if local != "/" {
		local = strings.TrimRight(local, "/")
	}
	if group == "/" {
		return local
	}
	return group + local
}
