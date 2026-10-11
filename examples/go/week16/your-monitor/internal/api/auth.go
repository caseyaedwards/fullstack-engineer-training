package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"fsp/examples/week16/your-monitor/internal/auth"
)

// region: auth-interface

// Auth is what the api needs from the auth service. *auth.Service
// implements it; the tests use a fake.
type Auth interface {
	Login(ctx context.Context, email, password string) (auth.Session, error)
	User(ctx context.Context, token string) (auth.User, error)
	Logout(ctx context.Context, token string) error
}

// endregion: auth-interface

// region: cookie

// cookieName is the name of the session cookie.
const cookieName = "session"

// sessionCookie returns the cookie that carries token. maxAge is in
// seconds; a negative maxAge tells the browser to delete the cookie.
func sessionCookie(token string, maxAge int) *http.Cookie {
	return &http.Cookie{
		Name:     cookieName,
		Value:    token,
		Path:     "/",                  // send it with every request to this site
		MaxAge:   maxAge,               // then forget it
		HttpOnly: true,                 // JavaScript cannot read it: document.cookie leaves it out
		Secure:   true,                 // only over HTTPS, and http://localhost
		SameSite: http.SameSiteLaxMode, // not on requests that other sites start, except link clicks
	}
}

// endregion: cookie

// region: login

// loginRequest is the body of POST /api/login.
type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (h *handler) login(w http.ResponseWriter, r *http.Request) {
	if !isJSON(r) {
		writeError(w, http.StatusUnsupportedMediaType, "Content-Type must be application/json")
		return
	}
	req, status, err := decodeJSON[loginRequest](w, r)
	if err != nil {
		writeError(w, status, err.Error())
		return
	}
	if req.Email == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "email and password are required")
		return
	}
	if len(req.Email) > auth.MaxEmailBytes {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("email must be at most %d bytes", auth.MaxEmailBytes))
		return
	}

	sess, err := h.auth.Login(r.Context(), req.Email, req.Password)
	var tooMany *auth.TooManyAttemptsError
	switch {
	case errors.As(err, &tooMany):
		w.Header().Set("Retry-After", strconv.Itoa(tooMany.Seconds()))
		writeError(w, http.StatusTooManyRequests, err.Error())
		return
	case errors.Is(err, auth.ErrBadCredentials):
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	case err != nil:
		internalError(w, r, err)
		return
	}
	http.SetCookie(w, sessionCookie(sess.Token, int(auth.SessionTTL.Seconds())))
	writeJSON(w, http.StatusOK, sess.User)
}

// endregion: login

// region: logout

// logout ends the session in the cookie, if there is one, and tells the
// browser to delete the cookie. It answers 204 either way.
func (h *handler) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(cookieName); err == nil {
		if err := h.auth.Logout(r.Context(), c.Value); err != nil {
			internalError(w, r, err)
			return
		}
	}
	http.SetCookie(w, sessionCookie("", -1))
	w.WriteHeader(http.StatusNoContent)
}

// me answers with the logged-in user. requireUser runs first.
func (h *handler) me(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFrom(r.Context())
	writeJSON(w, http.StatusOK, u)
}

// endregion: logout

// region: require

// requireUser wraps a handler that only a logged-in user may reach. With
// no cookie, or a cookie whose session is gone or expired, it answers 401
// and next never runs.
func (h *handler) requireUser(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(cookieName)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "login required")
			return
		}
		u, err := h.auth.User(r.Context(), c.Value)
		if errors.Is(err, auth.ErrNoSession) {
			writeError(w, http.StatusUnauthorized, "login required")
			return
		}
		if err != nil {
			internalError(w, r, err)
			return
		}
		next(w, r.WithContext(auth.WithUser(r.Context(), u)))
	}
}

// endregion: require
