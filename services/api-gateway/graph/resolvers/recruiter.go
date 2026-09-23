package resolvers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"go.uber.org/zap"
	"google.golang.org/grpc/status"

	"github.com/knovate211/api-gateway/middleware"
	assessmentv1 "github.com/knovate211/proto/assessment/v1"
)

// RecruiterHandler serves /api/recruiter/* for test authoring, candidate
// invitations, results and shortlisting.
//
// REST rather than GraphQL to match the existing admin surface, and because
// this side of the product deals in spreadsheet uploads and CSV downloads.
//
// Authorization is two-layered: the role must be recruiter or admin, and every
// route that names a resource re-checks company membership through the
// assessment service. A company id in the request body is never trusted.
type RecruiterHandler struct {
	AssessmentSvc assessmentv1.AssessmentServiceClient
	Log           *zap.Logger
}

func (h *RecruiterHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	role := middleware.RoleFromContext(r.Context())
	userID := middleware.UserIDFromContext(r.Context())
	if userID == "" {
		h.fail(w, http.StatusUnauthorized, "authentication required")
		return
	}
	if role != "recruiter" && role != "admin" {
		h.fail(w, http.StatusForbidden, "recruiter access required")
		return
	}

	path := strings.TrimRight(strings.TrimPrefix(r.URL.Path, "/api/recruiter"), "/")
	seg := strings.Split(strings.TrimPrefix(path, "/"), "/")

	switch {
	// ── Companies ─────────────────────────────────────────────────────────────
	case path == "/companies" && r.Method == http.MethodGet:
		h.listCompanies(w, r, role, userID)
	case path == "/companies" && r.Method == http.MethodPost:
		h.adminOnly(w, r, role, h.createCompany)
	case len(seg) == 3 && seg[0] == "companies" && seg[2] == "members" && r.Method == http.MethodGet:
		h.listCompanyMembers(w, r, role, userID, seg[1])
	case len(seg) == 3 && seg[0] == "companies" && seg[2] == "members" && r.Method == http.MethodPost:
		h.adminOnly(w, r, role, func(w http.ResponseWriter, r *http.Request) { h.addCompanyMember(w, r, seg[1]) })

	// ── MCQ bank ──────────────────────────────────────────────────────────────
	case path == "/mcq-bank" && r.Method == http.MethodGet:
		h.listMcq(w, r)
	case path == "/mcq-bank" && r.Method == http.MethodPost:
		h.upsertMcq(w, r, userID)
	case path == "/mcq-bank/import" && r.Method == http.MethodPost:
		h.importMcq(w, r, userID)
	case len(seg) == 2 && seg[0] == "mcq-bank" && r.Method == http.MethodDelete:
		h.deleteMcq(w, r, seg[1])

	// ── Assessments ───────────────────────────────────────────────────────────
	case path == "/assessments" && r.Method == http.MethodGet:
		h.listAssessments(w, r, role, userID)
	case path == "/assessments" && r.Method == http.MethodPost:
		h.createAssessment(w, r, role, userID)
	case len(seg) == 2 && seg[0] == "assessments" && r.Method == http.MethodGet:
		h.guard(w, r, role, userID, seg[1], func() { h.getAssessment(w, r, seg[1]) })
	case len(seg) == 2 && seg[0] == "assessments" && r.Method == http.MethodPatch:
		h.guard(w, r, role, userID, seg[1], func() { h.updateAssessment(w, r, seg[1], userID) })
	case len(seg) == 2 && seg[0] == "assessments" && r.Method == http.MethodDelete:
		h.guard(w, r, role, userID, seg[1], func() { h.deleteAssessment(w, r, seg[1], userID) })
	case len(seg) == 3 && seg[0] == "assessments" && seg[2] == "publish" && r.Method == http.MethodPost:
		h.guard(w, r, role, userID, seg[1], func() { h.publishAssessment(w, r, seg[1], userID) })
	case len(seg) == 3 && seg[0] == "assessments" && seg[2] == "sections" && r.Method == http.MethodPost:
		h.guard(w, r, role, userID, seg[1], func() { h.upsertSection(w, r, seg[1], userID) })
	case len(seg) == 4 && seg[0] == "assessments" && seg[2] == "sections" && r.Method == http.MethodDelete:
		h.guard(w, r, role, userID, seg[1], func() { h.deleteSection(w, r, seg[3], userID) })
	case len(seg) == 5 && seg[0] == "assessments" && seg[2] == "sections" && seg[4] == "questions" && r.Method == http.MethodPut:
		h.guard(w, r, role, userID, seg[1], func() { h.setSectionQuestions(w, r, seg[3], userID) })

	// ── Invitations ───────────────────────────────────────────────────────────
	case len(seg) == 3 && seg[0] == "assessments" && seg[2] == "invite" && r.Method == http.MethodPost:
		h.guard(w, r, role, userID, seg[1], func() { h.invite(w, r, seg[1], userID) })
	case len(seg) == 3 && seg[0] == "assessments" && seg[2] == "invites" && r.Method == http.MethodGet:
		h.guard(w, r, role, userID, seg[1], func() { h.listInvites(w, r, seg[1]) })

	// ── Results ───────────────────────────────────────────────────────────────
	case len(seg) == 3 && seg[0] == "assessments" && seg[2] == "attempts" && r.Method == http.MethodGet:
		h.guard(w, r, role, userID, seg[1], func() { h.listAttempts(w, r, seg[1]) })
	case len(seg) == 3 && seg[0] == "assessments" && seg[2] == "export.csv" && r.Method == http.MethodGet:
		h.guard(w, r, role, userID, seg[1], func() { h.exportResults(w, r, seg[1]) })
	case len(seg) == 3 && seg[0] == "attempts" && seg[2] == "report" && r.Method == http.MethodGet:
		h.guardAttempt(w, r, role, userID, seg[1], func() { h.attemptReport(w, r, seg[1]) })
	case len(seg) == 4 && seg[0] == "attempts" && seg[2] == "questions" && r.Method == http.MethodPatch:
		h.guardAttempt(w, r, role, userID, seg[1], func() { h.gradeDescriptive(w, r, seg[1], seg[3], userID) })

	// ── Shortlisting ──────────────────────────────────────────────────────────
	case len(seg) == 3 && seg[0] == "assessments" && seg[2] == "shortlist" && r.Method == http.MethodPost:
		h.guard(w, r, role, userID, seg[1], func() { h.computeShortlist(w, r, seg[1], userID) })
	case len(seg) == 3 && seg[0] == "assessments" && seg[2] == "shortlists" && r.Method == http.MethodGet:
		h.guard(w, r, role, userID, seg[1], func() { h.listShortlists(w, r, seg[1]) })
	case len(seg) == 2 && seg[0] == "shortlists" && r.Method == http.MethodGet:
		h.guardShortlist(w, r, role, userID, seg[1], func() { h.getShortlist(w, r, seg[1]) })
	case len(seg) == 4 && seg[0] == "shortlists" && seg[2] == "entries" && r.Method == http.MethodPatch:
		h.guardShortlist(w, r, role, userID, seg[1], func() { h.setDecision(w, r, seg[1], seg[3], userID) })

	default:
		h.fail(w, http.StatusNotFound, "unknown recruiter endpoint")
	}
}

// ─── Authorization helpers ────────────────────────────────────────────────────

// guard checks that the caller may administer this assessment before running
// the handler.
func (h *RecruiterHandler) guard(w http.ResponseWriter, r *http.Request, role, userID, assessmentID string, next func()) {
	resp, err := h.AssessmentSvc.Authorize(r.Context(), &assessmentv1.AuthorizeRequest{
		UserId: userID, Role: role, AssessmentId: assessmentID,
	})
	if err != nil {
		h.grpcFail(w, err, "authorization check failed")
		return
	}
	if !resp.Allowed {
		h.fail(w, http.StatusForbidden, resp.Reason)
		return
	}
	next()
}

// guardAttempt resolves the attempt's assessment first, so an attempt id alone
// cannot be used to reach another company's data.
func (h *RecruiterHandler) guardAttempt(w http.ResponseWriter, r *http.Request, role, userID, attemptID string, next func()) {
	report, err := h.AssessmentSvc.GetAttemptReport(r.Context(), &assessmentv1.GetAttemptReportRequest{AttemptId: attemptID})
	if err != nil {
		h.grpcFail(w, err, "attempt not found")
		return
	}
	h.guard(w, r, role, userID, report.Summary.AssessmentId, next)
}

func (h *RecruiterHandler) guardShortlist(w http.ResponseWriter, r *http.Request, role, userID, shortlistID string, next func()) {
	sl, err := h.AssessmentSvc.GetShortlist(r.Context(), &assessmentv1.GetShortlistRequest{Id: shortlistID})
	if err != nil {
		h.grpcFail(w, err, "shortlist not found")
		return
	}
	h.guard(w, r, role, userID, sl.AssessmentId, next)
}

func (h *RecruiterHandler) adminOnly(w http.ResponseWriter, r *http.Request, role string, next func(http.ResponseWriter, *http.Request)) {
	if role != "admin" {
		h.fail(w, http.StatusForbidden, "admin access required")
		return
	}
	next(w, r)
}

// ─── Companies ────────────────────────────────────────────────────────────────

func (h *RecruiterHandler) listCompanies(w http.ResponseWriter, r *http.Request, role, userID string) {
	req := &assessmentv1.ListCompaniesRequest{}
	if role != "admin" {
		req.UserId = userID // recruiters see only their own companies
	}
	resp, err := h.AssessmentSvc.ListCompanies(r.Context(), req)
	h.respond(w, resp, err, "could not list companies")
}

func (h *RecruiterHandler) createCompany(w http.ResponseWriter, r *http.Request) {
	var req assessmentv1.CreateCompanyRequest
	if !h.decode(w, r, &req) {
		return
	}
	resp, err := h.AssessmentSvc.CreateCompany(r.Context(), &req)
	h.respond(w, resp, err, "could not create company")
}

func (h *RecruiterHandler) listCompanyMembers(w http.ResponseWriter, r *http.Request, role, userID, companyID string) {
	auth, err := h.AssessmentSvc.Authorize(r.Context(), &assessmentv1.AuthorizeRequest{
		UserId: userID, Role: role, CompanyId: companyID,
	})
	if err != nil || !auth.Allowed {
		h.fail(w, http.StatusForbidden, "not a member of this company")
		return
	}
	resp, err := h.AssessmentSvc.ListCompanyMembers(r.Context(), &assessmentv1.ListCompanyMembersRequest{CompanyId: companyID})
	h.respond(w, resp, err, "could not list members")
}

func (h *RecruiterHandler) addCompanyMember(w http.ResponseWriter, r *http.Request, companyID string) {
	var req assessmentv1.AddCompanyMemberRequest
	if !h.decode(w, r, &req) {
		return
	}
	req.CompanyId = companyID
	resp, err := h.AssessmentSvc.AddCompanyMember(r.Context(), &req)
	h.respond(w, resp, err, "could not add member")
}

// ─── MCQ bank ─────────────────────────────────────────────────────────────────

func (h *RecruiterHandler) listMcq(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	resp, err := h.AssessmentSvc.ListMcqQuestions(r.Context(), &assessmentv1.ListMcqQuestionsRequest{
		CompanyId:      q.Get("companyId"),
		CourseId:       q.Get("course"),
		IncludeRetired: q.Get("includeRetired") == "true",
		Topic:          q.Get("topic"),
		Difficulty:     q.Get("difficulty"),
		Search:         q.Get("search"),
		Page:           intQuery(q.Get("page"), 1),
		PageSize:       intQuery(q.Get("pageSize"), 50),
	})
	h.respond(w, resp, err, "could not load the question bank")
}

func (h *RecruiterHandler) upsertMcq(w http.ResponseWriter, r *http.Request, userID string) {
	var q assessmentv1.McqQuestion
	if !h.decode(w, r, &q) {
		return
	}
	resp, err := h.AssessmentSvc.UpsertMcqQuestion(r.Context(), &assessmentv1.UpsertMcqQuestionRequest{
		ActorId: userID, Question: &q,
	})
	h.respond(w, resp, err, "could not save the question")
}

func (h *RecruiterHandler) importMcq(w http.ResponseWriter, r *http.Request, userID string) {
	var req assessmentv1.BulkImportMcqRequest
	if !h.decode(w, r, &req) {
		return
	}
	req.ActorId = userID
	resp, err := h.AssessmentSvc.BulkImportMcq(r.Context(), &req)
	h.respond(w, resp, err, "could not import questions")
}

func (h *RecruiterHandler) deleteMcq(w http.ResponseWriter, r *http.Request, id string) {
	resp, err := h.AssessmentSvc.DeleteMcqQuestion(r.Context(), &assessmentv1.DeleteMcqQuestionRequest{Id: id})
	h.respond(w, resp, err, "could not delete the question")
}

// ─── Assessments ──────────────────────────────────────────────────────────────

func (h *RecruiterHandler) listAssessments(w http.ResponseWriter, r *http.Request, role, userID string) {
	q := r.URL.Query()
	companyID := q.Get("companyId")

	// A recruiter may only list within a company they belong to. Without an
	// explicit company filter, scope the listing to their memberships.
	if role != "admin" {
		if companyID != "" {
			auth, err := h.AssessmentSvc.Authorize(r.Context(), &assessmentv1.AuthorizeRequest{
				UserId: userID, Role: role, CompanyId: companyID,
			})
			if err != nil || !auth.Allowed {
				h.fail(w, http.StatusForbidden, "not a member of this company")
				return
			}
		} else {
			h.listAssessmentsForMemberships(w, r, userID, q)
			return
		}
	}

	resp, err := h.AssessmentSvc.ListAssessments(r.Context(), &assessmentv1.ListAssessmentsRequest{
		CompanyId: companyID,
		Purpose:   q.Get("purpose"),
		Status:    q.Get("status"),
		Page:      intQuery(q.Get("page"), 1),
		PageSize:  intQuery(q.Get("pageSize"), 50),
	})
	h.respond(w, resp, err, "could not list assessments")
}

// listAssessmentsForMemberships unions the assessments of every company the
// recruiter belongs to.
func (h *RecruiterHandler) listAssessmentsForMemberships(w http.ResponseWriter, r *http.Request, userID string, q map[string][]string) {
	companies, err := h.AssessmentSvc.ListCompanies(r.Context(), &assessmentv1.ListCompaniesRequest{UserId: userID})
	if err != nil {
		h.grpcFail(w, err, "could not list companies")
		return
	}

	out := &assessmentv1.ListAssessmentsResponse{Assessments: []*assessmentv1.Assessment{}}
	for _, c := range companies.Companies {
		resp, err := h.AssessmentSvc.ListAssessments(r.Context(), &assessmentv1.ListAssessmentsRequest{
			CompanyId: c.Id,
			Purpose:   first(q["purpose"]),
			Status:    first(q["status"]),
			PageSize:  200,
		})
		if err != nil {
			h.grpcFail(w, err, "could not list assessments")
			return
		}
		out.Assessments = append(out.Assessments, resp.Assessments...)
		out.Total += resp.Total
	}
	h.respond(w, out, nil, "")
}

func (h *RecruiterHandler) createAssessment(w http.ResponseWriter, r *http.Request, role, userID string) {
	var a assessmentv1.Assessment
	if !h.decode(w, r, &a) {
		return
	}
	// The owning company must be one the caller belongs to; this is the one
	// place a client-supplied company id is accepted, and it is checked here.
	if a.CompanyId != "" {
		auth, err := h.AssessmentSvc.Authorize(r.Context(), &assessmentv1.AuthorizeRequest{
			UserId: userID, Role: role, CompanyId: a.CompanyId,
		})
		if err != nil || !auth.Allowed {
			h.fail(w, http.StatusForbidden, "not a member of this company")
			return
		}
	} else if role != "admin" {
		h.fail(w, http.StatusForbidden, "a recruiter must create tests under a company")
		return
	}

	resp, err := h.AssessmentSvc.CreateAssessment(r.Context(), &assessmentv1.CreateAssessmentRequest{
		ActorId: userID, Assessment: &a,
	})
	h.respond(w, resp, err, "could not create the test")
}

func (h *RecruiterHandler) getAssessment(w http.ResponseWriter, r *http.Request, id string) {
	resp, err := h.AssessmentSvc.GetAssessment(r.Context(), &assessmentv1.GetAssessmentRequest{
		Id: id, IncludeAnswerKey: true,
	})
	h.respond(w, resp, err, "could not load the test")
}

func (h *RecruiterHandler) updateAssessment(w http.ResponseWriter, r *http.Request, id, userID string) {
	var a assessmentv1.Assessment
	if !h.decode(w, r, &a) {
		return
	}
	a.Id = id
	resp, err := h.AssessmentSvc.UpdateAssessment(r.Context(), &assessmentv1.UpdateAssessmentRequest{
		ActorId: userID, Assessment: &a,
	})
	h.respond(w, resp, err, "could not save the test")
}

func (h *RecruiterHandler) deleteAssessment(w http.ResponseWriter, r *http.Request, id, userID string) {
	resp, err := h.AssessmentSvc.DeleteAssessment(r.Context(), &assessmentv1.DeleteAssessmentRequest{Id: id, ActorId: userID})
	h.respond(w, resp, err, "could not delete the test")
}

func (h *RecruiterHandler) publishAssessment(w http.ResponseWriter, r *http.Request, id, userID string) {
	var body struct {
		Publish *bool `json:"publish"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	publish := true
	if body.Publish != nil {
		publish = *body.Publish
	}
	resp, err := h.AssessmentSvc.PublishAssessment(r.Context(), &assessmentv1.PublishAssessmentRequest{
		Id: id, ActorId: userID, Publish: publish,
	})
	h.respond(w, resp, err, "could not publish the test")
}

func (h *RecruiterHandler) upsertSection(w http.ResponseWriter, r *http.Request, assessmentID, userID string) {
	var s assessmentv1.Section
	if !h.decode(w, r, &s) {
		return
	}
	s.AssessmentId = assessmentID
	resp, err := h.AssessmentSvc.UpsertSection(r.Context(), &assessmentv1.UpsertSectionRequest{ActorId: userID, Section: &s})
	h.respond(w, resp, err, "could not save the section")
}

func (h *RecruiterHandler) deleteSection(w http.ResponseWriter, r *http.Request, sectionID, userID string) {
	resp, err := h.AssessmentSvc.DeleteSection(r.Context(), &assessmentv1.DeleteSectionRequest{Id: sectionID, ActorId: userID})
	h.respond(w, resp, err, "could not delete the section")
}

func (h *RecruiterHandler) setSectionQuestions(w http.ResponseWriter, r *http.Request, sectionID, userID string) {
	var body struct {
		Questions []*assessmentv1.SectionQuestion `json:"questions"`
	}
	if !h.decode(w, r, &body) {
		return
	}
	resp, err := h.AssessmentSvc.SetSectionQuestions(r.Context(), &assessmentv1.SetSectionQuestionsRequest{
		SectionId: sectionID, ActorId: userID, Questions: body.Questions,
	})
	h.respond(w, resp, err, "could not save the questions")
}

// ─── Invitations ──────────────────────────────────────────────────────────────

func (h *RecruiterHandler) invite(w http.ResponseWriter, r *http.Request, assessmentID, userID string) {
	var req assessmentv1.InviteCandidatesRequest
	if !h.decode(w, r, &req) {
		return
	}
	req.AssessmentId, req.ActorId = assessmentID, userID
	resp, err := h.AssessmentSvc.InviteCandidates(r.Context(), &req)
	h.respond(w, resp, err, "could not send invitations")
}

func (h *RecruiterHandler) listInvites(w http.ResponseWriter, r *http.Request, assessmentID string) {
	resp, err := h.AssessmentSvc.ListInvites(r.Context(), &assessmentv1.ListInvitesRequest{AssessmentId: assessmentID})
	h.respond(w, resp, err, "could not load invitations")
}

// ─── Results ──────────────────────────────────────────────────────────────────

func (h *RecruiterHandler) listAttempts(w http.ResponseWriter, r *http.Request, assessmentID string) {
	q := r.URL.Query()
	minScore, _ := strconv.ParseFloat(q.Get("minScore"), 64)
	resp, err := h.AssessmentSvc.ListAttempts(r.Context(), &assessmentv1.ListAttemptsRequest{
		AssessmentId: assessmentID,
		Status:       q.Get("status"),
		MinScore:     minScore,
		Search:       q.Get("search"),
		SortBy:       q.Get("sortBy"),
		SortDir:      q.Get("sortDir"),
		Page:         intQuery(q.Get("page"), 1),
		PageSize:     intQuery(q.Get("pageSize"), 50),
	})
	h.respond(w, resp, err, "could not load candidates")
}

func (h *RecruiterHandler) attemptReport(w http.ResponseWriter, r *http.Request, attemptID string) {
	resp, err := h.AssessmentSvc.GetAttemptReport(r.Context(), &assessmentv1.GetAttemptReportRequest{AttemptId: attemptID})
	h.respond(w, resp, err, "could not load the report")
}

func (h *RecruiterHandler) gradeDescriptive(w http.ResponseWriter, r *http.Request, attemptID, questionID, userID string) {
	var body struct {
		Marks float64 `json:"marks"`
	}
	if !h.decode(w, r, &body) {
		return
	}
	resp, err := h.AssessmentSvc.GradeDescriptive(r.Context(), &assessmentv1.GradeDescriptiveRequest{
		AttemptId: attemptID, QuestionId: questionID, ActorId: userID, Marks: body.Marks,
	})
	h.respond(w, resp, err, "could not save the mark")
}

func (h *RecruiterHandler) exportResults(w http.ResponseWriter, r *http.Request, assessmentID string) {
	resp, err := h.AssessmentSvc.ExportResults(r.Context(), &assessmentv1.ExportResultsRequest{
		AssessmentId: assessmentID,
		ShortlistId:  r.URL.Query().Get("shortlistId"),
	})
	if err != nil {
		h.grpcFail(w, err, "could not export results")
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", resp.Filename))
	_, _ = w.Write([]byte(resp.Csv))
}

// ─── Shortlisting ─────────────────────────────────────────────────────────────

func (h *RecruiterHandler) computeShortlist(w http.ResponseWriter, r *http.Request, assessmentID, userID string) {
	var req assessmentv1.ComputeShortlistRequest
	if !h.decode(w, r, &req) {
		return
	}
	req.AssessmentId, req.ActorId = assessmentID, userID
	resp, err := h.AssessmentSvc.ComputeShortlist(r.Context(), &req)
	h.respond(w, resp, err, "could not compute the shortlist")
}

func (h *RecruiterHandler) listShortlists(w http.ResponseWriter, r *http.Request, assessmentID string) {
	resp, err := h.AssessmentSvc.ListShortlists(r.Context(), &assessmentv1.ListShortlistsRequest{AssessmentId: assessmentID})
	h.respond(w, resp, err, "could not load shortlists")
}

func (h *RecruiterHandler) getShortlist(w http.ResponseWriter, r *http.Request, id string) {
	resp, err := h.AssessmentSvc.GetShortlist(r.Context(), &assessmentv1.GetShortlistRequest{Id: id})
	h.respond(w, resp, err, "could not load the shortlist")
}

func (h *RecruiterHandler) setDecision(w http.ResponseWriter, r *http.Request, shortlistID, attemptID, userID string) {
	var body struct {
		Decision string `json:"decision"`
		Notes    string `json:"notes"`
	}
	if !h.decode(w, r, &body) {
		return
	}
	resp, err := h.AssessmentSvc.SetCandidateDecision(r.Context(), &assessmentv1.SetCandidateDecisionRequest{
		ShortlistId: shortlistID, AttemptId: attemptID, ActorId: userID,
		Decision: body.Decision, Notes: body.Notes,
	})
	h.respond(w, resp, err, "could not save the decision")
}

// ─── Plumbing ─────────────────────────────────────────────────────────────────

func (h *RecruiterHandler) decode(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		h.fail(w, http.StatusBadRequest, "invalid request body")
		return false
	}
	return true
}

func (h *RecruiterHandler) respond(w http.ResponseWriter, payload any, err error, fallback string) {
	if err != nil {
		h.grpcFail(w, err, fallback)
		return
	}
	_ = json.NewEncoder(w).Encode(payload)
}

// grpcFail translates a gRPC status into the matching HTTP status, so the UI
// can tell "you may not do that" from "that does not exist" from "we broke".
func (h *RecruiterHandler) grpcFail(w http.ResponseWriter, err error, fallback string) {
	st, ok := status.FromError(err)
	if !ok {
		h.fail(w, http.StatusInternalServerError, fallback)
		return
	}
	code := http.StatusInternalServerError
	switch st.Code().String() {
	case "NotFound":
		code = http.StatusNotFound
	case "PermissionDenied":
		code = http.StatusForbidden
	case "InvalidArgument":
		code = http.StatusBadRequest
	case "FailedPrecondition", "ResourceExhausted":
		code = http.StatusConflict
	}
	if code == http.StatusInternalServerError {
		h.Log.Error("recruiter request failed", zap.Error(err))
	}
	h.fail(w, code, st.Message())
}

func (h *RecruiterHandler) fail(w http.ResponseWriter, code int, msg string) {
	if msg == "" {
		msg = http.StatusText(code)
	}
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func intQuery(v string, fallback int32) int32 {
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return fallback
	}
	return int32(n)
}

func first(v []string) string {
	if len(v) == 0 {
		return ""
	}
	return v[0]
}
