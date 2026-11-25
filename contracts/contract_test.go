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
			RequestBody struct {
				Required bool `yaml:"required"`
				Content  map[string]struct {
					Schema struct {
						Ref string `yaml:"$ref"`
					} `yaml:"schema"`
				} `yaml:"content"`
			} `yaml:"requestBody"`
			Responses map[string]struct {
				Content map[string]struct {
					Schema struct {
						Ref   string `yaml:"$ref"`
						OneOf []struct {
							Ref string `yaml:"$ref"`
						} `yaml:"oneOf"`
					} `yaml:"schema"`
				} `yaml:"content"`
			} `yaml:"responses"`
		} `yaml:"paths"`
		Components struct {
			Schemas map[string]struct {
				Required   []string `yaml:"required"`
				Properties map[string]struct {
					Ref    string   `yaml:"$ref"`
					Type   string   `yaml:"type"`
					Format string   `yaml:"format"`
					Enum   []string `yaml:"enum"`
					Items  struct {
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
	events, ok := doc.Paths["/api/v1/line/events"]["get"]
	if !ok {
		t.Fatal("GET /api/v1/line/events missing")
	}
	stream, ok := events.Responses["200"].Content["text/event-stream"]
	if !ok || len(stream.Schema.OneOf) != 2 || stream.Schema.OneOf[0].Ref != "#/components/schemas/LineSnapshot" || stream.Schema.OneOf[1].Ref != "#/components/schemas/MatchUpdated" {
		t.Error("event stream must describe snapshot and match_updated payloads")
	}
	updated := doc.Components.Schemas["MatchUpdated"]
	for _, field := range []string{"version", "match"} {
		found := false
		for _, required := range updated.Required {
			if required == field {
				found = true
			}
		}
		if !found {
			t.Errorf("MatchUpdated.%s is not required", field)
		}
	}
	if updated.Properties["version"].Type != "integer" || updated.Properties["match"].Ref != "#/components/schemas/Match" {
		t.Error("match_updated must contain a version and a Match")
	}

	offerOp, ok := doc.Paths["/api/v1/line/offers"]["post"]
	if !ok {
		t.Fatal("POST /api/v1/line/offers missing")
	}
	if !offerOp.RequestBody.Required || offerOp.RequestBody.Content["application/json"].Schema.Ref != "#/components/schemas/OfferRequest" {
		t.Error("required JSON offer request missing")
	}
	if offerOp.Responses["200"].Content["application/json"].Schema.Ref != "#/components/schemas/Offer" {
		t.Error("200 JSON offer missing")
	}
	for _, status := range []string{"400", "409", "503"} {
		if offerOp.Responses[status].Content["application/json"].Schema.Ref != "#/components/schemas/Error" {
			t.Errorf("%s JSON error missing", status)
		}
	}
	for name, fields := range map[string][]string{"OfferRequest": {"matchId", "outcome"}, "Offer": {"offerId", "matchId", "outcome", "odds", "expiresAt"}} {
		schema := doc.Components.Schemas[name]
		for _, field := range fields {
			found := false
			for _, required := range schema.Required {
				if required == field {
					found = true
				}
			}
			if _, exists := schema.Properties[field]; !found || !exists {
				t.Errorf("%s.%s must be present and required", name, field)
			}
		}
	}
	for _, name := range []string{"OfferRequest", "Offer"} {
		outcome := doc.Components.Schemas[name].Properties["outcome"].Enum
		if len(outcome) != 3 || outcome[0] != "HOME" || outcome[1] != "DRAW" || outcome[2] != "AWAY" {
			t.Errorf("%s outcomes: %v", name, outcome)
		}
	}
	offer := doc.Components.Schemas["Offer"].Properties
	if offer["odds"].Type != "string" || offer["expiresAt"].Type != "string" || offer["expiresAt"].Format != "date-time" {
		t.Error("offer odds must be a string and expiresAt a date-time")
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
