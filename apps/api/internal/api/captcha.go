package api

import (
	"bytes"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"math"
	"math/big"
	"net/http"
	"sync"
	"time"
)

const captchaTTL = 2 * time.Minute

type captchaProof struct {
	ID     string `json:"id"`
	Answer string `json:"answer"`
}

type captchaEntry struct {
	answer  string
	ip      string
	expires time.Time
}

type captchaStore struct {
	mu      sync.Mutex
	entries map[string]captchaEntry
}

func (s *captchaStore) issue(ip string, now time.Time) (string, string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.entries == nil {
		s.entries = make(map[string]captchaEntry)
	}
	perIP := 0
	for id, entry := range s.entries {
		if !now.Before(entry.expires) {
			delete(s.entries, id)
		} else if entry.ip == ip {
			perIP++
		}
	}
	if len(s.entries) >= 4096 || perIP >= 24 {
		return "", "", false
	}
	var digits [6]byte
	for i := range digits {
		digits[i] = byte('0' + captchaRand(10))
	}
	id, answer := newID("cap"), string(digits[:])
	s.entries[id] = captchaEntry{answer: answer, ip: ip, expires: now.Add(captchaTTL)}
	return id, answer, true
}

func (a *API) requireCaptcha(w http.ResponseWriter, r *http.Request, proof captchaProof) bool {
	a.captchas.mu.Lock()
	entry, exists := a.captchas.entries[proof.ID]
	sameSource := exists && entry.ip == a.clientIP(r)
	if sameSource {
		delete(a.captchas.entries, proof.ID)
	}
	a.captchas.mu.Unlock()
	if sameSource && a.now().Before(entry.expires) && subtle.ConstantTimeCompare([]byte(entry.answer), []byte(proof.Answer)) == 1 {
		return true
	}
	writeErr(w, 400, "captcha_invalid", "验证码错误或已失效，请重新输入")
	return false
}

func (a *API) handleCaptcha(w http.ResponseWriter, r *http.Request) {
	id, answer, ok := a.captchas.issue(a.clientIP(r), a.now())
	if !ok {
		w.Header().Set("Retry-After", "120")
		writeErr(w, 429, "rate_limited", "验证码请求过多，请稍后重试")
		return
	}
	img, err := captchaImage(answer)
	if err != nil {
		internalErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]string{
		"id": id, "image": "data:image/png;base64," + base64.StdEncoding.EncodeToString(img),
	})
}

func captchaRand(max int64) int {
	n, err := rand.Int(rand.Reader, big.NewInt(max))
	if err != nil {
		panic("api: secure random unavailable: " + err.Error())
	}
	return int(n.Int64())
}

// Pixel glyphs keep the answer out of markup, metadata and accessibility text.
var captchaDigits = [10][7]uint8{
	{14, 17, 19, 21, 25, 17, 14},
	{4, 12, 4, 4, 4, 4, 14},
	{14, 17, 1, 2, 4, 8, 31},
	{30, 1, 1, 14, 1, 1, 30},
	{2, 6, 10, 18, 31, 2, 2},
	{31, 16, 16, 30, 1, 1, 30},
	{14, 16, 16, 30, 17, 17, 14},
	{31, 1, 2, 4, 8, 8, 8},
	{14, 17, 17, 14, 17, 17, 14},
	{14, 17, 17, 15, 1, 1, 14},
}

func captchaImage(answer string) ([]byte, error) {
	const width, height = 202, 56
	img := image.NewGray(image.Rect(0, 0, width, height))
	for i := range img.Pix {
		img.Pix[i] = 248
	}
	for i := 0; i < 500; i++ {
		img.SetGray(captchaRand(width), captchaRand(height), color.Gray{Y: uint8(120 + captchaRand(100))})
	}
	for i, digit := range answer {
		glyph := captchaDigits[digit-'0']
		centerX, centerY := 21+i*32+captchaRand(7)-3, 28+captchaRand(11)-5
		angle := float64(captchaRand(41)-20) * math.Pi / 180
		sin, cos := math.Sincos(angle)
		shade := color.Gray{Y: uint8(20 + captchaRand(55))}
		for y := -19; y <= 19; y++ {
			for x := -16; x <= 16; x++ {
				gx := int(math.Floor((float64(x)*cos + float64(y)*sin + 10) / 4))
				gy := int(math.Floor((-float64(x)*sin + float64(y)*cos + 14) / 4))
				if gx >= 0 && gx < 5 && gy >= 0 && gy < 7 && glyph[gy]&(1<<(4-gx)) != 0 {
					img.SetGray(centerX+x, centerY+y, shade)
				}
			}
		}
	}
	for i := 0; i < 4; i++ {
		phase, level := float64(captchaRand(60)), float64(8+captchaRand(40))
		for x := 0; x < width; x++ {
			y := int(level + 6*math.Sin((float64(x)+phase)/17))
			img.SetGray(x, y, color.Gray{Y: 116})
		}
	}
	var out bytes.Buffer
	err := png.Encode(&out, img)
	return out.Bytes(), err
}
