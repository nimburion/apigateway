package portalmeta

import (
	"reflect"
	"sync"

	gatewaycfg "github.com/nimburion/apigateway/internal/config"
	"github.com/nimburion/nimburion/pkg/http/router"
)

type Metadata struct {
	Resource      gatewaycfg.ResourceMetadata
	AuthRequired  bool
	Scopes        []string
	HasRateLimit  bool
}

var registry sync.Map

func Annotate(handler router.HandlerFunc, metadata Metadata) router.HandlerFunc {
	if handler == nil {
		return nil
	}
	registry.Store(handlerKey(handler), metadata)
	return handler
}

func MetadataForHandler(handler router.HandlerFunc) (Metadata, bool) {
	if handler == nil {
		return Metadata{}, false
	}
	raw, ok := registry.Load(handlerKey(handler))
	if !ok {
		return Metadata{}, false
	}
	meta, ok := raw.(Metadata)
	return meta, ok
}

func handlerKey(handler router.HandlerFunc) uintptr {
	return reflect.ValueOf(handler).Pointer()
}

func AuthRuntimeMetadata(summaryDocsURL string) Metadata {
	return Metadata{
		Resource: gatewaycfg.ResourceMetadata{
			OwnerTeam:      "platform",
			Domain:         "auth",
			Visibility:     gatewaycfg.MetadataVisibilityInternal,
			Status:         gatewaycfg.MetadataStatusActive,
			DocsURL:        summaryDocsURL,
			SupportChannel: "#api-platform",
		},
	}
}
