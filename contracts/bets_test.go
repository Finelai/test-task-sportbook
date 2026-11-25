package contracts

import "testing"

func testCreateBet(t *testing.T, doc openAPIDocument) {
	betOp, ok := doc.Paths["/api/v1/bets"]["post"]
	if !ok {
		t.Fatal("POST /api/v1/bets missing")
	}
	keyRequired := false
	for _, p := range betOp.Parameters {
		if p.Name == "Idempotency-Key" && p.In == "header" && p.Required {
			keyRequired = true
		}
	}
	if !keyRequired {
		t.Error("Idempotency-Key header must be required")
	}
	if !betOp.RequestBody.Required || betOp.RequestBody.Content["application/json"].Schema.Ref != "#/components/schemas/CreateBetRequest" {
		t.Error("required JSON bet request missing")
	}
	if betOp.Responses["202"].Content["application/json"].Schema.Ref != "#/components/schemas/Bet" {
		t.Error("202 JSON bet missing")
	}
	for _, status := range []string{"400", "409", "503"} {
		if betOp.Responses[status].Content["application/json"].Schema.Ref != "#/components/schemas/Error" {
			t.Errorf("%s JSON error missing", status)
		}
	}
	for name, fields := range map[string][]string{"CreateBetRequest": {"offerId", "odds", "amount"}, "ActiveBet": {"betId", "status", "amount", "odds"}, "RejectedBet": {"betId", "status", "amount", "odds", "reason"}} {
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
		for _, field := range []string{"amount", "odds"} {
			if schema.Properties[field].Type != "string" {
				t.Errorf("%s.%s must be a string", name, field)
			}
		}
	}
	variants := doc.Components.Schemas["Bet"].OneOf
	if len(variants) != 2 || variants[0].Ref != "#/components/schemas/ActiveBet" || variants[1].Ref != "#/components/schemas/RejectedBet" {
		t.Error("bet must describe active and rejected variants")
	}
	for name, expected := range map[string][]string{"ActiveBet": {"pending", "accepted"}, "RejectedBet": {"rejected"}} {
		got := doc.Components.Schemas[name].Properties["status"].Enum
		if len(got) != len(expected) {
			t.Errorf("%s statuses: %v", name, got)
			continue
		}
		for i := range expected {
			if got[i] != expected[i] {
				t.Errorf("%s statuses: %v", name, got)
				break
			}
		}
	}
	reasons := doc.Components.Schemas["RejectedBet"].Properties["reason"].Enum
	wantReasons := []string{"offer_expired", "odds_changed", "offer_unavailable", "offer_mismatch", "market_closed", "insufficient_funds"}
	if len(reasons) != len(wantReasons) {
		t.Errorf("rejection reasons: %v", reasons)
	} else {
		for i := range wantReasons {
			if reasons[i] != wantReasons[i] {
				t.Errorf("rejection reasons: %v", reasons)
				break
			}
		}
	}

}

func testGetBet(t *testing.T, doc openAPIDocument) {
	op, ok := doc.Paths["/api/v1/bets/{betId}"]["get"]
	if !ok {
		t.Fatal("GET /api/v1/bets/{betId} missing")
	}
	required := false
	for _, p := range op.Parameters {
		if p.Name == "betId" && p.In == "path" && p.Required {
			required = true
		}
	}
	if !required {
		t.Error("betId path parameter must be required")
	}
	if op.Responses["200"].Content["application/json"].Schema.Ref != "#/components/schemas/Bet" {
		t.Error("200 must return JSON Bet")
	}
	for _, status := range []string{"404", "503"} {
		if op.Responses[status].Content["application/json"].Schema.Ref != "#/components/schemas/Error" {
			t.Errorf("%s must return JSON Error", status)
		}
	}
}
