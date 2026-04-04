package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSessionCookieRoundTrip(t *testing.T) {
	manager := New("test-secret")
	recorder := httptest.NewRecorder()

	if err := manager.SetSessionCookie(recorder, 42, time.Hour); err != nil {
		t.Fatalf("set cookie: %v", err)
	}

	request := httptest.NewRequest("GET", "/", nil)
	for _, cookie := range recorder.Result().Cookies() {
		request.AddCookie(cookie)
	}

	userID, err := manager.ParseSessionCookie(request)
	if err != nil {
		t.Fatalf("parse cookie: %v", err)
	}

	if userID != 42 {
		t.Fatalf("unexpected user id: %d", userID)
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
