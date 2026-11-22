package api

import (
	"context"
	"database/sql"
	"errors"
	"net/http"

	"tinynest/api/internal/db"
	"tinynest/api/internal/ledger"
)

var errInvalidAccountOwner = errors.New("归属成员不存在或已停用")

// accountInfo is the persisted shape of one account row.
type accountInfo struct {
	ID      string
	Kind    string
	Name    string
	Code    string
	Status  string
	Subtype sql.NullString
	Owner   sql.NullString
	Note    sql.NullString
}

func scanAccount(row *sql.Row) (accountInfo, error) {
	var info accountInfo
	err := row.Scan(&info.ID, &info.Kind, &info.Name, &info.Code, &info.Status,
		&info.Subtype, &info.Owner, &info.Note)
	return info, err
}

const accountColumns = `id,kind,name,ledger_code,status,subtype,owner_member_id,note`

// loadAccount returns one household-scoped account in any status.
func loadAccount(ctx context.Context, q db.Queryer, householdID, id string) (accountInfo, error) {
	return scanAccount(q.QueryRowContext(ctx,
		`SELECT `+accountColumns+` FROM accounts WHERE id=? AND household_id=?`, id, householdID))
}

// loadActiveAccount returns one ACTIVE household-scoped account.
func loadActiveAccount(ctx context.Context, q db.Queryer, householdID, id string) (accountInfo, error) {
	return scanAccount(q.QueryRowContext(ctx,
		`SELECT `+accountColumns+` FROM accounts WHERE id=? AND household_id=? AND status='active'`, id, householdID))
}

// validateAccountOwner rejects an owner_member_id that is missing or disabled.
func (a *API) validateAccountOwner(r *http.Request, tx *sql.Tx, householdID string, owner *string) error {
	if owner == nil || *owner == "" {
		return nil
	}
	status, err := a.lockMember(r, tx, householdID, *owner)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && status != "active") {
		return errInvalidAccountOwner
	}
	return err
}

// validateMemberInput locks an optional member_id for a financial write.
func (a *API) validateMemberInput(r *http.Request, tx *sql.Tx, householdID string, memberID *string) error {
	if memberID == nil || *memberID == "" {
		return nil
	}
	status, err := a.lockMember(r, tx, householdID, *memberID)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && status != "active") {
		return errors.New("家庭成员不存在或已停用")
	}
	return err
}

// accountLedgerCode returns the chart-of-accounts code an account must use.
func accountLedgerCode(kind, accountID string) string {
	if kind == "credit" {
		return ledger.CreditSubject(accountID)
	}
	return ledger.CashSubject(accountID)
}

// accountLedgerCategory maps an account kind to the ledger_accounts category
// (1 = asset, 2 = liability).
func accountLedgerCategory(kind string) int {
	if kind == "credit" {
		return 2
	}
	return 1
}

// isNoRows is a local alias so handlers that branch on "not found" read cleanly.
func isNoRows(err error) bool { return errors.Is(err, sql.ErrNoRows) }
