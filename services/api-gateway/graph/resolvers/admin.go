package resolvers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"github.com/knovate211/api-gateway/middleware"
	pkgauth "github.com/knovate211/pkg/auth"
	executionv1 "github.com/knovate211/proto/execution/v1"
)

// AdminHandler handles /api/admin/* REST endpoints.
// It holds its own Postgres pool so it stays independent of the user-service module.
type AdminHandler struct {
	Pool *pgxpool.Pool
	Log  *zap.Logger
	// Inquiries serves the enquiry list; nil disables those routes.
	Inquiries *InquiryHandler
	// Scholarships serves the scholarship application and programme screens;
	// nil disables those routes.
	Scholarships *ScholarshipHandler
	// Attendance serves live-class schedules and rosters; nil disables those routes.
	Attendance *AttendanceHandler
	// Certifications serves the exam setup and registration screens; nil
	// disables those routes.
	Certifications *CertificationHandler
	// Referrals serves the referral programme screens; nil disables those routes.
	Referrals *ReferralHandler
	// Mailer sends a new account its login details; nil sends nothing (the
	// account is still created and its password still returned to the admin).
	Mailer *userMailer
	// Enroll lists online course purchases; nil disables that route.
	Enroll *EnrollHandler
	// Exec generates starter code and verifies reference solutions for the
	// coding-problem editor; nil disables those two actions.
	Exec executionv1.ExecutionServiceClient
}

// ServeHTTP dispatches admin REST routes.
func (h *AdminHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/api/admin")
	path = strings.TrimRight(path, "/")

	// Role guard — admin only, except two read-only lookups the recruiter
	// screens share with the admin ones (see serveRecruiterLookup).
	if role := middleware.RoleFromContext(r.Context()); role != "admin" {
		if role == "recruiter" && r.Method == http.MethodGet && (path == "/courses" || path == "/mcq-bank/facets") {
			h.serveRecruiterLookup(w, r, path)
			return
		}
		h.jsonErr(w, http.StatusForbidden, "admin access required")
		return
	}

	switch {
	// POST /api/admin/bulk-import
	case path == "/bulk-import" && r.Method == http.MethodPost:
		h.handleBulkImport(w, r)

	// GET /api/admin/inquiries
	case path == "/inquiries" && r.Method == http.MethodGet && h.Inquiries != nil:
		h.Inquiries.ListInquiries(w, r)

	// GET /api/admin/inquiries/facets — filter values with counts
	case path == "/inquiries/facets" && r.Method == http.MethodGet && h.Inquiries != nil:
		h.handleInquiryFacets(w, r)

	// GET /api/admin/inquiries/export.csv — same filters as the table
	case path == "/inquiries/export.csv" && r.Method == http.MethodGet && h.Inquiries != nil:
		h.handleInquiryExport(w, r)

	// POST /api/admin/inquiries/bulk — status change / delete
	case path == "/inquiries/bulk" && r.Method == http.MethodPost && h.Inquiries != nil:
		h.handleInquiryBulk(w, r)

	// GET /api/admin/scholarships/facets — filter values with counts
	case path == "/scholarships/facets" && r.Method == http.MethodGet && h.Scholarships != nil:
		h.handleScholarshipFacets(w, r)

	// POST /api/admin/scholarships/bulk — delete many applications
	case path == "/scholarships/bulk" && r.Method == http.MethodPost && h.Scholarships != nil:
		h.handleScholarshipBulk(w, r)

	// PATCH /api/admin/inquiries/{id}   — status / notes
	case strings.HasPrefix(path, "/inquiries/") && r.Method == http.MethodPatch && h.Inquiries != nil:
		h.Inquiries.UpdateInquiry(w, r, strings.TrimPrefix(path, "/inquiries/"))

	// GET /api/admin/scholarships — applications, with live attempt scores
	case path == "/scholarships" && r.Method == http.MethodGet && h.Scholarships != nil:
		h.Scholarships.ListApplications(w, r)

	// GET /api/admin/scholarships/export.csv — same filters as the table
	case path == "/scholarships/export.csv" && r.Method == http.MethodGet && h.Scholarships != nil:
		h.Scholarships.ExportApplications(w, r)

	// POST /api/admin/scholarships/{id}/enrol — applicant becomes a student
	case strings.HasPrefix(path, "/scholarships/") && strings.HasSuffix(path, "/enrol") &&
		r.Method == http.MethodPost && h.Scholarships != nil:
		inner := strings.TrimPrefix(path, "/scholarships/")
		h.Scholarships.EnrolApplicant(w, r, strings.TrimSuffix(inner, "/enrol"))

	// POST /api/admin/scholarships/{id}/resend — a fresh link, emailed again
	case strings.HasPrefix(path, "/scholarships/") && strings.HasSuffix(path, "/resend") &&
		r.Method == http.MethodPost && h.Scholarships != nil:
		inner := strings.TrimPrefix(path, "/scholarships/")
		h.Scholarships.ResendLink(w, r, strings.TrimSuffix(inner, "/resend"))

	// DELETE /api/admin/scholarships/{id}  — remove the application entirely
	case strings.HasPrefix(path, "/scholarships/") && r.Method == http.MethodDelete && h.Scholarships != nil:
		h.Scholarships.DeleteApplication(w, r, strings.TrimPrefix(path, "/scholarships/"))

	// PATCH /api/admin/scholarships/{id}   — award decision / notes
	case strings.HasPrefix(path, "/scholarships/") && r.Method == http.MethodPatch && h.Scholarships != nil:
		h.Scholarships.UpdateApplication(w, r, strings.TrimPrefix(path, "/scholarships/"))

	// GET /api/admin/scholarship-programs — course-to-paper mapping
	case path == "/scholarship-programs" && r.Method == http.MethodGet && h.Scholarships != nil:
		h.Scholarships.ListPrograms(w, r)

	// POST /api/admin/scholarship-programs — create or repoint a programme
	case path == "/scholarship-programs" && r.Method == http.MethodPost && h.Scholarships != nil:
		h.Scholarships.UpsertProgram(w, r)

	// ─── Referral programme ────────────────────────────────────────────────
	// GET|POST /api/admin/referral-program — the offer's settings
	case path == "/referral-program" && r.Method == http.MethodGet && h.Referrals != nil:
		h.Referrals.GetProgram(w, r)
	case path == "/referral-program" && r.Method == http.MethodPost && h.Referrals != nil:
		h.Referrals.UpdateProgram(w, r)

	// GET /api/admin/referrers — who is referring
	case path == "/referrers" && r.Method == http.MethodGet && h.Referrals != nil:
		h.Referrals.ListReferrers(w, r)

	// PATCH /api/admin/referrers/{id} — payout details, notes, block
	case strings.HasPrefix(path, "/referrers/") && r.Method == http.MethodPatch && h.Referrals != nil:
		h.Referrals.UpdateReferrer(w, r, strings.TrimPrefix(path, "/referrers/"))

	// GET /api/admin/referrals/export.csv — the payout sheet
	case path == "/referrals/export.csv" && r.Method == http.MethodGet && h.Referrals != nil:
		h.Referrals.ExportConversions(w, r)

	// POST /api/admin/referrals/bulk — approve or pay a whole run
	case path == "/referrals/bulk" && r.Method == http.MethodPost && h.Referrals != nil:
		h.Referrals.BulkUpdate(w, r)

	// GET /api/admin/referrals — rewards owed
	case path == "/referrals" && r.Method == http.MethodGet && h.Referrals != nil:
		h.Referrals.ListConversions(w, r)

	// PATCH /api/admin/referrals/{id} — approve, reject, pay
	case strings.HasPrefix(path, "/referrals/") && r.Method == http.MethodPatch && h.Referrals != nil:
		h.Referrals.UpdateConversion(w, r, strings.TrimPrefix(path, "/referrals/"))

	// ─── Certification exams ───────────────────────────────────────────────
	// GET /api/admin/certification-exams — what is on offer
	case path == "/certification-exams" && r.Method == http.MethodGet && h.Certifications != nil:
		h.Certifications.ListExams(w, r)

	// POST /api/admin/certification-exams — create or repoint an exam
	case path == "/certification-exams" && r.Method == http.MethodPost && h.Certifications != nil:
		h.Certifications.UpsertExam(w, r)

	// DELETE /api/admin/certification-exams/{id}
	case strings.HasPrefix(path, "/certification-exams/") && r.Method == http.MethodDelete && h.Certifications != nil:
		h.Certifications.DeleteExam(w, r, strings.TrimPrefix(path, "/certification-exams/"))

	// GET /api/admin/certifications/export.csv — same filters as the table
	case path == "/certifications/export.csv" && r.Method == http.MethodGet && h.Certifications != nil:
		h.Certifications.ExportRegistrations(w, r)

	// GET /api/admin/certifications — registrations, with live attempt state
	case path == "/certifications" && r.Method == http.MethodGet && h.Certifications != nil:
		h.Certifications.ListRegistrations(w, r)

	// POST /api/admin/certifications/{id}/resend — rotate and re-send the link
	case strings.HasPrefix(path, "/certifications/") && strings.HasSuffix(path, "/resend") &&
		r.Method == http.MethodPost && h.Certifications != nil:
		inner := strings.TrimPrefix(path, "/certifications/")
		h.Certifications.ResendLink(w, r, strings.TrimSuffix(inner, "/resend"))

	// POST /api/admin/certifications/{id}/extend — same link, more time
	case strings.HasPrefix(path, "/certifications/") && strings.HasSuffix(path, "/extend") &&
		r.Method == http.MethodPost && h.Certifications != nil:
		inner := strings.TrimPrefix(path, "/certifications/")
		h.Certifications.ExtendLink(w, r, strings.TrimSuffix(inner, "/extend"))

	// PATCH /api/admin/certifications/{id} — notes, refund, issue or revoke
	case strings.HasPrefix(path, "/certifications/") && r.Method == http.MethodPatch && h.Certifications != nil:
		h.Certifications.UpdateRegistration(w, r, strings.TrimPrefix(path, "/certifications/"))

	// GET /api/admin/classes — live-class schedules
	case path == "/classes" && r.Method == http.MethodGet && h.Attendance != nil:
		h.Attendance.ListSchedules(w, r)

	// POST /api/admin/classes — schedule a recurring class
	case path == "/classes" && r.Method == http.MethodPost && h.Attendance != nil:
		h.Attendance.CreateSchedule(w, r)

	// GET /api/admin/classes/{id}/attendance?date= — who marked that session
	case strings.HasPrefix(path, "/classes/") && strings.HasSuffix(path, "/attendance") &&
		r.Method == http.MethodGet && h.Attendance != nil:
		inner := strings.TrimPrefix(path, "/classes/")
		h.Attendance.SessionRoster(w, r, strings.TrimSuffix(inner, "/attendance"))

	// PUT /api/admin/classes/{id}
	case strings.HasPrefix(path, "/classes/") && r.Method == http.MethodPut && h.Attendance != nil:
		h.Attendance.UpdateSchedule(w, r, strings.TrimPrefix(path, "/classes/"))

	// DELETE /api/admin/classes/{id}
	case strings.HasPrefix(path, "/classes/") && r.Method == http.MethodDelete && h.Attendance != nil:
		h.Attendance.DeleteSchedule(w, r, strings.TrimPrefix(path, "/classes/"))

	// GET /api/admin/courses — catalog for the admin UI dropdowns
	case path == "/courses" && r.Method == http.MethodGet:
		h.handleListCourses(w, r)

	// GET /api/admin/stats — dashboard headline numbers
	case path == "/stats" && r.Method == http.MethodGet:
		h.handleStats(w, r)

	// GET /api/admin/grading-queue — descriptive answers awaiting a mark
	// GET /api/admin/activity — recent-activity feed for the dashboard
	case path == "/activity" && r.Method == http.MethodGet:
		h.handleActivity(w, r)

	case path == "/grading-queue" && r.Method == http.MethodGet:
		h.handleGradingQueue(w, r)

	// POST /api/admin/tests/{id}/duplicate — copy a test as a new draft
	case strings.HasPrefix(path, "/tests/") && strings.HasSuffix(path, "/duplicate") && r.Method == http.MethodPost:
		h.handleDuplicateTest(w, r, strings.TrimSuffix(strings.TrimPrefix(path, "/tests/"), "/duplicate"))

	// Coding problems — list / create / preview starters / read / update / delete / verify
	case path == "/problems" && r.Method == http.MethodGet:
		h.handleListProblems(w, r)
	case path == "/problems" && r.Method == http.MethodPost:
		h.handleSaveProblem(w, r, "")
	case path == "/problems/starters" && r.Method == http.MethodPost:
		h.handleProblemStarters(w, r)
	case strings.HasPrefix(path, "/problems/") && strings.HasSuffix(path, "/verify") && r.Method == http.MethodPost:
		h.handleVerifyProblem(w, r, strings.TrimSuffix(strings.TrimPrefix(path, "/problems/"), "/verify"))
	case strings.HasPrefix(path, "/problems/") && !strings.Contains(path[len("/problems/"):], "/") && r.Method == http.MethodGet:
		h.handleGetProblem(w, r, strings.TrimPrefix(path, "/problems/"))
	case strings.HasPrefix(path, "/problems/") && !strings.Contains(path[len("/problems/"):], "/") && r.Method == http.MethodPut:
		h.handleSaveProblem(w, r, strings.TrimPrefix(path, "/problems/"))
	case strings.HasPrefix(path, "/problems/") && !strings.Contains(path[len("/problems/"):], "/") && r.Method == http.MethodDelete:
		h.handleDeleteProblem(w, r, strings.TrimPrefix(path, "/problems/"))

	// GET /api/admin/enrollments — online course purchases
	case path == "/enrollments" && r.Method == http.MethodGet && h.Enroll != nil:
		h.Enroll.ListOrders(w, r)

	// GET /api/admin/mcq-bank/facets — question counts per course / topic / difficulty
	case path == "/mcq-bank/facets" && r.Method == http.MethodGet:
		h.handleMcqFacets(w, r)

	// GET /api/admin/audit — who changed what
	case path == "/audit" && r.Method == http.MethodGet:
		h.handleListAudit(w, r)

	// GET /api/admin/users
	case path == "/users" && r.Method == http.MethodGet:
		h.handleListUsers(w, r)

	// GET /api/admin/users/export.csv — same filters as the list
	case path == "/users/export.csv" && r.Method == http.MethodGet:
		h.handleExportUsers(w, r)

	// POST /api/admin/users/bulk — role / course / delete over many users
	case path == "/users/bulk" && r.Method == http.MethodPost:
		h.handleBulkUsers(w, r)

	// PATCH /api/admin/users/{id}   — update role
	case strings.HasPrefix(path, "/users/") && !strings.Contains(path[len("/users/"):], "/") && r.Method == http.MethodPatch:
		userID := strings.TrimPrefix(path, "/users/")
		h.handleUpdateUser(w, r, userID)

	// DELETE /api/admin/users/{id}
	case strings.HasPrefix(path, "/users/") && !strings.Contains(path[len("/users/"):], "/") && r.Method == http.MethodDelete:
		userID := strings.TrimPrefix(path, "/users/")
		h.handleDeleteUser(w, r, userID)

	// POST /api/admin/users/{id}/courses
	case strings.HasPrefix(path, "/users/") && strings.HasSuffix(path, "/courses") && r.Method == http.MethodPost:
		inner := strings.TrimPrefix(path, "/users/")
		userID := strings.TrimSuffix(inner, "/courses")
		h.handleGrantCourse(w, r, userID)

	// DELETE /api/admin/users/{id}/courses/{courseId}
	case strings.HasPrefix(path, "/users/") && strings.Contains(path, "/courses/") && r.Method == http.MethodDelete:
		// path: /users/{id}/courses/{courseId}
		inner := strings.TrimPrefix(path, "/users/")
		parts := strings.SplitN(inner, "/courses/", 2)
		if len(parts) == 2 {
			h.handleRevokeCourse(w, r, parts[0], parts[1])
		} else {
			h.jsonErr(w, http.StatusNotFound, "not found")
		}

	default:
		h.jsonErr(w, http.StatusNotFound, "not found")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Data types
// ─────────────────────────────────────────────────────────────────────────────

type importUserRow struct {
	Name      string   `json:"name"`
	Email     string   `json:"email"`
	Phone     string   `json:"phone"`
	Password  string   `json:"password"`
	Role      string   `json:"role"`
	CourseIDs []string `json:"course_ids"`
}

type importRowResult struct {
	Email   string `json:"email"`
	Success bool   `json:"success"`
	Message string `json:"message"`
	// Emailed reports whether a welcome email was actually dispatched — false
	// when welcome was not requested, the row was an update, or SMTP is off.
	Emailed bool `json:"emailed"`
	// IsNewUser distinguishes a created account from an updated one.
	IsNewUser bool `json:"is_new_user"`
}

type adminUserRow struct {
	ID        string   `json:"id"`
	Email     string   `json:"email"`
	Name      string   `json:"name"`
	Role      string   `json:"role"`
	CourseIDs []string `json:"course_ids"`
	CreatedAt string   `json:"created_at"`
	// Companies the user recruits for (company_members). Empty for anyone who
	// is not a recruiter; shown on the Users screen so staff can see who works
	// for which company without opening every company.
	Companies json.RawMessage `json:"companies"`
}

// ─────────────────────────────────────────────────────────────────────────────
// Handlers
// ─────────────────────────────────────────────────────────────────────────────

func (h *AdminHandler) handleBulkImport(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Users []importUserRow `json:"users"`
		// SendWelcome emails each newly created account its credentials. The
		// single Add-user form sets it; a CSV import leaves it off so a large
		// upload does not fan out into a surprise mail blast.
		SendWelcome bool `json:"send_welcome"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.jsonErr(w, http.StatusBadRequest, "invalid request body")
		return
	}

	ctx := context.Background()
	results := h.bulkUpsertUsers(ctx, req.Users, req.SendWelcome)

	successCount := 0
	for _, res := range results {
		if res.Success {
			successCount++
		}
	}
	h.audit(r.Context(), "users.imported", "", "", map[string]interface{}{
		"total": len(results), "success": successCount, "send_welcome": req.SendWelcome,
	})

	h.json(w, http.StatusOK, map[string]interface{}{
		"total":   len(results),
		"success": successCount,
		"failed":  len(results) - successCount,
		"results": results,
	})
}

func (h *AdminHandler) handleListUsers(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	page, _ := strconv.Atoi(q.Get("page"))
	pageSize, _ := strconv.Atoi(q.Get("page_size"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 500 {
		pageSize = 50
	}

	users, total, err := h.listUsers(r.Context(), page, pageSize, userFiltersFromQuery(r))
	if err != nil {
		h.Log.Error("list admin users failed", zap.Error(err))
		h.jsonErr(w, http.StatusInternalServerError, "failed to list users")
		return
	}

	h.json(w, http.StatusOK, map[string]interface{}{
		"users":    users,
		"total":    total,
		"page":     page,
		"pageSize": pageSize,
	})
}

func (h *AdminHandler) handleUpdateUser(w http.ResponseWriter, r *http.Request, userID string) {
	// All fields optional; only the ones provided are changed. Pointers let us
	// tell "omitted" apart from "set to empty".
	var req struct {
		Name  *string `json:"name"`
		Email *string `json:"email"`
		Phone *string `json:"phone"`
		Role  *string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.jsonErr(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Role != nil {
		// `recruiter` grants access to the partner hiring portal; a recruiter still
		// needs a company_members row before they can see any drive.
		if *req.Role != "student" && *req.Role != "admin" && *req.Role != "recruiter" {
			h.jsonErr(w, http.StatusBadRequest, "role must be 'student', 'recruiter' or 'admin'")
			return
		}
	}

	// Build a dynamic UPDATE over whichever core fields were supplied.
	sets := []string{}
	args := []interface{}{userID}
	add := func(col string, val interface{}) {
		args = append(args, val)
		sets = append(sets, fmt.Sprintf("%s = $%d", col, len(args)))
	}
	if req.Name != nil {
		add("name", *req.Name)
	}
	if req.Email != nil {
		add("email", strings.ToLower(strings.TrimSpace(*req.Email)))
	}
	if req.Role != nil {
		add("role", *req.Role)
	}

	if len(sets) > 0 {
		q := fmt.Sprintf(`UPDATE users SET %s, updated_at = now() WHERE id = $1::uuid`, strings.Join(sets, ", "))
		tag, err := h.Pool.Exec(r.Context(), q, args...)
		if err != nil {
			h.Log.Error("update user failed", zap.String("userID", userID), zap.Error(err))
			h.jsonErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		if tag.RowsAffected() == 0 {
			h.jsonErr(w, http.StatusNotFound, "user not found")
			return
		}
	}

	// Phone lives on the profile row; upsert it independently.
	if req.Phone != nil {
		_, err := h.Pool.Exec(r.Context(), `
			INSERT INTO user_profiles (user_id, phone)
			VALUES ($1::uuid, $2)
			ON CONFLICT (user_id) DO UPDATE SET phone = EXCLUDED.phone, updated_at = now()
		`, userID, *req.Phone)
		if err != nil {
			h.Log.Error("update user phone failed", zap.String("userID", userID), zap.Error(err))
			h.jsonErr(w, http.StatusInternalServerError, err.Error())
			return
		}
	}

	detail := map[string]interface{}{}
	action := "user.updated"
	if req.Role != nil {
		detail["role"] = *req.Role
		action = "user.role_changed"
	}
	for k, v := range map[string]*string{"name": req.Name, "email": req.Email, "phone": req.Phone} {
		if v != nil {
			detail[k] = *v
			action = "user.updated"
		}
	}
	h.audit(r.Context(), action, userID, h.emailOf(r.Context(), userID), detail)

	h.json(w, http.StatusOK, map[string]bool{"success": true})
}

// handleListCourses returns the course catalog used by the admin UI. This is the
// single source of truth the frontend reads so its dropdowns never drift from the
// backend's programModules access map.
func (h *AdminHandler) handleListCourses(w http.ResponseWriter, r *http.Request) {
	type course struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	h.json(w, http.StatusOK, map[string]interface{}{"courses": []course{
		{ID: "1", Name: "Java Development"},
		{ID: "2", Name: "Front-End Technologies"},
		{ID: "3", Name: "Mastering SQL"},
		{ID: "4", Name: "Golang"},
		{ID: "5", Name: "Full Stack Development"},
		{ID: "genai", Name: "GenAI & Forward Deployed Engineering"},
		{ID: "seo", Name: "SEO"},
		{ID: "digital-marketing", Name: "Digital Marketing"},
		{ID: "testing", Name: "Software Testing"},
	}})
}

func (h *AdminHandler) handleDeleteUser(w http.ResponseWriter, r *http.Request, userID string) {
	if userID == middleware.UserIDFromContext(r.Context()) {
		h.jsonErr(w, http.StatusBadRequest, "you cannot delete your own account")
		return
	}
	email := h.emailOf(r.Context(), userID)
	tag, err := h.Pool.Exec(r.Context(), `DELETE FROM users WHERE id = $1::uuid`, userID)
	if err != nil {
		h.Log.Error("delete user failed", zap.String("userID", userID), zap.Error(err))
		h.jsonErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if tag.RowsAffected() == 0 {
		h.jsonErr(w, http.StatusNotFound, "user not found")
		return
	}
	h.audit(r.Context(), "user.deleted", userID, email, nil)
	h.json(w, http.StatusOK, map[string]bool{"success": true})
}

func (h *AdminHandler) handleGrantCourse(w http.ResponseWriter, r *http.Request, userID string) {
	var req struct {
		CourseID string `json:"course_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.CourseID == "" {
		h.jsonErr(w, http.StatusBadRequest, "course_id is required")
		return
	}
	_, err := h.Pool.Exec(r.Context(), `
		INSERT INTO user_courses (user_id, course_id)
		VALUES ($1::uuid, $2)
		ON CONFLICT (user_id, course_id) DO NOTHING
	`, userID, req.CourseID)
	if err != nil {
		h.Log.Error("grant course access failed", zap.Error(err))
		h.jsonErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.audit(r.Context(), "course.granted", userID, h.emailOf(r.Context(), userID),
		map[string]interface{}{"course_id": req.CourseID})
	h.json(w, http.StatusOK, map[string]bool{"success": true})
}

func (h *AdminHandler) handleRevokeCourse(w http.ResponseWriter, r *http.Request, userID, courseID string) {
	_, err := h.Pool.Exec(r.Context(),
		`DELETE FROM user_courses WHERE user_id = $1::uuid AND course_id = $2`, userID, courseID)
	if err != nil {
		h.Log.Error("revoke course access failed", zap.Error(err))
		h.jsonErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.audit(r.Context(), "course.revoked", userID, h.emailOf(r.Context(), userID),
		map[string]interface{}{"course_id": courseID})
	h.json(w, http.StatusOK, map[string]bool{"success": true})
}

// ─────────────────────────────────────────────────────────────────────────────
// Internal DB helpers
// ─────────────────────────────────────────────────────────────────────────────

func (h *AdminHandler) bulkUpsertUsers(ctx context.Context, rows []importUserRow, sendWelcome bool) []importRowResult {
	results := make([]importRowResult, 0, len(rows))
	for _, row := range rows {
		if row.Email == "" || row.Name == "" || row.Password == "" {
			results = append(results, importRowResult{
				Email:   row.Email,
				Success: false,
				Message: "email, name and password are required",
			})
			continue
		}
		role := row.Role
		if role == "" {
			role = "student"
		}

		hashed, err := pkgauth.HashPassword(row.Password)
		if err != nil {
			results = append(results, importRowResult{
				Email:   row.Email,
				Success: false,
				Message: fmt.Sprintf("hash password: %v", err),
			})
			continue
		}

		// (xmax = 0) is true only when this row was inserted, not updated —
		// so a welcome email goes to genuinely new accounts and never re-mails
		// (and silently resets the password of) somebody who already exists.
		var userID string
		var isNewAccount bool
		err = h.Pool.QueryRow(ctx, `
			INSERT INTO users (email, name, password, role)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (email)
			DO UPDATE SET name = EXCLUDED.name, password = EXCLUDED.password, role = EXCLUDED.role, updated_at = now()
			RETURNING id::text, (xmax = 0)
		`, row.Email, row.Name, hashed, role).Scan(&userID, &isNewAccount)
		if err != nil {
			results = append(results, importRowResult{
				Email:   row.Email,
				Success: false,
				Message: fmt.Sprintf("upsert user: %v", err),
			})
			continue
		}

		// Persist phone on the profile row. Blank phone still creates the row so
		// the profile page has something to show; a later edit overwrites it.
		if row.Phone != "" {
			_, _ = h.Pool.Exec(ctx, `
				INSERT INTO user_profiles (user_id, phone)
				VALUES ($1::uuid, $2)
				ON CONFLICT (user_id) DO UPDATE SET phone = EXCLUDED.phone, updated_at = now()
			`, userID, row.Phone)
		}

		for _, courseID := range row.CourseIDs {
			if courseID == "" {
				continue
			}
			_, _ = h.Pool.Exec(ctx, `
				INSERT INTO user_courses (user_id, course_id)
				VALUES ($1::uuid, $2)
				ON CONFLICT (user_id, course_id) DO NOTHING
			`, userID, courseID)
		}

		if sendWelcome && isNewAccount && h.Mailer != nil {
			// row.Password is the plaintext the admin generated; it exists only
			// here, before it is hashed away, which is the one moment we can
			// tell the new user what it is.
			h.Mailer.notifyNewUser(row.Name, row.Email, row.Password)
		}

		results = append(results, importRowResult{
			Email:     row.Email,
			Success:   true,
			Message:   "imported successfully",
			Emailed:   sendWelcome && isNewAccount && h.Mailer != nil && h.Mailer.enabled(),
			IsNewUser: isNewAccount,
		})
	}
	return results
}

func (h *AdminHandler) listUsers(ctx context.Context, page, pageSize int, f userFilters) ([]adminUserRow, int, error) {
	offset := (page - 1) * pageSize

	// Scholarship applicants are excluded (by userFilters.where). They hold a
	// users row because the assessment engine keys an attempt to a user id, but
	// they are not on a course and nobody has enrolled them — showing them here
	// would make the Users screen a list of everyone who ever filled in a form,
	// and would put people in it that staff never added. They appear under
	// Scholarship, and join this list when someone enrols them.
	cond, args := f.where()

	var total int
	if err := h.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM users u WHERE `+cond, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count users: %w", err)
	}

	args = append(args, pageSize, offset)
	query := fmt.Sprintf(`
		SELECT u.id::text, u.email, u.name, u.role, u.created_at,
		       COALESCE(array_agg(uc.course_id) FILTER (WHERE uc.course_id IS NOT NULL), '{}') AS course_ids,
		       COALESCE((
		         SELECT json_agg(json_build_object('id', c.id, 'name', c.name, 'role', m.role) ORDER BY c.name)
		         FROM   company_members m JOIN companies c ON c.id = m.company_id
		         WHERE  m.user_id = u.id
		       ), '[]'::json) AS companies
		FROM users u
		LEFT JOIN user_courses uc ON uc.user_id = u.id
		WHERE %s
		GROUP BY u.id, u.email, u.name, u.role, u.created_at
		ORDER BY u.created_at DESC
		LIMIT $%d OFFSET $%d`, cond, len(args)-1, len(args))

	pgRows, err := h.Pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list users: %w", err)
	}
	defer pgRows.Close()

	var users []adminUserRow
	for pgRows.Next() {
		var u adminUserRow
		var createdAt time.Time
		var courseIDs []string
		var companies []byte
		if err := pgRows.Scan(&u.ID, &u.Email, &u.Name, &u.Role, &createdAt, &courseIDs, &companies); err != nil {
			return nil, 0, fmt.Errorf("scan user: %w", err)
		}
		u.CreatedAt = createdAt.Format(time.RFC3339)
		if courseIDs == nil {
			courseIDs = []string{}
		}
		u.CourseIDs = courseIDs
		u.Companies = json.RawMessage(companies)
		users = append(users, u)
	}
	if users == nil {
		users = []adminUserRow{}
	}
	return users, total, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// JSON helpers
// ─────────────────────────────────────────────────────────────────────────────

func (h *AdminHandler) json(w http.ResponseWriter, status int, v interface{}) {
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v) //nolint:errcheck
}

func (h *AdminHandler) jsonErr(w http.ResponseWriter, status int, msg string) {
	h.json(w, status, map[string]string{"error": msg})
}

// Ensure pgx is used (avoid import cycle if pool comes from api-gateway's own init).
var _ = pgx.ErrNoRows
