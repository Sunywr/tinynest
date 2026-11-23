package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"testing"
	"time"
)

func TestLoginRequiresCaptcha(t *testing.T) {
	e := newRemEnv(t, Options{SetupToken: testSetupToken})
	h := bootstrapHousehold(t, e, "captcha")
	r := e.serveRaw(http.MethodPost, "/api/v1/auth/session", correctLogin(h.username), "203.0.113.11:5000", nil)
	if r.Status != 400 {
		t.Fatalf("login without CAPTCHA returned %d", r.Status)
	}
}

func TestRegisterRequiresCaptcha(t *testing.T) {
	e := newRemEnv(t, Options{Clock: time.Now})
	r := e.serveRaw(http.MethodPost, "/api/v1/auth/register", map[string]string{"username": "captcha-test", "password": "abcdefgh1234"}, "203.0.113.11:5000", nil)
	if r.Status != 400 {
		t.Fatalf("registration without CAPTCHA returned %d", r.Status)
	}
}

var captchaTestServers sync.Map

func registerCaptchaTestServer(t testing.TB, url string, api *API) {
	captchaTestServers.Store(url, api)
	t.Cleanup(func() { captchaTestServers.Delete(url) })
}

func attachCaptchaForTest(t testing.TB, req *http.Request, server *API) {
	t.Helper()
	if req.Method != http.MethodPost || (req.URL.Path != "/api/v1/auth/session" && req.URL.Path != "/api/v1/auth/register") || server == nil {
		return
	}
	var body map[string]any
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if _, explicit := body["captcha"]; !explicit {
		ip := server.clientIP(req)
		if req.RemoteAddr == "" {
			req.RemoteAddr = "127.0.0.1:5000"
			ip = server.clientIP(req)
			req.RemoteAddr = ""
		}
		id, answer, ok := server.captchas.issue(ip, server.now())
		if !ok {
			t.Fatal("test CAPTCHA quota exceeded")
		}
		body["captcha"] = captchaProof{ID: id, Answer: answer}
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req.Body = io.NopCloser(bytes.NewReader(encoded))
	req.ContentLength = int64(len(encoded))
	req.GetBody = func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(encoded)), nil }
}

func TestCaptchaRejectsWrongExpiredAndReplayedAnswers(t *testing.T) {
	clock := &mutableClock{now: time.Date(2026, 11, 23, 4, 0, 0, 0, time.UTC)}
	e := newRemEnv(t, Options{Clock: clock.Now, SetupToken: testSetupToken})
	h := bootstrapHousehold(t, e, "captcha-replay")
	const host = "203.0.113.10"
	const remote = host + ":5000"
	newProof := func() captchaProof {
		id, answer, ok := e.a.captchas.issue(host, clock.Now())
		if !ok {
			t.Fatal("challenge could not be issued")
		}
		return captchaProof{ID: id, Answer: answer}
	}
	login := func(proof captchaProof) remResult {
		body := correctLogin(h.username)
		body["captcha"] = proof
		return e.serveRaw(http.MethodPost, "/api/v1/auth/session", body, remote, nil)
	}
	original := newProof()
	wrong := original
	wrong.Answer = "wrong"
	if r := login(wrong); r.Status != 400 {
		t.Fatalf("wrong code: %v", r)
	}
	if r := login(original); r.Status != 400 {
		t.Fatalf("used code: %v", r)
	}
	expired := newProof()
	clock.Advance(captchaTTL)
	if r := login(expired); r.Status != 400 {
		t.Fatalf("expired code: %v", r)
	}
	valid := newProof()
	if r := login(valid); r.Status != 200 {
		t.Fatalf("valid code: %v", r)
	}
	if r := login(valid); r.Status != 400 {
		t.Fatalf("replayed code: %v", r)
	}
}

func TestAccountLockEscalatesAcrossIPsUntilShanghaiMidnight(t *testing.T) {
	clock := &mutableClock{now: time.Date(2026, 11, 23, 4, 0, 0, 0, time.UTC)}
	e := newRemEnv(t, Options{Clock: clock.Now, SetupToken: testSetupToken})
	h := bootstrapHousehold(t, e, "account-lock")
	for round := 1; round <= 5; round++ {
		for fail := 1; fail <= 5; fail++ {
			ip := fmt.Sprintf("203.0.113.%d:5000", round*5+fail)
			result := e.serve(http.MethodPost, "/api/v1/auth/session", wrongLogin(h.username), ip, nil)
			want := 401
			if fail == 5 {
				want = 429
			}
			if result.Status != want {
				t.Fatalf("round %d failure %d: %v", round, fail, result)
			}
		}
		result := e.serve(http.MethodPost, "/api/v1/auth/session", correctLogin(h.username), "198.51.100.7:5000", nil)
		if result.Status != 429 {
			t.Fatalf("round %d lock not enforced across IPs: %v", round, result)
		}
		if round < 5 {
			duration := time.Duration(round) * 5 * time.Minute
			clock.Advance(duration - time.Second)
			if r := e.serve(http.MethodPost, "/api/v1/auth/session", correctLogin(h.username), "198.51.100.7:5000", nil); r.Status != 429 {
				t.Fatalf("round %d ended early: %v", round, r)
			}
			clock.Advance(time.Second)
			if r := e.serve(http.MethodPost, "/api/v1/auth/session", correctLogin(h.username), "198.51.100.7:5000", nil); r.Status != 200 {
				t.Fatalf("round %d did not unlock: %v", round, r)
			}
		}
	}
	midnight := time.Date(2026, 11, 23, 16, 0, 0, 0, time.UTC)
	clock.Advance(midnight.Sub(clock.Now()) - time.Second)
	if r := e.serve(http.MethodPost, "/api/v1/auth/session", correctLogin(h.username), "198.51.100.7:5000", nil); r.Status != 429 {
		t.Fatalf("daily lock ended early: %v", r)
	}
	clock.Advance(time.Second)
	if r := e.serve(http.MethodPost, "/api/v1/auth/session", correctLogin(h.username), "198.51.100.7:5000", nil); r.Status != 200 {
		t.Fatalf("daily lock did not expire at Shanghai midnight: %v", r)
	}
}
func TestAccountLockPersistsInDatabase(t *testing.T) {
	clock := &mutableClock{now: time.Date(2026, 11, 23, 4, 0, 0, 0, time.UTC)}
	server, client, database := newTestServerWithOptions(t, Options{Clock: clock.Now})
	defer database.Close()
	defer server.Close()
	for i := 0; i < failuresPerLock; i++ {
		status, _ := client.do("POST", "/api/v1/auth/session", wrongLogin("admin"))
		if i < failuresPerLock-1 && status != 401 || i == failuresPerLock-1 && status != 429 {
			t.Fatalf("failure %d returned %d", i+1, status)
		}
	}
	var round int
	var until int64
	err := database.QueryRow(`SELECT rounds,locked_until FROM auth_login_locks
		WHERE user_id=(SELECT id FROM users WHERE username='admin')`).Scan(&round, &until)
	if err != nil || round != 1 || until != clock.Now().Add(5*time.Minute).Unix() {
		t.Fatalf("persisted lock = (%d,%d), %v", round, until, err)
	}
	clock.Advance(5 * time.Minute)
	client.mustOK("POST", "/api/v1/auth/session", correctLogin("admin"))
}
