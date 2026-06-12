package llm

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/invopop/jsonschema"
)

// generateJSONSchema accepts a Go struct, map[string]any, or raw JSON bytes and returns a fully dereferenced JSON schema map with all fields marked as required.
func generateJSONSchema(v any) (map[string]any, error) {
	if v == nil {
		return nil, errors.New("schema is nil")
	}

	var result map[string]any

	switch val := v.(type) {
	case map[string]any:
		// Deep copy the map
		data, err := json.Marshal(val)
		if err != nil {
			return nil, err
		}
		if err := json.Unmarshal(data, &result); err != nil {
			return nil, err
		}
	case []byte:
		if err := json.Unmarshal(val, &result); err != nil {
			return nil, fmt.Errorf("failed to parse schema bytes: %w", err)
		}
	default:
		res, err := reflectJSONSchema(val)
		if err != nil {
			return nil, err
		}
		result = res
	}

	// Extract definitions
	var defs map[string]any
	if defsVal, ok := result["$defs"].(map[string]any); ok {
		defs = defsVal
	} else if defsVal, ok := result["definitions"].(map[string]any); ok {
		defs = defsVal
	}

	// Remove schema metadata fields not supported or required by Structured Outputs APIs
	delete(result, "$schema")
	delete(result, "$defs")
	delete(result, "definitions")

	// Recursively dereference all local $ref values
	dereferenced := dereferenceRefs(result, defs)
	if resultMap, ok := dereferenced.(map[string]any); ok {
		result = resultMap
	}

	// Enforce all properties are marked as required recursively
	makeAllPropertiesRequired(result)

	return result, nil
}

// reflectJSONSchema reflects a Go struct definition dynamically with panic recovery.
func reflectJSONSchema(v any) (map[string]any, error) {
	var reflectErr error
	var schema *jsonschema.Schema
	func() {
		defer func() {
			if r := recover(); r != nil {
				reflectErr = fmt.Errorf("jsonschema reflection panic: %v", r)
			}
		}()
		reflector := jsonschema.Reflector{
			AllowAdditionalProperties:  false,
			RequiredFromJSONSchemaTags: true,
			ExpandedStruct:             true,
		}
		schema = reflector.Reflect(v)
	}()

	if reflectErr != nil {
		return nil, reflectErr
	}
	if schema == nil {
		return nil, fmt.Errorf("failed to reflect schema for type: %T", v)
	}
	data, err := json.Marshal(schema)
	if err != nil {
		return nil, err
	}
	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// dereferenceRefs recursively replaces any "$ref" nodes with their definitions from defs.
func dereferenceRefs(node any, defs map[string]any) any {
	m, ok := node.(map[string]any)
	if !ok {
		if s, ok := node.([]any); ok {
			for i, v := range s {
				s[i] = dereferenceRefs(v, defs)
			}
			return s
		}
		return node
	}

	if resolved, ok := resolveRef(m, defs); ok {
		return dereferenceRefs(resolved, defs)
	}

	for k, v := range m {
		m[k] = dereferenceRefs(v, defs)
	}
	return m
}

// resolveRef checks if node is a $ref and resolves it.
func resolveRef(m map[string]any, defs map[string]any) (any, bool) {
	ref, exists := m["$ref"]
	if !exists {
		return nil, false
	}
	refStr, ok := ref.(string)
	if !ok {
		return nil, false
	}
	// E.g. "#/$defs/Nested" or "#/definitions/Nested"
	parts := strings.Split(refStr, "/")
	if len(parts) != 3 || (parts[1] != "$defs" && parts[1] != "definitions") {
		return nil, false
	}
	defName := parts[2]
	defVal, found := defs[defName]
	if !found {
		return nil, false
	}

	// Deep clone the definition value to avoid mutation issues
	clonedDef := cloneVal(defVal)
	// Merge other keys if any
	if clonedDefMap, isMap := clonedDef.(map[string]any); isMap {
		for k, v := range m {
			if k != "$ref" {
				clonedDefMap[k] = v
			}
		}
		return clonedDefMap, true
	}
	return clonedDef, true
}

// makeAllPropertiesRequired recursively marks all properties of type "object" as required.
func makeAllPropertiesRequired(node any) {
	m, ok := node.(map[string]any)
	if !ok {
		return
	}

	if props, exists := m["properties"]; exists {
		if propsMap, isMap := props.(map[string]any); isMap {
			var reqList []any
			for k := range propsMap {
				reqList = append(reqList, k)
			}
			m["required"] = reqList
		}
	}

	for _, val := range m {
		if valMap, isMap := val.(map[string]any); isMap {
			makeAllPropertiesRequired(valMap)
		} else if valSlice, isSlice := val.([]any); isSlice {
			for _, item := range valSlice {
				if itemMap, isMap := item.(map[string]any); isMap {
					makeAllPropertiesRequired(itemMap)
				}
			}
		}
	}
}

func cloneVal(v any) any {
	b, _ := json.Marshal(v)
	var res any
	_ = json.Unmarshal(b, &res)
	return res
}
