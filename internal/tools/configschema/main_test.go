package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
)

func TestSchemaVersionHashStable(t *testing.T) {
	schema := &jsonschema.Schema{Title: "Test"}
	first := schemaVersionHash(schema)
	second := schemaVersionHash(schema)
	if first == "" || first != second {
		t.Fatalf("expected stable schema hash")
	}
}

func TestRemoveProperty(t *testing.T) {
	schema := &jsonschema.Schema{
		Properties: map[string]*jsonschema.Schema{"secret": {}},
		Required:   []string{"secret"},
		PropertyOrder: []string{
			"secret",
		},
	}

	removeProperty(schema, "secret")
	if _, ok := schema.Properties["secret"]; ok {
		t.Fatalf("expected property to be removed")
	}
	if len(schema.Required) != 0 {
		t.Fatalf("expected required to be pruned")
	}
}

func TestAddRoutingConstraints(t *testing.T) {
	schema := &jsonschema.Schema{}
	addRoutingConstraints(schema)
	if len(schema.AnyOf) != 2 {
		t.Fatalf("expected anyOf constraints, got %d", len(schema.AnyOf))
	}
	if schema.Not == nil {
		t.Fatalf("expected not constraint")
	}
}

func TestApplyGatewaySchemaConstraints(t *testing.T) {
	schema := &jsonschema.Schema{
		Properties: map[string]*jsonschema.Schema{
			"openapi": {
				Properties: map[string]*jsonschema.Schema{
					"file": {},
				},
			},
			"rate_limit": {
				Properties: map[string]*jsonschema.Schema{},
			},
		},
	}

	applyGatewaySchemaConstraints(schema)
	openapiSchema := schema.Properties["openapi"]
	if openapiSchema == nil || len(openapiSchema.Properties["mode"].Enum) != 2 {
		t.Fatalf("expected openapi.mode enum to be set")
	}
	if !containsString(openapiSchema.Required, "file") {
		t.Fatalf("expected openapi.file to be required")
	}
	if openapiSchema.Properties["file"].MinLength == nil || *openapiSchema.Properties["file"].MinLength != 1 {
		t.Fatalf("expected openapi.file minLength")
	}

	limit := schema.Properties["rate_limit"]
	if limit.Properties["requests_per_second"].Minimum == nil || *limit.Properties["requests_per_second"].Minimum != 1 {
		t.Fatalf("expected rate_limit.requests_per_second minimum")
	}
	if limit.Properties["burst"].Minimum == nil || *limit.Properties["burst"].Minimum != 1 {
		t.Fatalf("expected rate_limit.burst minimum")
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

func TestRemovePropertyAtPathAndWriteSchema(t *testing.T) {
	schema := &jsonschema.Schema{
		Properties: map[string]*jsonschema.Schema{
			"routes": {
				Properties: map[string]*jsonschema.Schema{
					"groups": {
						AdditionalProperties: &jsonschema.Schema{
							Properties: map[string]*jsonschema.Schema{
								"routes": {
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
	removePropertyAtPath(schema, []string{"routes", "groups", "*", "routes", "*", "group"})
	target := schema.Properties["routes"].Properties["groups"].AdditionalProperties.Properties["routes"].Items
	if _, ok := target.Properties["group"]; ok {
		t.Fatalf("expected nested property to be removed")
	}

	out := filepath.Join(t.TempDir(), "config", "schema.json")
	if err := writeSchema(out, schema); err != nil {
		t.Fatalf("writeSchema error: %v", err)
	}
	if _, err := os.Stat(out); err != nil {
		t.Fatalf("expected schema output file: %v", err)
	}
}
