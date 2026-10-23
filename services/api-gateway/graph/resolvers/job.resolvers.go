package resolvers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/graphql-go/graphql"
)

// JSearch (RapidAPI) aggregates postings from LinkedIn, Indeed, Glassdoor and
// company boards behind one endpoint. It replaced Careerjet, whose public
// endpoint requires an approved affiliate ID and rejected our requests with 403.
const (
	jsearchHost     = "jsearch.p.rapidapi.com"
	jsearchEndpoint = "https://jsearch.p.rapidapi.com/search"

	// JSearch returns a fixed-size page; used only to guess whether another
	// page exists, since the API reports no total count.
	jsearchPageSize = 10

	// JSearch is billed per request and the same few searches ("Software
	// Engineer", the chips on the Placement page) are repeated by every
	// learner, so results are reused for a while. Listings a few minutes old
	// are as good as new for a job board.
	jobCacheTTL     = 10 * time.Minute
	jobCacheEntries = 500
)

type JobClients struct {
	apiKey   string
	endpoint string
	http     *http.Client

	mu    sync.Mutex
	cache map[string]cachedJobs
	now   func() time.Time
}

type cachedJobs struct {
	at     time.Time
	result map[string]interface{}
}

func NewJobClients() *JobClients {
	return &JobClients{
		// RapidAPI key. Without it the resolver returns a clear setup error
		// rather than an opaque 401 from upstream.
		apiKey:   os.Getenv("JSEARCH_API_KEY"),
		endpoint: jsearchEndpoint,
		http:     &http.Client{Timeout: 60 * time.Second},
		cache:    map[string]cachedJobs{},
		now:      time.Now,
	}
}

func (c *JobClients) cached(key string) (map[string]interface{}, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.cache[key]
	if !ok || c.now().Sub(e.at) > jobCacheTTL {
		return nil, false
	}
	return e.result, true
}

func (c *JobClients) store(key string, result map[string]interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.cache) >= jobCacheEntries {
		// Drop what has expired; if that frees nothing, drop the oldest.
		var oldestKey string
		var oldest time.Time
		for k, e := range c.cache {
			if c.now().Sub(e.at) > jobCacheTTL {
				delete(c.cache, k)
			} else if oldestKey == "" || e.at.Before(oldest) {
				oldestKey, oldest = k, e.at
			}
		}
		if len(c.cache) >= jobCacheEntries {
			delete(c.cache, oldestKey)
		}
	}
	c.cache[key] = cachedJobs{at: c.now(), result: result}
}

// jsearchJob is the subset of JSearch's response we surface. Salary and
// location arrive as separate components, so both are assembled below.
type jsearchJob struct {
	JobTitle       string   `json:"job_title"`
	EmployerName   string   `json:"employer_name"`
	EmployerLogo   string   `json:"employer_logo"`
	Publisher      string   `json:"job_publisher"`
	ApplyLink      string   `json:"job_apply_link"`
	Description    string   `json:"job_description"`
	EmploymentType string   `json:"job_employment_type"`
	IsRemote       bool     `json:"job_is_remote"`
	PostedAt       string   `json:"job_posted_at_datetime_utc"`
	City           string   `json:"job_city"`
	State          string   `json:"job_state"`
	Country        string   `json:"job_country"`
	Location       string   `json:"job_location"`
	MinSalary      *float64 `json:"job_min_salary"`
	MaxSalary      *float64 `json:"job_max_salary"`
	SalaryCurrency string   `json:"job_salary_currency"`
	SalaryPeriod   string   `json:"job_salary_period"`
}

// SearchJobs queries JSearch and maps its payload onto the Job GraphQL type.
func (c *JobClients) SearchJobs(p graphql.ResolveParams) (interface{}, error) {
	if c.apiKey == "" {
		return nil, fmt.Errorf("job search is not configured: set JSEARCH_API_KEY to a RapidAPI key subscribed to JSearch")
	}

	keywords, _ := p.Args["keywords"].(string)
	location, _ := p.Args["location"].(string)
	page, ok := p.Args["page"].(int)
	if !ok || page < 1 {
		page = 1
	}

	// JSearch takes a single free-text query, so the location is folded in
	// ("Backend Developer in India") rather than passed separately.
	query := strings.TrimSpace(keywords)
	if query == "" {
		query = "software developer"
	}
	if loc := strings.TrimSpace(location); loc != "" {
		query = fmt.Sprintf("%s in %s", query, loc)
	}

	key := fmt.Sprintf("%s|%d", strings.ToLower(query), page)
	if result, ok := c.cached(key); ok {
		return result, nil
	}
	result, err := c.fetch(p.Context, query, page)
	if err != nil {
		return nil, err
	}
	c.store(key, result)
	return result, nil
}

// fetch asks JSearch for one page of results and maps them onto the Job type.
func (c *JobClients) fetch(ctx context.Context, query string, page int) (map[string]interface{}, error) {
	u, _ := url.Parse(c.endpoint)
	q := u.Query()
	q.Set("query", query)
	q.Set("page", fmt.Sprintf("%d", page))
	q.Set("num_pages", "1")
	q.Set("date_posted", "month")
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}
	req.Header.Set("X-RapidAPI-Key", c.apiKey)
	req.Header.Set("X-RapidAPI-Host", jsearchHost)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to reach the job search service: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		// The key and quota are the usual causes, so name them rather than
		// leaving a bare status code on screen.
		switch resp.StatusCode {
		case http.StatusUnauthorized, http.StatusForbidden:
			return nil, fmt.Errorf("job search rejected the API key — check JSEARCH_API_KEY and that the RapidAPI account is subscribed to JSearch")
		case http.StatusTooManyRequests:
			return nil, fmt.Errorf("job search rate limit reached — try again shortly")
		default:
			return nil, fmt.Errorf("job search returned status %d", resp.StatusCode)
		}
	}

	var result struct {
		Status string       `json:"status"`
		Data   []jsearchJob `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse job search response: %v", err)
	}

	jobs := make([]map[string]interface{}, 0, len(result.Data))
	for _, j := range result.Data {
		jobs = append(jobs, map[string]interface{}{
			"url":            j.ApplyLink,
			"title":          j.JobTitle,
			"company":        j.EmployerName,
			"locations":      jsearchLocation(j),
			"description":    j.Description,
			"salary":         jsearchSalary(j),
			"date":           j.PostedAt,
			"site":           orDefault(j.Publisher, "JSearch"),
			"employerLogo":   emptyToNil(j.EmployerLogo),
			"employmentType": emptyToNil(prettyEmploymentType(j.EmploymentType)),
			"isRemote":       j.IsRemote,
		})
	}

	// JSearch reports no total. Assume another page exists whenever this one
	// came back full, which is what the pager actually needs to know.
	pages := page
	if len(result.Data) >= jsearchPageSize {
		pages = page + 1
	}

	return map[string]interface{}{
		"jobs":  jobs,
		"total": len(jobs),
		"pages": pages,
	}, nil
}

// jsearchLocation prefers the pre-joined location string newer JSearch
// responses include, falling back to assembling the city/state/country parts.
func jsearchLocation(j jsearchJob) string {
	if s := strings.TrimSpace(j.Location); s != "" {
		return s
	}
	parts := make([]string, 0, 3)
	for _, p := range []string{j.City, j.State, j.Country} {
		if s := strings.TrimSpace(p); s != "" {
			parts = append(parts, s)
		}
	}
	if len(parts) == 0 && j.IsRemote {
		return "Remote"
	}
	return strings.Join(parts, ", ")
}

// jsearchSalary renders the salary components as a single readable range.
// Most postings omit them, in which case the field stays empty and the UI
// shows "Not disclosed".
func jsearchSalary(j jsearchJob) string {
	if j.MinSalary == nil && j.MaxSalary == nil {
		return ""
	}

	currency := orDefault(strings.TrimSpace(j.SalaryCurrency), "")
	amount := ""
	switch {
	case j.MinSalary != nil && j.MaxSalary != nil && *j.MinSalary != *j.MaxSalary:
		amount = fmt.Sprintf("%s – %s", humanAmount(*j.MinSalary), humanAmount(*j.MaxSalary))
	case j.MinSalary != nil:
		amount = humanAmount(*j.MinSalary)
	default:
		amount = humanAmount(*j.MaxSalary)
	}

	out := strings.TrimSpace(currency + " " + amount)
	switch strings.ToUpper(strings.TrimSpace(j.SalaryPeriod)) {
	case "YEAR":
		out += " / yr"
	case "MONTH":
		out += " / mo"
	case "WEEK":
		out += " / wk"
	case "DAY":
		out += " / day"
	case "HOUR":
		out += " / hr"
	}
	return out
}

// humanAmount shortens large figures (1200000 -> 1.2M) so a salary range fits
// on one line in the job card.
func humanAmount(v float64) string {
	switch {
	case v >= 1_000_000:
		return strings.TrimSuffix(fmt.Sprintf("%.1f", v/1_000_000), ".0") + "M"
	case v >= 1_000:
		return strings.TrimSuffix(fmt.Sprintf("%.1f", v/1_000), ".0") + "K"
	default:
		return fmt.Sprintf("%.0f", v)
	}
}

// prettyEmploymentType turns JSearch's FULLTIME / PARTTIME constants into
// labels worth showing on a card.
func prettyEmploymentType(t string) string {
	switch strings.ToUpper(strings.TrimSpace(t)) {
	case "FULLTIME":
		return "Full-time"
	case "PARTTIME":
		return "Part-time"
	case "CONTRACTOR":
		return "Contract"
	case "INTERN":
		return "Internship"
	case "":
		return ""
	default:
		return strings.Title(strings.ToLower(t)) //nolint:staticcheck // ASCII constants only
	}
}

func orDefault(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}

// emptyToNil keeps optional GraphQL fields null rather than "", so the UI can
// branch on presence instead of checking for blank strings.
func emptyToNil(v string) interface{} {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	return v
}
