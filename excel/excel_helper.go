package excel

import (
	"fmt"
	"log/slog"

	"github.com/vechain/thor/v2/thor"

	"github.com/xuri/excelize/v2"
)

type Owner struct {
	MasterAddress  thor.Address
	Owner          string
	PointOfContact string
	Network        string
}

func ParseOwnersFromXLSX(filePath string) (*[]Owner, error) {
	f, err := excelize.OpenFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer func() {
		if err := f.Close(); err != nil {
			slog.Error("Failed to close Excel file", "error", err)
		}
	}()

	rows, err := f.GetRows("Owners")
	if err != nil {
		return nil, fmt.Errorf("failed to get rows: %w", err)
	}

	if len(rows) == 0 {
		return nil, fmt.Errorf("empty sheet")
	}

	owners := make([]Owner, 0, len(rows)-1)

	for i, row := range rows {
		if i == 0 {
			continue
		}

		if len(row) < 5 {
			slog.Warn("Row has insufficient columns", "row", i+1, "columns", len(row))
			continue
		}

		network := row[1]
		masterAddr, err := thor.ParseAddress(row[2])
		if err != nil {
			slog.Warn("Failed to parse master address", "row", i+1, "value", row[2], "error", err)
			continue
		}

		owner := row[3]
		pointOfContact := row[4]
		owners = append(owners, Owner{
			MasterAddress:  masterAddr,
			Network:        network,
			PointOfContact: pointOfContact,
			Owner:          owner,
		})
	}
	return &owners, nil
}
