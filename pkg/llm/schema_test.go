package llm

import (
	"testing"

	"github.com/invopop/jsonschema"
)

type SubStruct struct {
	Field string `json:"field"`
}

type RootStruct struct {
	Name    *string   `json:"name" jsonschema:"nullable"`
	Age     int       `json:"age"`
	Details SubStruct `json:"details"`
}

func TestGenerateJSONSchema_Struct(t *testing.T) {
	schema, err := generateJSONSchema(&RootStruct{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if schema == nil {
		t.Fatal("expected schema, got nil")
	}

	// Verify $schema is not present
	if _, exists := schema["$schema"]; exists {
		t.Error("expected $schema to be deleted from root")
	}

	// Verify defs are not present at the root
	if _, exists := schema["$defs"]; exists {
		t.Error("expected $defs to be deleted from root")
	}
	if _, exists := schema["definitions"]; exists {
		t.Error("expected definitions to be deleted from root")
	}

	// Verify type and required
	if tVal := schema["type"]; tVal != "object" {
		t.Errorf("expected root type 'object', got %v", tVal)
	}

	reqs, ok := schema["required"].([]any)
	if !ok {
		t.Fatal("expected required array at root")
	}
	if len(reqs) != 3 {
		t.Errorf("expected 3 required properties at root, got %d", len(reqs))
	}

	// Verify properties details
	props, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatal("expected properties map at root")
	}

	nameProp, ok := props["name"].(map[string]any)
	if !ok {
		t.Fatal("expected name property map")
	}
	if _, exists := nameProp["oneOf"]; !exists {
		t.Error("expected name property to use oneOf for nullable")
	}

	detailsProp, ok := props["details"].(map[string]any)
	if !ok {
		t.Fatal("expected details property map")
	}
	if _, exists := detailsProp["$ref"]; exists {
		t.Error("expected details $ref to be dereferenced and inlined")
	}
	if detailsType := detailsProp["type"]; detailsType != "object" {
		t.Errorf("expected details type 'object', got %v", detailsType)
	}
	detailsReqs, ok := detailsProp["required"].([]any)
	if !ok {
		t.Fatal("expected required array in details")
	}
	if len(detailsReqs) != 1 || detailsReqs[0] != "field" {
		t.Errorf("expected required field 'field' in details, got %v", detailsReqs)
	}
}

func TestGenerateJSONSchema_Map(t *testing.T) {
	inputMap := map[string]any{
		"$schema": "http://json-schema.org/draft-07/schema#",
		"type":    "object",
		"properties": map[string]any{
			"foo": map[string]any{
				"type": "string",
			},
		},
	}

	schema, err := generateJSONSchema(inputMap)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, exists := schema["$schema"]; exists {
		t.Error("expected $schema to be deleted from root")
	}

	reqs, ok := schema["required"].([]any)
	if !ok {
		t.Fatal("expected required array")
	}
	if len(reqs) != 1 || reqs[0] != "foo" {
		t.Errorf("expected required properties [foo], got %v", reqs)
	}
	if addProps, exists := schema["additionalProperties"]; !exists || addProps != false {
		t.Errorf("expected additionalProperties to be false, got %v", addProps)
	}
}

func TestGenerateJSONSchema_Bytes(t *testing.T) {
	inputJSON := `{
		"$schema": "http://json-schema.org/draft-07/schema#",
		"type": "object",
		"properties": {
			"bar": {
				"type": "integer"
			}
		}
	}`

	schema, err := generateJSONSchema([]byte(inputJSON))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, exists := schema["$schema"]; exists {
		t.Error("expected $schema to be deleted from root")
	}

	reqs, ok := schema["required"].([]any)
	if !ok {
		t.Fatal("expected required array")
	}
	if len(reqs) != 1 || reqs[0] != "bar" {
		t.Errorf("expected required properties [bar], got %v", reqs)
	}
}

func TestGenerateJSONSchema_BytesError(t *testing.T) {
	_, err := generateJSONSchema([]byte(`{invalid-json`))
	if err == nil {
		t.Error("expected error for invalid JSON bytes, got nil")
	}
}

func TestGenerateJSONSchema_Nil(t *testing.T) {
	_, err := generateJSONSchema(nil)
	if err == nil {
		t.Error("expected error for nil schema, got nil")
	}
}

func TestGenerateJSONSchema_UnsupportedType(t *testing.T) {
	// A channel cannot be marshaled to JSON, which will trigger an error in jsonschema
	ch := make(chan int)
	_, err := generateJSONSchema(ch)
	if err == nil {
		t.Error("expected reflection error for channel, got nil")
	}
}

func TestDereferenceRefs_DefinitionsAlias(t *testing.T) {
	// Test that dereferencing supports "definitions" alias as well as "$defs"
	schema := map[string]any{
		"definitions": map[string]any{
			"Value": map[string]any{
				"type": "string",
			},
		},
		"properties": map[string]any{
			"val": map[string]any{
				"$ref": "#/definitions/Value",
			},
		},
	}

	// generateJSONSchema handles extraction of "definitions"
	res, err := generateJSONSchema(schema)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	props, ok := res["properties"].(map[string]any)
	if !ok {
		t.Fatal("expected properties map")
	}

	valProp, ok := props["val"].(map[string]any)
	if !ok {
		t.Fatal("expected val property map")
	}

	if tVal := valProp["type"]; tVal != "string" {
		t.Errorf("expected dereferenced type 'string', got %v", tVal)
	}
}

func TestGenerateJSONSchema_RefEdgeCases(t *testing.T) {
	// 1. $ref is not a string
	schema1 := map[string]any{
		"properties": map[string]any{
			"val": map[string]any{
				"$ref": 123,
			},
		},
	}
	_, err := generateJSONSchema(schema1)
	if err != nil {
		t.Errorf("unexpected error for non-string $ref: %v", err)
	}

	// 2. $ref has invalid format (does not split to 3 parts)
	schema2 := map[string]any{
		"properties": map[string]any{
			"val": map[string]any{
				"$ref": "#/defs/Nested/Extra",
			},
		},
	}
	_, err = generateJSONSchema(schema2)
	if err != nil {
		t.Errorf("unexpected error for invalid path $ref: %v", err)
	}

	// 3. $ref points to non-existent definition
	schema3 := map[string]any{
		"$defs": map[string]any{
			"Existing": map[string]any{"type": "string"},
		},
		"properties": map[string]any{
			"val": map[string]any{
				"$ref": "#/$defs/NonExistent",
			},
		},
	}
	_, err = generateJSONSchema(schema3)
	if err != nil {
		t.Errorf("unexpected error for non-existent $ref: %v", err)
	}

	// 4. $ref points to non-map definition (e.g., string)
	schema4 := map[string]any{
		"$defs": map[string]any{
			"Primitive": "just-a-string-definition",
		},
		"properties": map[string]any{
			"val": map[string]any{
				"$ref": "#/$defs/Primitive",
			},
		},
	}
	res, err := generateJSONSchema(schema4)
	if err != nil {
		t.Fatalf("unexpected error for primitive $ref: %v", err)
	}
	props, ok := res["properties"].(map[string]any)
	if !ok {
		t.Fatal("expected properties map")
	}
	valProp, ok := props["val"].(string)
	if !ok || valProp != "just-a-string-definition" {
		t.Errorf("expected primitive definition value to be inlined, got %v", props["val"])
	}
}

type PanicSchemaStruct struct{}

func (PanicSchemaStruct) JSONSchema() *jsonschema.Schema {
	panic("forced reflection panic")
}

func TestGenerateJSONSchema_PanicRecovery(t *testing.T) {
	_, err := generateJSONSchema(PanicSchemaStruct{})
	if err == nil {
		t.Error("expected error from panic recovery, got nil")
	}
}
