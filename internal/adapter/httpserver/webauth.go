package httpserver

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"time"

	"github.com/serverkraken/flow/internal/adapter/webui"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/usecase"
)

const (
	sessionCookie = "flow_session"
	stateCookie   = "flow_oidc_state"
)

// Authenticator is the OIDC auth-code-flow port (oidcauth.Authenticator).
type Authenticator interface {
	AuthCodeURL(state string) string
	Exchange(ctx context.Context, code string) (ports.Identity, error)
}

// SessionCodec issues/parses the signed browser session cookie value.
type SessionCodec interface {
	Issue(userID string) (string, error)
	Parse(token string) (string, error)
}

func randToken() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	state := randToken()
	http.SetCookie(w, &http.Cookie{
		Name: stateCookie, Value: state, Path: "/", HttpOnly: true,
		Secure: !s.Dev, SameSite: http.SameSiteLaxMode, MaxAge: 600,
	})
	http.Redirect(w, r, s.OIDCAuth.AuthCodeURL(state), http.StatusFound)
}

// authErrorPage renders a Kristall error page for an OIDC-callback failure.
// Status must be written BEFORE the body — order matters for a correct
// response.
func (s *Server) authErrorPage(w http.ResponseWriter, r *http.Request, status int, titleKey, msgKey string, showLogin bool) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_ = webui.AuthPage(webui.AuthVM{TitleKey: titleKey, MsgKey: msgKey, ShowLogin: showLogin}).Render(r.Context(), w)
}

func (s *Server) handleCallback(w http.ResponseWriter, r *http.Request) {
	st, err := r.Cookie(stateCookie)
	if err != nil || st.Value == "" || st.Value != r.URL.Query().Get("state") {
		s.authErrorPage(w, r, http.StatusBadRequest, "auth.badState.title", "auth.badState.msg", true)
		return
	}
	id, err := s.OIDCAuth.Exchange(r.Context(), r.URL.Query().Get("code"))
	if err != nil {
		s.authErrorPage(w, r, http.StatusUnauthorized, "auth.failed.title", "auth.failed.msg", true)
		return
	}
	u, err := s.Ensure.Execute(r.Context(), id)
	if errors.Is(err, usecase.ErrNotAllowed) {
		s.authErrorPage(w, r, http.StatusForbidden, "auth.forbidden.title", "auth.forbidden.msg", false)
		return
	}
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	val, err := s.Session.Issue(u.ID)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: val, Path: "/", HttpOnly: true,
		Secure: !s.Dev, SameSite: http.SameSiteLaxMode, MaxAge: int((7 * 24 * time.Hour).Seconds()),
	})
	http.SetCookie(w, &http.Cookie{Name: stateCookie, Value: "", Path: "/", MaxAge: -1})
	http.Redirect(w, r, "/", http.StatusFound)
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", MaxAge: -1})
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.AuthPage(webui.AuthVM{TitleKey: "auth.loggedOut.title", MsgKey: "auth.loggedOut.msg", ShowLogin: true}).Render(r.Context(), w)
}

// resolveCookie loads the user from a valid session cookie.
func (s *Server) resolveCookie(r *http.Request) (domain.User, bool) {
	c, err := r.Cookie(sessionCookie)
	if err != nil {
		return domain.User{}, false
	}
	uid, err := s.Session.Parse(c.Value)
	if err != nil {
		return domain.User{}, false
	}
	u, err := s.Users.GetByID(r.Context(), uid)
	if err != nil {
		return domain.User{}, false
	}
	return u, true
}

// webAuth gates WebUI pages on a session cookie, redirecting to login.
func (s *Server) webAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, ok := s.resolveCookie(r)
		if !ok {
			http.Redirect(w, r, "/auth/login", http.StatusFound)
			return
		}
		if !s.validCookieWriteSource(r) {
			http.Error(w, "forbidden request origin", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r.WithContext(ctxWithUser(r.Context(), u)))
	})
}

// authAny accepts a bearer token (TUI) OR a session cookie (browser). Used by
// the SSE endpoint, which the browser EventSource reaches without an
// Authorization header.
func (s *Server) authAny(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if u, ok := s.resolveBearer(r); ok {
			next.ServeHTTP(w, r.WithContext(ctxWithUser(r.Context(), u)))
			return
		}
		if u, ok := s.resolveCookie(r); ok {
			if !s.validCookieWriteSource(r) {
				http.Error(w, "forbidden request origin", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r.WithContext(ctxWithUser(r.Context(), u)))
			return
		}
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	})
}
