package api

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"image/jpeg"
	"net/http"
	"regexp"
	"strings"
	"unicode/utf8"

	"tinynest/api/internal/auth"
	"tinynest/api/internal/db"
)

var emailPattern = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)

const avatarJPEGPrefix = "data:image/jpeg;base64,"

func validAvatarJPEG(encoded string) bool {
	if len(encoded) > 32*1024 {
		return false
	}
	data, err := base64.StdEncoding.Strict().DecodeString(encoded)
	if err != nil {
		return false
	}
	config, err := jpeg.DecodeConfig(bytes.NewReader(data))
	if err != nil || config.Width != 64 || config.Height != 64 {
		return false
	}
	_, err = jpeg.Decode(bytes.NewReader(data))
	return err == nil
}

// errProfileRejected carries a client-facing validation message for profile writes.
type profileRejected struct{ msg string }

func (e *profileRejected) Error() string { return e.msg }

func (a *API) registerV014(mux *http.ServeMux) {
	a.route(mux, "PATCH /api/v1/auth/me/profile", a.handleUpdateMyProfile)
	a.route(mux, "POST /api/v1/auth/me/password", a.handleChangeMyPassword)
	a.adminRoute(mux, "PATCH /api/v1/admin/users/{id}/profile", a.handleAdminUpdateUserProfile)
	a.adminRoute(mux, "POST /api/v1/admin/users/{id}/password", a.handleAdminResetUserPassword)
	a.adminRoute(mux, "DELETE /api/v1/admin/users/{id}", a.handleAdminDeleteUser)
}

// profilePatch distinguishes an absent key (leave unchanged) from an explicit
// null (clear the value), which a typed struct cannot express.
type profilePatch map[string]json.RawMessage

func (p profilePatch) has(key string) bool {
	_, ok := p[key]
	return ok
}

func (p profilePatch) isNull(key string) bool {
	return strings.TrimSpace(string(p[key])) == "null"
}

func (p profilePatch) str(key string) (string, bool) {
	var s string
	if err := json.Unmarshal(p[key], &s); err != nil {
		return "", false
	}
	return s, true
}

func (p profilePatch) num(key string) (int64, bool) {
	var n int64
	if err := json.Unmarshal(p[key], &n); err != nil {
		return 0, false
	}
	return n, true
}

// bindOrUpdateMemberName renames the target user's bound member profile, or
// creates the binding (and a profile named after the user) when missing.
func bindOrUpdateMemberName(ctx context.Context, q db.Queryer, householdID, userID, name string) error {
	var memberID string
	err := q.QueryRowContext(ctx, `
		SELECT b.member_id FROM household_user_members b
		JOIN household_members m ON m.id=b.member_id
		WHERE b.user_id=? AND m.household_id=?`, userID, householdID).Scan(&memberID)
	if errors.Is(err, sql.ErrNoRows) {
		memberID = newID("mem")
		if _, err := q.ExecContext(ctx, `INSERT INTO household_members(id,household_id,name) VALUES(?,?,?)`,
			memberID, householdID, name); err != nil {
			return err
		}
		_, err = q.ExecContext(ctx, `INSERT INTO household_user_members(user_id,member_id) VALUES(?,?)`, userID, memberID)
		return err
	}
	if err != nil {
		return err
	}
	_, err = q.ExecContext(ctx, `UPDATE household_members SET name=? WHERE id=? AND household_id=?`, name, memberID, householdID)
	return err
}

// applyProfileUpdate validates and writes the provided fields for the target
// user inside the caller's transaction. It returns the audit payload.
func (a *API) applyProfileUpdate(ctx context.Context, q db.Queryer, householdID, targetUserID string, patch profilePatch) (string, error) {
	name, avatar, birthday, payday, email, problem := a.validateProfile(patch)
	if problem != "" {
		return "", &profileRejected{problem}
	}
	var found string
	err := q.QueryRowContext(ctx, `SELECT id FROM users WHERE id=? AND household_id=?`,
		targetUserID, householdID).Scan(&found)
	if err != nil {
		return "", err
	}
	changed := []string{}
	if patch.has("name") {
		if err := bindOrUpdateMemberName(ctx, q, householdID, targetUserID, name); err != nil {
			return "", err
		}
		changed = append(changed, "name")
	}
	sets, args := []string{}, []any{}
	if patch.has("avatar") {
		sets = append(sets, "avatar=?")
		if patch.isNull("avatar") || avatar == "" {
			args = append(args, nil)
		} else {
			args = append(args, avatar)
		}
		changed = append(changed, "avatar")
	}
	if patch.has("birthday") {
		sets = append(sets, "birthday=?")
		if patch.isNull("birthday") || birthday == "" {
			args = append(args, nil)
		} else {
			args = append(args, birthday)
		}
		changed = append(changed, "birthday")
	}
	if patch.has("payday") {
		sets = append(sets, "payday=?")
		if patch.isNull("payday") {
			args = append(args, nil)
		} else {
			args = append(args, payday)
		}
		changed = append(changed, "payday")
	}
	if patch.has("email") {
		sets = append(sets, "email=?")
		if patch.isNull("email") || email == "" {
			args = append(args, nil)
		} else {
			args = append(args, email)
		}
		changed = append(changed, "email")
	}
	if len(sets) > 0 {
		args = append(args, targetUserID, householdID)
		if _, err := q.ExecContext(ctx, `UPDATE users SET `+strings.Join(sets, ",")+` WHERE id=? AND household_id=?`, args...); err != nil {
			return "", err
		}
	}
	payload, err := json.Marshal(map[string]any{"fields": changed})
	if err != nil {
		return "", err
	}
	return string(payload), nil
}

// validateProfile normalizes provided fields and reports the first problem.
// Empty results mean "clear" for the nullable columns.
func (a *API) validateProfile(patch profilePatch) (name, avatar, birthday string, payday int64, email string, problem string) {
	if patch.has("username") {
		return "", "", "", 0, "", "用户名不可修改"
	}
	if patch.has("name") {
		s, ok := patch.str("name")
		if !ok {
			return "", "", "", 0, "", "姓名格式不正确"
		}
		name = strings.TrimSpace(s)
		if utf8.RuneCountInString(name) < 1 || utf8.RuneCountInString(name) > 50 {
			return "", "", "", 0, "", "姓名须为 1 至 50 个字符"
		}
	}
	if patch.has("avatar") && !patch.isNull("avatar") {
		s, ok := patch.str("avatar")
		if !ok {
			return "", "", "", 0, "", "头像格式不正确"
		}
		avatar = strings.TrimSpace(s)
		if data, isImage := strings.CutPrefix(avatar, avatarJPEGPrefix); isImage {
			if !validAvatarJPEG(data) {
				return "", "", "", 0, "", "头像图片须为有效的 64×64 JPEG"
			}
		} else if utf8.RuneCountInString(avatar) > 8 {
			return "", "", "", 0, "", "头像至多 8 个字符"
		}
	}
	if patch.has("birthday") && !patch.isNull("birthday") {
		s, ok := patch.str("birthday")
		if !ok {
			return "", "", "", 0, "", "生日格式不正确"
		}
		birthday = strings.TrimSpace(s)
		if birthday != "" && (!validDate(birthday) || birthday < "1900-01-01" || birthday > a.businessToday().Format("2006-01-02")) {
			return "", "", "", 0, "", "生日须为 1900 年至今的有效日期"
		}
	}
	if patch.has("payday") && !patch.isNull("payday") {
		n, ok := patch.num("payday")
		if !ok || n < 1 || n > 31 {
			return "", "", "", 0, "", "发薪日须为 1 至 31"
		}
		payday = n
	}
	if patch.has("email") && !patch.isNull("email") {
		s, ok := patch.str("email")
		if !ok {
			return "", "", "", 0, "", "邮箱格式不正确"
		}
		email = strings.ToLower(strings.TrimSpace(s))
		if email != "" && (!emailPattern.MatchString(email) || len(email) > 254) {
			return "", "", "", 0, "", "邮箱格式不正确"
		}
	}
	return name, avatar, birthday, payday, email, ""
}

func decodeProfilePatch(w http.ResponseWriter, r *http.Request) (profilePatch, bool) {
	var patch profilePatch
	if err := decode(r, &patch); err != nil || patch == nil {
		writeErr(w, 400, "invalid_request", "请求格式不正确")
		return nil, false
	}
	return patch, true
}

func writeProfileResult(w http.ResponseWriter, payload string) {
	var out map[string]any
	_ = json.Unmarshal([]byte(payload), &out)
	writeJSON(w, 200, map[string]any{"status": "ok", "updated": out["fields"]})
}

func (a *API) handleUpdateMyProfile(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	patch, ok := decodeProfilePatch(w, r)
	if !ok {
		return
	}
	var payload string
	err := a.DB.WithTx(r.Context(), func(tx *sql.Tx) error {
		var err error
		payload, err = a.applyProfileUpdate(r.Context(), tx, p.HouseholdID, p.UserID, patch)
		if err != nil {
			return err
		}
		return audit(r.Context(), tx, p, "update_user_profile", "user", p.UserID, payload)
	})
	var rejected *profileRejected
	switch {
	case errors.As(err, &rejected):
		writeErr(w, 400, "invalid_request", rejected.msg)
	case err != nil:
		internalErr(w, err)
	default:
		writeProfileResult(w, payload)
	}
}

func (a *API) handleChangeMyPassword(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	var in struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if err := decode(r, &in); err != nil || len(in.NewPassword) < 8 || len(in.NewPassword) > 256 {
		writeErr(w, 400, "invalid_request", "新密码须为 8 至 256 位")
		return
	}
	var hash string
	if err := a.DB.QueryRowContext(r.Context(), `SELECT password_hash FROM users WHERE id=? AND household_id=?`,
		p.UserID, p.HouseholdID).Scan(&hash); err != nil {
		writeErr(w, 404, "not_found", "账号不存在")
		return
	}
	if !auth.VerifyPassword(hash, in.CurrentPassword) {
		writeErr(w, 400, "invalid_request", "当前密码不正确")
		return
	}
	newHash, err := auth.HashPassword(in.NewPassword)
	if err != nil {
		internalErr(w, err)
		return
	}
	err = a.DB.WithTx(r.Context(), func(tx *sql.Tx) error {
		result, err := tx.ExecContext(r.Context(), `UPDATE users SET password_hash=? WHERE id=? AND household_id=? AND password_hash=? AND status='active'`,
			newHash, p.UserID, p.HouseholdID, hash)
		if err != nil {
			return err
		}
		n, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if n == 0 {
			return &profileRejected{"账号或密码已变更，请重新登录"}
		}
		return audit(r.Context(), tx, p, "change_password", "user", p.UserID, "{}")
	})
	var rejected *profileRejected
	if errors.As(err, &rejected) {
		writeErr(w, 409, "conflict", rejected.msg)
		return
	}
	if err != nil {
		internalErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

func (a *API) handleAdminUpdateUserProfile(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	patch, ok := decodeProfilePatch(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	var payload string
	err := a.DB.WithTx(r.Context(), func(tx *sql.Tx) error {
		var found string
		if err := tx.QueryRowContext(r.Context(), `SELECT id FROM users WHERE id=? AND household_id=? AND role IN ('member','viewer')`,
			id, p.HouseholdID).Scan(&found); err != nil {
			return err
		}
		var err error
		payload, err = a.applyProfileUpdate(r.Context(), tx, p.HouseholdID, id, patch)
		if err != nil {
			return err
		}
		return audit(r.Context(), tx, p, "update_user_profile", "user", id, payload)
	})
	var rejected *profileRejected
	switch {
	case errors.Is(err, sql.ErrNoRows):
		writeErr(w, 404, "not_found", "成员账号不存在")
	case errors.As(err, &rejected):
		writeErr(w, 400, "invalid_request", rejected.msg)
	case err != nil:
		internalErr(w, err)
	default:
		writeProfileResult(w, payload)
	}
}

func (a *API) handleAdminResetUserPassword(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	var in struct {
		NewPassword string `json:"new_password"`
	}
	if err := decode(r, &in); err != nil || len(in.NewPassword) < 8 || len(in.NewPassword) > 256 {
		writeErr(w, 400, "invalid_request", "新密码须为 8 至 256 位")
		return
	}
	id := r.PathValue("id")
	newHash, err := auth.HashPassword(in.NewPassword)
	if err != nil {
		internalErr(w, err)
		return
	}
	err = a.DB.WithTx(r.Context(), func(tx *sql.Tx) error {
		result, err := tx.ExecContext(r.Context(), `UPDATE users SET password_hash=?
			WHERE id=? AND household_id=? AND role IN ('member','viewer')`, newHash, id, p.HouseholdID)
		if err != nil {
			return err
		}
		if n, _ := result.RowsAffected(); n == 0 {
			return sql.ErrNoRows
		}
		if _, err := tx.ExecContext(r.Context(), `UPDATE sessions SET revoked_at=? WHERE user_id=? AND revoked_at IS NULL`,
			a.nowUTC(), id); err != nil {
			return err
		}
		return audit(r.Context(), tx, p, "reset_user_password", "user", id, "{}")
	})
	switch {
	case errors.Is(err, sql.ErrNoRows):
		writeErr(w, 404, "not_found", "成员账号不存在")
	case err != nil:
		internalErr(w, err)
	default:
		writeJSON(w, 200, map[string]string{"id": id, "status": "ok"})
	}
}

func (a *API) handleAdminDeleteUser(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	id := r.PathValue("id")
	var deletedUsername string
	err := a.DB.WithTx(r.Context(), func(tx *sql.Tx) error {
		var role string
		if err := tx.QueryRowContext(r.Context(), `SELECT username, role FROM users WHERE id=? AND household_id=?`,
			id, p.HouseholdID).Scan(&deletedUsername, &role); err != nil {
			return err
		}
		if role == "admin" {
			return sql.ErrNoRows // the single admin account is never deletable
		}
		if _, err := tx.ExecContext(r.Context(), `DELETE FROM sessions WHERE user_id=?`, id); err != nil {
			return err
		}
		if _, err := tx.ExecContext(r.Context(), `DELETE FROM household_user_members WHERE user_id=?`, id); err != nil {
			return err
		}
		if _, err := tx.ExecContext(r.Context(), `DELETE FROM users WHERE id=? AND household_id=?`, id, p.HouseholdID); err != nil {
			return err
		}
		payload, _ := json.Marshal(map[string]string{"username": deletedUsername})
		return audit(r.Context(), tx, p, "delete_user", "user", id, string(payload))
	})
	switch {
	case errors.Is(err, sql.ErrNoRows):
		writeErr(w, 404, "not_found", "成员账号不存在或不可删除")
	case err != nil:
		internalErr(w, err)
	default:
		writeJSON(w, 200, map[string]string{"id": id, "status": "deleted"})
	}
}
