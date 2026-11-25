// Package dbadmin implements the destructive operator operations that are kept
// out of the HTTP API on purpose: purging business data while preserving user
// and permission rows, verifying the result, and moving identity data between
// schema baselines.
//
// The package never runs during normal request handling. It is reachable only
// through cmd/tinynest-admin, which requires the same protected credentials as
// the API service and an explicit confirmation string.
package dbadmin

// Group is a named batch of tables for operator-facing reporting.
type Group struct {
	Name   string
	Note   string
	Tables []string
}

// PurgeGroups is the canonical list of tables whose rows are business data.
// Every table here is emptied by Purge. The list is intentionally explicit: a
// new table added in a future migration must be classified here or it will show
// up in Report.Unclassified and the operator is told to review it.
var PurgeGroups = []Group{
	{
		Name:   "结构与账本",
		Note:   "分类、项目、账户、科目表",
		Tables: []string{"categories", "projects", "accounts", "ledger_accounts"},
	},
	{
		Name:   "复式分录",
		Note:   "余额的唯一事实来源；清空即所有余额归零",
		Tables: []string{"journal_entries", "journal_lines", "account_opening_balances"},
	},
	{
		Name:   "记账数据",
		Note:   "流水、拆分、退款与月度预算",
		Tables: []string{"transactions", "transaction_splits", "refunds", "budgets", "project_budgets", "transaction_budget_links", "transaction_source_metadata"},
	},
	{
		Name:   "信用账单",
		Note:   "信用工具、账单、还款与调整记录",
		Tables: []string{"credit_instruments", "credit_statements", "credit_statement_items", "credit_payments", "credit_instrument_changes", "credit_statement_adjustments", "credit_statement_provisions"},
	},
	{
		Name:   "银行账单与绑卡",
		Note:   "迁移 010-012 的只读对账数据：银行汇总、邮件来源与绑卡关系；加密授权码在保留组",
		Tables: []string{"card_statement_sources", "card_statements", "statement_mailbox_card_links", "credit_statement_bindings"},
	},
	{
		Name:   "分期",
		Note:   "分期合同、期次、付款与调整",
		Tables: []string{"installment_contracts", "installment_schedule_items", "installment_payments", "installment_adjustments"},
	},
	{
		Name:   "负债",
		Note:   "固定负债合同、期次与付款",
		Tables: []string{"debt_contracts", "debt_schedule_items", "debt_payments"},
	},
	{
		Name:   "到期调度",
		Note:   "待办投影；清空后调度器会按新数据重新生成",
		Tables: []string{"schedule_due_items"},
	},
	{
		Name:   "投资",
		Note:   "资产、费率、成交、分红、估值与持仓投影",
		Tables: []string{"investment_assets", "investment_fee_rules", "investment_trades", "investment_dividends", "investment_valuations", "investment_positions"},
	},
	{
		Name:   "投资估值快照",
		Note:   "迁移 005 从 pfm 导入的只读市值，不写分录",
		Tables: []string{"investment_holdings", "investment_accounts"},
	},
	{
		Name:   "信用卡账单邮件",
		Note:   "IMAP 配置含加密口令，按隐私数据清理",
		Tables: []string{"credit_mailboxes", "credit_mail_bindings", "credit_mail_previews"},
	},
	{
		Name:   "账单导入",
		Note:   "导入行与绑定关系",
		Tables: []string{"statement_import_rows", "statement_import_bindings"},
	},
	{
		Name:   "股票自选",
		Note:   "自选列表（行情缓存见运营组）",
		Tables: []string{"stock_watch"},
	},
	{
		Name:   "待发邮件",
		Note:   "排队的对外邮件正文可能含家庭内容，按业务数据清理",
		Tables: []string{"mail_queue"},
	},
	{
		Name:   "运营与审计",
		Note:   "审计流水、调度运行记录、行情缓存；不含用户与权限",
		Tables: []string{"audit_logs", "schedule_runs", "stock_quotes"},
	},
	{
		Name:   "成员档案与邀请",
		Note:   "按确认口径清空；用户下次登录由 /household/members/current 自动重建档案",
		Tables: []string{"household_members", "household_user_members", "household_invites"},
	},
}

// IdentityTables survive a purge and move unchanged into the V2 baseline.
// Order matters for import: a child row may only be inserted after its parent.
var IdentityTables = []string{
	"households",
	"users",
	"sessions",
	"site_administrators",
	"pending_registrations",
	"email_identities",
	"email_challenges",
	"mail_configuration",
	"mail_preferences",
	"mail_daily_usage",
	"mail_recipient_daily_usage",
}

// PreservedConfigTables survive a purge like identity rows but are not part of
// the identity export/import, so an operator decides separately whether the
// encrypted mailbox credentials move into a fresh baseline. The card links that
// reference accounts are purged; only the mailbox credential itself is kept.
var PreservedConfigTables = []string{
	"statement_mailbox_configs",
	"credit_statement_mailboxes",
}

// OptionalTables are cleared only when the operator does not ask to keep them.
// Today no table is optional by default; the slice exists so the flag has a
// single definition rather than scattered conditionals.
var OptionalTables = []string{}

// deleteOrder lists every purged table in child-before-parent order. The
// ordering is the correctness argument: foreign keys are never disabled, so a
// successful run proves no orphan rows were created.
var deleteOrder = []string{
	// Leaves that reference entries, contracts, statements, assets or accounts.
	"refunds",
	"account_opening_balances",
	"journal_lines",
	"credit_statement_items",
	"credit_statement_adjustments",
	"credit_instrument_changes",
	"credit_statement_provisions",
	"credit_payments",
	"installment_payments",
	"installment_adjustments",
	"debt_payments",
	"investment_trades",
	"investment_dividends",
	"investment_valuations",
	"investment_positions",
	"investment_fee_rules",
	"credit_mail_bindings",
	"credit_mail_previews",
	"credit_mailboxes",
	// Bank-reported statements and the card bindings that point at accounts.
	"card_statement_sources",
	"card_statements",
	"statement_mailbox_card_links",
	"credit_statement_bindings",
	"investment_holdings",
	"investment_accounts",
	"statement_import_rows",
	"statement_import_bindings",
	"stock_watch",
	"schedule_due_items",
	// Transaction and contract children, then the transactions they reference.
	"transaction_budget_links",
	"transaction_source_metadata",
	"project_budgets",
	"transaction_splits",
	"installment_schedule_items",
	"debt_schedule_items",
	"installment_contracts",
	"debt_contracts",
	"credit_statements",
	"credit_instruments",
	"transactions",
	// Parents.
	"investment_assets",
	"journal_entries",
	"accounts",
	"ledger_accounts",
	"projects",
	"categories",
	// No foreign keys; order is irrelevant but the result is deterministic.
	"audit_logs",
	"schedule_runs",
	"stock_quotes",
	"mail_queue",
	// V2 bookkeeping extras (absent from a v1 schema; skipped automatically).
	"budgets",
	"household_invites",
	"household_user_members",
	"household_members",
}

// DeleteOrder returns a copy of the child-before-parent purge order.
func DeleteOrder() []string {
	out := make([]string, len(deleteOrder))
	copy(out, deleteOrder)
	return out
}

// PurgeTableSet returns every purged table exactly once, in reporting order.
func PurgeTableSet() []string {
	seen := make(map[string]bool, len(deleteOrder))
	out := make([]string, 0, len(deleteOrder))
	for _, g := range PurgeGroups {
		for _, table := range g.Tables {
			if !seen[table] {
				seen[table] = true
				out = append(out, table)
			}
		}
	}
	return out
}

// GroupOf reports the reporting group a table belongs to.
func GroupOf(table string) string {
	for _, g := range PurgeGroups {
		for _, t := range g.Tables {
			if t == table {
				return g.Name
			}
		}
	}
	return ""
}

// SelfReferences lists tables with a nullable self foreign key that must be
// detached before deletion.
var SelfReferences = []struct {
	Table  string
	Column string
	Set    string
}{
	{Table: "journal_entries", Column: "reversal_of"},
	{Table: "categories", Column: "parent_id"},
}

// DetachSelfReferences returns the statements that break self-referencing
// foreign keys before the rows are deleted.
func DetachSelfReferences() []string {
	out := make([]string, 0, len(SelfReferences))
	for _, ref := range SelfReferences {
		out = append(out, "UPDATE "+ref.Table+" SET "+ref.Column+"=NULL WHERE "+ref.Column+" IS NOT NULL")
	}
	return out
}
