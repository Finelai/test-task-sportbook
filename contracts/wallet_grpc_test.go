package contracts

import (
	"testing"

	walletv1 "github.com/finelai/test-task-sportbook/contracts/generated/wallet/v1"
	"google.golang.org/protobuf/reflect/protoreflect"
)

func testWalletGRPCContract(t *testing.T) {
	file := walletv1.File_wallet_v1_wallet_proto
	service := file.Services().ByName("WalletService")
	if service == nil {
		t.Fatal("WalletService missing")
	}
	method := service.Methods().ByName("Reserve")
	if method == nil {
		t.Fatal("Reserve missing")
	}
	for name, kind := range map[protoreflect.Name]protoreflect.Kind{"bet_id": protoreflect.StringKind, "player_id": protoreflect.StringKind, "amount_minor": protoreflect.Int64Kind} {
		field := method.Input().Fields().ByName(name)
		if field == nil || field.Kind() != kind {
			t.Errorf("ReserveRequest.%s must have kind %v", name, kind)
		}
	}
	result := method.Output().Fields().ByName("result")
	if result == nil || result.Kind() != protoreflect.EnumKind {
		t.Fatal("Reserve result must be an enum")
	}
	for _, name := range []protoreflect.Name{"RESERVED", "INSUFFICIENT_FUNDS", "PARAMETER_CONFLICT"} {
		if result.Enum().Values().ByName(name) == nil {
			t.Errorf("reserve result %s missing", name)
		}
	}
	capture := service.Methods().ByName("Capture")
	if capture == nil {
		t.Fatal("Capture missing")
	}
	if capture.Input().Name() != "ReserveKey" {
		t.Error("Capture must accept ReserveKey")
	}
	for _, name := range []protoreflect.Name{"bet_id", "player_id"} {
		field := capture.Input().Fields().ByName(name)
		if field == nil || field.Kind() != protoreflect.StringKind {
			t.Errorf("ReserveKey.%s must be a string", name)
		}
	}
	captureResult := capture.Output().Fields().ByName("result")
	if captureResult == nil || captureResult.Kind() != protoreflect.EnumKind {
		t.Fatal("Capture result must be an enum")
	}
	for _, name := range []protoreflect.Name{"CAPTURED", "CAPTURE_PARAMETER_CONFLICT", "ALREADY_RELEASED"} {
		if captureResult.Enum().Values().ByName(name) == nil {
			t.Errorf("capture result %s missing", name)
		}
	}

}
