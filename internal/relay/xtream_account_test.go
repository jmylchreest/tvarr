package relay

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestParseXtreamStreamURL(t *testing.T) {
	tests := []struct {
		name                     string
		url                      string
		base, username, password string
		ok                       bool
	}{
		{"live path", "http://host.example/live/USER/PASS/451.ts", "http://host.example", "USER", "PASS", true},
		{"extensionless", "http://host.example/USER/PASS/451", "http://host.example", "USER", "PASS", true},
		{"with port", "http://host.example:8080/live/U/P/1.ts", "http://host.example:8080", "U", "P", true},
		{"https", "https://host.example/live/U/P/1.m3u8", "https://host.example", "U", "P", true},
		{"too short", "http://host.example/1.ts", "", "", "", false},
		{"not a url", "://nonsense", "", "", "", false},
		{"empty credential", "http://host.example/live//PASS/1.ts", "", "", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			base, user, pass, ok := ParseXtreamStreamURL(tt.url)
			if ok != tt.ok {
				t.Fatalf("ok = %v, want %v", ok, tt.ok)
			}
			if !ok {
				return
			}
			if base != tt.base || user != tt.username || pass != tt.password {
				t.Errorf("got (%q,%q,%q), want (%q,%q,%q)", base, user, pass, tt.base, tt.username, tt.password)
			}
		})
	}
}

// TestXtreamAccountRefine covers the whole point of asking the panel: turning an
// opaque refusal into something the viewer can act on, and staying quiet when
// the account explains nothing.
func TestXtreamAccountRefine(t *testing.T) {
	refused := NewUpstreamStatusError(999)

	tests := []struct {
		name     string
		account  *XtreamAccount
		headline string
		kind     StreamErrorKind
	}{
		{
			name:     "credentials rejected",
			account:  &XtreamAccount{Auth: 0, Status: "Active", MaxConnections: 1},
			headline: "Check Credentials",
			kind:     StreamErrorUnavailable,
		},
		{
			name:     "account disabled",
			account:  &XtreamAccount{Auth: 1, Status: "Disabled", MaxConnections: 1},
			headline: "Check Credentials",
			kind:     StreamErrorUnavailable,
		},
		{
			name:     "subscription expired",
			account:  &XtreamAccount{Auth: 1, Status: "Active", MaxConnections: 1, Expires: time.Now().Add(-24 * time.Hour)},
			headline: "Subscription Expired",
			kind:     StreamErrorEnded,
		},
		{
			name:     "every connection in use",
			account:  &XtreamAccount{Auth: 1, Status: "Active", ActiveCons: 1, MaxConnections: 1, Expires: time.Now().Add(48 * time.Hour)},
			headline: "All Connections In Use",
			kind:     StreamErrorLimitReached,
		},
		{
			// Exactly the state observed live while every stream was refused.
			// The account explains nothing, so the slate must not invent a cause.
			name:     "healthy account, unexplained refusal",
			account:  &XtreamAccount{Auth: 1, Status: "Active", ActiveCons: 0, MaxConnections: 1, Expires: time.Now().Add(48 * time.Hour)},
			headline: refused.Headline,
			kind:     refused.Kind,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.account.Refine(refused)
			if got.Headline != tt.headline {
				t.Errorf("Headline = %q, want %q", got.Headline, tt.headline)
			}
			if got.Kind != tt.kind {
				t.Errorf("Kind = %q, want %q", got.Kind, tt.kind)
			}
			if got.HTTPStatus != refused.HTTPStatus {
				t.Errorf("lost the observed status: %d", got.HTTPStatus)
			}
		})
	}

	t.Run("nil account leaves the error alone", func(t *testing.T) {
		var a *XtreamAccount
		if got := a.Refine(refused); got != refused {
			t.Errorf("nil account altered the error: %+v", got)
		}
	})
}

// TestAccountProberCaches guards the provider from a lookup per failed channel.
// A source that refuses one stream usually refuses all of them, and the panel
// may well be rate limiting already.
func TestAccountProberCaches(t *testing.T) {
	var calls atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		// Mixed string/number typing, exactly as real panels reply.
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"user_info":{"auth":1,"status":"Active","active_cons":"0","max_connections":"1","exp_date":"1788980041"}}`))
	}))
	defer srv.Close()

	p := NewAccountProber(srv.Client(), nil)
	streamURL := srv.URL + "/live/USER/PASS/451.ts"

	for range 5 {
		account, err := p.Account(context.Background(), streamURL)
		if err != nil {
			t.Fatalf("Account() failed: %v", err)
		}
		if account.Auth != 1 || account.MaxConnections != 1 || account.ActiveCons != 0 {
			t.Fatalf("decoded wrongly: %+v", account)
		}
		if account.Expires.IsZero() {
			t.Error("exp_date was not decoded")
		}
	}

	if got := calls.Load(); got != 1 {
		t.Errorf("panel queried %d times for 5 failures; the cache is not working", got)
	}
}

func TestAccountProberRejectsNonXtreamURL(t *testing.T) {
	p := NewAccountProber(nil, nil)
	if _, err := p.Account(context.Background(), "http://example.com/playlist.m3u8"); err == nil {
		t.Error("expected an error for a URL with no embedded credentials")
	}
}
