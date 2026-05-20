package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
	gatewaycfg "github.com/nimburion/apigateway/internal/config"
)

func TestSchemaVersionHashStable(t *testing.T) {
	schema := &jsonschema.Schema{Title: "Test"}
	first := schemaVersionHash(schema)
	second := schemaVersionHash(schema)
	if first == "" || first != second {
		t.Fatalf("expected stable schema hash")
	}
}

func TestGatewayCustomizeSchemaAddsConstraints(t *testing.T) {
	schema := &jsonschema.Schema{
		Properties: map[string]*jsonschema.Schema{
			"config_dir": {},
			"portal": {
				Properties: map[string]*jsonschema.Schema{
					"mode": {},
				},
			},
			"config_store": {
				Properties: map[string]*jsonschema.Schema{
					"source_of_truth": {},
					"backend":         {},
				},
			},
			"routes": {
				Properties: map[string]*jsonschema.Schema{
					"groups": {
						AdditionalProperties: &jsonschema.Schema{
							Properties: map[string]*jsonschema.Schema{
								"metadata": {
									Properties: map[string]*jsonschema.Schema{
										"visibility": {},
										"status":     {},
									},
								},
								"rate_limit": {
									Properties: map[string]*jsonschema.Schema{},
								},
								"routes": {
									Items: &jsonschema.Schema{
										Properties: map[string]*jsonschema.Schema{
											"group": {},
											"openapi": {
												Properties: map[string]*jsonschema.Schema{
													"file":          {},
													"mode":          {},
													"resolved_file": {},
												},
											},
										},
									},
								},
								"websockets": {
									Items: &jsonschema.Schema{
										Properties: map[string]*jsonschema.Schema{
											"group": {},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	if err := gatewaycfg.NewDefaultConfig().CustomizeSchema(schema); err != nil {
		t.Fatalf("CustomizeSchema error: %v", err)
	}

	if _, ok := schema.Properties["config_dir"]; ok {
		t.Fatalf("expected config_dir to be removed from schema")
	}
	if len(schema.AnyOf) != 3 {
		t.Fatalf("expected routing anyOf constraints, got %d", len(schema.AnyOf))
	}
	if schema.Not == nil {
		t.Fatalf("expected root not routing constraint")
	}

	routesGroups := schema.Properties["routes"].Properties["groups"].AdditionalProperties
	routeItem := routesGroups.Properties["routes"].Items
	if _, ok := routeItem.Properties["group"]; ok {
		t.Fatalf("expected routes[*].group to be removed")
	}
	openapiSchema := routeItem.Properties["openapi"]
	if _, ok := openapiSchema.Properties["resolved_file"]; ok {
		t.Fatalf("expected openapi.resolved_file to be removed")
	}
	if len(openapiSchema.Properties["mode"].Enum) != 2 {
		t.Fatalf("expected openapi.mode enum")
	}
	if !containsRequired(openapiSchema.Required, "file") {
		t.Fatalf("expected openapi.file to be required")
	}
	if openapiSchema.Properties["file"].MinLength == nil || *openapiSchema.Properties["file"].MinLength != 1 {
		t.Fatalf("expected openapi.file minLength")
	}

	rateLimitSchema := routesGroups.Properties["rate_limit"]
	if rateLimitSchema.Properties["requests_per_second"].Minimum == nil || *rateLimitSchema.Properties["requests_per_second"].Minimum != 1 {
		t.Fatalf("expected rate_limit.requests_per_second minimum")
	}
	if rateLimitSchema.Properties["burst"].Minimum == nil || *rateLimitSchema.Properties["burst"].Minimum != 1 {
		t.Fatalf("expected rate_limit.burst minimum")
	}

	portalMode := schema.Properties["portal"].Properties["mode"]
	if len(portalMode.Enum) != 2 {
		t.Fatalf("expected portal.mode enum")
	}
	configStore := schema.Properties["config_store"]
	if len(configStore.Properties["source_of_truth"].Enum) != 2 {
		t.Fatalf("expected config_store.source_of_truth enum")
	}
	if len(configStore.Properties["backend"].Enum) != 2 {
		t.Fatalf("expected config_store.backend enum")
	}

	metadataSchema := routesGroups.Properties["metadata"]
	if len(metadataSchema.Properties["visibility"].Enum) != 3 {
		t.Fatalf("expected metadata.visibility enum")
	}
	if len(metadataSchema.Properties["status"].Enum) != 3 {
		t.Fatalf("expected metadata.status enum")
	}
}

func TestModulePathAndRepoRoot(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "go.mod"), []byte("module github.com/acme/test\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	if got := modulePath(tmp); got != "github.com/acme/test" {
		t.Fatalf("unexpected module path: %q", got)
	}

	wd, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(wd) })
	work := filepath.Join(tmp, "a", "b")
	if err := os.MkdirAll(work, 0o755); err != nil {
		t.Fatalf("mkdir work: %v", err)
	}
	if err := os.Chdir(work); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	root, err := repoRoot()
	if err != nil {
		t.Fatalf("repoRoot error: %v", err)
	}
	expected := filepath.Clean(tmp)
	if resolvedExpected, err := filepath.EvalSymlinks(expected); err == nil && resolvedExpected != "" {
		expected = resolvedExpected
	}
	if root != expected {
		t.Fatalf("unexpected repo root: %q", root)
	}
}

func TestWriteSchema(t *testing.T) {
	out := filepath.Join(t.TempDir(), "config", "schema.json")
	if err := writeSchema(out, &jsonschema.Schema{Title: "Test"}); err != nil {
		t.Fatalf("writeSchema error: %v", err)
	}
	if _, err := os.Stat(out); err != nil {
		t.Fatalf("expected schema output file: %v", err)
	}
}

func TestWriteSchemaProducesStableJSONShape(t *testing.T) {
	out := filepath.Join(t.TempDir(), "config", "schema.json")
	schema := &jsonschema.Schema{
		Title:       "Gateway Schema",
		Description: "Schema for gateway config.",
		Properties: map[string]*jsonschema.Schema{
			"routes": {Type: "object"},
			"portal": {
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"mode": {Type: "string"},
				},
			},
		},
	}

	if err := writeSchema(out, schema); err != nil {
		t.Fatalf("writeSchema error: %v", err)
	}

	payload, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read schema output: %v", err)
	}
	content := string(payload)
	for _, want := range []string{
		`"title": "Gateway Schema"`,
		`"description": "Schema for gateway config."`,
		`"routes": {`,
		`"portal": {`,
		`"mode": {`,
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("expected schema output to contain %q, got %q", want, content)
		}
	}
}

func containsRequired(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
