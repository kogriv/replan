package domain_test

import (
	"testing"

	"github.com/kogriv/replan/internal/domain"
)

func TestBaselineSealVerifiesAndChangesWithContent(t *testing.T) {
	amount := 3.0
	baseline := domain.Baseline{
		ID: "baseline.test", Name: "Test", CreatedAt: "2026-10-28T10:00:00+07:00",
		ApprovedBy: domain.Ref{Kind: "actor", ID: "actor.test"},
		IncludedDemands: []domain.BaselineDemandEntry{{
			Ref: domain.Ref{Kind: "resource_demand", ID: "demand.test"}, Task: "task.test",
			Resource: "resource.test", Unit: "hours", PlannedAmount: &amount,
		}},
	}
	if err := domain.SealBaseline(&baseline); err != nil {
		t.Fatal(err)
	}
	digest, err := domain.BaselineDigest(baseline)
	if err != nil {
		t.Fatal(err)
	}
	if digest != baseline.Seal.Digest || baseline.Seal.Algorithm != "sha256" {
		t.Fatalf("seal = %#v digest = %s", baseline.Seal, digest)
	}
	baseline.Name = "Changed"
	changed, err := domain.BaselineDigest(baseline)
	if err != nil {
		t.Fatal(err)
	}
	if changed == digest {
		t.Fatal("content change did not change baseline digest")
	}
}
