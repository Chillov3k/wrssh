package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const CookieName = "rssh_web_session"

type Manager struct {
	secret []byte
}

func New(secret string) *Manager {
	return &Manager{secret: []byte(secret)}
}

func (m *Manager) SetSessionCookie(w http.ResponseWriter, userID uint, ttl time.Duration) error {
	if ttl <= 0 {
		ttl = 12 * time.Hour
	}

	expiresAt := time.Now().Add(ttl).Unix()
	payload := fmt.Sprintf("%d:%d", userID, expiresAt)
	signature := m.sign(payload)
	value := base64.RawURLEncoding.EncodeToString([]byte(payload + ":" + signature))

	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    value,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Unix(expiresAt, 0),
	})
	return nil
}

func (m *Manager) ClearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
	})
}

func (m *Manager) ParseSessionCookie(r *http.Request) (uint, error) {
	cookie, err := r.Cookie(CookieName)
	if err != nil {
		return 0, err
	}

	decoded, err := base64.RawURLEncoding.DecodeString(cookie.Value)
	if err != nil {
		return 0, errors.New("decode cookie")
	}

	parts := strings.Split(string(decoded), ":")
	if len(parts) != 3 {
		return 0, errors.New("invalid cookie payload")
	}

	payload := strings.Join(parts[:2], ":")
	if !hmac.Equal([]byte(m.sign(payload)), []byte(parts[2])) {
		return 0, errors.New("invalid cookie signature")
	}

	expiresAt, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return 0, errors.New("invalid cookie expiry")
	}
	if time.Now().Unix() > expiresAt {
		return 0, errors.New("cookie expired")
	}

	userID, err := strconv.ParseUint(parts[0], 10, 64)
	if err != nil {
		return 0, errors.New("invalid cookie user")
	}

	return uint(userID), nil
}

func (m *Manager) sign(payload string) string {
	mac := hmac.New(sha256.New, m.secret)
	mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
