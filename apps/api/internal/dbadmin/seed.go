package dbadmin

import (
	"context"
	"database/sql"

	"tinynest/api/internal/db"
)

// SeedCategories re-applies the built-in category set to every household.
//
// It is idempotent and also fills in the flat icon and palette colour for
// default rows that an older binary seeded without them, which is how an
// existing production household picks up the current styling without losing a
// user-renamed category.
func (a *Admin) SeedCategories(ctx context.Context, householdID string) ([]string, error) {
	var households []string
	err := a.DB.WithTx(ctx, func(tx *sql.Tx) error {
		list, err := survivingHouseholds(ctx, tx, householdID)
		if err != nil {
			return err
		}
		for _, household := range list {
			if err := db.SeedForHousehold(ctx, tx, household); err != nil {
				return err
			}
		}
		households = list
		return nil
	})
	if err != nil {
		return nil, err
	}
	return households, nil
}
