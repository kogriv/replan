package domain_test

import (
	"testing"

	"github.com/kogriv/replan/internal/domain"
)

func TestCalibrationSealVerifiesAndDetectsChanges(t *testing.T) {
	calibration := domain.Calibration{ID: "calibration.test", Result: domain.CalibrationResult{Factor: .5}}
	if err := domain.SealCalibration(&calibration); err != nil {
		t.Fatal(err)
	}
	digest, err := domain.CalibrationDigest(calibration)
	if err != nil || digest != calibration.SealSHA256 {
		t.Fatalf("digest=%q seal=%q err=%v", digest, calibration.SealSHA256, err)
	}
	calibration.Result.Factor = .75
	changed, err := domain.CalibrationDigest(calibration)
	if err != nil {
		t.Fatal(err)
	}
	if changed == calibration.SealSHA256 {
		t.Fatal("changed calibration still verifies")
	}
}
