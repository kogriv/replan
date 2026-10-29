package reports

import (
	"encoding/csv"
	"io"
)

type WorkloadCSV struct {
	Week       string
	ScenarioID string
	Resources  []WorkloadCSVRow
}

type WorkloadCSVRow struct {
	ResourceID    string
	ResourceName  string
	ResourceClass string
	Unit          string
	Capacity      float64
	Planned       float64
	Reserved      float64
	Actual        float64
	Remaining     float64
	Overload      float64
}

func WriteWeeklyCSV(w io.Writer, report WeeklyReport) error {
	writer := csv.NewWriter(w)
	header := []string{
		"resource_id",
		"resource_name",
		"resource_class",
		"unit",
		"capacity",
		"planned",
		"reserved",
		"actual",
		"remaining",
		"overload",
	}
	if report.ScenarioID != "" {
		header = append([]string{"scenario_id"}, header...)
	}
	if err := writer.Write(header); err != nil {
		return err
	}
	for _, row := range report.Resources {
		values := []string{
			row.ResourceID,
			row.ResourceName,
			row.ResourceClass,
			row.Unit,
			formatAmount(row.Capacity),
			formatAmount(row.Planned),
			formatAmount(row.Reserved),
			formatAmount(row.Actual),
			formatAmount(row.Remaining),
			formatAmount(row.Overload),
		}
		if report.ScenarioID != "" {
			values = append([]string{report.ScenarioID}, values...)
		}
		if err := writer.Write(values); err != nil {
			return err
		}
	}
	writer.Flush()
	return writer.Error()
}

func WriteWorkloadCSV(w io.Writer, result WorkloadCSV) error {
	writer := csv.NewWriter(w)
	if err := writer.Write([]string{
		"scenario_id",
		"week",
		"resource_id",
		"resource_name",
		"resource_class",
		"unit",
		"capacity",
		"planned",
		"reserved",
		"actual",
		"remaining",
		"overload",
	}); err != nil {
		return err
	}
	for _, row := range result.Resources {
		if err := writer.Write([]string{
			result.ScenarioID,
			result.Week,
			row.ResourceID,
			row.ResourceName,
			row.ResourceClass,
			row.Unit,
			formatAmount(row.Capacity),
			formatAmount(row.Planned),
			formatAmount(row.Reserved),
			formatAmount(row.Actual),
			formatAmount(row.Remaining),
			formatAmount(row.Overload),
		}); err != nil {
			return err
		}
	}
	writer.Flush()
	return writer.Error()
}
