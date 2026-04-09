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

type Session struct {
	UserID  uint
	Version uint64
}

type Manager struct {
	secret []byte
}

func New(secret string) *Manager {
	return &Manager{secret: []byte(secret)}
}

func (m *Manager) SetSessionCookie(w http.ResponseWriter, userID uint, version uint64, ttl time.Duration) error {
	if ttl <= 0 {
		ttl = 12 * time.Hour
	}

	expiresAt := time.Now().Add(ttl).Unix()
	payload := fmt.Sprintf("%d:%d:%d", userID, version, expiresAt)
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

func (m *Manager) ParseSessionCookie(r *http.Request) (Session, error) {
	cookie, err := r.Cookie(CookieName)
	if err != nil {
		return Session{}, err
	}

	decoded, err := base64.RawURLEncoding.DecodeString(cookie.Value)
	if err != nil {
		return Session{}, errors.New("decode cookie")
	}

	parts := strings.Split(string(decoded), ":")
	var payload string
	var userIDPart string
	var versionPart string
	var expiresPart string
	var signaturePart string

	switch len(parts) {
	case 3:
		payload = strings.Join(parts[:2], ":")
		userIDPart = parts[0]
		versionPart = "0"
		expiresPart = parts[1]
		signaturePart = parts[2]
	case 4:
		payload = strings.Join(parts[:3], ":")
		userIDPart = parts[0]
		versionPart = parts[1]
		expiresPart = parts[2]
		signaturePart = parts[3]
	default:
		return Session{}, errors.New("invalid cookie payload")
	}

	if !hmac.Equal([]byte(m.sign(payload)), []byte(signaturePart)) {
		return Session{}, errors.New("invalid cookie signature")
	}

	expiresAt, err := strconv.ParseInt(expiresPart, 10, 64)
	if err != nil {
		return Session{}, errors.New("invalid cookie expiry")
	}
	if time.Now().Unix() > expiresAt {
		return Session{}, errors.New("cookie expired")
	}

	userID, err := strconv.ParseUint(userIDPart, 10, 64)
	if err != nil {
		return Session{}, errors.New("invalid cookie user")
	}
	version, err := strconv.ParseUint(versionPart, 10, 64)
	if err != nil {
		return Session{}, errors.New("invalid cookie version")
	}

	return Session{
		UserID:  uint(userID),
		Version: version,
	}, nil
}

func (m *Manager) sign(payload string) string {
	mac := hmac.New(sha256.New, m.secret)
	mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
