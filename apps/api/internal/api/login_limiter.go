package api

import (
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"strconv"
	"sync"
	"time"
)

// Defaults are deliberately conservative but bounded: a wrong password never
// causes a permanent denial of service for the legitimate owner.
const (
	defaultMaxFailures = 5
	defaultLoginWindow = 5 * time.Minute
	defaultLockout     = 60 * time.Second
	// The IP-wide bucket is looser than the per-principal bucket so that a
	// household behind one NAT address is not locked out by a single typo.
	ipGlobalMultiplier = 3
)

type limitBucket struct {
	failures    int
	windowStart time.Time
	lockedUntil time.Time
}

// loginLimiter tracks failed logins per (principal, IP) and per IP.
// All state is guarded by one mutex; the clock is injectable for tests.
type loginLimiter struct {
	mu          sync.Mutex
	maxFailures int
	window      time.Duration
	lockout     time.Duration
	clock       func() time.Time
	principals  map[string]*limitBucket
	ips         map[string]*limitBucket
}

func newLoginLimiter(cfg LoginLimiterConfig, clock func() time.Time) *loginLimiter {
	max := cfg.MaxFailures
	if max <= 0 {
		max = defaultMaxFailures
	}
	win := cfg.Window
	if win <= 0 {
		win = defaultLoginWindow
	}
	lock := cfg.Lockout
	if lock <= 0 {
		lock = defaultLockout
	}
	if clock == nil {
		clock = time.Now
	}
	return &loginLimiter{
		maxFailures: max,
		window:      win,
		lockout:     lock,
		clock:       clock,
		principals:  make(map[string]*limitBucket),
		ips:         make(map[string]*limitBucket),
	}
}

// allow reports whether the attempt may proceed and, when blocked, how long
// the caller should wait. Lockouts are always finite (l.lockout).
func (l *loginLimiter) allow(principal, ip string) (bool, time.Duration) {
	now := l.clock()
	l.mu.Lock()
	defer l.mu.Unlock()
	l.prune(now)
	if b := l.principals[limiterKey(principal, ip)]; b != nil && now.Before(b.lockedUntil) {
		return false, b.lockedUntil.Sub(now)
	}
	if b := l.ips[ip]; b != nil && now.Before(b.lockedUntil) {
		return false, b.lockedUntil.Sub(now)
	}
	return true, 0
}

// failure records a rejected attempt. It also closes the TOCTOU window between
// allow() and the actual password check: concurrent requests that all passed
// allow() before the lock tripped must still be answered with 429, not 401.
// The request that *trips* the threshold itself still gets the ordinary 401;
// only requests arriving after the lock are throttled.
func (l *loginLimiter) failure(principal, ip string) (bool, time.Duration) {
	now := l.clock()
	l.mu.Lock()
	l.prune(now)
	key := limiterKey(principal, ip)
	if b := l.principals[key]; b != nil && now.Before(b.lockedUntil) {
		retry := b.lockedUntil.Sub(now)
		l.mu.Unlock()
		return true, retry
	}
	if b := l.ips[ip]; b != nil && now.Before(b.lockedUntil) {
		retry := b.lockedUntil.Sub(now)
		l.mu.Unlock()
		return true, retry
	}
	l.record(l.principals, key, now, l.maxFailures)
	l.record(l.ips, ip, now, l.maxFailures*ipGlobalMultiplier)
	l.mu.Unlock()

	// Security audit event: NO password, NO session token, NO raw username.
	slog.Warn("auth: login failure recorded",
		"action", "login_failed",
		"source_ip", ip,
		"principal_digest", limiterDigest(principal))
	return false, 0
}

// reset clears the per-principal bucket after a successful authentication.
// The IP-wide bucket is intentionally retained: one successful login must not
// let an attacker wipe the global counters for that source address.
func (l *loginLimiter) reset(principal, ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.principals, limiterKey(principal, ip))
}

func (l *loginLimiter) record(m map[string]*limitBucket, key string, now time.Time, threshold int) {
	b := m[key]
	if b == nil || now.Sub(b.windowStart) > l.window {
		b = &limitBucket{windowStart: now}
		m[key] = b
	}
	b.failures++
	if b.failures >= threshold {
		b.lockedUntil = now.Add(l.lockout)
		b.failures = 0 // bounded cooldown, never a permanent lock
	}
}

// prune drops stale buckets so a long-running server cannot grow unbounded.
func (l *loginLimiter) prune(now time.Time) {
	cutoff := 2 * l.window
	for k, b := range l.principals {
		if now.Sub(b.windowStart) > cutoff && now.After(b.lockedUntil) {
			delete(l.principals, k)
		}
	}
	for k, b := range l.ips {
		if now.Sub(b.windowStart) > cutoff && now.After(b.lockedUntil) {
			delete(l.ips, k)
		}
	}
}

func limiterKey(principal, ip string) string { return ip + "|" + principal }

func limiterDigest(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:4]) // 8 hex chars, non-reversible enough for correlation
}

func retryAfterSeconds(d time.Duration) string {
	secs := int(d/time.Second) + 1
	if secs < 1 {
		secs = 1
	}
	return strconv.Itoa(secs)
}
