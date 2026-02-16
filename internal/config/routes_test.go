package config

import "testing"

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
			Routes: []Route{
				{
					PathPrefix: "/users",
					TargetURL:  "http://example.com",
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
	}, "g", 0, seen, supported)
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
	}, "g", 1, map[string]struct{}{}, supported); err == nil {
		t.Fatalf("expected error for non ws/wss scheme")
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
