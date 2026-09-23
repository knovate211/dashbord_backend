// Package assessmentv1 contains the Assessment service types and gRPC service
// definitions. Like the other services in this workspace it is hand-written
// against the JSON codec in proto/codec — there is no generated protobuf code.
//
// The Assessment service owns placement tests: the MCQ/descriptive question
// bank, test composition (mixed MCQ + coding sections), attempts, timing,
// proctoring, scoring and shortlisting. Coding questions are *not* duplicated
// here — sections reference problem-service problems by id, and code is graded
// by the existing submission → execution → judge pipeline.
package assessmentv1

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ─── Companies ────────────────────────────────────────────────────────────────

type Company struct {
	Id        string `json:"id"`
	Name      string `json:"name"`
	Slug      string `json:"slug"`
	LogoUrl   string `json:"logo_url"`
	Website   string `json:"website"`
	CreatedAt string `json:"created_at"`
}

type CreateCompanyRequest struct {
	Name    string `json:"name"`
	Slug    string `json:"slug"`
	LogoUrl string `json:"logo_url"`
	Website string `json:"website"`
}

type ListCompaniesRequest struct {
	// UserId, when set, restricts the result to companies the user belongs to.
	UserId string `json:"user_id,omitempty"`
}

type ListCompaniesResponse struct {
	Companies []*Company `json:"companies"`
}

type CompanyMember struct {
	CompanyId string `json:"company_id"`
	UserId    string `json:"user_id"`
	Email     string `json:"email,omitempty"`
	Name      string `json:"name,omitempty"`
	Role      string `json:"role"` // recruiter | owner
}

type AddCompanyMemberRequest struct {
	CompanyId string `json:"company_id"`
	UserId    string `json:"user_id"`
	Role      string `json:"role"`
}

type ListCompanyMembersRequest struct {
	CompanyId string `json:"company_id"`
}

type ListCompanyMembersResponse struct {
	Members []*CompanyMember `json:"members"`
}

// ─── MCQ bank ─────────────────────────────────────────────────────────────────

type McqOption struct {
	Id         string `json:"id,omitempty"`
	Body       string `json:"body"`
	IsCorrect  bool   `json:"is_correct,omitempty"`
	OrderIndex int32  `json:"order_index"`
}

type McqQuestion struct {
	Id        string `json:"id,omitempty"`
	CompanyId string `json:"company_id,omitempty"` // empty = platform bank
	// CourseId ties the question to one course ('5', 'genai', …); empty is a
	// general question usable by any course.
	CourseId    string       `json:"course_id"`
	Topic       string       `json:"topic"`
	Difficulty  string       `json:"difficulty"` // Easy | Medium | Hard
	Body        string       `json:"body"`
	Kind        string       `json:"kind"` // single | multiple | numeric
	Explanation string       `json:"explanation,omitempty"`
	IsActive    bool         `json:"is_active"`
	Options     []*McqOption `json:"options"`
	CreatedBy   string       `json:"created_by,omitempty"`
	CreatedAt   string       `json:"created_at,omitempty"`
}

type UpsertMcqQuestionRequest struct {
	ActorId  string       `json:"actor_id"`
	Question *McqQuestion `json:"question"`
}

type UpsertMcqQuestionResponse struct {
	Id string `json:"id"`
}

type ListMcqQuestionsRequest struct {
	CompanyId string `json:"company_id,omitempty"`
	// CourseId filters to one course; McqGeneralCourse selects the general
	// (course-less) questions; empty means every course.
	CourseId string `json:"course_id,omitempty"`
	// IncludeRetired also returns deleted (soft-deleted) questions.
	IncludeRetired bool   `json:"include_retired,omitempty"`
	Topic          string `json:"topic,omitempty"`
	Difficulty     string `json:"difficulty,omitempty"`
	Search         string `json:"search,omitempty"`
	Page           int32  `json:"page"`
	PageSize       int32  `json:"page_size"`
}

// McqGeneralCourse is the ListMcqQuestionsRequest.CourseId value that selects
// questions with no course. An empty CourseId already means "no filter".
const McqGeneralCourse = "__general"

type ListMcqQuestionsResponse struct {
	Questions []*McqQuestion `json:"questions"`
	Total     int32          `json:"total"`
	Page      int32          `json:"page"`
	PageSize  int32          `json:"page_size"`
}

type DeleteMcqQuestionRequest struct {
	Id string `json:"id"`
}

type BulkImportMcqRequest struct {
	ActorId   string         `json:"actor_id"`
	CompanyId string         `json:"company_id,omitempty"`
	Questions []*McqQuestion `json:"questions"`
}

type BulkImportMcqResponse struct {
	Imported int32    `json:"imported"`
	Failed   int32    `json:"failed"`
	Errors   []string `json:"errors,omitempty"`
}

// ─── Assessment definition ────────────────────────────────────────────────────

// Proctoring holds the per-assessment anti-cheat configuration. It is stored as
// JSONB so new signals can be added without a migration.
type Proctoring struct {
	RequireFullscreen bool  `json:"require_fullscreen"`
	TabSwitchLimit    int32 `json:"tab_switch_limit"` // 0 = unlimited
	BlockCopyPaste    bool  `json:"block_copy_paste"`
	Webcam            bool  `json:"webcam"`
}

type SectionQuestion struct {
	Id            string `json:"id,omitempty"`
	SectionId     string `json:"section_id,omitempty"`
	McqQuestionId string `json:"mcq_question_id,omitempty"`
	ProblemId     string `json:"problem_id,omitempty"`
	Marks         int32  `json:"marks"`
	OrderIndex    int32  `json:"order_index"`

	// Denormalized preview fields, filled on authoring reads only.
	Title      string `json:"title,omitempty"`
	Difficulty string `json:"difficulty,omitempty"`
}

type Section struct {
	Id              string `json:"id,omitempty"`
	AssessmentId    string `json:"assessment_id,omitempty"`
	Title           string `json:"title"`
	Kind            string `json:"kind"` // mcq | coding | descriptive
	OrderIndex      int32  `json:"order_index"`
	DurationMinutes int32  `json:"duration_minutes,omitempty"` // 0 = shares global timer
	CutoffMarks     int32  `json:"cutoff_marks"`
	// PickCount > 0 draws that many questions per attempt: from the explicit
	// question list when it is non-empty, otherwise from the MCQ bank filtered
	// by PickTopic/PickDifficulty.
	PickCount int32  `json:"pick_count,omitempty"`
	PickTopic string `json:"pick_topic,omitempty"`
	// PickCourse limits the bank draw to one course's questions; empty = any.
	PickCourse     string             `json:"pick_course,omitempty"`
	PickDifficulty string             `json:"pick_difficulty,omitempty"`
	PickMarks      int32              `json:"pick_marks,omitempty"`
	PartialCredit  bool               `json:"partial_credit"`
	Questions      []*SectionQuestion `json:"questions,omitempty"`
}

type Assessment struct {
	Id               string  `json:"id,omitempty"`
	CompanyId        string  `json:"company_id,omitempty"`
	CompanyName      string  `json:"company_name,omitempty"`
	Title            string  `json:"title"`
	Description      string  `json:"description"`
	Purpose          string  `json:"purpose"` // practice | hiring | scholarship
	DurationMinutes  int32   `json:"duration_minutes"`
	TotalMarks       int32   `json:"total_marks"`
	PassingMarks     int32   `json:"passing_marks"`
	NegativeMarking  float64 `json:"negative_marking"`
	ShuffleQuestions bool    `json:"shuffle_questions"`
	ShuffleOptions   bool    `json:"shuffle_options"`
	AllowBacktrack   bool    `json:"allow_backtrack"`
	// LockForward hides the rest of the paper until the current question is
	// answered. Orthogonal to AllowBacktrack, which governs the other direction.
	LockForward   bool        `json:"lock_forward"`
	RevealResults bool        `json:"reveal_results"`
	Proctoring    *Proctoring `json:"proctoring,omitempty"`
	Status        string      `json:"status"` // draft | published | archived
	OpensAt       string      `json:"opens_at,omitempty"`
	ClosesAt      string      `json:"closes_at,omitempty"`
	MaxAttempts   int32       `json:"max_attempts"`
	CreatedBy     string      `json:"created_by,omitempty"`
	CreatedAt     string      `json:"created_at,omitempty"`
	UpdatedAt     string      `json:"updated_at,omitempty"`
	Sections      []*Section  `json:"sections,omitempty"`

	// Read-only rollups used by list views.
	QuestionCount int32 `json:"question_count,omitempty"`
	// CourseIds limits a practice test to students enrolled in one of these
	// courses; empty means every student. A pointer so an update can tell
	// "not sent" (keep) from "sent empty" (open to everyone).
	CourseIds    *[]string `json:"course_ids,omitempty"`
	AttemptCount int32     `json:"attempt_count,omitempty"`
}

type CreateAssessmentRequest struct {
	ActorId    string      `json:"actor_id"`
	Assessment *Assessment `json:"assessment"`
}

type CreateAssessmentResponse struct {
	Id string `json:"id"`
}

type UpdateAssessmentRequest struct {
	ActorId    string      `json:"actor_id"`
	Assessment *Assessment `json:"assessment"`
}

type GetAssessmentRequest struct {
	Id string `json:"id"`
	// IncludeAnswerKey is honoured only for authoring callers. Student-facing
	// paths must always leave it false.
	IncludeAnswerKey bool `json:"include_answer_key,omitempty"`
}

type ListAssessmentsRequest struct {
	CompanyId string `json:"company_id,omitempty"`
	Purpose   string `json:"purpose,omitempty"`
	Status    string `json:"status,omitempty"`
	Page      int32  `json:"page"`
	PageSize  int32  `json:"page_size"`
}

type ListAssessmentsResponse struct {
	Assessments []*Assessment `json:"assessments"`
	Total       int32         `json:"total"`
}

type PublishAssessmentRequest struct {
	Id      string `json:"id"`
	ActorId string `json:"actor_id"`
	Publish bool   `json:"publish"`
}

type PublishAssessmentResponse struct {
	Status     string   `json:"status"`
	TotalMarks int32    `json:"total_marks"`
	Warnings   []string `json:"warnings,omitempty"`
}

type DeleteAssessmentRequest struct {
	Id      string `json:"id"`
	ActorId string `json:"actor_id"`
}

type UpsertSectionRequest struct {
	ActorId string   `json:"actor_id"`
	Section *Section `json:"section"`
}

type UpsertSectionResponse struct {
	Id string `json:"id"`
}

type DeleteSectionRequest struct {
	Id      string `json:"id"`
	ActorId string `json:"actor_id"`
}

// SetSectionQuestions replaces the whole question list of a section, which is
// how the builder saves a drag-reordered list in one round trip.
type SetSectionQuestionsRequest struct {
	SectionId string             `json:"section_id"`
	ActorId   string             `json:"actor_id"`
	Questions []*SectionQuestion `json:"questions"`
}

type Empty struct{}

// ─── Invitations ──────────────────────────────────────────────────────────────

type Invite struct {
	Id           string `json:"id"`
	AssessmentId string `json:"assessment_id"`
	Email        string `json:"email"`
	UserId       string `json:"user_id,omitempty"`
	Token        string `json:"token,omitempty"`
	Status       string `json:"status"`
	ExpiresAt    string `json:"expires_at,omitempty"`
	SentAt       string `json:"sent_at,omitempty"`
}

type InviteCandidatesRequest struct {
	AssessmentId string   `json:"assessment_id"`
	ActorId      string   `json:"actor_id"`
	Emails       []string `json:"emails"`
	ExpiresAt    string   `json:"expires_at,omitempty"`
}

type InviteCandidatesResponse struct {
	Invites []*Invite `json:"invites"`
	Skipped int32     `json:"skipped"`
}

type ListInvitesRequest struct {
	AssessmentId string `json:"assessment_id"`
}

type ListInvitesResponse struct {
	Invites []*Invite `json:"invites"`
}

// ─── Attempts ─────────────────────────────────────────────────────────────────

// AttemptQuestion is the student-facing projection of one question in a live
// paper. It deliberately carries no answer key: Options never include
// is_correct, and coding questions expose only visible test cases.
type AttemptQuestion struct {
	Id         string  `json:"id"`
	SectionId  string  `json:"section_id"`
	Kind       string  `json:"kind"` // mcq | coding | descriptive
	OrderIndex int32   `json:"order_index"`
	Marks      float64 `json:"marks"`

	// MCQ / descriptive
	Body    string       `json:"body,omitempty"`
	McqKind string       `json:"mcq_kind,omitempty"`
	Options []*McqOption `json:"options,omitempty"`

	// Coding — the full problem statement is fetched separately by id so the
	// paper payload stays small.
	ProblemId    string `json:"problem_id,omitempty"`
	ProblemTitle string `json:"problem_title,omitempty"`

	// Answer state
	SelectedOptionIds []string `json:"selected_option_ids,omitempty"`
	TextAnswer        string   `json:"text_answer,omitempty"`
	SubmissionId      string   `json:"submission_id,omitempty"`
	Language          string   `json:"language,omitempty"`
	Code              string   `json:"code,omitempty"`
	GradingStatus     string   `json:"grading_status"`
	Visited           bool     `json:"visited"`
	MarkedReview      bool     `json:"marked_review"`
	TimeSpentMs       int64    `json:"time_spent_ms"`

	// Scoring — populated only in recruiter reports and revealed results.
	AwardedMarks *float64 `json:"awarded_marks,omitempty"`
}

type AttemptSection struct {
	Id              string `json:"id"`
	Title           string `json:"title"`
	Kind            string `json:"kind"`
	OrderIndex      int32  `json:"order_index"`
	DurationMinutes int32  `json:"duration_minutes,omitempty"`
}

type AttemptState struct {
	AttemptId       string             `json:"attempt_id"`
	AssessmentId    string             `json:"assessment_id"`
	Title           string             `json:"title"`
	Status          string             `json:"status"`
	AllowBacktrack  bool               `json:"allow_backtrack"`
	LockForward     bool               `json:"lock_forward"`
	Proctoring      *Proctoring        `json:"proctoring,omitempty"`
	ServerNow       string             `json:"server_now"`
	ExpiresAt       string             `json:"expires_at"`
	SecondsLeft     int64              `json:"seconds_left"`
	Sections        []*AttemptSection  `json:"sections"`
	Questions       []*AttemptQuestion `json:"questions"`
	MaxScore        float64            `json:"max_score"`
	NegativeMarking float64            `json:"negative_marking"`
	// ResultsWithheld marks a paper whose score is never shown to the person
	// sitting it. Scholarship results are communicated by staff, by email,
	// after review — so the player must not end on a score screen.
	ResultsWithheld bool `json:"results_withheld"`
}

type AttemptSummary struct {
	Id             string  `json:"id"`
	AssessmentId   string  `json:"assessment_id"`
	AssessmentName string  `json:"assessment_name,omitempty"`
	UserId         string  `json:"user_id"`
	UserName       string  `json:"user_name,omitempty"`
	UserEmail      string  `json:"user_email,omitempty"`
	AttemptNo      int32   `json:"attempt_no"`
	Status         string  `json:"status"`
	StartedAt      string  `json:"started_at"`
	SubmittedAt    string  `json:"submitted_at,omitempty"`
	EvaluatedAt    string  `json:"evaluated_at,omitempty"`
	Score          float64 `json:"score"`
	MaxScore       float64 `json:"max_score"`
	Percent        float64 `json:"percent"`
	IntegrityScore float64 `json:"integrity_score"`
	Passed         bool    `json:"passed"`
	// SectionScores maps section id → awarded marks.
	SectionScores map[string]float64 `json:"section_scores,omitempty"`
	// SectionMax maps section id → maximum marks, so the recruiter table can
	// render "12/20" per section without a second query.
	SectionMax map[string]float64 `json:"section_max,omitempty"`
	Decision   string             `json:"decision,omitempty"`
	// Purpose is the parent assessment's purpose. It is not exposed to
	// candidates; it is carried so the candidate-facing paths can tell which
	// summaries must have their score stripped before they leave the service.
	Purpose string `json:"purpose,omitempty"`
}

type StartAttemptRequest struct {
	AssessmentId string `json:"assessment_id"`
	UserId       string `json:"user_id"`
	InviteToken  string `json:"invite_token,omitempty"`
}

type GetAttemptStateRequest struct {
	AttemptId string `json:"attempt_id"`
	UserId    string `json:"user_id"`
}

type SaveAnswerRequest struct {
	AttemptId         string   `json:"attempt_id"`
	UserId            string   `json:"user_id"`
	QuestionId        string   `json:"question_id"`
	SelectedOptionIds []string `json:"selected_option_ids,omitempty"`
	TextAnswer        string   `json:"text_answer,omitempty"`
	TimeSpentMs       int64    `json:"time_spent_ms,omitempty"`
	MarkedReview      bool     `json:"marked_review,omitempty"`
	ClearAnswer       bool     `json:"clear_answer,omitempty"`
	// Coding drafts. Saving these grades nothing — it only keeps the editor's
	// contents so a candidate who navigates away, refreshes or crashes does not
	// lose work in a one-attempt timed exam.
	Language string `json:"language,omitempty"`
	Code     string `json:"code,omitempty"`
}

type SaveAnswerResponse struct {
	Saved       bool  `json:"saved"`
	SecondsLeft int64 `json:"seconds_left"`
}

type RunAttemptCodeRequest struct {
	AttemptId  string `json:"attempt_id"`
	UserId     string `json:"user_id"`
	QuestionId string `json:"question_id"`
	Language   string `json:"language"`
	Code       string `json:"code"`
}

type AttemptTestResult struct {
	Input          string `json:"input"`
	ExpectedOutput string `json:"expected_output"`
	ActualOutput   string `json:"actual_output"`
	Status         string `json:"status"`
	ExecutionMs    int64  `json:"execution_ms"`
	Error          string `json:"error,omitempty"`
}

type RunAttemptCodeResponse struct {
	OverallStatus string               `json:"overall_status"`
	TestResults   []*AttemptTestResult `json:"test_results"`
	CompileError  string               `json:"compile_error,omitempty"`
	RuntimeMs     int64                `json:"runtime_ms"`
}

type SubmitAttemptCodeRequest struct {
	AttemptId  string `json:"attempt_id"`
	UserId     string `json:"user_id"`
	QuestionId string `json:"question_id"`
	Language   string `json:"language"`
	Code       string `json:"code"`
}

type SubmitAttemptCodeResponse struct {
	SubmissionId string `json:"submission_id"`
	SecondsLeft  int64  `json:"seconds_left"`
}

// GetAttemptSubmission lets the player poll for the judge verdict without
// leaking hidden test-case data: only pass counts and status come back.
type GetAttemptSubmissionRequest struct {
	AttemptId    string `json:"attempt_id"`
	UserId       string `json:"user_id"`
	SubmissionId string `json:"submission_id"`
}

type GetAttemptSubmissionResponse struct {
	SubmissionId string `json:"submission_id"`
	Status       string `json:"status"`
	PassedCount  int32  `json:"passed_count"`
	TotalCount   int32  `json:"total_count"`
	CompileError string `json:"compile_error,omitempty"`
	RuntimeMs    int64  `json:"runtime_ms"`
}

type SubmitAttemptRequest struct {
	AttemptId string `json:"attempt_id"`
	UserId    string `json:"user_id"`
	// Reason distinguishes a voluntary submit from an automatic one.
	Reason string `json:"reason,omitempty"` // user | timeout | proctor
}

type RecordProctorEventRequest struct {
	AttemptId string `json:"attempt_id"`
	UserId    string `json:"user_id"`
	Kind      string `json:"kind"`
	Detail    string `json:"detail,omitempty"`
}

type RecordProctorEventResponse struct {
	IntegrityScore float64 `json:"integrity_score"`
	Terminated     bool    `json:"terminated"`
	Warning        string  `json:"warning,omitempty"`
}

type ListAvailableAssessmentsRequest struct {
	UserId string `json:"user_id"`
	Scope  string `json:"scope,omitempty"` // available | invited | completed
}

type AssessmentSummary struct {
	Id              string `json:"id"`
	Title           string `json:"title"`
	Description     string `json:"description"`
	Purpose         string `json:"purpose"`
	CompanyName     string `json:"company_name,omitempty"`
	CompanyLogo     string `json:"company_logo,omitempty"`
	DurationMinutes int32  `json:"duration_minutes"`
	TotalMarks      int32  `json:"total_marks"`
	QuestionCount   int32  `json:"question_count"`
	SectionSummary  string `json:"section_summary"` // e.g. "20 MCQ · 2 Coding"
	OpensAt         string `json:"opens_at,omitempty"`
	ClosesAt        string `json:"closes_at,omitempty"`
	MaxAttempts     int32  `json:"max_attempts"`
	AttemptsUsed    int32  `json:"attempts_used"`
	InviteToken     string `json:"invite_token,omitempty"`
	// LiveAttemptId is set when the student has an in-progress attempt to resume.
	LiveAttemptId string `json:"live_attempt_id,omitempty"`
	CanStart      bool   `json:"can_start"`
	BlockedReason string `json:"blocked_reason,omitempty"`
	// Proctoring travels with the summary so a candidate can be told what will
	// be monitored *before* they consent and the clock starts. Learning about a
	// camera at the moment it turns on is not consent.
	Proctoring *Proctoring `json:"proctoring,omitempty"`
}

type ListAvailableAssessmentsResponse struct {
	Assessments []*AssessmentSummary `json:"assessments"`
}

type ListMyAttemptsRequest struct {
	UserId string `json:"user_id"`
}

type ListMyAttemptsResponse struct {
	Attempts []*AttemptSummary `json:"attempts"`
}

type GetAttemptResultRequest struct {
	AttemptId string `json:"attempt_id"`
	UserId    string `json:"user_id"`
}

type AttemptResult struct {
	Summary *AttemptSummary `json:"summary"`
	// Questions is populated only when the assessment allows result reveal.
	Questions []*AttemptQuestion `json:"questions,omitempty"`
	Revealed  bool               `json:"revealed"`
	// Withheld says the score exists but is deliberately not being served to
	// this candidate. Distinct from Revealed, which is about the answer key:
	// a withheld result carries no marks at all, not even the total.
	Withheld bool `json:"withheld"`
}

// ─── Recruiter reporting ──────────────────────────────────────────────────────

type ListAttemptsRequest struct {
	AssessmentId string  `json:"assessment_id"`
	Status       string  `json:"status,omitempty"`
	MinScore     float64 `json:"min_score,omitempty"`
	Search       string  `json:"search,omitempty"`
	SortBy       string  `json:"sort_by,omitempty"`  // score | submitted_at | integrity
	SortDir      string  `json:"sort_dir,omitempty"` // asc | desc
	Page         int32   `json:"page"`
	PageSize     int32   `json:"page_size"`
}

type ListAttemptsResponse struct {
	Attempts []*AttemptSummary `json:"attempts"`
	Total    int32             `json:"total"`
	Page     int32             `json:"page"`
	PageSize int32             `json:"page_size"`
}

type ProctorEvent struct {
	Kind       string `json:"kind"`
	Detail     string `json:"detail,omitempty"`
	OccurredAt string `json:"occurred_at"`
}

type AttemptReport struct {
	Summary       *AttemptSummary    `json:"summary"`
	Questions     []*AttemptQuestion `json:"questions"`
	ProctorEvents []*ProctorEvent    `json:"proctor_events"`
}

type GetAttemptReportRequest struct {
	AttemptId string `json:"attempt_id"`
}

type GradeDescriptiveRequest struct {
	AttemptId  string  `json:"attempt_id"`
	QuestionId string  `json:"question_id"`
	ActorId    string  `json:"actor_id"`
	Marks      float64 `json:"marks"`
}

type GradeDescriptiveResponse struct {
	Score    float64 `json:"score"`
	MaxScore float64 `json:"max_score"`
}

// ─── Shortlisting ─────────────────────────────────────────────────────────────

// ShortlistCriteria is the rule set a recruiter applies to rank and cut the
// candidate pool. Zero values mean "no constraint".
type ShortlistCriteria struct {
	MinScorePercent float64            `json:"min_score_percent"`
	SectionCutoffs  map[string]float64 `json:"section_cutoffs,omitempty"` // section id → min marks
	MinIntegrity    float64            `json:"min_integrity"`
	TopN            int32              `json:"top_n"`
	ExcludeFlagged  bool               `json:"exclude_flagged"`
}

type ShortlistEntry struct {
	AttemptId string  `json:"attempt_id"`
	UserId    string  `json:"user_id"`
	UserName  string  `json:"user_name,omitempty"`
	UserEmail string  `json:"user_email,omitempty"`
	Rank      int32   `json:"rank"`
	Score     float64 `json:"score"`
	MaxScore  float64 `json:"max_score"`
	Percent   float64 `json:"percent"`
	Integrity float64 `json:"integrity_score"`
	Decision  string  `json:"decision"`
	Notes     string  `json:"notes,omitempty"`
}

type Shortlist struct {
	Id           string             `json:"id,omitempty"`
	AssessmentId string             `json:"assessment_id"`
	Name         string             `json:"name"`
	Criteria     *ShortlistCriteria `json:"criteria"`
	CreatedBy    string             `json:"created_by,omitempty"`
	CreatedAt    string             `json:"created_at,omitempty"`
	Entries      []*ShortlistEntry  `json:"entries,omitempty"`
	Total        int32              `json:"total"`
}

// ComputeShortlist previews or persists a shortlist. With Save false it only
// returns the ranked preview, so a recruiter can tune criteria before
// committing.
type ComputeShortlistRequest struct {
	AssessmentId string             `json:"assessment_id"`
	ActorId      string             `json:"actor_id"`
	Name         string             `json:"name,omitempty"`
	Criteria     *ShortlistCriteria `json:"criteria"`
	Save         bool               `json:"save"`
}

type ListShortlistsRequest struct {
	AssessmentId string `json:"assessment_id"`
}

type ListShortlistsResponse struct {
	Shortlists []*Shortlist `json:"shortlists"`
}

type GetShortlistRequest struct {
	Id string `json:"id"`
}

type SetCandidateDecisionRequest struct {
	ShortlistId string `json:"shortlist_id"`
	AttemptId   string `json:"attempt_id"`
	ActorId     string `json:"actor_id"`
	Decision    string `json:"decision"` // shortlisted | rejected | on_hold | hired
	Notes       string `json:"notes,omitempty"`
}

type ExportResultsRequest struct {
	AssessmentId string `json:"assessment_id"`
	ShortlistId  string `json:"shortlist_id,omitempty"`
}

type ExportResultsResponse struct {
	Filename string `json:"filename"`
	Csv      string `json:"csv"`
}

// ─── Authorization helper ─────────────────────────────────────────────────────

// AuthorizeRequest answers "may this user administer this assessment?" so the
// gateway can enforce company scoping without owning the membership tables.
type AuthorizeRequest struct {
	UserId       string `json:"user_id"`
	Role         string `json:"role"`
	AssessmentId string `json:"assessment_id,omitempty"`
	CompanyId    string `json:"company_id,omitempty"`
}

type AuthorizeResponse struct {
	Allowed bool   `json:"allowed"`
	Reason  string `json:"reason,omitempty"`
}

// ─── Server Interface ─────────────────────────────────────────────────────────

type AssessmentServiceServer interface {
	// Companies
	CreateCompany(context.Context, *CreateCompanyRequest) (*Company, error)
	ListCompanies(context.Context, *ListCompaniesRequest) (*ListCompaniesResponse, error)
	AddCompanyMember(context.Context, *AddCompanyMemberRequest) (*Empty, error)
	ListCompanyMembers(context.Context, *ListCompanyMembersRequest) (*ListCompanyMembersResponse, error)

	// MCQ bank
	UpsertMcqQuestion(context.Context, *UpsertMcqQuestionRequest) (*UpsertMcqQuestionResponse, error)
	ListMcqQuestions(context.Context, *ListMcqQuestionsRequest) (*ListMcqQuestionsResponse, error)
	DeleteMcqQuestion(context.Context, *DeleteMcqQuestionRequest) (*Empty, error)
	BulkImportMcq(context.Context, *BulkImportMcqRequest) (*BulkImportMcqResponse, error)

	// Authoring
	CreateAssessment(context.Context, *CreateAssessmentRequest) (*CreateAssessmentResponse, error)
	UpdateAssessment(context.Context, *UpdateAssessmentRequest) (*Empty, error)
	GetAssessment(context.Context, *GetAssessmentRequest) (*Assessment, error)
	ListAssessments(context.Context, *ListAssessmentsRequest) (*ListAssessmentsResponse, error)
	PublishAssessment(context.Context, *PublishAssessmentRequest) (*PublishAssessmentResponse, error)
	DeleteAssessment(context.Context, *DeleteAssessmentRequest) (*Empty, error)
	UpsertSection(context.Context, *UpsertSectionRequest) (*UpsertSectionResponse, error)
	DeleteSection(context.Context, *DeleteSectionRequest) (*Empty, error)
	SetSectionQuestions(context.Context, *SetSectionQuestionsRequest) (*Empty, error)

	// Invitations
	InviteCandidates(context.Context, *InviteCandidatesRequest) (*InviteCandidatesResponse, error)
	ListInvites(context.Context, *ListInvitesRequest) (*ListInvitesResponse, error)

	// Taking
	ListAvailableAssessments(context.Context, *ListAvailableAssessmentsRequest) (*ListAvailableAssessmentsResponse, error)
	StartAttempt(context.Context, *StartAttemptRequest) (*AttemptState, error)
	GetAttemptState(context.Context, *GetAttemptStateRequest) (*AttemptState, error)
	SaveAnswer(context.Context, *SaveAnswerRequest) (*SaveAnswerResponse, error)
	RunAttemptCode(context.Context, *RunAttemptCodeRequest) (*RunAttemptCodeResponse, error)
	SubmitAttemptCode(context.Context, *SubmitAttemptCodeRequest) (*SubmitAttemptCodeResponse, error)
	GetAttemptSubmission(context.Context, *GetAttemptSubmissionRequest) (*GetAttemptSubmissionResponse, error)
	SubmitAttempt(context.Context, *SubmitAttemptRequest) (*AttemptSummary, error)
	RecordProctorEvent(context.Context, *RecordProctorEventRequest) (*RecordProctorEventResponse, error)
	ListMyAttempts(context.Context, *ListMyAttemptsRequest) (*ListMyAttemptsResponse, error)
	GetAttemptResult(context.Context, *GetAttemptResultRequest) (*AttemptResult, error)

	// Reporting & shortlisting
	ListAttempts(context.Context, *ListAttemptsRequest) (*ListAttemptsResponse, error)
	GetAttemptReport(context.Context, *GetAttemptReportRequest) (*AttemptReport, error)
	GradeDescriptive(context.Context, *GradeDescriptiveRequest) (*GradeDescriptiveResponse, error)
	ComputeShortlist(context.Context, *ComputeShortlistRequest) (*Shortlist, error)
	ListShortlists(context.Context, *ListShortlistsRequest) (*ListShortlistsResponse, error)
	GetShortlist(context.Context, *GetShortlistRequest) (*Shortlist, error)
	SetCandidateDecision(context.Context, *SetCandidateDecisionRequest) (*Empty, error)
	ExportResults(context.Context, *ExportResultsRequest) (*ExportResultsResponse, error)

	// Authorization
	Authorize(context.Context, *AuthorizeRequest) (*AuthorizeResponse, error)
}

type UnimplementedAssessmentServiceServer struct{}

func unimpl(name string) error {
	return status.Errorf(codes.Unimplemented, "method %s not implemented", name)
}

func (UnimplementedAssessmentServiceServer) CreateCompany(context.Context, *CreateCompanyRequest) (*Company, error) {
	return nil, unimpl("CreateCompany")
}
func (UnimplementedAssessmentServiceServer) ListCompanies(context.Context, *ListCompaniesRequest) (*ListCompaniesResponse, error) {
	return nil, unimpl("ListCompanies")
}
func (UnimplementedAssessmentServiceServer) AddCompanyMember(context.Context, *AddCompanyMemberRequest) (*Empty, error) {
	return nil, unimpl("AddCompanyMember")
}
func (UnimplementedAssessmentServiceServer) ListCompanyMembers(context.Context, *ListCompanyMembersRequest) (*ListCompanyMembersResponse, error) {
	return nil, unimpl("ListCompanyMembers")
}
func (UnimplementedAssessmentServiceServer) UpsertMcqQuestion(context.Context, *UpsertMcqQuestionRequest) (*UpsertMcqQuestionResponse, error) {
	return nil, unimpl("UpsertMcqQuestion")
}
func (UnimplementedAssessmentServiceServer) ListMcqQuestions(context.Context, *ListMcqQuestionsRequest) (*ListMcqQuestionsResponse, error) {
	return nil, unimpl("ListMcqQuestions")
}
func (UnimplementedAssessmentServiceServer) DeleteMcqQuestion(context.Context, *DeleteMcqQuestionRequest) (*Empty, error) {
	return nil, unimpl("DeleteMcqQuestion")
}
func (UnimplementedAssessmentServiceServer) BulkImportMcq(context.Context, *BulkImportMcqRequest) (*BulkImportMcqResponse, error) {
	return nil, unimpl("BulkImportMcq")
}
func (UnimplementedAssessmentServiceServer) CreateAssessment(context.Context, *CreateAssessmentRequest) (*CreateAssessmentResponse, error) {
	return nil, unimpl("CreateAssessment")
}
func (UnimplementedAssessmentServiceServer) UpdateAssessment(context.Context, *UpdateAssessmentRequest) (*Empty, error) {
	return nil, unimpl("UpdateAssessment")
}
func (UnimplementedAssessmentServiceServer) GetAssessment(context.Context, *GetAssessmentRequest) (*Assessment, error) {
	return nil, unimpl("GetAssessment")
}
func (UnimplementedAssessmentServiceServer) ListAssessments(context.Context, *ListAssessmentsRequest) (*ListAssessmentsResponse, error) {
	return nil, unimpl("ListAssessments")
}
func (UnimplementedAssessmentServiceServer) PublishAssessment(context.Context, *PublishAssessmentRequest) (*PublishAssessmentResponse, error) {
	return nil, unimpl("PublishAssessment")
}
func (UnimplementedAssessmentServiceServer) DeleteAssessment(context.Context, *DeleteAssessmentRequest) (*Empty, error) {
	return nil, unimpl("DeleteAssessment")
}
func (UnimplementedAssessmentServiceServer) UpsertSection(context.Context, *UpsertSectionRequest) (*UpsertSectionResponse, error) {
	return nil, unimpl("UpsertSection")
}
func (UnimplementedAssessmentServiceServer) DeleteSection(context.Context, *DeleteSectionRequest) (*Empty, error) {
	return nil, unimpl("DeleteSection")
}
func (UnimplementedAssessmentServiceServer) SetSectionQuestions(context.Context, *SetSectionQuestionsRequest) (*Empty, error) {
	return nil, unimpl("SetSectionQuestions")
}
func (UnimplementedAssessmentServiceServer) InviteCandidates(context.Context, *InviteCandidatesRequest) (*InviteCandidatesResponse, error) {
	return nil, unimpl("InviteCandidates")
}
func (UnimplementedAssessmentServiceServer) ListInvites(context.Context, *ListInvitesRequest) (*ListInvitesResponse, error) {
	return nil, unimpl("ListInvites")
}
func (UnimplementedAssessmentServiceServer) ListAvailableAssessments(context.Context, *ListAvailableAssessmentsRequest) (*ListAvailableAssessmentsResponse, error) {
	return nil, unimpl("ListAvailableAssessments")
}
func (UnimplementedAssessmentServiceServer) StartAttempt(context.Context, *StartAttemptRequest) (*AttemptState, error) {
	return nil, unimpl("StartAttempt")
}
func (UnimplementedAssessmentServiceServer) GetAttemptState(context.Context, *GetAttemptStateRequest) (*AttemptState, error) {
	return nil, unimpl("GetAttemptState")
}
func (UnimplementedAssessmentServiceServer) SaveAnswer(context.Context, *SaveAnswerRequest) (*SaveAnswerResponse, error) {
	return nil, unimpl("SaveAnswer")
}
func (UnimplementedAssessmentServiceServer) RunAttemptCode(context.Context, *RunAttemptCodeRequest) (*RunAttemptCodeResponse, error) {
	return nil, unimpl("RunAttemptCode")
}
func (UnimplementedAssessmentServiceServer) SubmitAttemptCode(context.Context, *SubmitAttemptCodeRequest) (*SubmitAttemptCodeResponse, error) {
	return nil, unimpl("SubmitAttemptCode")
}
func (UnimplementedAssessmentServiceServer) GetAttemptSubmission(context.Context, *GetAttemptSubmissionRequest) (*GetAttemptSubmissionResponse, error) {
	return nil, unimpl("GetAttemptSubmission")
}
func (UnimplementedAssessmentServiceServer) SubmitAttempt(context.Context, *SubmitAttemptRequest) (*AttemptSummary, error) {
	return nil, unimpl("SubmitAttempt")
}
func (UnimplementedAssessmentServiceServer) RecordProctorEvent(context.Context, *RecordProctorEventRequest) (*RecordProctorEventResponse, error) {
	return nil, unimpl("RecordProctorEvent")
}
func (UnimplementedAssessmentServiceServer) ListMyAttempts(context.Context, *ListMyAttemptsRequest) (*ListMyAttemptsResponse, error) {
	return nil, unimpl("ListMyAttempts")
}
func (UnimplementedAssessmentServiceServer) GetAttemptResult(context.Context, *GetAttemptResultRequest) (*AttemptResult, error) {
	return nil, unimpl("GetAttemptResult")
}
func (UnimplementedAssessmentServiceServer) ListAttempts(context.Context, *ListAttemptsRequest) (*ListAttemptsResponse, error) {
	return nil, unimpl("ListAttempts")
}
func (UnimplementedAssessmentServiceServer) GetAttemptReport(context.Context, *GetAttemptReportRequest) (*AttemptReport, error) {
	return nil, unimpl("GetAttemptReport")
}
func (UnimplementedAssessmentServiceServer) GradeDescriptive(context.Context, *GradeDescriptiveRequest) (*GradeDescriptiveResponse, error) {
	return nil, unimpl("GradeDescriptive")
}
func (UnimplementedAssessmentServiceServer) ComputeShortlist(context.Context, *ComputeShortlistRequest) (*Shortlist, error) {
	return nil, unimpl("ComputeShortlist")
}
func (UnimplementedAssessmentServiceServer) ListShortlists(context.Context, *ListShortlistsRequest) (*ListShortlistsResponse, error) {
	return nil, unimpl("ListShortlists")
}
func (UnimplementedAssessmentServiceServer) GetShortlist(context.Context, *GetShortlistRequest) (*Shortlist, error) {
	return nil, unimpl("GetShortlist")
}
func (UnimplementedAssessmentServiceServer) SetCandidateDecision(context.Context, *SetCandidateDecisionRequest) (*Empty, error) {
	return nil, unimpl("SetCandidateDecision")
}
func (UnimplementedAssessmentServiceServer) ExportResults(context.Context, *ExportResultsRequest) (*ExportResultsResponse, error) {
	return nil, unimpl("ExportResults")
}
func (UnimplementedAssessmentServiceServer) Authorize(context.Context, *AuthorizeRequest) (*AuthorizeResponse, error) {
	return nil, unimpl("Authorize")
}

// ─── Client Interface & Implementation ───────────────────────────────────────

type AssessmentServiceClient interface {
	CreateCompany(ctx context.Context, in *CreateCompanyRequest, opts ...grpc.CallOption) (*Company, error)
	ListCompanies(ctx context.Context, in *ListCompaniesRequest, opts ...grpc.CallOption) (*ListCompaniesResponse, error)
	AddCompanyMember(ctx context.Context, in *AddCompanyMemberRequest, opts ...grpc.CallOption) (*Empty, error)
	ListCompanyMembers(ctx context.Context, in *ListCompanyMembersRequest, opts ...grpc.CallOption) (*ListCompanyMembersResponse, error)
	UpsertMcqQuestion(ctx context.Context, in *UpsertMcqQuestionRequest, opts ...grpc.CallOption) (*UpsertMcqQuestionResponse, error)
	ListMcqQuestions(ctx context.Context, in *ListMcqQuestionsRequest, opts ...grpc.CallOption) (*ListMcqQuestionsResponse, error)
	DeleteMcqQuestion(ctx context.Context, in *DeleteMcqQuestionRequest, opts ...grpc.CallOption) (*Empty, error)
	BulkImportMcq(ctx context.Context, in *BulkImportMcqRequest, opts ...grpc.CallOption) (*BulkImportMcqResponse, error)
	CreateAssessment(ctx context.Context, in *CreateAssessmentRequest, opts ...grpc.CallOption) (*CreateAssessmentResponse, error)
	UpdateAssessment(ctx context.Context, in *UpdateAssessmentRequest, opts ...grpc.CallOption) (*Empty, error)
	GetAssessment(ctx context.Context, in *GetAssessmentRequest, opts ...grpc.CallOption) (*Assessment, error)
	ListAssessments(ctx context.Context, in *ListAssessmentsRequest, opts ...grpc.CallOption) (*ListAssessmentsResponse, error)
	PublishAssessment(ctx context.Context, in *PublishAssessmentRequest, opts ...grpc.CallOption) (*PublishAssessmentResponse, error)
	DeleteAssessment(ctx context.Context, in *DeleteAssessmentRequest, opts ...grpc.CallOption) (*Empty, error)
	UpsertSection(ctx context.Context, in *UpsertSectionRequest, opts ...grpc.CallOption) (*UpsertSectionResponse, error)
	DeleteSection(ctx context.Context, in *DeleteSectionRequest, opts ...grpc.CallOption) (*Empty, error)
	SetSectionQuestions(ctx context.Context, in *SetSectionQuestionsRequest, opts ...grpc.CallOption) (*Empty, error)
	InviteCandidates(ctx context.Context, in *InviteCandidatesRequest, opts ...grpc.CallOption) (*InviteCandidatesResponse, error)
	ListInvites(ctx context.Context, in *ListInvitesRequest, opts ...grpc.CallOption) (*ListInvitesResponse, error)
	ListAvailableAssessments(ctx context.Context, in *ListAvailableAssessmentsRequest, opts ...grpc.CallOption) (*ListAvailableAssessmentsResponse, error)
	StartAttempt(ctx context.Context, in *StartAttemptRequest, opts ...grpc.CallOption) (*AttemptState, error)
	GetAttemptState(ctx context.Context, in *GetAttemptStateRequest, opts ...grpc.CallOption) (*AttemptState, error)
	SaveAnswer(ctx context.Context, in *SaveAnswerRequest, opts ...grpc.CallOption) (*SaveAnswerResponse, error)
	RunAttemptCode(ctx context.Context, in *RunAttemptCodeRequest, opts ...grpc.CallOption) (*RunAttemptCodeResponse, error)
	SubmitAttemptCode(ctx context.Context, in *SubmitAttemptCodeRequest, opts ...grpc.CallOption) (*SubmitAttemptCodeResponse, error)
	GetAttemptSubmission(ctx context.Context, in *GetAttemptSubmissionRequest, opts ...grpc.CallOption) (*GetAttemptSubmissionResponse, error)
	SubmitAttempt(ctx context.Context, in *SubmitAttemptRequest, opts ...grpc.CallOption) (*AttemptSummary, error)
	RecordProctorEvent(ctx context.Context, in *RecordProctorEventRequest, opts ...grpc.CallOption) (*RecordProctorEventResponse, error)
	ListMyAttempts(ctx context.Context, in *ListMyAttemptsRequest, opts ...grpc.CallOption) (*ListMyAttemptsResponse, error)
	GetAttemptResult(ctx context.Context, in *GetAttemptResultRequest, opts ...grpc.CallOption) (*AttemptResult, error)
	ListAttempts(ctx context.Context, in *ListAttemptsRequest, opts ...grpc.CallOption) (*ListAttemptsResponse, error)
	GetAttemptReport(ctx context.Context, in *GetAttemptReportRequest, opts ...grpc.CallOption) (*AttemptReport, error)
	GradeDescriptive(ctx context.Context, in *GradeDescriptiveRequest, opts ...grpc.CallOption) (*GradeDescriptiveResponse, error)
	ComputeShortlist(ctx context.Context, in *ComputeShortlistRequest, opts ...grpc.CallOption) (*Shortlist, error)
	ListShortlists(ctx context.Context, in *ListShortlistsRequest, opts ...grpc.CallOption) (*ListShortlistsResponse, error)
	GetShortlist(ctx context.Context, in *GetShortlistRequest, opts ...grpc.CallOption) (*Shortlist, error)
	SetCandidateDecision(ctx context.Context, in *SetCandidateDecisionRequest, opts ...grpc.CallOption) (*Empty, error)
	ExportResults(ctx context.Context, in *ExportResultsRequest, opts ...grpc.CallOption) (*ExportResultsResponse, error)
	Authorize(ctx context.Context, in *AuthorizeRequest, opts ...grpc.CallOption) (*AuthorizeResponse, error)
}

type assessmentServiceClient struct{ cc grpc.ClientConnInterface }

func NewAssessmentServiceClient(cc grpc.ClientConnInterface) AssessmentServiceClient {
	return &assessmentServiceClient{cc}
}

const svc = "/assessment.v1.AssessmentService/"

func invoke[Out any](c *assessmentServiceClient, ctx context.Context, method string, in interface{}, opts []grpc.CallOption) (*Out, error) {
	out := new(Out)
	if err := c.cc.Invoke(ctx, svc+method, in, out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *assessmentServiceClient) CreateCompany(ctx context.Context, in *CreateCompanyRequest, opts ...grpc.CallOption) (*Company, error) {
	return invoke[Company](c, ctx, "CreateCompany", in, opts)
}
func (c *assessmentServiceClient) ListCompanies(ctx context.Context, in *ListCompaniesRequest, opts ...grpc.CallOption) (*ListCompaniesResponse, error) {
	return invoke[ListCompaniesResponse](c, ctx, "ListCompanies", in, opts)
}
func (c *assessmentServiceClient) AddCompanyMember(ctx context.Context, in *AddCompanyMemberRequest, opts ...grpc.CallOption) (*Empty, error) {
	return invoke[Empty](c, ctx, "AddCompanyMember", in, opts)
}
func (c *assessmentServiceClient) ListCompanyMembers(ctx context.Context, in *ListCompanyMembersRequest, opts ...grpc.CallOption) (*ListCompanyMembersResponse, error) {
	return invoke[ListCompanyMembersResponse](c, ctx, "ListCompanyMembers", in, opts)
}
func (c *assessmentServiceClient) UpsertMcqQuestion(ctx context.Context, in *UpsertMcqQuestionRequest, opts ...grpc.CallOption) (*UpsertMcqQuestionResponse, error) {
	return invoke[UpsertMcqQuestionResponse](c, ctx, "UpsertMcqQuestion", in, opts)
}
func (c *assessmentServiceClient) ListMcqQuestions(ctx context.Context, in *ListMcqQuestionsRequest, opts ...grpc.CallOption) (*ListMcqQuestionsResponse, error) {
	return invoke[ListMcqQuestionsResponse](c, ctx, "ListMcqQuestions", in, opts)
}
func (c *assessmentServiceClient) DeleteMcqQuestion(ctx context.Context, in *DeleteMcqQuestionRequest, opts ...grpc.CallOption) (*Empty, error) {
	return invoke[Empty](c, ctx, "DeleteMcqQuestion", in, opts)
}
func (c *assessmentServiceClient) BulkImportMcq(ctx context.Context, in *BulkImportMcqRequest, opts ...grpc.CallOption) (*BulkImportMcqResponse, error) {
	return invoke[BulkImportMcqResponse](c, ctx, "BulkImportMcq", in, opts)
}
func (c *assessmentServiceClient) CreateAssessment(ctx context.Context, in *CreateAssessmentRequest, opts ...grpc.CallOption) (*CreateAssessmentResponse, error) {
	return invoke[CreateAssessmentResponse](c, ctx, "CreateAssessment", in, opts)
}
func (c *assessmentServiceClient) UpdateAssessment(ctx context.Context, in *UpdateAssessmentRequest, opts ...grpc.CallOption) (*Empty, error) {
	return invoke[Empty](c, ctx, "UpdateAssessment", in, opts)
}
func (c *assessmentServiceClient) GetAssessment(ctx context.Context, in *GetAssessmentRequest, opts ...grpc.CallOption) (*Assessment, error) {
	return invoke[Assessment](c, ctx, "GetAssessment", in, opts)
}
func (c *assessmentServiceClient) ListAssessments(ctx context.Context, in *ListAssessmentsRequest, opts ...grpc.CallOption) (*ListAssessmentsResponse, error) {
	return invoke[ListAssessmentsResponse](c, ctx, "ListAssessments", in, opts)
}
func (c *assessmentServiceClient) PublishAssessment(ctx context.Context, in *PublishAssessmentRequest, opts ...grpc.CallOption) (*PublishAssessmentResponse, error) {
	return invoke[PublishAssessmentResponse](c, ctx, "PublishAssessment", in, opts)
}
func (c *assessmentServiceClient) DeleteAssessment(ctx context.Context, in *DeleteAssessmentRequest, opts ...grpc.CallOption) (*Empty, error) {
	return invoke[Empty](c, ctx, "DeleteAssessment", in, opts)
}
func (c *assessmentServiceClient) UpsertSection(ctx context.Context, in *UpsertSectionRequest, opts ...grpc.CallOption) (*UpsertSectionResponse, error) {
	return invoke[UpsertSectionResponse](c, ctx, "UpsertSection", in, opts)
}
func (c *assessmentServiceClient) DeleteSection(ctx context.Context, in *DeleteSectionRequest, opts ...grpc.CallOption) (*Empty, error) {
	return invoke[Empty](c, ctx, "DeleteSection", in, opts)
}
func (c *assessmentServiceClient) SetSectionQuestions(ctx context.Context, in *SetSectionQuestionsRequest, opts ...grpc.CallOption) (*Empty, error) {
	return invoke[Empty](c, ctx, "SetSectionQuestions", in, opts)
}
func (c *assessmentServiceClient) InviteCandidates(ctx context.Context, in *InviteCandidatesRequest, opts ...grpc.CallOption) (*InviteCandidatesResponse, error) {
	return invoke[InviteCandidatesResponse](c, ctx, "InviteCandidates", in, opts)
}
func (c *assessmentServiceClient) ListInvites(ctx context.Context, in *ListInvitesRequest, opts ...grpc.CallOption) (*ListInvitesResponse, error) {
	return invoke[ListInvitesResponse](c, ctx, "ListInvites", in, opts)
}
func (c *assessmentServiceClient) ListAvailableAssessments(ctx context.Context, in *ListAvailableAssessmentsRequest, opts ...grpc.CallOption) (*ListAvailableAssessmentsResponse, error) {
	return invoke[ListAvailableAssessmentsResponse](c, ctx, "ListAvailableAssessments", in, opts)
}
func (c *assessmentServiceClient) StartAttempt(ctx context.Context, in *StartAttemptRequest, opts ...grpc.CallOption) (*AttemptState, error) {
	return invoke[AttemptState](c, ctx, "StartAttempt", in, opts)
}
func (c *assessmentServiceClient) GetAttemptState(ctx context.Context, in *GetAttemptStateRequest, opts ...grpc.CallOption) (*AttemptState, error) {
	return invoke[AttemptState](c, ctx, "GetAttemptState", in, opts)
}
func (c *assessmentServiceClient) SaveAnswer(ctx context.Context, in *SaveAnswerRequest, opts ...grpc.CallOption) (*SaveAnswerResponse, error) {
	return invoke[SaveAnswerResponse](c, ctx, "SaveAnswer", in, opts)
}
func (c *assessmentServiceClient) RunAttemptCode(ctx context.Context, in *RunAttemptCodeRequest, opts ...grpc.CallOption) (*RunAttemptCodeResponse, error) {
	return invoke[RunAttemptCodeResponse](c, ctx, "RunAttemptCode", in, opts)
}
func (c *assessmentServiceClient) SubmitAttemptCode(ctx context.Context, in *SubmitAttemptCodeRequest, opts ...grpc.CallOption) (*SubmitAttemptCodeResponse, error) {
	return invoke[SubmitAttemptCodeResponse](c, ctx, "SubmitAttemptCode", in, opts)
}
func (c *assessmentServiceClient) GetAttemptSubmission(ctx context.Context, in *GetAttemptSubmissionRequest, opts ...grpc.CallOption) (*GetAttemptSubmissionResponse, error) {
	return invoke[GetAttemptSubmissionResponse](c, ctx, "GetAttemptSubmission", in, opts)
}
func (c *assessmentServiceClient) SubmitAttempt(ctx context.Context, in *SubmitAttemptRequest, opts ...grpc.CallOption) (*AttemptSummary, error) {
	return invoke[AttemptSummary](c, ctx, "SubmitAttempt", in, opts)
}
func (c *assessmentServiceClient) RecordProctorEvent(ctx context.Context, in *RecordProctorEventRequest, opts ...grpc.CallOption) (*RecordProctorEventResponse, error) {
	return invoke[RecordProctorEventResponse](c, ctx, "RecordProctorEvent", in, opts)
}
func (c *assessmentServiceClient) ListMyAttempts(ctx context.Context, in *ListMyAttemptsRequest, opts ...grpc.CallOption) (*ListMyAttemptsResponse, error) {
	return invoke[ListMyAttemptsResponse](c, ctx, "ListMyAttempts", in, opts)
}
func (c *assessmentServiceClient) GetAttemptResult(ctx context.Context, in *GetAttemptResultRequest, opts ...grpc.CallOption) (*AttemptResult, error) {
	return invoke[AttemptResult](c, ctx, "GetAttemptResult", in, opts)
}
func (c *assessmentServiceClient) ListAttempts(ctx context.Context, in *ListAttemptsRequest, opts ...grpc.CallOption) (*ListAttemptsResponse, error) {
	return invoke[ListAttemptsResponse](c, ctx, "ListAttempts", in, opts)
}
func (c *assessmentServiceClient) GetAttemptReport(ctx context.Context, in *GetAttemptReportRequest, opts ...grpc.CallOption) (*AttemptReport, error) {
	return invoke[AttemptReport](c, ctx, "GetAttemptReport", in, opts)
}
func (c *assessmentServiceClient) GradeDescriptive(ctx context.Context, in *GradeDescriptiveRequest, opts ...grpc.CallOption) (*GradeDescriptiveResponse, error) {
	return invoke[GradeDescriptiveResponse](c, ctx, "GradeDescriptive", in, opts)
}
func (c *assessmentServiceClient) ComputeShortlist(ctx context.Context, in *ComputeShortlistRequest, opts ...grpc.CallOption) (*Shortlist, error) {
	return invoke[Shortlist](c, ctx, "ComputeShortlist", in, opts)
}
func (c *assessmentServiceClient) ListShortlists(ctx context.Context, in *ListShortlistsRequest, opts ...grpc.CallOption) (*ListShortlistsResponse, error) {
	return invoke[ListShortlistsResponse](c, ctx, "ListShortlists", in, opts)
}
func (c *assessmentServiceClient) GetShortlist(ctx context.Context, in *GetShortlistRequest, opts ...grpc.CallOption) (*Shortlist, error) {
	return invoke[Shortlist](c, ctx, "GetShortlist", in, opts)
}
func (c *assessmentServiceClient) SetCandidateDecision(ctx context.Context, in *SetCandidateDecisionRequest, opts ...grpc.CallOption) (*Empty, error) {
	return invoke[Empty](c, ctx, "SetCandidateDecision", in, opts)
}
func (c *assessmentServiceClient) ExportResults(ctx context.Context, in *ExportResultsRequest, opts ...grpc.CallOption) (*ExportResultsResponse, error) {
	return invoke[ExportResultsResponse](c, ctx, "ExportResults", in, opts)
}
func (c *assessmentServiceClient) Authorize(ctx context.Context, in *AuthorizeRequest, opts ...grpc.CallOption) (*AuthorizeResponse, error) {
	return invoke[AuthorizeResponse](c, ctx, "Authorize", in, opts)
}

// ─── Service Registration & Descriptor ───────────────────────────────────────

func RegisterAssessmentServiceServer(s grpc.ServiceRegistrar, srv AssessmentServiceServer) {
	s.RegisterService(&AssessmentService_ServiceDesc, srv)
}

// handlerFor builds a gRPC method handler for a (request, call) pair. It keeps
// the descriptor below to one line per RPC instead of a 12-line boilerplate
// block each, which matters at 40+ methods.
// The return type is spelled out rather than named because grpc keeps its
// methodHandler type unexported; an unnamed func with the same signature is
// assignable to MethodDesc.Handler.
func handlerFor[In any, Out any](method string, call func(AssessmentServiceServer, context.Context, *In) (*Out, error)) func(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	full := svc + method
	return func(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
		in := new(In)
		if err := dec(in); err != nil {
			return nil, err
		}
		if interceptor == nil {
			return call(srv.(AssessmentServiceServer), ctx, in)
		}
		return interceptor(ctx, in, &grpc.UnaryServerInfo{Server: srv, FullMethod: full},
			func(ctx context.Context, req interface{}) (interface{}, error) {
				return call(srv.(AssessmentServiceServer), ctx, req.(*In))
			})
	}
}

func method[In any, Out any](name string, call func(AssessmentServiceServer, context.Context, *In) (*Out, error)) grpc.MethodDesc {
	return grpc.MethodDesc{MethodName: name, Handler: handlerFor(name, call)}
}

var AssessmentService_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "assessment.v1.AssessmentService",
	HandlerType: (*AssessmentServiceServer)(nil),
	Methods: []grpc.MethodDesc{
		method("CreateCompany", AssessmentServiceServer.CreateCompany),
		method("ListCompanies", AssessmentServiceServer.ListCompanies),
		method("AddCompanyMember", AssessmentServiceServer.AddCompanyMember),
		method("ListCompanyMembers", AssessmentServiceServer.ListCompanyMembers),
		method("UpsertMcqQuestion", AssessmentServiceServer.UpsertMcqQuestion),
		method("ListMcqQuestions", AssessmentServiceServer.ListMcqQuestions),
		method("DeleteMcqQuestion", AssessmentServiceServer.DeleteMcqQuestion),
		method("BulkImportMcq", AssessmentServiceServer.BulkImportMcq),
		method("CreateAssessment", AssessmentServiceServer.CreateAssessment),
		method("UpdateAssessment", AssessmentServiceServer.UpdateAssessment),
		method("GetAssessment", AssessmentServiceServer.GetAssessment),
		method("ListAssessments", AssessmentServiceServer.ListAssessments),
		method("PublishAssessment", AssessmentServiceServer.PublishAssessment),
		method("DeleteAssessment", AssessmentServiceServer.DeleteAssessment),
		method("UpsertSection", AssessmentServiceServer.UpsertSection),
		method("DeleteSection", AssessmentServiceServer.DeleteSection),
		method("SetSectionQuestions", AssessmentServiceServer.SetSectionQuestions),
		method("InviteCandidates", AssessmentServiceServer.InviteCandidates),
		method("ListInvites", AssessmentServiceServer.ListInvites),
		method("ListAvailableAssessments", AssessmentServiceServer.ListAvailableAssessments),
		method("StartAttempt", AssessmentServiceServer.StartAttempt),
		method("GetAttemptState", AssessmentServiceServer.GetAttemptState),
		method("SaveAnswer", AssessmentServiceServer.SaveAnswer),
		method("RunAttemptCode", AssessmentServiceServer.RunAttemptCode),
		method("SubmitAttemptCode", AssessmentServiceServer.SubmitAttemptCode),
		method("GetAttemptSubmission", AssessmentServiceServer.GetAttemptSubmission),
		method("SubmitAttempt", AssessmentServiceServer.SubmitAttempt),
		method("RecordProctorEvent", AssessmentServiceServer.RecordProctorEvent),
		method("ListMyAttempts", AssessmentServiceServer.ListMyAttempts),
		method("GetAttemptResult", AssessmentServiceServer.GetAttemptResult),
		method("ListAttempts", AssessmentServiceServer.ListAttempts),
		method("GetAttemptReport", AssessmentServiceServer.GetAttemptReport),
		method("GradeDescriptive", AssessmentServiceServer.GradeDescriptive),
		method("ComputeShortlist", AssessmentServiceServer.ComputeShortlist),
		method("ListShortlists", AssessmentServiceServer.ListShortlists),
		method("GetShortlist", AssessmentServiceServer.GetShortlist),
		method("SetCandidateDecision", AssessmentServiceServer.SetCandidateDecision),
		method("ExportResults", AssessmentServiceServer.ExportResults),
		method("Authorize", AssessmentServiceServer.Authorize),
	},
	Streams: []grpc.StreamDesc{},
}
