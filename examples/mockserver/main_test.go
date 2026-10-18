package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ReBase-Dev-Studio/goppelganger"
)

var requiredUserFields = []string{"id", "username", "email", "role", "createdAt", "address"}

func newTestServer(t *testing.T, seed int64) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(newHandler(goppelganger.New(goppelganger.Options{Seed: &seed})))
	t.Cleanup(server.Close)
	return server
}

// fetch sends a request and returns the status code and body, checking that
// every response is JSON.
func fetch(t *testing.T, method, url string) (int, []byte) {
	t.Helper()
	request, err := http.NewRequest(method, url, nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if response.StatusCode != http.StatusMethodNotAllowed {
		if contentType := response.Header.Get("Content-Type"); contentType != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", contentType)
		}
	}
	return response.StatusCode, body
}

func decode[T any](t *testing.T, body []byte) T {
	t.Helper()
	var value T
	if err := json.Unmarshal(body, &value); err != nil {
		t.Fatalf("decode %s: %v", body, err)
	}
	return value
}

func assertUser(t *testing.T, user map[string]any) {
	t.Helper()
	for _, field := range requiredUserFields {
		if _, ok := user[field]; !ok {
			t.Errorf("user %v is missing required field %q", user, field)
		}
	}
}

func TestListUsers(t *testing.T) {
	server := newTestServer(t, 1)
	for _, test := range []struct {
		query string
		want  int
	}{
		{"", defaultUserCount},
		{"?count=1", 1},
		{"?count=5", 5},
		{"?count=50", maxUserCount},
	} {
		status, body := fetch(t, http.MethodGet, server.URL+"/users"+test.query)
		if status != http.StatusOK {
			t.Fatalf("GET /users%s = %d %s, want 200", test.query, status, body)
		}
		users := decode[[]map[string]any](t, body)
		if len(users) != test.want {
			t.Errorf("GET /users%s returned %d users, want %d", test.query, len(users), test.want)
		}
		for _, user := range users {
			assertUser(t, user)
		}
	}
}

func TestListUsersRejectsBadCount(t *testing.T) {
	server := newTestServer(t, 1)
	for _, count := range []string{"0", "-1", "51", "abc", "2.5"} {
		status, body := fetch(t, http.MethodGet, server.URL+"/users?count="+count)
		if status != http.StatusBadRequest {
			t.Errorf("count=%s: status = %d, want 400", count, status)
		}
		if decode[map[string]string](t, body)["error"] == "" {
			t.Errorf("count=%s: body %s has no error message", count, body)
		}
	}
}

func TestGetUserUsesPathID(t *testing.T) {
	server := newTestServer(t, 1)
	status, body := fetch(t, http.MethodGet, server.URL+"/users/7")
	if status != http.StatusOK {
		t.Fatalf("GET /users/7 = %d %s, want 200", status, body)
	}
	user := decode[map[string]any](t, body)
	assertUser(t, user)
	if user["id"] != float64(7) {
		t.Errorf("id = %#v, want 7", user["id"])
	}
}

func TestGetUserRejectsBadID(t *testing.T) {
	server := newTestServer(t, 1)
	for _, id := range []string{"0", "-3", "abc", "99999999999999999999"} {
		status, body := fetch(t, http.MethodGet, server.URL+"/users/"+id)
		if status != http.StatusBadRequest {
			t.Errorf("id=%s: status = %d, want 400", id, status)
		}
		if decode[map[string]string](t, body)["error"] == "" {
			t.Errorf("id=%s: body %s has no error message", id, body)
		}
	}
}

func TestRejectsUnsupportedMethod(t *testing.T) {
	server := newTestServer(t, 1)
	status, _ := fetch(t, http.MethodPost, server.URL+"/users")
	if status != http.StatusMethodNotAllowed {
		t.Errorf("POST /users = %d, want 405", status)
	}
}

func TestSeededServersRespondIdentically(t *testing.T) {
	first, second := newTestServer(t, 42), newTestServer(t, 42)
	for _, path := range []string{"/users?count=4", "/users/3"} {
		_, firstBody := fetch(t, http.MethodGet, first.URL+path)
		_, secondBody := fetch(t, http.MethodGet, second.URL+path)
		if !bytes.Equal(firstBody, secondBody) {
			t.Errorf("GET %s differs between seeded servers:\n%s\n%s", path, firstBody, secondBody)
		}
	}
}
