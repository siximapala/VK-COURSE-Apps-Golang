package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

// RoundTripperFunc allows creating custom transports in tests.
type RoundTripperFunc func(*http.Request) (*http.Response, error)

func (f RoundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestFindUsers_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("AccessToken") != "tok" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		// Return 3 users to let client detect NextPage (client increments requested limit)
		users := []User{{ID: 1, Name: "A"}, {ID: 2, Name: "B"}, {ID: 3, Name: "C"}}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(users)
	}))
	defer ts.Close()

	c := &SearchClient{AccessToken: "tok", URL: ts.URL}
	resp, err := c.FindUsers(SearchRequest{Limit: 2, Offset: 0})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.NextPage {
		t.Fatalf("expected NextPage=true")
	}
	if len(resp.Users) != 2 {
		t.Fatalf("expected 2 users, got %d", len(resp.Users))
	}
	if resp.Users[0].ID != 1 || resp.Users[1].ID != 2 {
		t.Fatalf("unexpected users: %+v", resp.Users)
	}
}

func TestFindUsers_Errors(t *testing.T) {
	t.Run("bad order field", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(SearchErrorResponse{Error: ErrorBadOrderField})
		}))
		defer ts.Close()

		c := &SearchClient{AccessToken: "x", URL: ts.URL}
		_, err := c.FindUsers(SearchRequest{Limit: 1, OrderField: "bad", OrderBy: 1})
		if err == nil || !strings.Contains(err.Error(), "OrderFeld") {
			t.Fatalf("expected OrderFeld error, got %v", err)
		}
	})

	t.Run("internal server error", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer ts.Close()

		c := &SearchClient{AccessToken: "x", URL: ts.URL}
		_, err := c.FindUsers(SearchRequest{Limit: 1})
		if err == nil || !strings.Contains(err.Error(), "SearchServer fatal error") {
			t.Fatalf("expected SearchServer fatal error, got %v", err)
		}
	})

	t.Run("unauthorized", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		}))
		defer ts.Close()

		c := &SearchClient{AccessToken: "x", URL: ts.URL}
		_, err := c.FindUsers(SearchRequest{Limit: 1})
		if err == nil || !strings.Contains(err.Error(), "bad AccessToken") {
			t.Fatalf("expected bad AccessToken error, got %v", err)
		}
	})

	t.Run("timeout", func(t *testing.T) {
		// server sleeps; reduce client timeout to trigger timeout
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// try to parse limit so test is realistic
			_, _ = strconv.Atoi(r.URL.Query().Get("limit"))
			time.Sleep(50 * time.Millisecond)
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode([]User{})
		}))
		defer ts.Close()

		oldClient := client
		client = &http.Client{Timeout: 5 * time.Millisecond}
		defer func() { client = oldClient }()

		c := &SearchClient{AccessToken: "x", URL: ts.URL}
		_, err := c.FindUsers(SearchRequest{Limit: 1})
		if err == nil || !strings.Contains(err.Error(), "timeout for") {
			t.Fatalf("expected timeout error, got %v", err)
		}
	})
}

func TestFindUsers_AdditionalCases(t *testing.T) {
	t.Run("network unknown error", func(t *testing.T) {
		// симулируем сетевую ошибку, не являющуюся net.Error
		oldClient := client
		client = &http.Client{Transport: RoundTripperFunc(func(r *http.Request) (*http.Response, error) {
			return nil, &struct{ error }{} // какая-то не net.Error ошибка
		})}
		defer func() { client = oldClient }()

		c := &SearchClient{AccessToken: "x", URL: "http://example.invalid"}
		_, err := c.FindUsers(SearchRequest{Limit: 1})
		if err == nil || !strings.Contains(err.Error(), "unknown error") {
			t.Fatalf("expected unknown network error, got %v", err)
		}
	})

	t.Run("bad error json", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			w.Header().Set("Content-Type", "application/json")
			// невалидный JSON
			w.Write([]byte("not-json"))
		}))
		defer ts.Close()

		c := &SearchClient{AccessToken: "x", URL: ts.URL}
		_, err := c.FindUsers(SearchRequest{Limit: 1})
		if err == nil || !strings.Contains(err.Error(), "cant unpack error json") {
			t.Fatalf("expected cant unpack error json, got %v", err)
		}
	})

	t.Run("unknown bad request error", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(SearchErrorResponse{Error: "something else"})
		}))
		defer ts.Close()

		c := &SearchClient{AccessToken: "x", URL: ts.URL}
		_, err := c.FindUsers(SearchRequest{Limit: 1})
		if err == nil || !strings.Contains(err.Error(), "unknown bad request error") {
			t.Fatalf("expected unknown bad request error, got %v", err)
		}
	})

	t.Run("no next page when fewer returned than requested", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// клиент запрашивает 2, сервер возвращает 2 - значит нет следующей страницы
			w.Header().Set("Content-Type", "application/json")
			users := []User{{ID: 1, Name: "A"}, {ID: 2, Name: "B"}}
			json.NewEncoder(w).Encode(users)
		}))
		defer ts.Close()

		c := &SearchClient{AccessToken: "tok", URL: ts.URL}
		resp, err := c.FindUsers(SearchRequest{Limit: 2, Offset: 0})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.NextPage {
			t.Fatalf("expected NextPage=false")
		}
		if len(resp.Users) != 2 {
			t.Fatalf("expected 2 users, got %d", len(resp.Users))
		}
	})
}
