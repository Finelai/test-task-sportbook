package contracts

import (
	"go.yaml.in/yaml/v3"
	"os"
	"testing"
)

type openAPIDocument struct {
	Paths map[string]map[string]struct {
		Parameters []struct {
			Name     string `yaml:"name"`
			In       string `yaml:"in"`
			Required bool   `yaml:"required"`
		} `yaml:"parameters"`
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
			OneOf []struct {
				Ref string `yaml:"$ref"`
			} `yaml:"oneOf"`
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

func loadOpenAPI(t *testing.T) openAPIDocument {
	t.Helper()
	data, err := os.ReadFile("openapi/v1.yaml")
	if err != nil {
		t.Fatalf("read OpenAPI contract: %v", err)
	}
	var doc openAPIDocument
	if err := yaml.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	return doc
}

func TestPublishedContracts(t *testing.T) {
	doc := loadOpenAPI(t)
	t.Run("line_snapshot", func(t *testing.T) { testLineSnapshot(t, doc) })
	t.Run("line_events", func(t *testing.T) { testLineEvents(t, doc) })
	t.Run("line_offer", func(t *testing.T) { testLineOffer(t, doc) })
	t.Run("create_bet", func(t *testing.T) { testCreateBet(t, doc) })
	t.Run("get_bet", func(t *testing.T) { testGetBet(t, doc) })
	t.Run("get_wallet", func(t *testing.T) { testGetWallet(t, doc) })
}
