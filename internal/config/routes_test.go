package config

import (
	"strings"
	"testing"
)

func TestValidateOpenAPI(t *testing.T) {
	_, err := validateOpenAPI(&OpenAPI{}, "route")
	if err == nil {
		t.Fatalf("expected error when openapi.file is empty")
	}

	cfg, err := validateOpenAPI(&OpenAPI{File: "spec.yml"}, "route")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Mode != OpenAPIValidationModeStrict {
		t.Fatalf("expected default mode strict, got %s", cfg.Mode)
	}

	_, err = validateOpenAPI(&OpenAPI{File: "spec.yml", Mode: "invalid"}, "route")
	if err == nil {
		t.Fatalf("expected error for invalid mode")
	}
}

func TestResolvePathWithBaseDir(t *testing.T) {
	path, err := ResolvePathWithBaseDir("routes.yaml", "/tmp")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if path != "/tmp/routes.yaml" {
		t.Fatalf("unexpected resolved path: %s", path)
	}
}

func TestValidateAndNormalize(t *testing.T) {
	routing := Routing{Groups: map[string]Group{
		"default": {
			Prefix: "/api",
			Metadata: ResourceMetadata{
				OwnerTeam:  " platform ",
				Visibility: " INTERNAL ",
				Status:     " ACTIVE ",
				DocsURL:    " https://docs.example.com/api ",
			},
			Routes: []Route{
				{
					PathPrefix: "/users",
					TargetURL:  "http://example.com",
					Metadata: ResourceMetadata{
						Domain:     " identity ",
						Visibility: "partner",
					},
					Endpoints: []Endpoint{{
						Path:    "/",
						Methods: map[string]*Method{"get": {}},
					}},
				},
			},
		},
	}}

	normalized, err := validateAndNormalize(routing, map[string]struct{}{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	methods := normalized.Groups["default"].Routes[0].Endpoints[0].Methods
	if _, ok := methods["GET"]; !ok {
		t.Fatalf("expected GET method to be normalized")
	}
	if normalized.Groups["default"].Metadata.OwnerTeam != "platform" {
		t.Fatalf("expected group metadata owner_team to be trimmed")
	}
	if normalized.Groups["default"].Metadata.Visibility != MetadataVisibilityInternal {
		t.Fatalf("expected normalized group visibility, got %q", normalized.Groups["default"].Metadata.Visibility)
	}
	if normalized.Groups["default"].Metadata.Status != MetadataStatusActive {
		t.Fatalf("expected normalized group status, got %q", normalized.Groups["default"].Metadata.Status)
	}
	if normalized.Groups["default"].Routes[0].Metadata.Domain != "identity" {
		t.Fatalf("expected route metadata domain to be trimmed")
	}
}

func TestValidateRateLimit(t *testing.T) {
	if err := validateRateLimit(&RateLimit{RequestsPerSecond: 0, Burst: 1}, "x"); err == nil {
		t.Fatalf("expected error for requests_per_second <= 0")
	}
	if err := validateRateLimit(&RateLimit{RequestsPerSecond: 1, Burst: 0}, "x"); err == nil {
		t.Fatalf("expected error for burst <= 0")
	}
	if err := validateRateLimit(&RateLimit{RequestsPerSecond: 1, Burst: 1}, "x"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateWebSocket(t *testing.T) {
	supported := map[string]struct{}{"Authenticate": {}}
	seen := map[string]struct{}{}

	normalized, err := validateWebSocket(WebSocket{
		Path:        " /ws/ ",
		TargetURL:   "wss://example.com/socket",
		StripPrefix: "/api/",
		Middlewares: []string{"Authenticate"},
		RateLimit:   &RateLimit{RequestsPerSecond: 1, Burst: 1},
	}, "g", "/api", 0, seen, supported)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if normalized.Path != "/ws" {
		t.Fatalf("expected normalized path /ws, got %s", normalized.Path)
	}
	if normalized.StripPrefix != "/api" {
		t.Fatalf("expected normalized strip_prefix /api, got %s", normalized.StripPrefix)
	}

	if _, err := validateWebSocket(WebSocket{
		Path:      "/ws",
		TargetURL: "http://example.com",
	}, "g", "/api", 1, map[string]struct{}{}, supported); err == nil {
		t.Fatalf("expected error for non ws/wss scheme")
	}
}

func TestValidateAndNormalizeAllowsSameRoutePathPrefixAcrossDifferentGroupPrefixes(t *testing.T) {
	routing := Routing{Groups: map[string]Group{
		"default": {
			Prefix: "/",
			Routes: []Route{
				{
					PathPrefix: "/healthz",
					TargetURL:  "http://example.com",
					Endpoints: []Endpoint{{
						Path:    "/",
						Methods: map[string]*Method{"GET": {}},
					}},
				},
			},
		},
		"central": {
			Prefix: "/central",
			Routes: []Route{
				{
					PathPrefix: "/healthz",
					TargetURL:  "http://example.com",
					Endpoints: []Endpoint{{
						Path:    "/",
						Methods: map[string]*Method{"GET": {}},
					}},
				},
			},
		},
	}}

	normalized, err := validateAndNormalize(routing, map[string]struct{}{})
	if err != nil {
		t.Fatalf("expected same path_prefix under different group prefixes to be allowed, got %v", err)
	}
	if normalized.Groups["default"].Routes[0].PathPrefix != "/healthz" {
		t.Fatalf("unexpected normalized default route path prefix: %q", normalized.Groups["default"].Routes[0].PathPrefix)
	}
	if normalized.Groups["central"].Routes[0].PathPrefix != "/healthz" {
		t.Fatalf("unexpected normalized central route path prefix: %q", normalized.Groups["central"].Routes[0].PathPrefix)
	}
}

func TestValidateAndNormalizeAllowsArbitraryGroupNames(t *testing.T) {
	routing := Routing{Groups: map[string]Group{
		"public": {
			Prefix: "/",
			Routes: []Route{
				{
					PathPrefix: "/healthz",
					TargetURL:  "http://example.com",
					Endpoints: []Endpoint{{
						Path:    "/",
						Methods: map[string]*Method{"GET": {}},
					}},
				},
			},
		},
		"api-v1": {
			Prefix: "/v1",
			Routes: []Route{
				{
					PathPrefix: "/users",
					TargetURL:  "http://example.com",
					Endpoints: []Endpoint{{
						Path:    "/",
						Methods: map[string]*Method{"GET": {}},
					}},
				},
			},
		},
	}}

	normalized, err := validateAndNormalize(routing, map[string]struct{}{})
	if err != nil {
		t.Fatalf("expected arbitrary group names to be allowed, got %v", err)
	}
	if _, ok := normalized.Groups["public"]; !ok {
		t.Fatalf("expected public group to survive normalization")
	}
	if _, ok := normalized.Groups["api-v1"]; !ok {
		t.Fatalf("expected api-v1 group to survive normalization")
	}
}

func TestValidateAndNormalizeRejectsDuplicateEffectiveRoutePath(t *testing.T) {
	routing := Routing{Groups: map[string]Group{
		"default": {
			Prefix: "/",
			Routes: []Route{
				{
					PathPrefix: "/healthz",
					TargetURL:  "http://example.com",
					Endpoints: []Endpoint{{
						Path:    "/",
						Methods: map[string]*Method{"GET": {}},
					}},
				},
			},
		},
		"central": {
			Prefix: "/",
			Routes: []Route{
				{
					PathPrefix: "/healthz",
					TargetURL:  "http://example.com",
					Endpoints: []Endpoint{{
						Path:    "/",
						Methods: map[string]*Method{"GET": {}},
					}},
				},
			},
		},
	}}

	_, err := validateAndNormalize(routing, map[string]struct{}{})
	if err == nil {
		t.Fatalf("expected duplicate effective path to fail")
	}
	if !strings.Contains(err.Error(), "duplicate path_prefix: /healthz") {
		t.Fatalf("expected duplicate effective path error, got %v", err)
	}
}

func TestOAuth2ConfigDefaultsAndGetters(t *testing.T) {
	c := &OAuth2Config{}
	c.SetDefaults()
	if c.StateCookieName != "auth_state" || c.CookieSameSite != "lax" {
		t.Fatalf("unexpected defaults: %#v", c)
	}

	c.Enabled = true
	c.AuthorizeURL = "https://idp/authorize"
	c.TokenURL = "https://idp/token"
	c.Audience = "aud"
	c.ClientID = "cid"
	c.ClientSecret = "secret"
	c.RedirectURL = "https://app/cb"
	c.Scopes = []string{"openid"}
	c.PostLoginRedirectURL = "https://app"
	c.CookieSecure = true
	c.CookieDomain = "example.com"

	if !c.IsEnabled() || c.GetAuthorizeURL() == "" || c.GetTokenURL() == "" || c.GetAudience() == "" ||
		c.GetClientID() == "" || c.GetClientSecret() == "" || c.GetRedirectURL() == "" ||
		len(c.GetScopes()) != 1 || c.GetPostLoginRedirectURL() == "" || c.GetStateCookieName() == "" ||
		!c.IsCookieSecure() || c.GetCookieDomain() == "" || c.GetCookieSameSite() == "" {
		t.Fatalf("getter coverage failed")
	}

	authCfg := c.ToAuthConfig()
	if authCfg.ClientID != "cid" || authCfg.TokenURL != "https://idp/token" {
		t.Fatalf("unexpected auth config mapping: %#v", authCfg)
	}
}

func TestRoutingGroups(t *testing.T) {
	if groups := (Routing{}).routingGroups(); len(groups) != 0 {
		t.Fatalf("expected empty map for nil groups")
	}
	r := Routing{Groups: map[string]Group{"g": {Prefix: "/"}}}
	if len(r.routingGroups()) != 1 {
		t.Fatalf("expected passthrough groups")
	}
}

func TestResourceMetadataFields(t *testing.T) {
	metadata := ResourceMetadata{
		OwnerTeam:      "platform",
		Domain:         "gateway",
		Visibility:     MetadataVisibilityInternal,
		Status:         MetadataStatusActive,
		DocsURL:        "https://docs.example.com",
		RunbookURL:     "https://runbooks.example.com/gateway",
		SupportChannel: "#api-platform",
	}

	group := Group{Prefix: "/api", Metadata: metadata}
	route := Route{PathPrefix: "/users", Metadata: metadata}
	ws := WebSocket{Path: "/ws", Metadata: metadata}

	if group.Metadata.OwnerTeam != "platform" || route.Metadata.Domain != "gateway" || ws.Metadata.Visibility != MetadataVisibilityInternal {
		t.Fatalf("expected metadata to be retained on group, route, and websocket")
	}
}

func TestValidateResourceMetadata(t *testing.T) {
	normalized, err := validateResourceMetadata(ResourceMetadata{
		OwnerTeam:      " platform ",
		Domain:         " gateway ",
		Visibility:     " INTERNAL ",
		Status:         " ACTIVE ",
		DocsURL:        " https://docs.example.com ",
		RunbookURL:     " https://runbooks.example.com/gateway ",
		SupportChannel: " #api-platform ",
	}, "groups.default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if normalized.OwnerTeam != "platform" || normalized.Domain != "gateway" {
		t.Fatalf("expected metadata strings to be trimmed: %#v", normalized)
	}
	if normalized.Visibility != MetadataVisibilityInternal || normalized.Status != MetadataStatusActive {
		t.Fatalf("expected normalized enum values: %#v", normalized)
	}
	if normalized.DocsURL != "https://docs.example.com" || normalized.RunbookURL != "https://runbooks.example.com/gateway" {
		t.Fatalf("expected urls to be trimmed: %#v", normalized)
	}
	if normalized.SupportChannel != "#api-platform" {
		t.Fatalf("expected support channel to be trimmed: %#v", normalized)
	}
}

func TestValidateResourceMetadataErrors(t *testing.T) {
	testCases := []struct {
		name    string
		path    string
		input   ResourceMetadata
		wantErr string
	}{
		{
			name:    "invalid visibility",
			path:    "groups.default",
			input:   ResourceMetadata{Visibility: "secret"},
			wantErr: "groups.default.metadata.visibility",
		},
		{
			name:    "invalid status",
			path:    "groups.default.routes[0]",
			input:   ResourceMetadata{Status: "paused"},
			wantErr: "groups.default.routes[0].metadata.status",
		},
		{
			name:    "invalid docs url",
			path:    "groups.default",
			input:   ResourceMetadata{DocsURL: "docs.local/path"},
			wantErr: "groups.default.metadata.docs_url",
		},
		{
			name:    "invalid runbook url",
			path:    "groups.default.websockets[0]",
			input:   ResourceMetadata{RunbookURL: "/runbooks/local"},
			wantErr: "groups.default.websockets[0].metadata.runbook_url",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := validateResourceMetadata(tc.input, tc.path)
			if err == nil {
				t.Fatalf("expected validation error")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("expected error to contain %q, got %q", tc.wantErr, err.Error())
			}
		})
	}
}
