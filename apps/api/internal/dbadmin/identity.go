package dbadmin

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

// identityRecord is one exported row. Every value is carried as text (or
// omitted for NULL) because every identity column is text or an integer
// counter; the driver coerces the text back on import. Working with text keeps
// the round trip independent of JSON number precision.
type identityRecord struct {
	Table  string            `json:"t"`
	Values map[string]string `json:"v"`
}

// ExportSummary reports what an identity export wrote.
type ExportSummary struct {
	Tables []TableCount
	Rows   int
}

// ImportSummary reports what an identity import inserted.
type ImportSummary struct {
	Tables []TableCount
	Rows   int
}

func (s ExportSummary) String() string {
	parts := make([]string, 0, len(s.Tables))
	for _, t := range s.Tables {
		parts = append(parts, fmt.Sprintf("%s=%d", t.Table, t.Rows))
	}
	return strings.Join(parts, " ")
}

func (s ImportSummary) String() string {
	parts := make([]string, 0, len(s.Tables))
	for _, t := range s.Tables {
		parts = append(parts, fmt.Sprintf("%s=%d", t.Table, t.Rows))
	}
	return strings.Join(parts, " ")
}

// ExportIdentity writes every identity table to w as JSON lines, in the same
// order ImportIdentity expects (parents before children).
func (a *Admin) ExportIdentity(ctx context.Context, w io.Writer) (ExportSummary, error) {
	schema, err := a.Tables(ctx)
	if err != nil {
		return ExportSummary{}, err
	}
	enc := json.NewEncoder(w)
	var summary ExportSummary
	for _, table := range IdentityTables {
		if !schema.Has(table) {
			return summary, fmt.Errorf("dbadmin: identity table %s is missing from the current schema", table)
		}
		count, err := a.exportTable(ctx, enc, table)
		if err != nil {
			return summary, err
		}
		summary.Tables = append(summary.Tables, TableCount{Table: table, Group: "身份与权限", Rows: count})
		summary.Rows += int(count)
	}
	return summary, nil
}

func (a *Admin) exportTable(ctx context.Context, enc *json.Encoder, table string) (int64, error) {
	rows, err := a.DB.QueryContext(ctx, "SELECT * FROM "+table+" ORDER BY 1") // #nosec G201 -- allow-listed table
	if err != nil {
		return 0, fmt.Errorf("dbadmin: read %s: %w", table, err)
	}
	defer rows.Close()
	columns, err := rows.Columns()
	if err != nil {
		return 0, err
	}
	var count int64
	for rows.Next() {
		raw := make([]sql.NullString, len(columns))
		targets := make([]any, len(columns))
		for i := range raw {
			targets[i] = &raw[i]
		}
		if err := rows.Scan(targets...); err != nil {
			return count, err
		}
		record := identityRecord{Table: table, Values: make(map[string]string, len(columns))}
		for i, column := range columns {
			if raw[i].Valid {
				record.Values[column] = raw[i].String
			}
		}
		if err := enc.Encode(record); err != nil {
			return count, err
		}
		count++
	}
	return count, rows.Err()
}

// ImportIdentity loads an identity export into an empty V2 baseline. It refuses
// to run against tables that already hold rows so a partial import can never be
// half-merged with existing accounts.
func (a *Admin) ImportIdentity(ctx context.Context, r io.Reader, dryRun bool) (ImportSummary, error) {
	schema, err := a.Tables(ctx)
	if err != nil {
		return ImportSummary{}, err
	}
	for _, table := range IdentityTables {
		if !schema.Has(table) {
			return ImportSummary{}, fmt.Errorf("dbadmin: target schema has no %s table; start the V2 binary once to create the baseline first", table)
		}
		var count int64
		if err := a.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table).Scan(&count); err != nil { // #nosec G201 -- allow-listed table
			return ImportSummary{}, err
		}
		if count != 0 {
			return ImportSummary{}, fmt.Errorf("dbadmin: %s already holds %d rows; import requires empty identity tables (use a freshly created V2 database)", table, count)
		}
	}

	grouped := map[string][]identityRecord{}
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	line := 0
	for scanner.Scan() {
		line++
		text := strings.TrimSpace(scanner.Text())
		if text == "" {
			continue
		}
		var record identityRecord
		if err := json.Unmarshal([]byte(text), &record); err != nil {
			return ImportSummary{}, fmt.Errorf("dbadmin: line %d is not a valid identity record: %w", line, err)
		}
		if !isIdentityTable(record.Table) {
			return ImportSummary{}, fmt.Errorf("dbadmin: line %d names %q, which is not an identity table", line, record.Table)
		}
		grouped[record.Table] = append(grouped[record.Table], record)
	}
	if err := scanner.Err(); err != nil {
		return ImportSummary{}, err
	}

	var summary ImportSummary
	for _, table := range IdentityTables {
		summary.Tables = append(summary.Tables, TableCount{Table: table, Group: "身份与权限", Rows: int64(len(grouped[table]))})
		summary.Rows += len(grouped[table])
	}
	if dryRun {
		return summary, nil
	}

	err = a.DB.WithTx(ctx, func(tx *sql.Tx) error {
		for _, table := range IdentityTables {
			records := grouped[table]
			if len(records) == 0 {
				continue
			}
			columns, err := recordColumns(records[0])
			if err != nil {
				return fmt.Errorf("dbadmin: %s: %w", table, err)
			}
			placeholders := strings.TrimSuffix(strings.Repeat("?,", len(columns)), ",")
			statement := "INSERT INTO " + table + " (" + strings.Join(columns, ",") + ") VALUES (" + placeholders + ")" // #nosec G201 -- allow-listed table and quoted column names
			for i, record := range records {
				args := make([]any, len(columns))
				for j, column := range columns {
					value, ok := record.Values[column]
					if !ok {
						args[j] = nil
						continue
					}
					args[j] = value
				}
				if _, err := tx.ExecContext(ctx, statement, args...); err != nil {
					return fmt.Errorf("dbadmin: insert %s row %d: %w", table, i+1, err)
				}
			}
		}
		return nil
	})
	if err != nil {
		return ImportSummary{}, err
	}
	return summary, nil
}

func recordColumns(record identityRecord) ([]string, error) {
	if len(record.Values) == 0 {
		return nil, errors.New("record has no columns")
	}
	columns := make([]string, 0, len(record.Values))
	for column := range record.Values {
		columns = append(columns, column)
	}
	// A deterministic order keeps the generated statement stable across runs.
	sortStrings(columns)
	return columns, nil
}

func isIdentityTable(table string) bool {
	for _, candidate := range IdentityTables {
		if candidate == table {
			return true
		}
	}
	return false
}

func sortStrings(values []string) {
	for i := 1; i < len(values); i++ {
		for j := i; j > 0 && values[j] < values[j-1]; j-- {
			values[j], values[j-1] = values[j-1], values[j]
		}
	}
}
