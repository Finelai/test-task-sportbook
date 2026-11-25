package contracts

import "testing"

func testLineOffer(t *testing.T, doc openAPIDocument) {
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

}
