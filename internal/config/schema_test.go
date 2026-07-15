package config

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"gopkg.in/yaml.v3"
)

const schemaPath = "../../schema/mox.schema.json"

// compileSchema compiles the published config schema. The schema is
// editor-side assistance (yaml-language-server); these tests keep it in sync
// with what mox actually accepts.
func compileSchema(t *testing.T) *jsonschema.Schema {
	t.Helper()
	c := jsonschema.NewCompiler()
	sch, err := c.Compile(schemaPath)
	if err != nil {
		t.Fatalf("compile %s: %v", schemaPath, err)
	}
	return sch
}

// yamlToJSONValue converts YAML bytes to the JSON-decoded value form the
// jsonschema library validates against.
func yamlToJSONValue(t *testing.T, data []byte) any {
	t.Helper()
	var v any
	if err := yaml.Unmarshal(data, &v); err != nil {
		t.Fatalf("parse yaml: %v", err)
	}
	jsonBytes, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("convert to json: %v", err)
	}
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(jsonBytes))
	if err != nil {
		t.Fatalf("decode json: %v", err)
	}
	return doc
}

func TestSchemaValidatesExampleConfig(t *testing.T) {
	sch := compileSchema(t)
	data, err := os.ReadFile("../../examples/config.yml")
	if err != nil {
		t.Fatalf("read example config: %v", err)
	}
	if err := sch.Validate(yamlToJSONValue(t, data)); err != nil {
		t.Errorf("examples/config.yml does not validate against schema: %v", err)
	}
}

func TestSchemaValidatesInitScaffold(t *testing.T) {
	sch := compileSchema(t)
	data, err := yaml.Marshal(exampleConfig())
	if err != nil {
		t.Fatalf("marshal scaffold config: %v", err)
	}
	if err := sch.Validate(yamlToJSONValue(t, data)); err != nil {
		t.Errorf("mox init scaffold does not validate against schema: %v", err)
	}
}

func TestSchemaRejectsInvalidConfigs(t *testing.T) {
	sch := compileSchema(t)
	cases := []struct {
		name string
		yaml string
	}{
		{"typo in session key", `
sessions:
  web:
    hots: [a, b]
`},
		{"typo in window key", `
sessions:
  web:
    windows:
      - name: w
        hoss: [a]
`},
		{"retry out of range", `
sessions:
  web:
    hosts: [a]
    retry: 42
`},
		{"invalid arrange", `
sessions:
  web:
    hosts: [a]
    arrange: diagonal
`},
		{"invalid split", `
sessions:
  web:
    windows:
      - name: w
        panes:
          - split: sideways
`},
		{"size out of range", `
sessions:
  web:
    windows:
      - name: w
        panes:
          - split: root
          - split: vertical
            size: 150
`},
		{"unknown top-level key", `
sesions:
  web:
    hosts: [a]
`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := sch.Validate(yamlToJSONValue(t, []byte(tc.yaml))); err == nil {
				t.Errorf("schema accepted invalid config:\n%s", tc.yaml)
			}
		})
	}
}
