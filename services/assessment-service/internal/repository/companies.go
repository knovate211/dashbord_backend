package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	assessmentv1 "github.com/knovate211/proto/assessment/v1"
)

// ErrForbidden is returned when a caller has no membership of the company that
// owns the target resource.
var ErrForbidden = errors.New("not authorized for this company")

// ErrNotFound is returned for a missing assessment, attempt or question.
var ErrNotFound = errors.New("not found")

// CreateCompany registers a partner company.
func (r *Repo) CreateCompany(ctx context.Context, req *assessmentv1.CreateCompanyRequest) (*assessmentv1.Company, error) {
	c := &assessmentv1.Company{
		Name:    req.Name,
		Slug:    req.Slug,
		LogoUrl: req.LogoUrl,
		Website: req.Website,
	}
	var created time.Time
	err := r.pool.QueryRow(ctx, `
		INSERT INTO companies (name, slug, logo_url, website)
		VALUES ($1, $2, $3, $4)
		RETURNING id, created_at
	`, req.Name, req.Slug, req.LogoUrl, req.Website).Scan(&c.Id, &created)
	if err != nil {
		return nil, fmt.Errorf("create company: %w", err)
	}
	c.CreatedAt = fmtTime(&created)
	return c, nil
}

// ListCompanies returns every company, or only those the given user belongs to.
func (r *Repo) ListCompanies(ctx context.Context, req *assessmentv1.ListCompaniesRequest) (*assessmentv1.ListCompaniesResponse, error) {
	query := `
		SELECT c.id, c.name, c.slug, c.logo_url, c.website, c.created_at
		FROM   companies c
		ORDER  BY c.name`
	args := []any{}

	if req.UserId != "" {
		query = `
			SELECT c.id, c.name, c.slug, c.logo_url, c.website, c.created_at
			FROM   companies c
			JOIN   company_members m ON m.company_id = c.id AND m.user_id = $1
			ORDER  BY c.name`
		args = append(args, req.UserId)
	}

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list companies: %w", err)
	}
	defer rows.Close()

	out := &assessmentv1.ListCompaniesResponse{Companies: []*assessmentv1.Company{}}
	for rows.Next() {
		c := &assessmentv1.Company{}
		var created time.Time
		if err := rows.Scan(&c.Id, &c.Name, &c.Slug, &c.LogoUrl, &c.Website, &created); err != nil {
			return nil, fmt.Errorf("scan company: %w", err)
		}
		c.CreatedAt = fmtTime(&created)
		out.Companies = append(out.Companies, c)
	}
	return out, rows.Err()
}

// AddCompanyMember grants a recruiter access to a company's drives.
func (r *Repo) AddCompanyMember(ctx context.Context, req *assessmentv1.AddCompanyMemberRequest) error {
	role := req.Role
	if role != "owner" {
		role = "recruiter"
	}
	_, err := r.pool.Exec(ctx, `
		INSERT INTO company_members (company_id, user_id, role)
		VALUES ($1, $2, $3)
		ON CONFLICT (company_id, user_id) DO UPDATE SET role = EXCLUDED.role
	`, req.CompanyId, req.UserId, role)
	if err != nil {
		return fmt.Errorf("add company member: %w", err)
	}
	return nil
}

// ListCompanyMembers joins the shared users table for display names.
func (r *Repo) ListCompanyMembers(ctx context.Context, companyID string) (*assessmentv1.ListCompanyMembersResponse, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT m.company_id, m.user_id, m.role,
		       COALESCE(u.email, ''), COALESCE(u.name, '')
		FROM   company_members m
		LEFT   JOIN users u ON u.id = m.user_id
		WHERE  m.company_id = $1
		ORDER  BY u.name
	`, companyID)
	if err != nil {
		return nil, fmt.Errorf("list company members: %w", err)
	}
	defer rows.Close()

	out := &assessmentv1.ListCompanyMembersResponse{Members: []*assessmentv1.CompanyMember{}}
	for rows.Next() {
		m := &assessmentv1.CompanyMember{}
		if err := rows.Scan(&m.CompanyId, &m.UserId, &m.Role, &m.Email, &m.Name); err != nil {
			return nil, fmt.Errorf("scan company member: %w", err)
		}
		out.Members = append(out.Members, m)
	}
	return out, rows.Err()
}

// Authorize answers whether a user may administer a company or one of its
// assessments. Admins pass unconditionally; recruiters must hold a membership
// row. A practice assessment (no owning company) is admin-only.
//
// The company id is always resolved from the assessment server-side — a
// company_id supplied by the client is never trusted for this decision.
func (r *Repo) Authorize(ctx context.Context, req *assessmentv1.AuthorizeRequest) (*assessmentv1.AuthorizeResponse, error) {
	if req.Role == "admin" {
		return &assessmentv1.AuthorizeResponse{Allowed: true}, nil
	}
	if req.Role != "recruiter" {
		return &assessmentv1.AuthorizeResponse{Allowed: false, Reason: "recruiter or admin role required"}, nil
	}

	companyID := req.CompanyId
	if req.AssessmentId != "" {
		owner, err := r.assessmentCompany(ctx, req.AssessmentId)
		if err != nil {
			return nil, err
		}
		if owner == "" {
			return &assessmentv1.AuthorizeResponse{Allowed: false, Reason: "platform assessments are admin-only"}, nil
		}
		companyID = owner
	}
	if companyID == "" {
		return &assessmentv1.AuthorizeResponse{Allowed: false, Reason: "no company scope"}, nil
	}

	var exists bool
	err := r.pool.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM company_members WHERE company_id = $1 AND user_id = $2)
	`, companyID, req.UserId).Scan(&exists)
	if err != nil {
		return nil, fmt.Errorf("check membership: %w", err)
	}
	if !exists {
		return &assessmentv1.AuthorizeResponse{Allowed: false, Reason: "not a member of the owning company"}, nil
	}
	return &assessmentv1.AuthorizeResponse{Allowed: true}, nil
}

// assessmentCompany returns the owning company id, or "" for a platform
// assessment.
func (r *Repo) assessmentCompany(ctx context.Context, assessmentID string) (string, error) {
	var companyID *string
	err := r.pool.QueryRow(ctx, `SELECT company_id FROM assessments WHERE id = $1`, assessmentID).Scan(&companyID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("lookup assessment company: %w", err)
	}
	if companyID == nil {
		return "", nil
	}
	return *companyID, nil
}

// AssessmentIDForAttempt lets the gateway authorize a recruiter action that
// names only an attempt.
func (r *Repo) AssessmentIDForAttempt(ctx context.Context, attemptID string) (string, error) {
	var id string
	err := r.pool.QueryRow(ctx, `SELECT assessment_id FROM attempts WHERE id = $1`, attemptID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("lookup attempt assessment: %w", err)
	}
	return id, nil
}

// AssessmentIDForShortlist mirrors AssessmentIDForAttempt for shortlist routes.
func (r *Repo) AssessmentIDForShortlist(ctx context.Context, shortlistID string) (string, error) {
	var id string
	err := r.pool.QueryRow(ctx, `SELECT assessment_id FROM shortlists WHERE id = $1`, shortlistID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("lookup shortlist assessment: %w", err)
	}
	return id, nil
}
