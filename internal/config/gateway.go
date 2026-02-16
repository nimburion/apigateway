package config

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/nimburion/nimburion/pkg/server/router"
)

//go:generate go run ../tools/configschema
type Gateway struct {
	Routes      Routing  `mapstructure:"routes"`
	RoutesFiles []string `mapstructure:"routes_files"`
	ConfigDir   string   `mapstructure:"-"`
}

func NewDefaultConfig() (config *Gateway) {
	config = &Gateway{
		Routes:      Routing{},
		RoutesFiles: nil,
		ConfigDir:   "",
	}
	return
}

func (cfg *Gateway) Validate() error {
	if len(cfg.RoutesFiles) == 0 && len(cfg.Routes.Groups) == 0 {
		return errors.New("either routes_files or inline routes must be provided")
	}
	for i, path := range cfg.RoutesFiles {
		trimmed := strings.TrimSpace(path)
		if trimmed == "" {
			return fmt.Errorf("routes_files[%d] cannot be empty", i)
		}
		cfg.RoutesFiles[i] = trimmed
	}
	return nil
}

func (cfg *Gateway) LoadRoutes(baseDir string, middlewareRegistry map[string]func() router.MiddlewareFunc) error {
	if len(cfg.RoutesFiles) == 0 && len(cfg.Routes.Groups) == 0 {
		return errors.New("routes configuration must define at least one group")
	}

	merged := cfg.Routes
	if merged.Groups == nil {
		merged.Groups = make(map[string]Group)
	}
	if len(cfg.RoutesFiles) > 0 {
		routing, err := LoadMultipleWithBaseDir(cfg.RoutesFiles, baseDir, middlewareRegistry)
		if err != nil {
			return err
		}
		for name, group := range routing.Groups {
			if existing, exists := merged.Groups[name]; exists {
				combined, ok := mergeOverlayGroup(existing, group)
				if !ok {
					return fmt.Errorf("duplicate group '%s' defined inline and in routes_files", name)
				}
				merged.Groups[name] = combined
				continue
			}
			merged.Groups[name] = group
		}
	}

	for name, group := range merged.Groups {
		if isOverlayGroup(group) {
			delete(merged.Groups, name)
		}
	}

	supportedMiddlewares := make(map[string]struct{}, len(middlewareRegistry))
	for name := range middlewareRegistry {
		supportedMiddlewares[name] = struct{}{}
	}

	normalized, err := validateAndNormalize(merged, supportedMiddlewares)
	if err != nil {
		return err
	}
	normalized, err = resolveRouteOpenAPIFiles(normalized, baseDir)
	if err != nil {
		return err
	}
	if err := validateRouteOpenAPIFiles(normalized); err != nil {
		return err
	}
	if err := validateOpenAPIAlignment(normalized); err != nil {
		return err
	}

	cfg.Routes = normalized
	return nil
}

func resolveRouteOpenAPIFiles(routingCfg Routing, baseDir string) (Routing, error) {
	for groupName, group := range routingCfg.Groups {
		for routeIndex := range group.Routes {
			route := group.Routes[routeIndex]
			if route.OpenAPI == nil {
				continue
			}
			resolvedPath, err := ResolvePathWithBaseDir(route.OpenAPI.File, baseDir)
			if err != nil {
				return Routing{}, fmt.Errorf("resolve groups.%s.routes[%d].openapi.file: %w", groupName, routeIndex, err)
			}
			route.OpenAPI.ResolvedFile = resolvedPath
			group.Routes[routeIndex] = route
		}
		routingCfg.Groups[groupName] = group
	}

	return routingCfg, nil
}

func validateRouteOpenAPIFiles(routingCfg Routing) error {
	loader := openapi3.NewLoader()
	loader.IsExternalRefsAllowed = true

	validated := make(map[string]struct{})
	for groupName, group := range routingCfg.Groups {
		for routeIndex, route := range group.Routes {
			if route.OpenAPI == nil {
				continue
			}
			specPath := route.OpenAPI.ResolvedFile
			if specPath == "" {
				specPath = route.OpenAPI.File
			}
			if specPath == "" {
				continue
			}
			if _, alreadyValidated := validated[specPath]; alreadyValidated {
				continue
			}

			doc, err := loader.LoadFromFile(specPath)
			if err != nil {
				routeRef := fmt.Sprintf("groups.%s.routes[%d] (path_prefix=%s)", groupName, routeIndex, route.PathPrefix)
				if errors.Is(err, os.ErrNotExist) {
					return fmt.Errorf("%s: OpenAPI file not found at %q (openapi.file). Relative paths are resolved from the config file directory", routeRef, specPath)
				}
				return fmt.Errorf("%s: cannot load OpenAPI file %q: %w", routeRef, specPath, err)
			}
			if err := doc.Validate(context.Background()); err != nil {
				return fmt.Errorf("groups.%s.routes[%d] (path_prefix=%s): OpenAPI file %q is invalid: %w", groupName, routeIndex, route.PathPrefix, specPath, err)
			}
			validated[specPath] = struct{}{}
		}
	}

	return nil
}

func validateOpenAPIAlignment(routingCfg Routing) error {
	loader := openapi3.NewLoader()
	loader.IsExternalRefsAllowed = true

	docCache := make(map[string]*openapi3.T)
	for groupName, group := range routingCfg.Groups {
		for routeIndex, route := range group.Routes {
			if route.OpenAPI == nil {
				continue
			}
			specPath := route.OpenAPI.ResolvedFile
			if specPath == "" {
				specPath = route.OpenAPI.File
			}
			if specPath == "" {
				continue
			}

			doc, ok := docCache[specPath]
			if !ok {
				loaded, err := loader.LoadFromFile(specPath)
				if err != nil {
					return fmt.Errorf("load groups.%s.routes[%d].openapi.file (%s): %w", groupName, routeIndex, specPath, err)
				}
				docCache[specPath] = loaded
				doc = loaded
			}

			specOps := collectOpenAPISpecOperations(doc, route.PathPrefix)
			routeOps := collectRouteOperations(route)

			missing := diffOperations(routeOps, specOps)
			if len(missing) > 0 {
				return fmt.Errorf("groups.%s.routes[%d] (path_prefix=%s): OpenAPI spec missing operations: %s", groupName, routeIndex, route.PathPrefix, strings.Join(missing, ", "))
			}

			extra := diffOperations(specOps, routeOps)
			if len(extra) > 0 {
				return fmt.Errorf("groups.%s.routes[%d] (path_prefix=%s): OpenAPI spec defines operations not in routes: %s", groupName, routeIndex, route.PathPrefix, strings.Join(extra, ", "))
			}
		}
	}

	return nil
}

func collectRouteOperations(route Route) map[string]struct{} {
	ops := make(map[string]struct{})
	for _, endpoint := range route.Endpoints {
		fullPath := joinRoutePath(route.PathPrefix, endpoint.Path)
		openapiPath := normalizeOpenAPIPath(fullPath)
		for method := range endpoint.Methods {
			key := strings.ToUpper(method) + " " + openapiPath
			ops[key] = struct{}{}
		}
	}
	return ops
}

func collectOpenAPISpecOperations(doc *openapi3.T, pathPrefix string) map[string]struct{} {
	ops := make(map[string]struct{})
	if doc == nil || doc.Paths == nil {
		return ops
	}

	prefix := normalizeOpenAPIPath(pathPrefix)
	if prefix == "" {
		prefix = "/"
	}

	addOp := func(method, path string, op *openapi3.Operation) {
		if op == nil {
			return
		}
		key := strings.ToUpper(method) + " " + path
		ops[key] = struct{}{}
	}

	for path, item := range doc.Paths.Map() {
		if item == nil {
			continue
		}
		if !matchesOpenAPIPrefix(path, prefix) {
			continue
		}
		addOp("GET", path, item.Get)
		addOp("POST", path, item.Post)
		addOp("PUT", path, item.Put)
		addOp("PATCH", path, item.Patch)
		addOp("DELETE", path, item.Delete)
		addOp("HEAD", path, item.Head)
		addOp("OPTIONS", path, item.Options)
		addOp("TRACE", path, item.Trace)
	}

	return ops
}

func diffOperations(expected, actual map[string]struct{}) []string {
	out := make([]string, 0)
	for key := range expected {
		if _, ok := actual[key]; !ok {
			out = append(out, key)
		}
	}
	sort.Strings(out)
	return out
}

func matchesOpenAPIPrefix(path, prefix string) bool {
	if prefix == "/" {
		return true
	}
	if path == prefix {
		return true
	}
	return strings.HasPrefix(path, prefix+"/")
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

func mergeOverlayGroup(existing, incoming Group) (Group, bool) {
	switch {
	case isOverlayGroup(existing) && !isOverlayGroup(incoming):
		return applyGroupOverlay(incoming, existing), true
	case !isOverlayGroup(existing) && isOverlayGroup(incoming):
		return applyGroupOverlay(existing, incoming), true
	default:
		return Group{}, false
	}
}

func isOverlayGroup(group Group) bool {
	return group.Prefix == "" &&
		len(group.Middlewares) == 0 &&
		group.RateLimit == nil &&
		len(group.Routes) == 0 &&
		len(group.WebSockets) == 0 &&
		group.AuthEndpoints != nil
}

func applyGroupOverlay(base, overlay Group) Group {
	if overlay.AuthEndpoints == nil {
		return base
	}
	if base.AuthEndpoints == nil {
		base.AuthEndpoints = &AuthEndpoints{}
	}
	if overlay.AuthEndpoints.OAuth2 != nil {
		if base.AuthEndpoints.OAuth2 == nil {
			base.AuthEndpoints.OAuth2 = &OAuth2Config{}
		}
		if overlay.AuthEndpoints.OAuth2.ClientSecret != "" {
			base.AuthEndpoints.OAuth2.ClientSecret = overlay.AuthEndpoints.OAuth2.ClientSecret
		}
	}
	return base
}
