package app_test

import (
	"context"
	"testing"

	"github.com/kogriv/replan/internal/app"
	"github.com/kogriv/replan/internal/storage/filestore"
)

func TestAuditListVerifySmallTeamFixture(t *testing.T) {
	store := filestore.New("../../examples/small-team")
	service := app.NewAuditService(store)

	list, err := service.List(context.Background(), app.AuditListRequest{Since: "2026-07-01"})
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if len(list.Records) != 1 {
		t.Fatalf("expected 1 audit record, got %d", len(list.Records))
	}
	if list.Records[0].Operation != "create_baseline" {
		t.Fatalf("unexpected operation: %s", list.Records[0].Operation)
	}

	verify, err := service.Verify(context.Background(), app.AuditVerifyRequest{})
	if err != nil {
		t.Fatalf("Verify returned error: %v", err)
	}
	if !verify.Valid {
		t.Fatalf("expected audit to be valid, errors: %#v", verify.Errors)
	}
	if verify.Records != 1 {
		t.Fatalf("expected 1 audit record, got %d", verify.Records)
	}
}
