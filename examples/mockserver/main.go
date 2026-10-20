// Command mockserver is a stub REST API whose responses are generated from
// JSON Schemas, so a client can be built before the real backend exists.
//
//	go run ./examples/mockserver -seed 42
//	curl 'localhost:3000/users?count=2'
//	curl localhost:3000/users/7
package main

import (
	_ "embed"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"strconv"

	"github.com/ReBase-Dev-Studio/goppelganger"
)

const (
	defaultUserCount = 3
	maxUserCount     = 50
)

//go:embed schemas/user.json
var userJSON []byte

// userSchema is compiled once at startup and shared by every request.
var userSchema = goppelganger.MustCompile(userJSON)

func main() {
	addr := flag.String("addr", ":3000", "listen address")
	var seed *int64
	flag.Func("seed", "int64 seed for reproducible responses (default: random)", func(raw string) error {
		parsed, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return err
		}
		seed = &parsed
		return nil
	})
	flag.Parse()

	faker := goppelganger.New(goppelganger.Options{Seed: seed})
	log.Printf("mockserver listening on %s", *addr)
	log.Fatal(http.ListenAndServe(*addr, newHandler(faker)))
}

// newHandler routes requests to generators backed by faker, which is safe
// for concurrent use by every request.
func newHandler(faker *goppelganger.Faker) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /users", listUsers(faker))
	mux.HandleFunc("GET /users/{id}", getUser(faker))
	return mux
}

// listUsers serves ?count= users (default 3, at most 50).
func listUsers(faker *goppelganger.Faker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		count, err := parseCount(r.URL.Query().Get("count"))
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		users := make([]map[string]any, 0, count)
		for range count {
			user, err := faker.GenerateSchema(userSchema)
			if err != nil {
				writeError(w, http.StatusInternalServerError, err)
				return
			}
			users = append(users, user)
		}
		writeJSON(w, http.StatusOK, users)
	}
}

// getUser serves one user whose id matches the path.
func getUser(faker *goppelganger.Faker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil || id < 1 {
			writeError(w, http.StatusBadRequest, fmt.Errorf("id must be a positive integer, got %q", r.PathValue("id")))
			return
		}
		user, err := faker.GenerateSchema(userSchema)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		user["id"] = id
		writeJSON(w, http.StatusOK, user)
	}
}

func parseCount(raw string) (int, error) {
	if raw == "" {
		return defaultUserCount, nil
	}
	count, err := strconv.Atoi(raw)
	if err != nil || count < 1 || count > maxUserCount {
		return 0, fmt.Errorf("count must be an integer in 1..%d, got %q", maxUserCount, raw)
	}
	return count, nil
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		log.Printf("write response: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}
