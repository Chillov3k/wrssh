package auth

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"
)

func TestSessionCookieRoundTrip(t *testing.T) {
	manager := New("test-secret")
	recorder := httptest.NewRecorder()

	if err := manager.SetSessionCookie(recorder, 42, 7, time.Hour, false); err != nil {
		t.Fatalf("set cookie: %v", err)
	}

	request := httptest.NewRequest("GET", "/", nil)
	for _, cookie := range recorder.Result().Cookies() {
		request.AddCookie(cookie)
	}

	session, err := manager.ParseSessionCookie(request)
	if err != nil {
		t.Fatalf("parse cookie: %v", err)
	}

	if session.UserID != 42 || session.Version != 7 {
		t.Fatalf("unexpected session: %+v", session)
	}
}

func TestSessionCookieDefaultTTLIsSevenDays(t *testing.T) {
	manager := New("test-secret")
	recorder := httptest.NewRecorder()
	before := time.Now().Add(SessionTTL - time.Minute)

	if err := manager.SetSessionCookie(recorder, 42, 7, 0, false); err != nil {
		t.Fatalf("set cookie: %v", err)
	}

	cookies := recorder.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("expected one cookie, got %d", len(cookies))
	}

	after := time.Now().Add(SessionTTL + time.Minute)
	if cookies[0].Expires.Before(before) || cookies[0].Expires.After(after) {
		t.Fatalf("cookie expiry = %s, expected around %s", cookies[0].Expires, SessionTTL)
	}
}

func TestSessionCookieRejectsInvalidSignature(t *testing.T) {
	manager := New("test-secret")
	request := httptest.NewRequest("GET", "/", nil)
	request.AddCookie(&http.Cookie{
		Name:  CookieName,
		Value: "broken",
	})

	if _, err := manager.ParseSessionCookie(request); err == nil {
		t.Fatal("expected parse error for invalid cookie")
	}
}

func TestSessionCookieParsesLegacyFormat(t *testing.T) {
	manager := New("test-secret")
	recorder := httptest.NewRecorder()

	expiresAt := time.Now().Add(time.Hour).Unix()
	payload := "42:" + strconv.FormatInt(expiresAt, 10)
	signature := manager.sign(payload)
	value := base64.RawURLEncoding.EncodeToString([]byte(payload + ":" + signature))

	http.SetCookie(recorder, &http.Cookie{
		Name:  CookieName,
		Value: value,
		Path:  "/",
	})

	request := httptest.NewRequest("GET", "/", nil)
	for _, cookie := range recorder.Result().Cookies() {
		request.AddCookie(cookie)
	}

	session, err := manager.ParseSessionCookie(request)
	if err != nil {
		t.Fatalf("parse legacy cookie: %v", err)
	}
	if session.UserID != 42 || session.Version != 0 {
		t.Fatalf("unexpected legacy session: %+v", session)
	}
}
