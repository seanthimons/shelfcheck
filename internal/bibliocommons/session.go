package bibliocommons

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"
)

// A Session is a logged-in patron session, created by Login and saved
// between runs. Its cookies act as the patron's password until the session
// expires, so store it privately.
type Session struct {
	Library    string            `json:"library"`
	UserID     string            `json:"user_id"`
	AccountID  string            `json:"account_id"`
	HomeBranch string            `json:"home_branch,omitempty"`
	Cookies    map[string]string `json:"cookies"`
	Created    time.Time         `json:"created"`
}

var (
	// ErrLoginFailed means the library rejected the card number or PIN, or
	// the login page changed shape. Don't retry automatically: repeated
	// failures can lock the card.
	ErrLoginFailed = errors.New("bibliocommons: login failed")
	// ErrSessionExpired means the saved session no longer works.
	ErrSessionExpired = errors.New("bibliocommons: session expired; log in again")
)

// SiteURL returns the library's catalog website, where logins happen.
func (c *Client) SiteURL() string {
	if c.Site != "" {
		return c.Site
	}
	return fmt.Sprintf("https://%s.bibliocommons.com", c.Library)
}

// RecordURL links to the catalog's own interface for a specific edition.
// Opening it lets the patron sign in and place a hold in their browser.
func (c *Client) RecordURL(bibID string) string {
	if bibID == "" {
		return ""
	}
	return strings.TrimRight(c.SiteURL(), "/") + "/v2/record/" + url.PathEscape(bibID)
}

var (
	loginFormRe = regexp.MustCompile(`(?s)<form[^>]*data-js="login_form"[^>]*>(.*?)</form>`)
	inputRe     = regexp.MustCompile(`<input[^>]*>`)
	attrRe      = regexp.MustCompile(`([a-zA-Z_-]+)="([^"]*)"`)
	alertRe     = regexp.MustCompile(`(?s)<div[^>]*class="[^"]*(?:alert|error|message)[^"]*"[^>]*>(.*?)</div>`)
)

// Login signs in with a library card number and PIN through the catalog's
// own login form, then looks up the patron's user and account IDs. It makes
// exactly one login attempt.
func (c *Client) Login(ctx context.Context, card, pin string) (*Session, error) {
	card, pin = strings.TrimSpace(card), strings.TrimSpace(pin)
	if card == "" || pin == "" {
		return nil, errors.New("bibliocommons: card number and PIN are required")
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	hc := &http.Client{Jar: jar, Timeout: 30 * time.Second}
	site := c.SiteURL()
	loginURL := site + "/user/login?destination=%2F"

	// 1. The login page supplies the form's one-time token and first cookies.
	page, err := c.sitePage(ctx, hc, http.MethodGet, loginURL, nil)
	if err != nil {
		return nil, err
	}
	m := loginFormRe.FindStringSubmatch(page)
	if m == nil {
		return nil, fmt.Errorf("%w: login form not found on %s", ErrLoginFailed, loginURL)
	}
	fields := url.Values{}
	for _, in := range inputRe.FindAllString(m[1], -1) {
		attrs := map[string]string{}
		for _, a := range attrRe.FindAllStringSubmatch(in, -1) {
			attrs[a[1]] = html.UnescapeString(a[2])
		}
		if attrs["type"] == "hidden" && attrs["name"] != "" {
			fields.Set(attrs["name"], attrs["value"])
		}
	}
	if fields.Get("authenticity_token") == "" {
		return nil, fmt.Errorf("%w: no authenticity token in login form", ErrLoginFailed)
	}
	fields.Set("name", card)
	fields.Set("user_pin", pin)
	fields.Set("remember_me", "true")

	// 2. Submit it. Success sets the session cookies.
	result, err := c.sitePage(ctx, hc, http.MethodPost, loginURL, fields)
	if err != nil {
		return nil, err
	}
	siteU, _ := url.Parse(site)
	s := &Session{Library: c.Library, Cookies: map[string]string{}, Created: time.Now()}
	for _, ck := range jar.Cookies(siteU) {
		s.Cookies[ck.Name] = ck.Value
	}
	if s.Cookies["bc_access_token"] == "" || s.Cookies["session_id"] == "" {
		msg := "the library didn't start a session"
		if a := alertRe.FindStringSubmatch(result); a != nil {
			if t := plainText(a[1]); t != "" {
				msg = t
			}
		}
		if len(msg) > 200 {
			msg = msg[:200]
		}
		return nil, fmt.Errorf("%w: %s", ErrLoginFailed, msg)
	}

	// 3. The gateway says who that session belongs to.
	if err := c.loadIdentity(ctx, s); err != nil {
		return nil, err
	}
	return s, nil
}

// sitePage requests a catalog web page with the login flow's cookie jar.
func (c *Client) sitePage(ctx context.Context, hc *http.Client, method, u string, form url.Values) (string, error) {
	if err := c.wait(ctx); err != nil {
		return "", err
	}
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req, err := http.NewRequestWithContext(ctx, method, u, body)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", c.UserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Origin", c.SiteURL())
		req.Header.Set("Referer", u)
	}
	resp, err := hc.Do(req)
	if err != nil {
		return "", fmt.Errorf("bibliocommons: %w", err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return "", err
	}
	if resp.StatusCode/100 != 2 {
		return "", fmt.Errorf("%w: %s %s returned %d", ErrLoginFailed, method, req.URL.Path, resp.StatusCode)
	}
	return string(b), nil
}

// loadIdentity fills in the session's user, account and home branch.
func (c *Client) loadIdentity(ctx context.Context, s *Session) error {
	var raw struct {
		Auth struct {
			CurrentUserID json.Number `json:"currentUserId"`
		} `json:"auth"`
		Entities struct {
			Users map[string]struct {
				Accounts []json.Number `json:"accounts"`
			} `json:"users"`
			Accounts map[string]struct {
				PreferredLocations []string `json:"preferredLocations"`
			} `json:"accounts"`
		} `json:"entities"`
	}
	if err := c.getAuthJSON(ctx, s, "sessions/current", nil, &raw); err != nil {
		return err
	}
	uid := raw.Auth.CurrentUserID.String()
	user, ok := raw.Entities.Users[uid]
	if uid == "" || !ok || len(user.Accounts) == 0 {
		return ErrSessionExpired
	}
	s.UserID = uid
	s.AccountID = user.Accounts[0].String()
	if locs := raw.Entities.Accounts[s.AccountID].PreferredLocations; len(locs) > 0 {
		s.HomeBranch = locs[0]
	}
	return nil
}

// Check confirms the session still works.
func (c *Client) Check(ctx context.Context, s *Session) error {
	want := s.UserID
	if err := c.loadIdentity(ctx, s); err != nil {
		return err
	}
	if want != "" && s.UserID != want {
		return ErrSessionExpired
	}
	return nil
}

// getAuthJSON GETs a gateway path as the session's patron. Responses are
// never cached.
func (c *Client) getAuthJSON(ctx context.Context, s *Session, path string, params url.Values, out any) error {
	var cookies []string
	for k, v := range s.Cookies {
		cookies = append(cookies, k+"="+v)
	}
	h := http.Header{}
	h.Set("Cookie", strings.Join(cookies, "; "))
	h.Set("X-Access-Token", s.Cookies["bc_access_token"])
	h.Set("X-Session-Id", s.Cookies["session_id"])
	h.Set("Origin", c.SiteURL())
	h.Set("Referer", c.SiteURL()+"/")
	u := c.gatewayURL(path, params)
	body, err := c.fetch(ctx, u, h)
	var apiErr *APIError
	if errors.As(err, &apiErr) && (apiErr.Status == http.StatusUnauthorized || apiErr.Status == http.StatusForbidden) {
		return ErrSessionExpired
	}
	if err != nil {
		return err
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("bibliocommons: decoding %s: %w", path, err)
	}
	return nil
}

// --- patron data ------------------------------------------------------------

// HistoryItem is one past checkout from borrowing history.
type HistoryItem struct {
	Bib        Bib
	CheckedOut string // YYYY-MM-DD
	Returned   string // YYYY-MM-DD; often empty
}

// Checkout is a current loan, physical or digital (Libby/OverDrive).
type Checkout struct {
	Bib     Bib
	Due     string // YYYY-MM-DD
	Status  string // "OUT", "OVERDUE", ...
	Digital bool
	Renewed int
}

// Hold is a current hold request, physical or digital.
type Hold struct {
	Bib      Bib
	Status   string // "NOT_YET_AVAILABLE", "READY_FOR_PICKUP", "SUSPENDED", ...
	Position int
	WaitDays int    `json:",omitempty"` // Libby estimated wait; zero if unknown
	Placed   string // YYYY-MM-DD
	Digital  bool
}

type rawBorrowing struct {
	Borrowing map[string]struct {
		Results    []string      `json:"results"`
		Items      []string      `json:"items"`
		Pagination rawPagination `json:"pagination"`
	} `json:"borrowing"`
	Entities struct {
		Bibs             map[string]rawBib `json:"bibs"`
		BorrowingHistory map[string]struct {
			MetadataID     string `json:"metadataId"`
			Title          string `json:"title"`
			Author         string `json:"author"`
			Format         string `json:"format"`
			CheckedoutDate string `json:"checkedoutDate"`
			ReturnedDate   string `json:"returnedDate"`
		} `json:"borrowingHistory"`
		Checkouts map[string]struct {
			MetadataID   string `json:"metadataId"`
			BibTitle     string `json:"bibTitle"`
			DueDate      string `json:"dueDate"`
			Status       string `json:"status"`
			MaterialType string `json:"materialType"`
			TimesRenewed int    `json:"timesRenewed"`
		} `json:"checkouts"`
		Holds map[string]struct {
			MetadataID     string `json:"metadataId"`
			BibTitle       string `json:"bibTitle"`
			Status         string `json:"status"`
			HoldsPosition  int    `json:"holdsPosition"`
			HoldPlacedDate string `json:"holdPlacedDate"`
			MaterialType   string `json:"materialType"`
		} `json:"holds"`
	} `json:"entities"`
}

// bibOr returns the full record for id, or a stub with the given title.
func (r rawBorrowing) bibOr(id, title string) Bib {
	if b, ok := r.Entities.Bibs[id]; ok {
		return b.toBib()
	}
	return Bib{ID: id, Title: title}
}

// borrowing fetches every page of one of the patron's borrowing lists.
func (c *Client) borrowing(ctx context.Context, s *Session, path, section string, each func(rawBorrowing, []string)) error {
	for page := 1; page <= 50; page++ {
		params := url.Values{"accountId": {s.AccountID}}
		if page > 1 {
			params.Set("page", fmt.Sprint(page))
		}
		var raw rawBorrowing
		if err := c.getAuthJSON(ctx, s, path, params, &raw); err != nil {
			return err
		}
		sec := raw.Borrowing[section]
		ids := sec.Results
		if ids == nil {
			ids = sec.Items
		}
		each(raw, ids)
		if page >= sec.Pagination.Pages {
			return nil
		}
	}
	return nil
}

// History returns the patron's borrowing history, newest first.
func (c *Client) History(ctx context.Context, s *Session) ([]HistoryItem, error) {
	var out []HistoryItem
	err := c.borrowing(ctx, s, "borrowinghistory", "borrowingHistory", func(raw rawBorrowing, ids []string) {
		for _, id := range ids {
			h, ok := raw.Entities.BorrowingHistory[id]
			if !ok {
				continue
			}
			b := raw.bibOr(h.MetadataID, h.Title)
			if len(b.Authors) == 0 && h.Author != "" {
				b.Authors = []string{h.Author}
			}
			if b.Format == "" {
				b.Format = h.Format
			}
			out = append(out, HistoryItem{Bib: b, CheckedOut: h.CheckedoutDate, Returned: h.ReturnedDate})
		}
	})
	return out, err
}

// Checkouts returns current loans.
func (c *Client) Checkouts(ctx context.Context, s *Session) ([]Checkout, error) {
	var out []Checkout
	err := c.borrowing(ctx, s, "checkouts", "checkouts", func(raw rawBorrowing, ids []string) {
		for _, id := range ids {
			ck, ok := raw.Entities.Checkouts[id]
			if !ok {
				continue
			}
			out = append(out, Checkout{
				Bib: raw.bibOr(ck.MetadataID, ck.BibTitle), Due: ck.DueDate, Status: ck.Status,
				Digital: ck.MaterialType == "DIGITAL", Renewed: ck.TimesRenewed,
			})
		}
	})
	return out, err
}

// Holds returns current holds.
func (c *Client) Holds(ctx context.Context, s *Session) ([]Hold, error) {
	var out []Hold
	err := c.borrowing(ctx, s, "holds", "holds", func(raw rawBorrowing, ids []string) {
		for _, id := range ids {
			h, ok := raw.Entities.Holds[id]
			if !ok {
				continue
			}
			out = append(out, Hold{
				Bib: raw.bibOr(h.MetadataID, h.BibTitle), Status: h.Status, Position: h.HoldsPosition,
				Placed: h.HoldPlacedDate, Digital: h.MaterialType == "DIGITAL",
			})
		}
	})
	return out, err
}

// --- persistence --------------------------------------------------------------

// SaveSession writes s to path, readable only by the current user.
func SaveSession(path string, s *Session) error {
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// LoadSession reads a session saved by SaveSession.
func LoadSession(path string) (*Session, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var s Session
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, fmt.Errorf("reading session %s: %w", path, err)
	}
	return &s, nil
}
