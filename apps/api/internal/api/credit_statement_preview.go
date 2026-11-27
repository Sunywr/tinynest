package api

import (
	"database/sql"
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var statementAccountTail = regexp.MustCompile(`[（(]([0-9]{4})[）)]\s*$`)

// statementReadTarget is the credential-bearing read context shared by the
// preview and sync endpoints. It is resolved before any network access and is
// never serialised.
type statementReadTarget struct {
	provider, sender, bank, tail string
	email, authCode              string
	subjectKeyword               string
	accountID, cardName          string
	userID, householdID          string
	version                      int64
}

// prepareStatementRead checks the binding, the card format, the mailbox version
// and the decrypted authorisation code. Nothing here touches the network.
func (a *API) prepareStatementRead(w http.ResponseWriter, r *http.Request, provider string, version int64) (statementReadTarget, bool) {
	p, _ := principal(r)
	target := statementReadTarget{
		provider: provider, version: version,
		accountID: r.PathValue("id"), userID: p.UserID, householdID: p.HouseholdID,
	}
	cardName, err := a.statementCardName(r.Context(), a.DB, target)
	if errors.Is(err, sql.ErrNoRows) {
		writeErr(w, 404, "not_found", "找不到当前邮箱已绑定的启用信用卡")
		return target, false
	}
	if err != nil {
		internalErr(w, err)
		return target, false
	}
	var bank, sender string
	for _, issuer := range statementBanks {
		if !statementBankMatches(cardName, issuer.bank) {
			continue
		}
		if bank != "" {
			bank = ""
			break
		}
		bank, sender = issuer.bank, issuer.sender
	}
	cardNameForTail := strings.TrimSpace(cardName)
	tail := statementAccountTail.FindStringSubmatch(cardNameForTail)
	if bank == "" || tail == nil {
		writeErr(w, 400, "unsupported_statement_card", "目前支持浦发、中信、交行、广发、光大和招行，信用卡名称末尾须有括号及四位卡号")
		return target, false
	}
	target.cardName, target.bank, target.sender, target.tail = cardName, bank, sender, tail[1]

	if err := a.DB.QueryRowContext(r.Context(), `SELECT subject_keyword FROM statement_mailbox_card_links
  WHERE household_id=? AND account_id=? AND user_id=? AND provider=?`, target.householdID,
		target.accountID, target.userID, target.provider).Scan(&target.subjectKeyword); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeStatementMailboxError(w, errConflict)
		} else {
			internalErr(w, err)
		}
		return target, false
	}

	config, err := loadStatementMailbox(r.Context(), a.DB, p, provider)
	if err != nil {
		internalErr(w, err)
		return target, false
	}
	if config.version != version || config.cipher == "" {
		writeStatementMailboxError(w, errConflict)
		return target, false
	}
	code, err := a.mailCryptWithPurpose(config.cipher, true, statementMailboxPurpose(p, config))
	if err != nil || !validStatementMailboxCredentials(provider, config.email, code) {
		writeErr(w, 503, "mail_unavailable", "账单邮箱授权码不可用，请重新配置")
		return target, false
	}
	target.email, target.authCode = config.email, code
	return target, true
}

// reserveStatementSlot shares one concurrency guard across preview, sync and
// the connection test.
func reserveStatementSlot(w http.ResponseWriter) bool {
	select {
	case mailboxProbeSlots <- struct{}{}:
		return true
	default:
		writeErr(w, 429, "rate_limited", "邮箱请求繁忙，请稍后重试")
		return false
	}
}

// readStatementMails performs the read-only inbox search and then proves the
// mailbox version and the binding still hold before anything is reported.
func (a *API) readStatementMails(w http.ResponseWriter, r *http.Request, target statementReadTarget,
	failedCode, failedMessage string, since time.Time) ([]statementMailPreview, bool) {
	ranges, ok := requestedStatementRanges(w, r)
	if !ok {
		return nil, false
	}
	items := []statementMailPreview{}
	seen := map[uint32]bool{}
	explicit := len(ranges) > 0
	if !explicit {
		ranges = []statementDateRange{{start: since}}
	}
	for _, period := range ranges {
		var batch []statementMailPreview
		var err error
		if fetch := a.opts.StatementMailboxFetch; fetch != nil {
			batch, err = fetch(r.Context(), target.email, target.authCode, target.sender, target.bank, target.tail, period.start)
		} else {
			batch, err = readStatementInboxWindow(r.Context(), target.email, target.authCode, target.sender, target.bank, target.tail, period, target.subjectKeyword)
		}
		if err != nil {
			writeErr(w, 502, failedCode, failedMessage)
			return nil, false
		}
		for _, item := range batch {
			if (explicit && !period.matchesMail(item)) || (item.UID != 0 && seen[item.UID]) {
				continue
			}
			seen[item.UID] = true
			items = append(items, item)
		}
	}
	current, err := loadStatementMailbox(r.Context(), a.DB, principalOf(target), target.provider)
	if err != nil {
		internalErr(w, err)
		return nil, false
	}
	currentCard, err := a.statementCardName(r.Context(), a.DB, target)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		internalErr(w, err)
		return nil, false
	}
	if err != nil || current.version != target.version || currentCard != target.cardName {
		writeStatementMailboxError(w, errConflict)
		return nil, false
	}
	return items, true
}

func statementRequestVersion(w http.ResponseWriter, r *http.Request) (int64, bool) {
	version, err := strconv.ParseInt(r.URL.Query().Get("version"), 10, 64)
	if err != nil || version < 1 {
		writeErr(w, 400, "invalid_request", "请提供邮箱配置版本")
		return 0, false
	}
	return version, true
}

func (a *API) handleStatementPreview(w http.ResponseWriter, r *http.Request) {
	provider := statementMailboxProvider(w, r)
	if provider == "" {
		return
	}
	version, ok := statementRequestVersion(w, r)
	if !ok {
		return
	}
	target, ok := a.prepareStatementRead(w, r, provider, version)
	if !ok {
		return
	}
	if !reserveStatementSlot(w) {
		return
	}
	defer func() { <-mailboxProbeSlots }()
	since := a.businessToday().AddDate(0, 0, -30)
	items, ok := a.readStatementMails(w, r, target,
		"statement_preview_failed", "无法读取账单邮件，请先测试已保存邮箱后重试", since)
	if !ok {
		return
	}
	writeJSON(w, 200, map[string]any{"items": items, "since": since.Format(time.DateOnly)})
}
