package api

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"net/http"
	"strings"
)

type creditMailbox struct {
	ID       string `json:"id"`
	Host     string `json:"host"`
	Username string `json:"username"`
	Folder   string `json:"folder"`
	Enabled  bool   `json:"enabled"`
	Version  int    `json:"version"`
	TestedAt string `json:"tested_at"`
}

type creditMailBinding struct {
	InstrumentID   string `json:"instrument_id"`
	MailboxID      string `json:"mailbox_id"`
	SubjectKeyword string `json:"subject_keyword"`
	CardTail       string `json:"card_tail"`
}

func (a *API) registerCreditMail(m *http.ServeMux) {
	register := func(pattern string, handler http.HandlerFunc) {
		a.route(m, pattern, func(w http.ResponseWriter, r *http.Request) {
			p, _ := principal(r)
			if p.Role != "admin" && p.Role != "member" {
				writeErr(w, 403, "forbidden", "仅有记账权限的家庭成员可配置账单邮箱")
				return
			}
			handler(w, r)
		})
	}
	register("GET /api/v1/credit-mail", a.listCreditMail)
	register("POST /api/v1/credit-mail/mailboxes", a.saveCreditMailbox)
	register("PUT /api/v1/credit-mail/mailboxes/{id}", a.saveCreditMailbox)
	register("PUT /api/v1/credit-mail/bindings/{id}", a.saveCreditMailBinding)
	register("POST /api/v1/credit-mail/bindings/{id}/preview", a.previewEverbrightMail)
}

func (a *API) listCreditMail(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	ctx := r.Context()
	boxes := []creditMailbox{}
	rows, err := a.DB.QueryContext(ctx, `SELECT id,host,username,folder,enabled,version,tested_at FROM credit_mailboxes WHERE household_id=? ORDER BY username,id`, p.HouseholdID)
	if err != nil {
		internalErr(w, err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var box creditMailbox
		if err := rows.Scan(&box.ID, &box.Host, &box.Username, &box.Folder, &box.Enabled, &box.Version, &box.TestedAt); err != nil {
			internalErr(w, err)
			return
		}
		boxes = append(boxes, box)
	}
	if err := rows.Err(); err != nil {
		internalErr(w, err)
		return
	}
	rows.Close()
	bindings := []creditMailBinding{}
	rows, err = a.DB.QueryContext(ctx, `SELECT b.instrument_id,b.mailbox_id,b.subject_keyword,b.card_tail
  FROM credit_mail_bindings b JOIN credit_instruments i ON i.id=b.instrument_id AND i.household_id=b.household_id
  WHERE b.household_id=? AND i.status='active' ORDER BY b.instrument_id`, p.HouseholdID)
	if err != nil {
		internalErr(w, err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var b creditMailBinding
		if err := rows.Scan(&b.InstrumentID, &b.MailboxID, &b.SubjectKeyword, &b.CardTail); err != nil {
			internalErr(w, err)
			return
		}
		bindings = append(bindings, b)
	}
	if err := rows.Err(); err != nil {
		internalErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"mailboxes": boxes, "bindings": bindings, "key_ready": len(a.opts.CreditMailKey) == 32})
}

func creditMailboxCrypt(key []byte, household, id, value string, decrypt bool) (string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	scope := []byte("tinynest-credit-mail-v1:" + household + ":" + id)
	if decrypt {
		data, err := base64.StdEncoding.DecodeString(value)
		if err != nil || len(data) < gcm.NonceSize() {
			return "", errors.New("invalid mailbox ciphertext")
		}
		plain, err := gcm.Open(nil, data[:gcm.NonceSize()], data[gcm.NonceSize():], scope)
		return string(plain), err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(gcm.Seal(nonce, nonce, []byte(value), scope)), nil
}

func (a *API) saveCreditMailbox(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	ctx := r.Context()
	var in struct {
		creditMailbox
		Password string `json:"password"`
	}
	if decode(r, &in) != nil {
		writeErr(w, 400, "invalid_request", "邮箱配置无效")
		return
	}
	in.Host = strings.ToLower(strings.TrimSpace(in.Host))
	in.Username = strings.TrimSpace(in.Username)
	in.Folder = strings.TrimSpace(in.Folder)
	if in.Folder == "" {
		in.Folder = "INBOX"
	}
	_, userErr := quoteIMAPProbeValue(in.Username)
	_, folderErr := quoteIMAPProbeValue(encodeIMAPFolder(in.Folder))
	if !validSMTPHost(in.Host) || len(in.Username) > 320 || len(in.Folder) > 255 || userErr != nil || folderErr != nil {
		writeErr(w, 400, "invalid_request", "请检查 IMAP 主机、邮箱账号和文件夹")
		return
	}
	id := r.PathValue("id")
	var old creditMailbox
	var stored string
	if id != "" {
		err := a.DB.QueryRowContext(ctx, `SELECT host,username,folder,enabled,version,tested_at,password_cipher FROM credit_mailboxes WHERE id=? AND household_id=?`, id, p.HouseholdID).
			Scan(&old.Host, &old.Username, &old.Folder, &old.Enabled, &old.Version, &old.TestedAt, &stored)
		if errors.Is(err, sql.ErrNoRows) {
			writeErr(w, 404, "not_found", "邮箱不存在")
			return
		}
		if err != nil {
			internalErr(w, err)
			return
		}
		if in.Version != old.Version {
			writeErr(w, 409, "conflict", "邮箱配置已变化，请刷新")
			return
		}
	} else {
		id = newID("cmb")
	}
	in.ID = id
	in.TestedAt = old.TestedAt
	if in.Enabled {
		if len(a.opts.CreditMailKey) != 32 {
			writeErr(w, 503, "mail_key_unavailable", "服务器尚未配置账单邮箱加密密钥")
			return
		}
		password := in.Password
		if password == "" && stored != "" && in.Host == old.Host && in.Username == old.Username {
			var err error
			password, err = creditMailboxCrypt(a.opts.CreditMailKey, p.HouseholdID, id, stored, true)
			if err != nil {
				writeErr(w, 409, "password_required", "请重新输入邮箱授权码")
				return
			}
		}
		if _, err := quoteIMAPProbeValue(password); err != nil {
			writeErr(w, 400, "password_required", "请输入有效的邮箱授权码")
			return
		}
		probe := a.opts.CreditIMAPProbe
		if probe == nil {
			probe = probeCreditIMAP
		}
		if err := probe(ctx, in.Host, in.Username, password, in.Folder); err != nil {
			message := "IMAP 测试失败，请检查服务器地址和 TLS 993 连通性"
			switch {
			case errors.Is(err, errCreditIMAPAuth):
				message = "IMAP 登录失败，请检查邮箱账号、客户端授权码及邮箱是否已启用 IMAP"
			case errors.Is(err, errCreditIMAPIdentity):
				message = "IMAP 登录成功，但服务器拒绝了客户端身份声明"
			case errors.Is(err, errCreditIMAPFolder):
				message = "IMAP 登录成功，但无法打开所选邮件文件夹"
			}
			writeErr(w, 502, "imap_test_failed", message)
			return
		}
		var err error
		stored, err = creditMailboxCrypt(a.opts.CreditMailKey, p.HouseholdID, id, password, false)
		if err != nil {
			internalErr(w, err)
			return
		}
		in.TestedAt = a.nowUTC()
	} else if old.Version == 0 || in.Host != old.Host || in.Username != old.Username || in.Folder != old.Folder || in.Password != "" {
		writeErr(w, 400, "invalid_request", "新增或修改连接信息时须启用并通过测试")
		return
	}
	in.Version = old.Version + 1
	err := a.DB.WithTx(ctx, func(tx *sql.Tx) error {
		if old.Version == 0 {
			if _, err := tx.ExecContext(ctx, `INSERT INTO credit_mailboxes(id,household_id,host,username,folder,password_cipher,enabled,version,tested_at) VALUES(?,?,?,?,?,?,?,?,?)`,
				id, p.HouseholdID, in.Host, in.Username, in.Folder, stored, in.Enabled, in.Version, in.TestedAt); err != nil {
				return err
			}
		} else {
			result, err := tx.ExecContext(ctx, `UPDATE credit_mailboxes SET host=?,username=?,folder=?,password_cipher=?,enabled=?,version=?,tested_at=? WHERE id=? AND household_id=? AND version=?`,
				in.Host, in.Username, in.Folder, stored, in.Enabled, in.Version, in.TestedAt, id, p.HouseholdID, old.Version)
			if err != nil {
				return err
			}
			affected, err := result.RowsAffected()
			if err != nil {
				return err
			}
			if affected != 1 {
				return errConflict
			}
		}
		return audit(ctx, tx, p, "configure", "credit_mailbox", id, "{}")
	})
	if errors.Is(err, errConflict) {
		writeErr(w, 409, "conflict", "邮箱配置已变化，请刷新")
		return
	}
	if err != nil {
		internalErr(w, err)
		return
	}
	writeJSON(w, 200, in.creditMailbox)
}

var errInvalidCreditMailBinding = errors.New("invalid credit mail binding")

func (a *API) saveCreditMailBinding(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	ctx := r.Context()
	var b creditMailBinding
	if decode(r, &b) != nil {
		writeErr(w, 400, "invalid_request", "绑定配置无效")
		return
	}
	b.InstrumentID = r.PathValue("id")
	b.SubjectKeyword = strings.TrimSpace(b.SubjectKeyword)
	if b.MailboxID != "" && (b.SubjectKeyword == "" || len(b.SubjectKeyword) > 255 || len(b.CardTail) != 4 || strings.Trim(b.CardTail, "0123456789") != "") {
		writeErr(w, 400, "invalid_request", "请填写标题关键字和四位卡尾号")
		return
	}
	err := a.DB.WithTx(ctx, func(tx *sql.Tx) error {
		var count int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM credit_instruments WHERE id=? AND household_id=? AND kind='credit_card' AND status='active'`, b.InstrumentID, p.HouseholdID).Scan(&count); err != nil {
			return err
		}
		if count != 1 {
			return errInvalidCreditMailBinding
		}
		if b.MailboxID != "" {
			if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM credit_mailboxes
				WHERE id=? AND household_id=? AND enabled=1`, b.MailboxID, p.HouseholdID).Scan(&count); err != nil {
				return err
			}
			if count != 1 {
				return errInvalidCreditMailBinding
			}
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM credit_mail_bindings WHERE instrument_id=? AND household_id=?`, b.InstrumentID, p.HouseholdID); err != nil {
			return err
		}
		if b.MailboxID != "" {
			// The legacy category column remains for compatibility with deployed schemas.
			if _, err := tx.ExecContext(ctx, `INSERT INTO credit_mail_bindings (instrument_id,household_id,mailbox_id,category_id,subject_keyword,card_tail) VALUES(?,?,?,'',?,?)`,
				b.InstrumentID, p.HouseholdID, b.MailboxID, b.SubjectKeyword, b.CardTail); err != nil {
				return err
			}
		}
		return audit(ctx, tx, p, "configure", "credit_mail_binding", b.InstrumentID, "{}")
	})
	if errors.Is(err, errInvalidCreditMailBinding) {
		writeErr(w, 400, "invalid_request", "请选择本家庭的有效信用卡和邮箱")
		return
	}
	if err != nil {
		internalErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"saved": true})
}
