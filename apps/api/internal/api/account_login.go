package api

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"time"
)

const (
	failuresPerLock = 5
	dailyLockRound  = 5
)

type accountLoginState struct {
	day         string
	failures    int
	rounds      int
	lockedUntil int64
}

func (a *API) loginState(r *http.Request, userID string, now time.Time) (accountLoginState, error) {
	var state accountLoginState
	err := a.DB.QueryRowContext(r.Context(), `SELECT business_day,failures,rounds,locked_until
		FROM auth_login_locks WHERE user_id=?`, userID).Scan(&state.day, &state.failures, &state.rounds, &state.lockedUntil)
	day := now.UTC().Add(8 * time.Hour).Format(time.DateOnly)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && state.day != day) {
		return accountLoginState{day: day}, nil
	}
	return state, err
}

func (a *API) recordAccountFailure(r *http.Request, userID string, state accountLoginState, now time.Time) (accountLoginState, error) {
	state.failures++
	if state.failures >= failuresPerLock {
		state.failures = 0
		state.rounds++
		shanghai := now.In(time.FixedZone("Asia/Shanghai", 8*60*60))
		midnight := time.Date(shanghai.Year(), shanghai.Month(), shanghai.Day()+1, 0, 0, 0, 0, shanghai.Location())
		until := now.Add(time.Duration(state.rounds) * 5 * time.Minute)
		if state.rounds >= dailyLockRound || until.After(midnight) {
			until = midnight
		}
		state.lockedUntil = until.Unix()
	}
	query := `INSERT INTO auth_login_locks(user_id,business_day,failures,rounds,locked_until) VALUES(?,?,?,?,?)
		ON CONFLICT(user_id) DO UPDATE SET business_day=excluded.business_day, failures=excluded.failures,
		rounds=excluded.rounds, locked_until=excluded.locked_until`
	if a.DB.IsMySQL() {
		query = `INSERT INTO auth_login_locks(user_id,business_day,failures,rounds,locked_until) VALUES(?,?,?,?,?) AS incoming
			ON DUPLICATE KEY UPDATE business_day=incoming.business_day, failures=incoming.failures,
			rounds=incoming.rounds, locked_until=incoming.locked_until`
	}
	_, err := a.DB.ExecContext(r.Context(), query, userID, state.day, state.failures, state.rounds, state.lockedUntil)
	return state, err
}

func writeAccountLocked(w http.ResponseWriter, state accountLoginState, now time.Time) {
	retry := time.Unix(state.lockedUntil, 0).Sub(now)
	w.Header().Set("Retry-After", retryAfterSeconds(retry))
	if state.rounds >= dailyLockRound {
		writeErr(w, 429, "account_locked_today", "今日登录已锁定，请在北京时间次日零点后重试")
		return
	}
	minutes := (retry + time.Minute - 1) / time.Minute
	writeErr(w, 429, "account_locked", "密码错误次数过多，请约 "+strconv.FormatInt(int64(minutes), 10)+" 分钟后重试")
}
