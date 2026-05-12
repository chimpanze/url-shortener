package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"time"

	"url-shortener/internal/store"
)

var ErrLoginFailed = errors.New("auth: invalid password")

type SessionConfig struct {
	CookieName    string
	TTL           time.Duration
	SecureCookies bool
	LoginPath     string
}

type ctxKey int

const sessionCtxKey ctxKey = 1

type SessionManager struct {
	store *store.Store
	cfg   SessionConfig
}

func NewSessionManager(s *store.Store, cfg SessionConfig) *SessionManager {
	if cfg.CookieName == "" {
		cfg.CookieName = "urlshortener_session"
	}
	if cfg.TTL == 0 {
		cfg.TTL = 7 * 24 * time.Hour
	}
	if cfg.LoginPath == "" {
		cfg.LoginPath = "/admin/login"
	}
	return &SessionManager{store: s, cfg: cfg}
}

func (m *SessionManager) Login(ctx context.Context, w http.ResponseWriter, password string) error {
	hash, err := m.store.GetAdminPasswordHash(ctx)
	if err != nil {
		return ErrLoginFailed
	}
	if err := VerifyPassword(hash, password); err != nil {
		return ErrLoginFailed
	}
	token, err := randomHex(32)
	if err != nil {
		return err
	}
	csrf, err := randomHex(32)
	if err != nil {
		return err
	}
	now := time.Now()
	sess := store.Session{
		Token: token, CSRFToken: csrf,
		CreatedAt: now, ExpiresAt: now.Add(m.cfg.TTL),
	}
	if err := m.store.CreateSession(ctx, sess); err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name:     m.cfg.CookieName,
		Value:    token,
		Path:     "/admin",
		HttpOnly: true,
		Secure:   m.cfg.SecureCookies,
		SameSite: http.SameSiteLaxMode,
		Expires:  sess.ExpiresAt,
	})
	return nil
}

func (m *SessionManager) Logout(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(m.cfg.CookieName); err == nil {
		_ = m.store.DeleteSession(ctx, c.Value)
	}
	http.SetCookie(w, &http.Cookie{
		Name:     m.cfg.CookieName,
		Value:    "",
		Path:     "/admin",
		HttpOnly: true,
		MaxAge:   -1,
	})
}

func (m *SessionManager) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(m.cfg.CookieName)
		if err != nil {
			http.Redirect(w, r, m.cfg.LoginPath, http.StatusSeeOther)
			return
		}
		sess, err := m.store.GetSession(r.Context(), c.Value)
		if err != nil {
			http.Redirect(w, r, m.cfg.LoginPath, http.StatusSeeOther)
			return
		}
		ctx := context.WithValue(r.Context(), sessionCtxKey, sess)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func SessionFromContext(ctx context.Context) (*store.Session, bool) {
	v, ok := ctx.Value(sessionCtxKey).(*store.Session)
	return v, ok
}

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
