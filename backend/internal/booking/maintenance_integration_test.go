package booking_test

import (
	"context"
	"testing"

	"campus-resource-platform/backend/internal/booking"
	"campus-resource-platform/backend/internal/dev"
)

func TestMaintenanceBlockOnInactiveRoomKeepsRoomInactive(t *testing.T) {
	h := setupHide(t)
	fixture := h.s.Catalog.(*dev.Fixture)
	fixture.RoomsData[0].Active = false

	actor := booking.Actor{Service: "M3"}
	block := booking.Operation{
		OperationID:      "inactive-room-block",
		Action:           "BLOCK_ROOM_MAINTENANCE",
		SourceModule:     "M3",
		SourceBusinessID: "inactive-room-workorder",
		SourceVersion:    1,
		RoomID:           "room-101",
	}
	receipt, err := h.s.Operate(context.Background(), actor, block, block.OperationID, "test")
	if err != nil || receipt.OperationStatus != "CONFIRMED" {
		t.Fatalf("block inactive room: receipt=%+v err=%v", receipt, err)
	}

	release := block
	release.OperationID = "inactive-room-release"
	release.Action = "RELEASE_SOURCE"
	release.SourceVersion = 2
	receipt, err = h.s.Operate(context.Background(), actor, release, release.OperationID, "test")
	if err != nil || receipt.OperationStatus != "CONFIRMED" {
		t.Fatalf("release maintenance source: receipt=%+v err=%v", receipt, err)
	}

	sources, err := h.s.Sources(context.Background(), actor)
	if err != nil || len(sources) != 1 || sources[0]["active"] != false {
		t.Fatalf("released source: sources=%v err=%v", sources, err)
	}
	if fixture.RoomsData[0].Active {
		t.Fatal("releasing maintenance must not reactivate the room")
	}
}
