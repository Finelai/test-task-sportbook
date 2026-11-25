package contracts

import "testing"

func testGetWallet(t *testing.T, doc openAPIDocument) {
	op, ok := doc.Paths["/api/v1/wallet"]["get"]
	if !ok {
		t.Fatal("GET /api/v1/wallet missing")
	}
	if op.Responses["200"].Content["application/json"].Schema.Ref != "#/components/schemas/WalletBalance" {
		t.Error("200 must return JSON WalletBalance")
	}
	if op.Responses["503"].Content["application/json"].Schema.Ref != "#/components/schemas/Error" {
		t.Error("503 must return JSON Error")
	}
	balance := doc.Components.Schemas["WalletBalance"]
	for _, field := range []string{"available", "reserved"} {
		found := false
		for _, name := range balance.Required {
			if name == field {
				found = true
			}
		}
		if !found || balance.Properties[field].Type != "string" {
			t.Errorf("WalletBalance.%s must be a required string", field)
		}
	}
}
