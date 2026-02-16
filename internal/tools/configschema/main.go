package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
	gatewaycfg "github.com/nimburion/apigateway/internal/config"
	"github.com/nimburion/nimburion/pkg/configschema"
)

func main() {
	root, err := repoRoot()
	if err != nil {
		exitErr(err)
	}

	schema, err := configschema.BuildSchemaWithDefaults(nil, gatewaycfg.NewDefaultConfig())
	if err != nil {
		exitErr(err)
	}
	schema.Title = "Service Configuration"
	schema.Description = "Schema for service configuration."
	if module := modulePath(root); module != "" {
		schema.ID = module + "/config/schema.json"
	}
	if version := schemaVersionHash(schema); version != "" {
		schema.Comment = "schema.version=" + version
	}
	removeProperty(schema, "config_dir")
	removePropertyAtPath(schema, []string{"routes", "groups", "*", "routes", "*", "group"})
	removePropertyAtPath(schema, []string{"routes", "groups", "*", "websockets", "*", "group"})
	removePropertyAtPath(schema, []string{"routes", "groups", "*", "routes", "*", "openapi", "resolved_file"})
	addRoutingConstraints(schema)
	applyGatewaySchemaConstraints(schema)

	outPath := filepath.Join(root, "config", "schema.json")
	if err := writeSchema(outPath, schema); err != nil {
		exitErr(err)
	}
}

func repoRoot() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("get working directory: %w", err)
	}
	return filepath.Clean(filepath.Join(wd, "..", "..")), nil
}

func modulePath(root string) string {
	data, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		return ""
	}
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module "))
		}
	}
	return ""
}

func schemaVersionHash(schema *jsonschema.Schema) string {
	if schema == nil {
		return ""
	}
	originalID := schema.ID
	originalComment := schema.Comment
	schema.ID = ""
	schema.Comment = ""
	payload, err := json.Marshal(schema)
	schema.ID = originalID
	schema.Comment = originalComment
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}

func addRoutingConstraints(schema *jsonschema.Schema) {
	if schema == nil {
		return
	}
	schema.AnyOf = append(schema.AnyOf,
		&jsonschema.Schema{
			Required: []string{"routes_files"},
			Properties: map[string]*jsonschema.Schema{
				"routes_files": {MinItems: intPtr(1)},
			},
		},
		&jsonschema.Schema{
			Required: []string{"routes"},
			Properties: map[string]*jsonschema.Schema{
				"routes": {
					Properties: map[string]*jsonschema.Schema{
						"groups": {MinProperties: intPtr(1)},
					},
				},
			},
		},
	)

	emptyRoutesFiles := &jsonschema.Schema{
		AnyOf: []*jsonschema.Schema{
			{Not: &jsonschema.Schema{Required: []string{"routes_files"}}},
			{Properties: map[string]*jsonschema.Schema{"routes_files": {MaxItems: intPtr(0)}}},
		},
	}
	emptyRoutesGroups := &jsonschema.Schema{
		AnyOf: []*jsonschema.Schema{
			{Not: &jsonschema.Schema{Required: []string{"routes"}}},
			{
				Properties: map[string]*jsonschema.Schema{
					"routes": {
						AnyOf: []*jsonschema.Schema{
							{Not: &jsonschema.Schema{Required: []string{"groups"}}},
							{Properties: map[string]*jsonschema.Schema{"groups": {MaxProperties: intPtr(0)}}},
						},
					},
				},
			},
		},
	}
	schema.Not = &jsonschema.Schema{
		AllOf: []*jsonschema.Schema{emptyRoutesFiles, emptyRoutesGroups},
	}
}

func applyGatewaySchemaConstraints(schema *jsonschema.Schema) {
	walkSchema(schema, func(s *jsonschema.Schema) {
		if s == nil || len(s.Properties) == 0 {
			return
		}
		if openapiSchema := s.Properties["openapi"]; openapiSchema != nil {
			if openapiSchema.Properties == nil {
				openapiSchema.Properties = map[string]*jsonschema.Schema{}
			}
			mode := openapiSchema.Properties["mode"]
			if mode == nil {
				mode = &jsonschema.Schema{}
				openapiSchema.Properties["mode"] = mode
			}
			mode.Enum = []any{"strict", "warn-only"}

			if fileSchema := openapiSchema.Properties["file"]; fileSchema != nil {
				if fileSchema.MinLength == nil {
					fileSchema.MinLength = intPtr(1)
				}
				if !containsString(openapiSchema.Required, "file") {
					openapiSchema.Required = append(openapiSchema.Required, "file")
				}
			}
		}

		if rateLimit := s.Properties["rate_limit"]; rateLimit != nil {
			if rateLimit.Properties == nil {
				rateLimit.Properties = map[string]*jsonschema.Schema{}
			}
			rps := ensureProperty(rateLimit, "requests_per_second")
			rps.Minimum = floatPtr(1)
			burst := ensureProperty(rateLimit, "burst")
			burst.Minimum = floatPtr(1)
		}
	})
}

func walkSchema(schema *jsonschema.Schema, fn func(*jsonschema.Schema)) {
	if schema == nil {
		return
	}
	fn(schema)
	for _, prop := range schema.Properties {
		walkSchema(prop, fn)
	}
	for _, prop := range schema.PatternProperties {
		walkSchema(prop, fn)
	}
	for _, item := range schema.ItemsArray {
		walkSchema(item, fn)
	}
	walkSchema(schema.Items, fn)
	walkSchema(schema.AdditionalProperties, fn)
	for _, sub := range schema.AllOf {
		walkSchema(sub, fn)
	}
	for _, sub := range schema.AnyOf {
		walkSchema(sub, fn)
	}
	for _, sub := range schema.OneOf {
		walkSchema(sub, fn)
	}
	walkSchema(schema.Not, fn)
}

func ensureProperty(schema *jsonschema.Schema, name string) *jsonschema.Schema {
	if schema.Properties == nil {
		schema.Properties = map[string]*jsonschema.Schema{}
	}
	if schema.Properties[name] == nil {
		schema.Properties[name] = &jsonschema.Schema{}
	}
	return schema.Properties[name]
}

func containsString(values []string, value string) bool {
	for _, item := range values {
		if item == value {
			return true
		}
	}
	return false
}

func floatPtr(value float64) *float64 {
	return &value
}

func removeProperty(schema *jsonschema.Schema, name string) {
	if schema == nil || len(schema.Properties) == 0 {
		return
	}
	delete(schema.Properties, name)
	if len(schema.Required) > 0 {
		kept := make([]string, 0, len(schema.Required))
		for _, req := range schema.Required {
			if req != name {
				kept = append(kept, req)
			}
		}
		schema.Required = kept
	}
	if len(schema.PropertyOrder) > 0 {
		kept := make([]string, 0, len(schema.PropertyOrder))
		for _, prop := range schema.PropertyOrder {
			if prop != name {
				kept = append(kept, prop)
			}
		}
		schema.PropertyOrder = kept
	}
}

func removePropertyAtPath(schema *jsonschema.Schema, path []string) {
	if schema == nil || len(path) == 0 {
		return
	}
	if len(path) == 1 {
		removeProperty(schema, path[0])
		return
	}
	segment := path[0]
	if segment == "*" {
		if schema.Items != nil {
			removePropertyAtPath(schema.Items, path[1:])
		}
		if schema.AdditionalProperties != nil {
			removePropertyAtPath(schema.AdditionalProperties, path[1:])
		}
		return
	}
	if schema.Properties == nil {
		return
	}
	child := schema.Properties[segment]
	if child == nil {
		return
	}
	removePropertyAtPath(child, path[1:])
}

func intPtr(value int) *int {
	return &value
}

func writeSchema(path string, schema any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create schema dir: %w", err)
	}
	payload, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal schema: %w", err)
	}
	if err := os.WriteFile(path, payload, 0o644); err != nil {
		return fmt.Errorf("write schema: %w", err)
	}
	return nil
}

func exitErr(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
