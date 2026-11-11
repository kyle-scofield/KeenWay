package app

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"golang.org/x/crypto/bcrypt"

	"keenway/internal/store"
)

func TestPanelLogin(t *testing.T) {
	st, err := store.OpenAt(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	hash, _ := bcrypt.GenerateFromPassword([]byte("секрет123"), bcrypt.MinCost)
	st.Update(func(s *store.State) { s.Panel.PassHash = string(hash) })
	ui := fstest.MapFS{"index.html": {Data: []byte("<html>KEENWAY-UI</html>")}, "icon.svg": {Data: []byte("<svg/>")}}
	s := NewPanel(st, ui)
	h := s.Handler()

	do := func(req *http.Request, remote string) *httptest.ResponseRecorder {
		req.RemoteAddr = remote
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		return w
	}
	home := "192.168.1.50:5000"

	// from the internet: nothing
	if w := do(httptest.NewRequest("GET", "/", nil), "8.8.8.8:1000"); w.Code != http.StatusForbidden {
		t.Fatalf("internet: %d", w.Code)
	}
	// at home without a session: the login page, not the app
	w := do(httptest.NewRequest("GET", "/", nil), home)
	if !strings.Contains(w.Body.String(), `name="password"`) || strings.Contains(w.Body.String(), "KEENWAY-UI") {
		t.Fatalf("login page expected: %s", w.Body.String())
	}
	// API without a session is refused even with the right header
	req := httptest.NewRequest("GET", "/api/state", nil)
	req.Header.Set("X-KeenWay", s.token)
	if w := do(req, home); w.Code != http.StatusForbidden {
		t.Fatalf("api without login: %d", w.Code)
	}
	// wrong password
	form := func(pw string) *http.Request {
		r := httptest.NewRequest("POST", "/login", strings.NewReader(url.Values{"password": {pw}}.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		return r
	}
	if w := do(form("неверно"), home); w.Code != http.StatusUnauthorized {
		t.Fatalf("wrong password: %d", w.Code)
	}
	// right password → session cookie → the app
	w = do(form("секрет123"), home)
	if w.Code != http.StatusFound {
		t.Fatalf("login: %d", w.Code)
	}
	cookie := w.Result().Cookies()[0]
	req = httptest.NewRequest("GET", "/", nil)
	req.AddCookie(cookie)
	if w := do(req, home); !strings.Contains(w.Body.String(), "KEENWAY-UI") {
		t.Fatalf("app after login: %s", w.Body.String())
	}
	req = httptest.NewRequest("GET", "/api/state", nil)
	req.AddCookie(cookie)
	req.Header.Set("X-KeenWay", cookie.Value)
	req.Host = "192.168.1.1:8090"
	w = do(req, home)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"panel":true`) || !strings.Contains(w.Body.String(), `"host":"192.168.1.1"`) {
		t.Fatalf("state: %d %s", w.Code, w.Body.String()[:200])
	}
}
