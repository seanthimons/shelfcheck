package hardcover

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Book is a Hardcover book, with enough to find it in a library catalog.
type Book struct {
	ID        int
	Slug      string
	Title     string
	Author    string
	Readers   int     // Hardcover users who have it
	Series    string  // featured series, if any
	SeriesPos float64 // its position there; 0 if none
}

// URL is the book's page on Hardcover.
func (b Book) URL() string {
	if b.Slug == "" {
		return ""
	}
	return "https://hardcover.app/books/" + b.Slug
}

// bookFields selects what Book needs.
const bookFields = `id slug title users_count compilation
  contributions(limit: 1) { author { name } }
  featured_book_series { position series { name } }`

type rawBook struct {
	ID            int    `json:"id"`
	Slug          string `json:"slug"`
	Title         string `json:"title"`
	UsersCount    int    `json:"users_count"`
	Compilation   bool   `json:"compilation"`
	Contributions []struct {
		Author struct {
			Name string `json:"name"`
		} `json:"author"`
	} `json:"contributions"`
	FeaturedBookSeries *struct {
		Position *float64 `json:"position"`
		Series   struct {
			Name string `json:"name"`
		} `json:"series"`
	} `json:"featured_book_series"`
}

func (r rawBook) book() Book {
	b := Book{ID: r.ID, Slug: r.Slug, Title: r.Title, Readers: r.UsersCount}
	if len(r.Contributions) > 0 {
		b.Author = r.Contributions[0].Author.Name
	}
	if s := r.FeaturedBookSeries; s != nil {
		b.Series = s.Series.Name
		if s.Position != nil {
			b.SeriesPos = *s.Position
		}
	}
	return b
}

const bookSearchQuery = `query($q: String!) {
  search(query: $q, query_type: "Book", per_page: 10, page: 1) { results }
}`

// FindBook looks a book up by title and author, preferring the most-read
// match by that author (imports and stray editions leave near-empty
// duplicates). ok is false if nothing matched.
func (c *Client) FindBook(ctx context.Context, title, author string) (Book, bool, error) {
	var found struct {
		Search struct {
			Results struct {
				Hits []struct {
					Document struct {
						ID          json.Number `json:"id"`
						Title       string      `json:"title"`
						AuthorNames []string    `json:"author_names"`
						UsersCount  int         `json:"users_count"`
					} `json:"document"`
				} `json:"hits"`
			} `json:"results"`
		} `json:"search"`
	}
	q := strings.TrimSpace(shortTitle(title) + " " + displayName(author))
	if err := c.Query(ctx, bookSearchQuery, map[string]any{"q": q}, &found); err != nil {
		return Book{}, false, err
	}
	authorLast := lastName(author)
	want := normName(shortTitle(title))
	var best Book
	for _, h := range found.Search.Results.Hits {
		d := h.Document
		if authorLast != "" && !anyContains(d.AuthorNames, authorLast) {
			continue
		}
		if normName(shortTitle(d.Title)) != want {
			continue
		}
		if d.UsersCount > best.Readers || best.ID == 0 {
			id, err := strconv.Atoi(d.ID.String())
			if err != nil {
				continue
			}
			best = Book{ID: id, Title: d.Title, Readers: d.UsersCount}
			if len(d.AuthorNames) > 0 {
				best.Author = d.AuthorNames[0]
			}
		}
	}
	return best, best.ID != 0, nil
}

// Prompt is a Hardcover prompt ("What is your favorite dark academia
// book?"), which readers answer with books.
type Prompt struct {
	ID       int
	Slug     string
	Question string
	Users    int // readers who answered
	Books    int // distinct books given
	Votes    int // answers given in all
	Answers  []PromptAnswer
}

func (p Prompt) URL() string { return "https://hardcover.app/prompts/" + p.Slug }

// untopicalRe spots prompts about a moment rather than a kind of book
// ("What are you most looking forward to in 2026?", "favorite childhood
// books"): answers there say little about taste.
var untopicalRe = regexp.MustCompile(`(?i)looking forward|anticipat|new release|upcoming|\b20\d\d\b|this (year|month|summer|winter|fall|spring)|currently|right now|\btbr\b|to be read|childhood|as a (kid|child)|growing up|read next|\bgoal\b|challenge|adapt(ed|ation)|\bfilm|movie|\btv\b|audiobook|narrat|\bcovers?\b`)

// Topical reports whether the prompt asks about a kind of book.
func (p Prompt) Topical() bool { return !untopicalRe.MatchString(p.Question) }

// personalListRe spots reading-queue lists ("On Deck", "Owned", "Kindle
// TBR", "2026 releases"), which pair books by chance rather than by kind.
var personalListRe = regexp.MustCompile(`(?i)on deck|owned|wish ?list|queue|\bpile\b|haul|kindle|audible|library|unread|\btbr\b|to be read|currently|releases|\bread(ing)? (in|list|next)\b|\bdnf\b|\b(my )?(favou?rites|reads) (of|in) `)

// PromptAnswer is a book given as an answer, and how many readers gave it.
type PromptAnswer struct {
	Book
	Votes int
}

// List is a Hardcover list.
type List struct {
	ID     int
	Name   string
	Slug   string
	Owner  string
	Books  int
	Likes  int
	Ranked bool
	Items  []Book
}

func (l List) URL() string { return "https://hardcover.app/@" + l.Owner + "/lists/" + l.Slug }

// CoPick is a book chosen alongside a seed book.
type CoPick struct {
	Book
	Weight float64 // summed over the readers and lists behind it
	// Prompt and List name the strongest source: the prompt where readers
	// who picked the seed also picked this, or a list holding both.
	Prompt *Prompt
	List   *List
}

// AlsoPicked says what readers pair with a seed book: in each prompt the
// seed answers, the books its readers give there more often than the
// prompt's readers at large (see coPicked); and the other books on
// well-liked, focused lists that include it. Prompts about a moment ("most
// anticipated in 2026") and reading-queue lists are skipped. It makes one
// request.
func (c *Client) AlsoPicked(ctx context.Context, seedID int) ([]CoPick, error) {
	cache := ""
	if c.CacheDir != "" {
		cache = filepath.Join(c.CacheDir, fmt.Sprintf("also-picked-%d.json", seedID))
		if fi, err := os.Stat(cache); err == nil && time.Since(fi.ModTime()) < alsoPickedTTL {
			var out []CoPick
			if b, err := os.ReadFile(cache); err == nil && json.Unmarshal(b, &out) == nil {
				return out, nil
			}
		}
	}
	out, err := c.alsoPicked(ctx, seedID)
	if err == nil && cache != "" {
		if b, err := json.Marshal(out); err == nil && os.MkdirAll(c.CacheDir, 0o700) == nil {
			os.WriteFile(cache, b, 0o600)
		}
	}
	return out, err
}

const alsoPickedTTL = 7 * 24 * time.Hour

func (c *Client) alsoPicked(ctx context.Context, seedID int) ([]CoPick, error) {
	// One top-level field: Hardcover counts each against the rate limit.
	// Readers "who picked the seed" are those who gave it in any prompt;
	// answers are ordered by reader so the 100-row cap cuts whole readers.
	const q = `query($b: Int!) {
  books_by_pk(id: $b) {
    prompt_summaries(order_by: {answers_count: desc}, limit: 10) {
      prompt { id slug question users_count books_count answers_count
        prompt_answers(where: {book_id: {_neq: $b}, user: {prompt_answers: {book_id: {_eq: $b}}}},
                       order_by: {user_id: asc}, limit: 100) { user_id book { ` + bookFields + ` } }
        prompt_books(order_by: {answers_count: desc}, limit: 100) { book_id answers_count }
      }
    }
    list_books(where: {list: {books_count: {_gte: 4, _lte: 250}, likes_count: {_gte: 5}}},
               order_by: {list: {likes_count: desc}}, limit: 6) {
      list { id name slug books_count likes_count ranked user { username }
        list_books(order_by: {position: asc}, limit: 80) { book { ` + bookFields + ` } } }
    }
  }
}`
	var raw struct {
		Book *struct {
			Prompts []struct {
				Prompt *struct {
					ID       int    `json:"id"`
					Slug     string `json:"slug"`
					Question string `json:"question"`
					Users    int    `json:"users_count"`
					Books    int    `json:"books_count"`
					Votes    int    `json:"answers_count"`
					Answers  []struct {
						UserID int     `json:"user_id"`
						Book   rawBook `json:"book"`
					} `json:"prompt_answers"`
					Top []struct {
						BookID int `json:"book_id"`
						Votes  int `json:"answers_count"`
					} `json:"prompt_books"`
				} `json:"prompt"`
			} `json:"prompt_summaries"`
			Lists []struct {
				List struct {
					ID     int    `json:"id"`
					Name   string `json:"name"`
					Slug   string `json:"slug"`
					Books  int    `json:"books_count"`
					Likes  int    `json:"likes_count"`
					Ranked bool   `json:"ranked"`
					User   struct {
						Username string `json:"username"`
					} `json:"user"`
					ListBooks []struct {
						Book rawBook `json:"book"`
					} `json:"list_books"`
				} `json:"list"`
			} `json:"list_books"`
		} `json:"books_by_pk"`
	}
	if err := c.Query(ctx, q, map[string]any{"b": seedID}, &raw); err != nil {
		return nil, err
	}
	if raw.Book == nil {
		return nil, nil
	}

	picks := map[int]*CoPick{}
	best := map[int]float64{} // strongest single contribution per book
	credit := func(b Book, w float64, p *Prompt, l *List) {
		if b.ID == seedID || b.ID == 0 {
			return
		}
		cp := picks[b.ID]
		if cp == nil {
			cp = &CoPick{Book: b}
			picks[b.ID] = cp
		}
		cp.Weight += w
		if w > best[b.ID] {
			best[b.ID] = w
			cp.Prompt, cp.List = p, l
		}
	}

	for _, ps := range raw.Book.Prompts {
		rp := ps.Prompt
		if rp == nil || rp.Users == 0 {
			continue
		}
		p := &Prompt{ID: rp.ID, Slug: rp.Slug, Question: strings.TrimSpace(rp.Question), Users: rp.Users, Books: rp.Books, Votes: rp.Votes}
		if !p.Topical() {
			continue
		}
		answers := make([]answer, 0, len(rp.Answers))
		for _, a := range rp.Answers {
			if !a.Book.Compilation { // omnibus editions aren't picks
				answers = append(answers, answer{a.UserID, a.Book.book()})
			}
		}
		votes := map[int]int{}
		for _, t := range rp.Top {
			votes[t.BookID] = t.Votes
		}
		for _, cp := range coPicked(p, answers, votes) {
			credit(cp.Book, cp.Weight, p, nil)
		}
	}

	// Lists: focused ones (few books) say more than catch-alls.
	for _, lb := range raw.Book.Lists {
		rl := lb.List
		l := &List{ID: rl.ID, Name: rl.Name, Slug: rl.Slug, Owner: rl.User.Username, Books: rl.Books, Likes: rl.Likes, Ranked: rl.Ranked}
		if personalListRe.MatchString(rl.Name) {
			continue
		}
		w := 1.5 / float64(max(rl.Books, 10))
		for _, it := range rl.ListBooks {
			if !it.Book.Compilation {
				credit(it.Book.book(), w, nil, l)
			}
		}
	}

	out := make([]CoPick, 0, len(picks))
	for _, cp := range picks {
		out = append(out, *cp)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Weight != out[j].Weight {
			return out[i].Weight > out[j].Weight
		}
		return out[i].Readers > out[j].Readers
	})
	return out, nil
}

type answer struct {
	user int
	book Book
}

// coPicked scores the books a seed's readers gave in prompt p (answers, one
// row per reader and book) by lift over the prompt's baseline (votes: the
// most-given books and their counts). A reader's say is capped: one
// naming 30 all-time favorites counts like one naming five, each at a
// sixth. Under chance, a reader giving n books picks book c with
// probability about n·share(c)/mean answers per reader; what a book gets
// above that is its weight. Broad prompts ("favorite books of all time")
// count less than focused ones.
func coPicked(p *Prompt, answers []answer, votes map[int]int) []CoPick {
	n := map[int]int{}
	for _, a := range answers {
		n[a.user]++
	}
	say := func(u int) float64 { return math.Min(1, 5/float64(n[u])) }
	// Books outside the top 100 have at most the 100th's votes.
	floor := 0
	if len(votes) >= 100 {
		floor = math.MaxInt
		for _, v := range votes {
			floor = min(floor, v)
		}
	}
	reach := 0.0 // Σ_u say(u)·n_u
	for u, k := range n {
		reach += say(u) * float64(k)
	}
	meanAnswers := math.Max(float64(p.Votes)/float64(p.Users), 1)
	focus := math.Sqrt(300 / math.Max(float64(p.Books), 300))

	obs := map[int]float64{}
	books := map[int]Book{}
	for _, a := range answers {
		obs[a.book.ID] += say(a.user)
		books[a.book.ID] = a.book
	}
	var out []CoPick
	for id, o := range obs {
		v, ok := votes[id]
		if !ok {
			v = floor
		}
		share := float64(v) / float64(p.Users)
		expected := reach * share / meanAnswers
		if lift := o - expected; lift > 0 {
			out = append(out, CoPick{Book: books[id], Weight: focus * lift})
		}
	}
	return out
}

// PromptBySlug fetches a prompt and its most-given answers.
func (c *Client) PromptBySlug(ctx context.Context, slug string, limit int) (Prompt, bool, error) {
	var raw struct {
		Prompts []struct {
			ID          int    `json:"id"`
			Slug        string `json:"slug"`
			Question    string `json:"question"`
			Users       int    `json:"users_count"`
			Books       int    `json:"books_count"`
			PromptBooks []struct {
				AnswersCount int     `json:"answers_count"`
				Book         rawBook `json:"book"`
			} `json:"prompt_books"`
		} `json:"prompts"`
	}
	q := `query($s: String!, $n: Int!) {
  prompts(where: {slug: {_eq: $s}}, limit: 1) {
    id slug question users_count books_count
    prompt_books(order_by: {answers_count: desc}, limit: $n) { answers_count book { ` + bookFields + ` } }
  }
}`
	if err := c.Query(ctx, q, map[string]any{"s": slug, "n": limit}, &raw); err != nil {
		return Prompt{}, false, err
	}
	if len(raw.Prompts) == 0 {
		return Prompt{}, false, nil
	}
	rp := raw.Prompts[0]
	p := Prompt{ID: rp.ID, Slug: rp.Slug, Question: strings.TrimSpace(rp.Question), Users: rp.Users, Books: rp.Books}
	for _, pb := range rp.PromptBooks {
		p.Answers = append(p.Answers, PromptAnswer{Book: pb.Book.book(), Votes: pb.AnswersCount})
	}
	return p, true, nil
}

// ListBySlug fetches a user's list and its books in list order.
func (c *Client) ListBySlug(ctx context.Context, owner, slug string, limit int) (List, bool, error) {
	var raw struct {
		Lists []struct {
			ID        int    `json:"id"`
			Name      string `json:"name"`
			Slug      string `json:"slug"`
			Books     int    `json:"books_count"`
			Likes     int    `json:"likes_count"`
			Ranked    bool   `json:"ranked"`
			ListBooks []struct {
				Book rawBook `json:"book"`
			} `json:"list_books"`
		} `json:"lists"`
	}
	q := `query($s: String!, $u: citext!, $n: Int!) {
  lists(where: {slug: {_eq: $s}, user: {username: {_eq: $u}}}, limit: 1) {
    id name slug books_count likes_count ranked
    list_books(order_by: {position: asc}, limit: $n) { book { ` + bookFields + ` } }
  }
}`
	if err := c.Query(ctx, q, map[string]any{"s": slug, "u": owner, "n": limit}, &raw); err != nil {
		return List{}, false, err
	}
	if len(raw.Lists) == 0 {
		return List{}, false, nil
	}
	rl := raw.Lists[0]
	l := List{ID: rl.ID, Name: rl.Name, Slug: rl.Slug, Owner: owner, Books: rl.Books, Likes: rl.Likes, Ranked: rl.Ranked}
	for _, lb := range rl.ListBooks {
		l.Items = append(l.Items, lb.Book.book())
	}
	return l, true, nil
}

// linkRe finds a Hardcover link in text.
var linkRe = regexp.MustCompile(`(?i)(?:https?://)?(?:www\.)?hardcover\.app/[^\s"'<>“”‘’]+`)

// Ref is a prompt, list or vibe named by a Hardcover URL.
type Ref struct {
	Kind  string // "prompt", "list" or "vibe"
	Owner string // list or vibe owner's username
	Slug  string
}

// ParseRef reads a Hardcover prompt, list or vibe URL
// (hardcover.app/prompts/<slug>, hardcover.app/@<user>/lists/<slug>,
// hardcover.app/@<user>/vibes/<slug>), or a
// bare prompt slug, ignoring quotes or angle brackets around it.
func ParseRef(s string) (Ref, bool) {
	if u := linkRe.FindString(s); u != "" {
		s = u // the link, wherever it sits in pasted text
	}
	s = strings.Trim(strings.TrimSpace(s), "\"'`<>“”‘’") // pasted with quotes or brackets
	s = strings.TrimSpace(s)
	if !strings.Contains(s, "/") {
		if !strings.Contains(s, "-") {
			return Ref{}, false // slugs are hyphenated questions
		}
		if s == "" || strings.ContainsAny(s, " \t") {
			return Ref{}, false
		}
		return Ref{Kind: "prompt", Slug: s}, true
	}
	if !strings.Contains(s, "://") {
		s = "https://" + s
	}
	u, err := url.Parse(s)
	if err != nil || !strings.HasSuffix(strings.TrimPrefix(u.Hostname(), "www."), "hardcover.app") {
		return Ref{}, false
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	switch {
	case len(parts) >= 2 && parts[0] == "prompts":
		return Ref{Kind: "prompt", Slug: parts[1]}, true
	case len(parts) >= 3 && strings.HasPrefix(parts[0], "@") && parts[1] == "lists":
		return Ref{Kind: "list", Owner: strings.TrimPrefix(parts[0], "@"), Slug: parts[2]}, true
	case len(parts) >= 3 && strings.HasPrefix(parts[0], "@") && parts[1] == "vibes":
		return Ref{Kind: "vibe", Owner: strings.TrimPrefix(parts[0], "@"), Slug: parts[2]}, true
	}
	return Ref{}, false
}

// shortTitle drops a subtitle ("Upgrade: A Novel" -> "Upgrade").
func shortTitle(t string) string {
	if i := strings.IndexAny(t, ":("); i > 3 {
		t = t[:i]
	}
	return strings.TrimSpace(t)
}

// displayName turns "Wells, Martha" into "Martha Wells".
func displayName(a string) string {
	last, first, ok := strings.Cut(a, ",")
	if !ok {
		return strings.TrimSpace(a)
	}
	return strings.TrimSpace(first) + " " + strings.TrimSpace(last)
}

func anyContains(names []string, sub string) bool {
	for _, n := range names {
		if strings.Contains(strings.ToLower(n), sub) {
			return true
		}
	}
	return false
}

// Collection is a prompt's answers (most given first), a list's books (in
// list order) or a vibe's picks (best first), whichever a Ref names.
type Collection struct {
	Kind    string // "prompt", "list" or "vibe"
	Title   string // the question, or the list's or vibe's name
	URL     string
	Readers int            // readers who answered a prompt
	Books   []PromptAnswer // Votes is 0 for lists
}

// Collection fetches the prompt or list ref names, up to limit books.
func (c *Client) Collection(ctx context.Context, ref Ref, limit int) (Collection, error) {
	switch ref.Kind {
	case "prompt":
		p, ok, err := c.PromptBySlug(ctx, ref.Slug, limit)
		if err != nil {
			return Collection{}, err
		}
		if !ok {
			return Collection{}, fmt.Errorf("hardcover: no prompt %q", ref.Slug)
		}
		return Collection{Kind: "prompt", Title: p.Question, URL: p.URL(), Readers: p.Users, Books: p.Answers}, nil
	case "list":
		l, ok, err := c.ListBySlug(ctx, ref.Owner, ref.Slug, limit)
		if err != nil {
			return Collection{}, err
		}
		if !ok {
			return Collection{}, fmt.Errorf("hardcover: no list %q by @%s", ref.Slug, ref.Owner)
		}
		col := Collection{Kind: "list", Title: l.Name, URL: l.URL()}
		for _, b := range l.Items {
			col.Books = append(col.Books, PromptAnswer{Book: b})
		}
		return col, nil
	case "vibe":
		v, ok, err := c.VibeBySlug(ctx, ref.Owner, ref.Slug, limit)
		if err != nil {
			return Collection{}, err
		}
		if !ok {
			return Collection{}, fmt.Errorf("hardcover: no vibe %q by @%s", ref.Slug, ref.Owner)
		}
		col := Collection{Kind: "vibe", Title: v.Title + " for @" + v.Owner, URL: v.URL()}
		for _, b := range v.Books {
			col.Books = append(col.Books, PromptAnswer{Book: b})
		}
		return col, nil
	}
	return Collection{}, fmt.Errorf("hardcover: can't read %q", ref.Kind)
}
