package api

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"tinynest/api/internal/credit"
)

type everbrightLedgerMatch struct {
	ID          string `json:"id"`
	Description string `json:"description"`
	CategoryID  string `json:"category_id"`
}

type everbrightRowReview struct {
	Key               string                  `json:"key"`
	Imported          bool                    `json:"imported"`
	SuggestedCategory string                  `json:"suggested_category,omitempty"`
	CategorySource    string                  `json:"category_source,omitempty"`
	Matches           []everbrightLedgerMatch `json:"matches"`
}

type everbrightLedgerIdentity struct {
	Date   string
	Amount int64
}

func (a *API) reviewEverbrightStatements(ctx context.Context, household, instrument string, statements []everbrightPreviewStatement) error {
	rows, err := a.DB.QueryContext(ctx, `SELECT t.id,t.occurred_on,t.amount_minor,COALESCE(t.merchant,''),COALESCE(t.idempotency_key,''),
  t.status,COALESCE(c.id,'')
  FROM transactions t
  LEFT JOIN transaction_splits s ON s.transaction_id=t.id
    AND NOT EXISTS (SELECT 1 FROM transaction_splits other WHERE other.transaction_id=t.id AND other.line_no<>s.line_no)
  LEFT JOIN categories c ON c.id=s.category_id AND c.household_id=t.household_id AND c.active=1 AND c.kind='expense'
  WHERE t.household_id=? AND t.credit_instrument_id=? AND t.type='expense'
  ORDER BY t.id`, household, instrument)
	if err != nil {
		return err
	}
	defer rows.Close()
	matches := make(map[everbrightLedgerIdentity][]everbrightLedgerMatch)
	previous := make(map[string]string)
	keys := make(map[string]bool)
	for rows.Next() {
		var identity everbrightLedgerIdentity
		var item everbrightLedgerMatch
		var key, status string
		if err := rows.Scan(&item.ID, &identity.Date, &identity.Amount, &item.Description, &key, &status, &item.CategoryID); err != nil {
			return err
		}
		if key != "" {
			keys[key] = true
		}
		if status != "posted" {
			continue
		}
		matches[identity] = append(matches[identity], item)
		if item.CategoryID != "" && item.Description != "" {
			merchant := statementMerchant(item.Description)
			old, ok := previous[merchant]
			if !ok {
				previous[merchant] = item.CategoryID
			} else if old != item.CategoryID {
				previous[merchant] = ""
			}
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	rows.Close()
	categories, err := a.DB.QueryContext(ctx, `SELECT id,name FROM categories WHERE household_id=? AND active=1 AND kind='expense'`, household)
	if err != nil {
		return err
	}
	defer categories.Close()
	active := make(map[string][]string)
	for categories.Next() {
		var id, name string
		if err := categories.Scan(&id, &name); err != nil {
			return err
		}
		active[statementMerchant(name)] = append(active[statementMerchant(name)], id)
	}
	if err := categories.Err(); err != nil {
		return err
	}
	for i := range statements {
		statement := &statements[i]
		occurrences := make(map[credit.EverbrightTransaction]int)
		for _, row := range statement.Transactions {
			occurrences[row]++
			// A repeat email carrying the same transaction tuple keeps its key. Identical
			// transactions within a single statement still have distinct occurrences.
			source, err := json.Marshal(row)
			if err != nil {
				return err
			}
			key := fmt.Sprintf("ceb:%x", sha256.Sum256([]byte(fmt.Sprintf("%s\x00%s\x00%d", instrument, source, occurrences[row]))))
			var category, categorySource string
			if row.Amount > 0 && !keys[key] {
				category, categorySource = suggestStatementCategory(row.Description, previous, active)
			}
			statement.Review = append(statement.Review, everbrightRowReview{
				Key: key, Imported: keys[key],
				SuggestedCategory: category,
				CategorySource:    categorySource,
				Matches:           matches[everbrightLedgerIdentity{Date: row.TransactionDate, Amount: int64(row.Amount)}],
			})
		}
	}
	return nil
}
