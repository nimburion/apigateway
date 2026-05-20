package approutes

import (
	"net/http"
	"strings"
	"testing"

	gatewaycfg "github.com/nimburion/apigateway/internal/config"
	"github.com/nimburion/apigateway/internal/portalmeta"
	"github.com/nimburion/nimburion/pkg/http/router"
)

func TestValidateSupportedMethodsRejectsHEADAndOPTIONS(t *testing.T) {
	for _, method := range []string{"HEAD", "OPTIONS"} {
		t.Run(method, func(t *testing.T) {
			err := ValidateSupportedMethods(gatewaycfg.Routing{
				Groups: map[string]gatewaycfg.Group{
					"default": {
						Prefix: "/",
						Routes: []gatewaycfg.Route{
							{
								PathPrefix: "/users",
								Endpoints: []gatewaycfg.Endpoint{
									{
										Path: "/",
										Methods: map[string]*gatewaycfg.Method{
											method: {},
										},
									},
								},
							},
						},
					},
				},
			})
			if err == nil {
				t.Fatalf("expected %s to be rejected", method)
			}
			if !strings.Contains(err.Error(), "unsupported HTTP method") {
				t.Fatalf("unexpected error for %s: %v", method, err)
			}
		})
	}
}

func TestCollectRuntimeRoutesPreservesPortalMetadataForRegisteredAuthEndpoints(t *testing.T) {
	routeDefs := gatewaycfg.Routing{
		Groups: map[string]gatewaycfg.Group{
			"public": {
				Prefix: "/",
				AuthEndpoints: &gatewaycfg.AuthEndpoints{
					Me: true,
				},
			},
		},
	}

	middlewareRegistry := map[string]func() router.MiddlewareFunc{}
	collected := CollectRuntimeRoutes(func(r router.Router) {
		if err := Register(r, routeDefs, middlewareRegistry, nil); err != nil {
			t.Fatalf("register routes: %v", err)
		}
	})

	var meRoute *RuntimeRoute
	for i := range collected {
		if collected[i].Method == http.MethodGet && collected[i].Path == "/auth/me" {
			meRoute = &collected[i]
			break
		}
	}

	if meRoute == nil {
		t.Fatalf("expected /auth/me route to be collected, got %#v", collected)
	}
	if meRoute.Metadata.OwnerTeam != "platform" {
		t.Fatalf("expected portalmeta owner_team to survive collector path, got %#v", meRoute.Metadata)
	}
	if meRoute.Metadata.Domain != "auth" {
		t.Fatalf("expected portalmeta domain to survive collector path, got %#v", meRoute.Metadata)
	}

	meta, ok := portalmeta.MetadataForHandler(portalmeta.Annotate(func(_ router.Context) error {
		return nil
	}, portalmeta.AuthMe()))
	if !ok || meta.Resource.OwnerTeam != "platform" {
		t.Fatalf("expected direct registry lookup to keep portal metadata, got %#v ok=%v", meta, ok)
	}
}
