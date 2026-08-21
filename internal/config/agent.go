package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

// ParseBytes parses a GitMake configuration from UTF-8 JSON, applying the same
// defaults and strict validation used by Load, without touching the filesystem.
func ParseBytes(data []byte) (Config, error) {
	if len(data) >= 2 && ((data[0] == 0xFF && data[1] == 0xFE) || (data[0] == 0xFE && data[1] == 0xFF)) {
		return Config{}, fmt.Errorf("parse config: gitmake.json must be UTF-8, not UTF-16")
	}
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var c Config
	if err := dec.Decode(&c); err != nil {
		return Config{}, fmt.Errorf("parse config: %w", err)
	}
	var trailing any
	if err := dec.Decode(&trailing); err != io.EOF {
		if err == nil {
			return Config{}, fmt.Errorf("parse config: multiple JSON values are not allowed")
		}
		return Config{}, fmt.Errorf("parse config trailing content: %w", err)
	}
	applyDefaults(&c)
	if err := c.Validate(); err != nil {
		return Config{}, fmt.Errorf("config validation: %w", err)
	}
	return c, nil
}

// MarshalNormalized returns a stable, default-expanded representation suitable
// for agent-authored configuration writes.
func MarshalNormalized(c Config) ([]byte, error) {
	applyDefaults(&c)
	if err := c.Validate(); err != nil {
		return nil, err
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("build config: %w", err)
	}
	return append(b, '\n'), nil
}

// SchemaDocument returns GitMake's machine-readable JSON Schema. It is kept
// local so an agent never needs network access or guessed documentation.
func SchemaDocument() map[string]any {
	return map[string]any{
		"$schema":              "https://json-schema.org/draft/2020-12/schema",
		"$id":                  "gitmake.config/v1",
		"title":                "GitMake configuration",
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"repo", "source"},
		"properties": map[string]any{
			"schema_version": map[string]any{"type": "integer", "const": CurrentSchemaVersion, "default": CurrentSchemaVersion},
			"repo": map[string]any{
				"type": "object", "additionalProperties": false, "required": []string{"name"},
				"properties": map[string]any{
					"owner":       map[string]any{"type": "string", "pattern": `^[A-Za-z0-9-]+$`},
					"name":        map[string]any{"type": "string", "pattern": `^[A-Za-z0-9._-]+$`},
					"description": map[string]any{"type": "string"},
					"visibility":  map[string]any{"type": "string", "enum": []string{"private", "public", "internal"}, "default": "private"},
				},
			},
			"source": map[string]any{
				"type": "object", "additionalProperties": false, "required": []string{"zip"},
				"properties": map[string]any{
					"zip":        map[string]any{"type": "string", "minLength": 1},
					"strip_root": map[string]any{"type": "boolean", "default": true},
				},
			},
			"git": map[string]any{
				"type": "object", "additionalProperties": false,
				"properties": map[string]any{
					"branch":                 map[string]any{"type": "string", "default": "main"},
					"initial_commit_message": map[string]any{"type": "string", "default": "Initial commit"},
					"commit_message":         map[string]any{"type": "string", "default": "Update repository"},
				},
			},
			"release": map[string]any{
				"type": "object", "additionalProperties": false,
				"properties": map[string]any{
					"enabled":        map[string]any{"type": "boolean", "default": false},
					"tag":            map[string]any{"type": "string"},
					"title":          map[string]any{"type": "string"},
					"notes":          map[string]any{"type": "string"},
					"notes_file":     map[string]any{"type": "string"},
					"generate_notes": map[string]any{"type": "boolean"},
					"assets":         map[string]any{"type": "array", "items": map[string]any{"type": "string", "minLength": 1}, "uniqueItems": true},
					"draft":          map[string]any{"type": "boolean", "default": false},
					"prerelease":     map[string]any{"type": "boolean", "default": false},
					"latest":         map[string]any{"type": "boolean"},
					"on_existing":    map[string]any{"type": "string", "enum": []string{"error", "skip", "resume"}, "default": "error"},
				},
				"allOf": []any{
					map[string]any{
						"if":   map[string]any{"properties": map[string]any{"enabled": map[string]any{"const": true}}, "required": []string{"enabled"}},
						"then": map[string]any{"required": []string{"tag"}},
					},
				},
			},
		},
	}
}
