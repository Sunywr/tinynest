package api

import (
	"context"
	"strings"

	"tinynest/api/internal/db"
)

type transactionMetadata struct {
	Labels       []string `json:"labels"`
	Remarks      []string `json:"remarks"`
	Descriptions []string `json:"descriptions"`
}

func loadTransactionMetadata(ctx context.Context, q db.Queryer, householdID string, ids []string) (map[string]*transactionMetadata, error) {
	result := make(map[string]*transactionMetadata)
	if len(ids) == 0 {
		return result, nil
	}
	args := []any{householdID}
	for _, id := range ids {
		args = append(args, id)
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	rows, err := q.QueryContext(ctx, `
        SELECT DISTINCT t.id,m.kind,m.value
        FROM transactions t JOIN transaction_source_metadata m ON m.transaction_id=t.id
        WHERE t.household_id=? AND t.id IN (`+placeholders+`)
        ORDER BY t.id,m.kind,m.value`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id, kind, value string
		if err := rows.Scan(&id, &kind, &value); err != nil {
			return nil, err
		}
		metadata := result[id]
		if metadata == nil {
			metadata = &transactionMetadata{Labels: []string{}, Remarks: []string{}, Descriptions: []string{}}
			result[id] = metadata
		}
		switch kind {
		case "label":
			metadata.Labels = append(metadata.Labels, value)
		case "remark":
			metadata.Remarks = append(metadata.Remarks, value)
		case "description":
			metadata.Descriptions = append(metadata.Descriptions, value)
		}
	}
	return result, rows.Err()
}
