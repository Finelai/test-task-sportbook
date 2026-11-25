package contracts

import (
	"os"
	"testing"

	"go.yaml.in/yaml/v3"
)

func TestPublishedContracts(t *testing.T) {
	data, err := os.ReadFile("openapi/v1.yaml")
	if err != nil {
		t.Fatalf("read OpenAPI contract: %v", err)
	}
	var doc struct {
		Paths map[string]map[string]struct {
			Responses map[string]struct {
				Content map[string]struct {
					Schema struct {
						Ref string `yaml:"$ref"`
					} `yaml:"schema"`
				} `yaml:"content"`
			} `yaml:"responses"`
		} `yaml:"paths"`
		Components struct {
			Schemas map[string]struct {
				Required   []string `yaml:"required"`
				Properties map[string]struct {
					Type  string   `yaml:"type"`
					Enum  []string `yaml:"enum"`
					Items struct {
						Ref  string   `yaml:"$ref"`
						Enum []string `yaml:"enum"`
					} `yaml:"items"`
					MinItems int `yaml:"minItems"`
					MaxItems int `yaml:"maxItems"`
				} `yaml:"properties"`
			} `yaml:"schemas"`
		} `yaml:"components"`
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	op, ok := doc.Paths["/api/v1/line"]["get"]
	if !ok {
		t.Fatal("GET /api/v1/line missing")
	}
	response, ok := op.Responses["200"]
	if !ok {
		t.Fatal("200 response missing")
	}
	content, ok := response.Content["application/json"]
	if !ok || content.Schema.Ref != "#/components/schemas/LineSnapshot" {
		t.Fatal("200 JSON snapshot missing")
	}
	for name, fields := range map[string][]string{"LineSnapshot": {"version", "matches"}, "Match": {"matchId", "homeTeam", "awayTeam", "open", "quotes"}, "Quote": {"outcome", "odds"}} {
		schema, ok := doc.Components.Schemas[name]
		if !ok {
			t.Fatalf("schema %s missing", name)
		}
		for _, field := range fields {
			found := false
			for _, required := range schema.Required {
				if required == field {
					found = true
				}
			}
			if !found {
				t.Errorf("%s.%s is not required", name, field)
			}
			if _, ok := schema.Properties[field]; !ok {
				t.Errorf("%s.%s missing", name, field)
			}
		}
	}
	schemas := doc.Components.Schemas
	if schemas["LineSnapshot"].Properties["matches"].Items.Ref != "#/components/schemas/Match" {
		t.Error("matches must contain Match")
	}
	quotes := schemas["Match"].Properties["quotes"]
	if quotes.Items.Ref != "#/components/schemas/Quote" || quotes.MinItems != 3 || quotes.MaxItems != 3 {
		t.Error("quotes must contain three Quote items")
	}
	quote := schemas["Quote"].Properties
	if quote["odds"].Type != "string" {
		t.Error("odds must be a string")
	}
	outcomes := quote["outcome"].Enum
	if len(outcomes) != 3 || outcomes[0] != "HOME" || outcomes[1] != "DRAW" || outcomes[2] != "AWAY" {
		t.Errorf("unexpected outcomes: %v", outcomes)
	}
}
