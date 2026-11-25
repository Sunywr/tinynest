package api

import (
	"database/sql"
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"time"
)

var statementAccountTail = regexp.MustCompile(`[（(]([0-9]{4})[）)]\s*$`)

func (a *API) statementPreviewCard(r *http.Request, p Principal, provider string) (string, error) {
	var name string
	err := a.DB.QueryRowContext(r.Context(), `
		SELECT a.name FROM accounts a
		JOIN statement_mailbox_card_links b ON b.household_id=a.household_id AND b.account_id=a.id
		JOIN users u ON u.household_id=b.household_id AND u.id=b.user_id
		WHERE a.household_id=? AND a.id=? AND a.kind='credit' AND a.status='active'
		  AND b.user_id=? AND b.provider=? AND u.status='active' AND u.role IN ('admin','member')`,
		p.HouseholdID, r.PathValue("id"), p.UserID, provider).Scan(&name)
	return name, err
}

func (a *API) handleStatementPreview(w http.ResponseWriter, r *http.Request) {
	provider := statementMailboxProvider(w, r)
	if provider == "" {
		return
	}
	version, err := strconv.ParseInt(r.URL.Query().Get("version"), 10, 64)
	if err != nil || version < 1 {
		writeErr(w, 400, "invalid_request", "请提供邮箱配置版本")
		return
	}
	p, _ := principal(r)
	cardName, err := a.statementPreviewCard(r, p, provider)
	if errors.Is(err, sql.ErrNoRows) {
		writeErr(w, 404, "not_found", "找不到当前邮箱已绑定的启用信用卡")
		return
	}
	if err != nil {
		internalErr(w, err)
		return
	}
	var bank, sender string
	switch {
	case statementBankMatches(cardName, "spdb") && !statementBankMatches(cardName, "citic"):
		bank, sender = "spdb", "estmtservice"
	case statementBankMatches(cardName, "citic") && !statementBankMatches(cardName, "spdb"):
		bank, sender = "citic", "citic"
	}
	tail := statementAccountTail.FindStringSubmatch(cardName)
	if bank == "" || tail == nil {
		writeErr(w, 400, "unsupported_statement_card", "目前支持浦发和中信，信用卡名称末尾须有括号及四位卡号")
		return
	}

	config, err := loadStatementMailbox(r.Context(), a.DB, p, provider)
	if err != nil {
		internalErr(w, err)
		return
	}
	if config.version != version || config.cipher == "" {
		writeStatementMailboxError(w, errConflict)
		return
	}
	code, err := a.mailCryptWithPurpose(config.cipher, true, statementMailboxPurpose(p, config))
	if err != nil || !validStatementMailboxCredentials(provider, config.email, code) {
		writeErr(w, 503, "mail_unavailable", "账单邮箱授权码不可用，请重新配置")
		return
	}
	select {
	case mailboxProbeSlots <- struct{}{}:
		defer func() { <-mailboxProbeSlots }()
	default:
		writeErr(w, 429, "rate_limited", "邮箱请求繁忙，请稍后重试")
		return
	}
	since := a.businessToday().AddDate(0, 0, -30)
	items, err := previewStatementInbox(r.Context(), config.email, code, sender, bank, tail[1], since)
	if err != nil {
		writeErr(w, 502, "statement_preview_failed", "无法读取账单邮件，请先测试已保存邮箱后重试")
		return
	}
	current, err := loadStatementMailbox(r.Context(), a.DB, p, provider)
	if err != nil {
		internalErr(w, err)
		return
	}
	currentCard, err := a.statementPreviewCard(r, p, provider)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		internalErr(w, err)
		return
	}
	if err != nil || current.version != version || currentCard != cardName {
		writeStatementMailboxError(w, errConflict)
		return
	}
	writeJSON(w, 200, map[string]any{"items": items, "since": since.Format(time.DateOnly)})
}
