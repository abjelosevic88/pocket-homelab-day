package sources

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Forgejo reads the user's own activity feed for the day and summarises it
// per repository: commits pushed, the latest commit message, and other
// actions (repositories created, pull requests, tags, issues).
type Forgejo struct {
	BaseURL string
	Token   string
	User    string
	HTTP    *http.Client
}

// NewForgejo creates a fetcher; user is the account whose feed to read.
func NewForgejo(baseURL, token, user string) *Forgejo {
	return &Forgejo{BaseURL: strings.TrimRight(baseURL, "/"), Token: token, User: strings.TrimSpace(user), HTTP: &http.Client{Timeout: 30 * time.Second}}
}

// Name implements Fetcher.
func (c *Forgejo) Name() string { return "forgejo" }

func (c *Forgejo) get(ctx context.Context, path string, q url.Values, v any) (http.Header, error) {
	if c.BaseURL == "" || c.Token == "" {
		return nil, errors.New("forgejo is not configured")
	}
	u := c.BaseURL + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "token "+c.Token)
	req.Header.Set("Accept", "application/json")
	res, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusUnauthorized || res.StatusCode == http.StatusForbidden {
		return nil, errors.New("forgejo rejected the token")
	}
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("forgejo HTTP %d", res.StatusCode)
	}
	return res.Header, json.NewDecoder(res.Body).Decode(v)
}

func (c *Forgejo) user(ctx context.Context) (string, error) {
	if c.User != "" {
		return c.User, nil
	}
	var me struct {
		Login string `json:"login"`
	}
	if _, err := c.get(ctx, "/api/v1/user", nil, &me); err != nil {
		return "", err
	}
	c.User = me.Login
	return me.Login, nil
}

// Test implements Fetcher.
func (c *Forgejo) Test(ctx context.Context) (string, error) {
	u, err := c.user(ctx)
	if err != nil {
		return "", err
	}
	var v struct {
		Version string `json:"version"`
	}
	_, _ = c.get(ctx, "/api/v1/version", nil, &v)
	return fmt.Sprintf("Forgejo %s, reading %s's activity", v.Version, u), nil
}

var forgejoVerb = map[string]string{
	"create_repo": "created the repository", "rename_repo": "renamed the repository", "push_tag": "tagged", "delete_tag": "deleted a tag",
	"create_pull_request": "opened a pull request", "merge_pull_request": "merged a pull request", "close_pull_request": "closed a pull request",
	"create_issue": "opened an issue", "close_issue": "closed an issue", "comment_issue": "commented", "create_branch": "created a branch", "delete_branch": "deleted a branch",
	"publish_release": "published a release", "mirror_sync_push": "", "transfer_repo": "transferred the repository",
}

// Fetch implements Fetcher.
func (c *Forgejo) Fetch(ctx context.Context, date string, loc *time.Location) (*Result, error) {
	user, err := c.user(ctx)
	if err != nil {
		return nil, err
	}
	type repoAgg struct {
		commits int
		first   string
		last    string
		message string
		lastAt  string
		extras  []string
		private bool
	}
	repos := map[string]*repoAgg{}
	for page := 1; page <= 20; page++ {
		var feed []struct {
			OpType  string `json:"op_type"`
			Created string `json:"created"`
			RefName string `json:"ref_name"`
			Content string `json:"content"`
			Repo    struct {
				FullName string `json:"full_name"`
				Private  bool   `json:"private"`
			} `json:"repo"`
		}
		h, err := c.get(ctx, "/api/v1/users/"+url.PathEscape(user)+"/activities/feeds", url.Values{"date": {date}, "only-performed-by": {"true"}, "limit": {"50"}, "page": {strconv.Itoa(page)}}, &feed)
		if err != nil {
			return nil, err
		}
		for _, f := range feed {
			if f.OpType == "mirror_sync_push" {
				continue
			}
			r := repos[f.Repo.FullName]
			if r == nil {
				r = &repoAgg{private: f.Repo.Private}
				repos[f.Repo.FullName] = r
			}
			at, _ := ParseAny(f.Created)
			clock := LocalClock(at, loc)
			if r.first == "" || clock < r.first {
				r.first = clock
			}
			if clock > r.last {
				r.last = clock
			}
			if f.OpType == "commit_repo" {
				var ct struct {
					Len        int `json:"Len"`
					HeadCommit *struct {
						Message string `json:"Message"`
					} `json:"HeadCommit"`
					Commits []struct {
						Message string `json:"Message"`
					} `json:"Commits"`
				}
				if json.Unmarshal([]byte(f.Content), &ct) == nil {
					n := ct.Len
					if n == 0 {
						n = len(ct.Commits)
					}
					r.commits += n
					msg := ""
					if ct.HeadCommit != nil {
						msg = ct.HeadCommit.Message
					} else if len(ct.Commits) > 0 {
						msg = ct.Commits[0].Message
					}
					if msg != "" && clock >= r.lastAt {
						r.message, r.lastAt = strings.TrimSpace(strings.SplitN(msg, "\n", 2)[0]), clock
					}
				}
				continue
			}
			if v, ok := forgejoVerb[f.OpType]; ok && v != "" {
				x := v
				if f.RefName != "" && (f.OpType == "push_tag" || strings.Contains(f.OpType, "branch")) {
					x += " " + strings.TrimPrefix(strings.TrimPrefix(f.RefName, "refs/tags/"), "refs/heads/")
				}
				r.extras = append(r.extras, x)
			}
		}
		total, _ := strconv.Atoi(h.Get("X-Total-Count"))
		if len(feed) < 50 || page*50 >= total {
			break
		}
	}
	act := Activity{}
	names := make([]string, 0, len(repos))
	for n := range repos {
		names = append(names, n)
	}
	sort.Slice(names, func(i, j int) bool { return repos[names[i]].commits > repos[names[j]].commits })
	for _, n := range names {
		r := repos[n]
		short := n
		if i := strings.LastIndex(n, "/"); i >= 0 {
			short = n[i+1:]
		}
		sub := ""
		if r.commits > 0 {
			sub = Plural(r.commits, "commit", "commits")
			if r.message != "" {
				sub += " · " + r.message
			}
		}
		if len(r.extras) > 0 {
			sub = JoinNonEmpty(" · ", sub, strings.Join(uniq(r.extras), ", "))
		}
		act.Items = append(act.Items, ActivityItem{Time: r.first, End: r.last, Title: short, Sub: sub, Group: short, Qty: float64(r.commits), Ref: n, URL: "/" + n})
	}
	act.Finish()
	sum := ""
	if act.Count > 0 {
		sum = fmt.Sprintf("%s in %s", Plural(int(act.Qty), "commit", "commits"), Plural(act.Count, "repository", "repositories"))
		sum += " · mostly " + act.Items[0].Title
	}
	return &Result{Payload: act, Headline: act.Qty, Summary: sum}, nil
}

func uniq(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}
