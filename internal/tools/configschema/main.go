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
	configschema "github.com/nimburion/nimburion/pkg/config/schema"
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
	// #nosec G304 -- generator reads go.mod from the repository root it discovered.
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

func writeSchema(path string, schema any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("create schema dir: %w", err)
	}
	payload, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal schema: %w", err)
	}
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		return fmt.Errorf("write schema: %w", err)
	}
	return nil
}

func exitErr(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
