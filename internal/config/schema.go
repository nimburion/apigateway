package config

import "github.com/google/jsonschema-go/jsonschema"

func (Gateway) CustomizeSchema(schema *jsonschema.Schema) error {
	if schema == nil {
		return nil
	}

	removeProperty(schema, "config_dir")
	removePropertyAtPath(schema, []string{"routes", "groups", "*", "routes", "*", "group"})
	removePropertyAtPath(schema, []string{"routes", "groups", "*", "websockets", "*", "group"})
	removePropertyAtPath(schema, []string{"routes", "groups", "*", "routes", "*", "openapi", "resolved_file"})

	addRoutingConstraints(schema)
	applyGatewaySchemaConstraints(schema)
	return nil
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
		databaseSourceSchema(),
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
		AllOf: []*jsonschema.Schema{
			emptyRoutesFiles,
			emptyRoutesGroups,
			{Not: databaseSourceSchema()},
		},
	}
}

func databaseSourceSchema() *jsonschema.Schema {
	databaseSource := any(ConfigSourceOfTruthDatabase)
	return &jsonschema.Schema{
		Required: []string{"config_store"},
		Properties: map[string]*jsonschema.Schema{
			"config_store": {
				Required: []string{"source_of_truth"},
				Properties: map[string]*jsonschema.Schema{
					"source_of_truth": {Const: &databaseSource},
				},
			},
		},
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
			mode := ensureProperty(openapiSchema, "mode")
			mode.Enum = []any{OpenAPIValidationModeStrict, OpenAPIValidationModeWarnOnly}

			fileSchema := ensureProperty(openapiSchema, "file")
			if fileSchema.MinLength == nil {
				fileSchema.MinLength = intPtr(1)
			}
			if !containsString(openapiSchema.Required, "file") {
				openapiSchema.Required = append(openapiSchema.Required, "file")
			}
			removeProperty(openapiSchema, "resolved_file")
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

		if portalSchema := s.Properties["portal"]; portalSchema != nil {
			if portalSchema.Properties == nil {
				portalSchema.Properties = map[string]*jsonschema.Schema{}
			}
			mode := ensureProperty(portalSchema, "mode")
			mode.Enum = []any{PortalModeReadOnly, PortalModeManaged}
		}

		if configStoreSchema := s.Properties["config_store"]; configStoreSchema != nil {
			if configStoreSchema.Properties == nil {
				configStoreSchema.Properties = map[string]*jsonschema.Schema{}
			}
			sourceOfTruth := ensureProperty(configStoreSchema, "source_of_truth")
			sourceOfTruth.Enum = []any{ConfigSourceOfTruthFile, ConfigSourceOfTruthDatabase}
			backend := ensureProperty(configStoreSchema, "backend")
			backend.Enum = []any{"", ConfigStoreBackendPostgres}
		}

		if metadataSchema := s.Properties["metadata"]; metadataSchema != nil {
			if metadataSchema.Properties == nil {
				metadataSchema.Properties = map[string]*jsonschema.Schema{}
			}
			visibility := ensureProperty(metadataSchema, "visibility")
			visibility.Enum = []any{MetadataVisibilityPublic, MetadataVisibilityPartner, MetadataVisibilityInternal}
			status := ensureProperty(metadataSchema, "status")
			status.Enum = []any{MetadataStatusActive, MetadataStatusDeprecated, MetadataStatusExperimental}
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

func intPtr(value int) *int {
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
