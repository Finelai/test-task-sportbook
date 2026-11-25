package contracts

import "testing"

func testLineEvents(t *testing.T, doc openAPIDocument) {
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

}
