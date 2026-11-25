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

func testCreditWallet(t *testing.T, doc openAPIDocument) {
	op, ok := doc.Paths["/api/v1/wallet/credits"]["post"]
	if !ok {
		t.Fatal("POST /api/v1/wallet/credits missing")
	}
	required := false
	for _, p := range op.Parameters {
		if p.Name == "Idempotency-Key" && p.In == "header" && p.Required {
			required = true
		}
	}
	if !required {
		t.Error("Idempotency-Key header must be required")
	}
	if !op.RequestBody.Required || op.RequestBody.Content["application/json"].Schema.Ref != "#/components/schemas/CreditRequest" {
		t.Error("required JSON credit request missing")
	}
	if op.Responses["201"].Content["application/json"].Schema.Ref != "#/components/schemas/CreditResult" {
		t.Error("201 must return JSON CreditResult")
	}
	for _, status := range []string{"400", "409", "503"} {
		if op.Responses[status].Content["application/json"].Schema.Ref != "#/components/schemas/Error" {
			t.Errorf("%s must return JSON Error", status)
		}
	}
	for schemaName, fields := range map[string][]string{"CreditRequest": {"amount"}, "CreditResult": {"operationId", "available", "reserved"}} {
		schema := doc.Components.Schemas[schemaName]
		for _, field := range fields {
			found := false
			for _, name := range schema.Required {
				if name == field {
					found = true
				}
			}
			if !found || schema.Properties[field].Type != "string" {
				t.Errorf("%s.%s must be a required string", schemaName, field)
			}
		}
	}
}
