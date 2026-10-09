package server

import (
	"crypto/rand"
	"math/big"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/metanoia/pali-reader/backend/internal/store"
	"golang.org/x/crypto/bcrypt"
)

// Accounts.
//
// Email verification is simulated in this build: no mail is sent, and the
// registration response carries the code so the client can show it. The flow
// is otherwise the real one — the account does not exist until the code is
// presented — so swapping in a mailer later changes one function.

var emailRe = regexp.MustCompile(`^[^@\s]+@[^@\s.]+\.[^@\s]+$`)

const (
	codeTTL      = 15 * time.Minute
	maxCodeTries = 8
)

type registerReq struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Name     string `json:"name"`
}

type verifyReq struct {
	Email string `json:"email"`
	Code  string `json:"code"`
}

type loginReq struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type authResp struct {
	Token string      `json:"token"`
	User  *store.User `json:"user"`
	// Code is only populated by register, and only because verification is
	// simulated here. It is never returned for a real account.
	Code string `json:"code,omitempty"`
}

func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	var req registerReq
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", "请求格式错误")
		return
	}
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	if !emailRe.MatchString(req.Email) {
		writeErr(w, http.StatusBadRequest, "bad_email", "请输入有效的邮箱地址")
		return
	}
	if len(req.Password) < 6 {
		writeErr(w, http.StatusBadRequest, "weak_password", "密码至少 6 位")
		return
	}
	if ip := clientIP(r); ip != "" && s.cache.Enabled() {
		n, err := s.cache.Incr(r.Context(), "rl:reg:"+ip, time.Hour)
		if err == nil && n > 20 {
			writeErr(w, http.StatusTooManyRequests, "rate_limited", "注册过于频繁，请稍后再试")
			return
		}
	}

	var existing store.User
	if err := s.db.WithContext(r.Context()).Where("email = ?", req.Email).First(&existing).Error; err == nil {
		writeErr(w, http.StatusConflict, "email_taken", "该邮箱已注册，请直接登录")
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		writeServerErr(w, err)
		return
	}
	code := randomCode()
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = strings.Split(req.Email, "@")[0]
	}

	pend := store.PendingRegistration{
		Email: req.Email, Code: code, PassHash: string(hash), Name: name,
		ExpiresAt: time.Now().Add(codeTTL), CreatedAt: time.Now(),
	}
	if err := s.db.WithContext(r.Context()).
		Where("email = ?", req.Email).Delete(&store.PendingRegistration{}).Error; err != nil {
		writeServerErr(w, err)
		return
	}
	if err := s.db.WithContext(r.Context()).Create(&pend).Error; err != nil {
		writeServerErr(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":        true,
		"sentTo":    req.Email,
		"expiresIn": int(codeTTL.Seconds()),
		// Simulated delivery: the code comes straight back to the client.
		"code": code,
		"msg":  "验证码已发送（演示环境直接显示）",
	})
}

func (s *Server) handleVerify(w http.ResponseWriter, r *http.Request) {
	var req verifyReq
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", "请求格式错误")
		return
	}
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	req.Code = strings.TrimSpace(req.Code)

	var pend store.PendingRegistration
	if err := s.db.WithContext(r.Context()).Where("email = ?", req.Email).First(&pend).Error; err != nil {
		writeErr(w, http.StatusBadRequest, "no_pending", "没有待验证的注册，请重新注册")
		return
	}
	if time.Now().After(pend.ExpiresAt) {
		s.db.Where("email = ?", req.Email).Delete(&store.PendingRegistration{})
		writeErr(w, http.StatusBadRequest, "expired", "验证码已过期，请重新获取")
		return
	}
	if pend.Attempts >= maxCodeTries {
		s.db.Where("email = ?", req.Email).Delete(&store.PendingRegistration{})
		writeErr(w, http.StatusTooManyRequests, "too_many_tries", "尝试次数过多，请重新注册")
		return
	}
	if pend.Code != req.Code {
		s.db.Model(&store.PendingRegistration{}).Where("email = ?", req.Email).
			UpdateColumn("attempts", pend.Attempts+1)
		writeErr(w, http.StatusBadRequest, "bad_code", "验证码不正确")
		return
	}

	u := store.User{
		Email: pend.Email, DisplayName: pend.Name, PassHash: pend.PassHash,
		CreatedAt: time.Now(), LastSeenAt: time.Now(),
	}
	if err := s.db.WithContext(r.Context()).Create(&u).Error; err != nil {
		if store.IsDuplicate(err) {
			writeErr(w, http.StatusConflict, "email_taken", "该邮箱已注册，请直接登录")
			return
		}
		writeServerErr(w, err)
		return
	}
	s.db.Where("email = ?", pend.Email).Delete(&store.PendingRegistration{})

	tok, err := s.issueSession(r, u.ID)
	if err != nil {
		writeServerErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, authResp{Token: tok, User: &u})
}

func (s *Server) handleResend(w http.ResponseWriter, r *http.Request) {
	var req verifyReq
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", "请求格式错误")
		return
	}
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	var pend store.PendingRegistration
	if err := s.db.WithContext(r.Context()).Where("email = ?", req.Email).First(&pend).Error; err != nil {
		writeErr(w, http.StatusBadRequest, "no_pending", "没有待验证的注册，请重新注册")
		return
	}
	pend.Code = randomCode()
	pend.Attempts = 0
	pend.ExpiresAt = time.Now().Add(codeTTL)
	if err := s.db.WithContext(r.Context()).Save(&pend).Error; err != nil {
		writeServerErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "code": pend.Code, "msg": "验证码已重新发送"})
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req loginReq
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", "请求格式错误")
		return
	}
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))

	if ip := clientIP(r); ip != "" && s.cache.Enabled() {
		n, err := s.cache.Incr(r.Context(), "rl:login:"+ip+":"+req.Email, 15*time.Minute)
		if err == nil && n > 30 {
			writeErr(w, http.StatusTooManyRequests, "rate_limited", "尝试过于频繁，请稍后再试")
			return
		}
	}

	var u store.User
	if err := s.db.WithContext(r.Context()).Where("email = ?", req.Email).First(&u).Error; err != nil {
		// Same message for unknown account and wrong password, so the endpoint
		// cannot be used to enumerate addresses.
		writeErr(w, http.StatusUnauthorized, "bad_credentials", "邮箱或密码不正确")
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(u.PassHash), []byte(req.Password)) != nil {
		writeErr(w, http.StatusUnauthorized, "bad_credentials", "邮箱或密码不正确")
		return
	}
	tok, err := s.issueSession(r, u.ID)
	if err != nil {
		writeServerErr(w, err)
		return
	}
	s.db.Model(&store.User{}).Where("id = ?", u.ID).Update("last_seen_at", time.Now())
	writeJSON(w, http.StatusOK, authResp{Token: tok, User: &u})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	tok := bearer(r)
	if tok != "" {
		s.db.WithContext(r.Context()).Where("token = ?", tok).Delete(&store.Session{})
		s.cache.Del(r.Context(), "s:"+tok)
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"user": user(r)})
}

func (s *Server) handleUpdateMe(w http.ResponseWriter, r *http.Request) {
	u := user(r)
	var req struct {
		Name     string `json:"name"`
		Password string `json:"password"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", "请求格式错误")
		return
	}
	updates := map[string]any{}
	if n := strings.TrimSpace(req.Name); n != "" {
		updates["display_name"] = n
	}
	if len(req.Password) >= 6 {
		h, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
		if err != nil {
			writeServerErr(w, err)
			return
		}
		updates["pass_hash"] = string(h)
	}
	if len(updates) == 0 {
		writeJSON(w, http.StatusOK, map[string]any{"user": u})
		return
	}
	if err := s.db.WithContext(r.Context()).Model(&store.User{}).Where("id = ?", u.ID).
		Updates(updates).Error; err != nil {
		writeServerErr(w, err)
		return
	}
	s.db.WithContext(r.Context()).Where("id = ?", u.ID).First(u)
	s.cache.Del(r.Context(), "s:"+bearer(r))
	writeJSON(w, http.StatusOK, map[string]any{"user": u})
}

func (s *Server) issueSession(r *http.Request, userID uint) (string, error) {
	tok := newToken()
	sess := store.Session{
		Token: tok, UserID: userID,
		ExpiresAt: time.Now().Add(s.cfg.SessionTTL), CreatedAt: time.Now(),
	}
	if err := s.db.WithContext(r.Context()).Create(&sess).Error; err != nil {
		return "", err
	}
	// Old sessions for the same account are pruned opportunistically.
	s.db.Where("user_id = ? AND expires_at < ?", userID, time.Now()).Delete(&store.Session{})
	return tok, nil
}

func randomCode() string {
	const digits = "0123456789"
	var b strings.Builder
	for i := 0; i < 6; i++ {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(digits))))
		if err != nil {
			return "000000"
		}
		b.WriteByte(digits[n.Int64()])
	}
	return b.String()
}

func clientIP(r *http.Request) string {
	if v := r.Header.Get("X-Real-IP"); v != "" {
		return v
	}
	if v := r.Header.Get("X-Forwarded-For"); v != "" {
		if i := strings.IndexByte(v, ','); i > 0 {
			return strings.TrimSpace(v[:i])
		}
		return strings.TrimSpace(v)
	}
	host := r.RemoteAddr
	if i := strings.LastIndexByte(host, ':'); i > 0 {
		host = host[:i]
	}
	return host
}
