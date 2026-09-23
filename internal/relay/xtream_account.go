package relay

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// XtreamAccount is the subset of an Xtream panel's user_info that says
// something useful about why a stream might be refused.
type XtreamAccount struct {
	Auth           int
	Status         string
	ActiveCons     int
	MaxConnections int
	Expires        time.Time
}

// flexInt reads a field that a panel may send as either a JSON number or a
// quoted string. Real responses mix the two in one object: auth arrives as 1
// while active_cons alongside it arrives as "0".
type flexInt int

func (f *flexInt) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	if s == "" || s == "null" {
		*f = 0
		return nil
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return fmt.Errorf("not an integer: %s", s)
	}
	*f = flexInt(n)
	return nil
}

type xtreamUserInfoEnvelope struct {
	UserInfo struct {
		Auth           flexInt `json:"auth"`
		Status         string  `json:"status"`
		ActiveCons     flexInt `json:"active_cons"`
		MaxConnections flexInt `json:"max_connections"`
		ExpDate        string  `json:"exp_date"`
	} `json:"user_info"`
}

// Expired reports whether the subscription's end date has passed.
func (a *XtreamAccount) Expired() bool {
	return !a.Expires.IsZero() && time.Now().After(a.Expires)
}

// Authorised reports whether the panel accepts the credentials.
func (a *XtreamAccount) Authorised() bool {
	return a.Auth == 1 && (a.Status == "" || strings.EqualFold(a.Status, "Active"))
}

// AtConnectionLimit reports whether every permitted connection is in use.
func (a *XtreamAccount) AtConnectionLimit() bool {
	return a.MaxConnections > 0 && a.ActiveCons >= a.MaxConnections
}

// Refine returns a StreamError describing what the account itself reports,
// falling back to the original when the account explains nothing.
//
// Nothing here guesses. The refusal codes an Xtream panel returns on the
// streaming endpoint carry no reliable meaning -- the same URL answers 555, then
// 999, and 666 elsewhere, all with empty bodies -- so the only trustworthy
// account state is the one the panel publishes through player_api.php. Asking it
// turns "the provider said 999" into "your subscription expired on the 9th".
func (a *XtreamAccount) Refine(se *StreamError) *StreamError {
	if a == nil || se == nil {
		return se
	}

	switch {
	case !a.Authorised():
		status := a.Status
		if status == "" {
			status = "credentials rejected"
		}
		return &StreamError{
			Kind:       StreamErrorUnavailable,
			Headline:   "Check Credentials",
			Detail:     fmt.Sprintf("Provider reports the account as %s.", status),
			HTTPStatus: se.HTTPStatus,
			Err:        se.Err,
		}

	case a.Expired():
		return &StreamError{
			Kind:       StreamErrorEnded,
			Headline:   "Subscription Expired",
			Detail:     fmt.Sprintf("This subscription ended on %s.", a.Expires.Format("2 January 2006")),
			HTTPStatus: se.HTTPStatus,
			Err:        se.Err,
		}

	case a.AtConnectionLimit():
		return &StreamError{
			Kind:     StreamErrorLimitReached,
			Headline: "All Connections In Use",
			Detail: fmt.Sprintf("This subscription allows %d stream(s) at once and %d are in use.",
				a.MaxConnections, a.ActiveCons),
			HTTPStatus: se.HTTPStatus,
			Err:        se.Err,
		}
	}

	// The account looks healthy, so the refusal is unexplained. Say only what was
	// observed rather than inventing a reason for it.
	return se
}

// ParseXtreamStreamURL pulls the panel base URL and credentials out of a stream
// URL of the form scheme://host/live/USER/PASS/ID.ts.
//
// Taken from the stream URL rather than the source configuration because that is
// what the request which actually failed used, so a stale credential baked into
// a channel during an earlier ingestion is checked as-is instead of being masked
// by newer settings.
func ParseXtreamStreamURL(streamURL string) (base, username, password string, ok bool) {
	u, err := url.Parse(streamURL)
	if err != nil || u.Host == "" {
		return "", "", "", false
	}

	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	// Either /live/user/pass/id.ts or the extensionless /user/pass/id.
	switch {
	case len(parts) >= 4:
		username, password = parts[len(parts)-3], parts[len(parts)-2]
	case len(parts) == 3:
		username, password = parts[0], parts[1]
	default:
		return "", "", "", false
	}

	if username == "" || password == "" {
		return "", "", "", false
	}

	return u.Scheme + "://" + u.Host, username, password, true
}

// accountCacheTTL bounds how often one panel is asked about an account.
//
// A failing source tends to fail for every channel at once, so without this a
// viewer flipping channels would fire a lookup per attempt at a provider that is
// already refusing us and may well be rate limiting.
const accountCacheTTL = 60 * time.Second

type cachedAccount struct {
	account *XtreamAccount
	err     error
	fetched time.Time
}

// AccountProber looks up Xtream account state, cached per credential pair.
type AccountProber struct {
	client *http.Client
	logger *slog.Logger

	mu    sync.Mutex
	cache map[string]cachedAccount
}

// NewAccountProber creates a prober.
func NewAccountProber(client *http.Client, logger *slog.Logger) *AccountProber {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &AccountProber{
		client: client,
		logger: logger,
		cache:  make(map[string]cachedAccount),
	}
}

// Account returns the panel's view of the account behind a stream URL.
func (p *AccountProber) Account(ctx context.Context, streamURL string) (*XtreamAccount, error) {
	base, username, password, ok := ParseXtreamStreamURL(streamURL)
	if !ok {
		return nil, fmt.Errorf("not an Xtream stream URL: %s", streamURL)
	}

	key := base + "\x00" + username + "\x00" + password

	p.mu.Lock()
	if c, hit := p.cache[key]; hit && time.Since(c.fetched) < accountCacheTTL {
		p.mu.Unlock()
		return c.account, c.err
	}
	p.mu.Unlock()

	account, err := p.fetch(ctx, base, username, password)

	p.mu.Lock()
	p.cache[key] = cachedAccount{account: account, err: err, fetched: time.Now()}
	p.mu.Unlock()

	return account, err
}

// fetch performs the player_api.php lookup.
func (p *AccountProber) fetch(ctx context.Context, base, username, password string) (*XtreamAccount, error) {
	endpoint := fmt.Sprintf("%s/player_api.php?username=%s&password=%s",
		base, url.QueryEscape(username), url.QueryEscape(password))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("building account request: %w", err)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("querying account: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("account lookup returned HTTP %d", resp.StatusCode)
	}

	var env xtreamUserInfoEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return nil, fmt.Errorf("decoding account: %w", err)
	}

	account := &XtreamAccount{
		Auth:           int(env.UserInfo.Auth),
		Status:         env.UserInfo.Status,
		ActiveCons:     int(env.UserInfo.ActiveCons),
		MaxConnections: int(env.UserInfo.MaxConnections),
	}
	if secs, err := strconv.ParseInt(env.UserInfo.ExpDate, 10, 64); err == nil && secs > 0 {
		account.Expires = time.Unix(secs, 0)
	}

	return account, nil
}
