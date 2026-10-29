package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

// CalibrationDigest hashes the stable JSON encoding with its seal omitted.
func CalibrationDigest(calibration Calibration) (string, error) {
	calibration.SealSHA256 = ""
	data, err := json.Marshal(calibration)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func SealCalibration(calibration *Calibration) error {
	digest, err := CalibrationDigest(*calibration)
	if err != nil {
		return err
	}
	calibration.SealSHA256 = digest
	return nil
}
