package contracts

import (
	linev1 "github.com/finelai/test-task-sportbook/contracts/generated/line/v1"
	"google.golang.org/protobuf/reflect/protoreflect"
	"testing"
)

func testLineGRPCContract(t *testing.T) {
	file := linev1.File_line_v1_line_proto
	svc := file.Services().ByName("LineService")
	if svc == nil {
		t.Fatal("LineService missing")
	}
	if svc.Methods().Len() != 1 {
		t.Errorf("line service has %d operations, want 1", svc.Methods().Len())
	}
	method := svc.Methods().ByName("ConfirmOffer")
	if method == nil {
		t.Fatal("ConfirmOffer missing")
	}
	for _, field := range []protoreflect.Name{"bet_id", "player_id", "offer_id", "expected_odds_hundredths"} {
		if method.Input().Fields().ByName(field) == nil {
			t.Errorf("request field %s missing", field)
		}
	}
	response := method.Output()
	if response.Oneofs().ByName("result") == nil || response.Fields().ByName("confirmed") == nil || response.Fields().ByName("rejected") == nil {
		t.Error("typed confirmation and rejection results required")
	}
	confirmed := file.Messages().ByName("ConfirmedOffer")
	if confirmed == nil {
		t.Fatal("ConfirmedOffer missing")
	}
	for _, field := range []protoreflect.Name{"match_id", "outcome", "odds_hundredths"} {
		if confirmed.Fields().ByName(field) == nil {
			t.Errorf("confirmation field %s missing", field)
		}
	}
	rejection := file.Messages().ByName("RejectedOffer")
	if rejection == nil || rejection.Fields().ByName("reason") == nil || rejection.Fields().ByName("reason").Kind() != protoreflect.EnumKind {
		t.Error("typed rejection reason required")
	}
}
