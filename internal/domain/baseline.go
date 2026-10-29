package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

// BaselineDigest hashes the compact JSON encoding of the baseline with Seal
// omitted. Baseline contains structs and slices, not maps, so field order is
// the declared domain order and does not depend on source-file formatting.
func BaselineDigest(baseline Baseline) (string, error) {
	baseline.Seal = nil
	data, err := json.Marshal(baseline)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func SealBaseline(baseline *Baseline) error {
	digest, err := BaselineDigest(*baseline)
	if err != nil {
		return err
	}
	baseline.Seal = &BaselineSeal{Algorithm: "sha256", Digest: digest}
	return nil
}

func BaselineCarriesEstimates(baseline Baseline) bool {
	for _, task := range baseline.IncludedTasks {
		if task.PlannedStart != "" || task.PlannedFinish != "" {
			return true
		}
	}
	for _, demand := range baseline.IncludedDemands {
		if demand.Task != "" || demand.Resource != "" || demand.Unit != "" || demand.PlannedAmount != nil {
			return true
		}
	}
	return false
}
