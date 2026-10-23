package resolvers

import (
	"fmt"

	"github.com/graphql-go/graphql"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"github.com/knovate211/api-gateway/middleware"
	userv1 "github.com/knovate211/proto/user/v1"
)

// UserClients holds the gRPC clients for user resolvers.
type UserClients struct {
	UserSvc userv1.UserServiceClient
	Log     *zap.Logger
	// Pool, when set, answers getMyCourses with one query instead of one
	// user-service call per programme.
	Pool *pgxpool.Pool
}

// GetProfile handles the getProfile GraphQL query.
func (c *UserClients) GetProfile(p graphql.ResolveParams) (interface{}, error) {
	userID := middleware.UserIDFromContext(p.Context)
	if userID == "" {
		return nil, fmt.Errorf("authentication required")
	}

	resp, err := c.UserSvc.GetProfile(p.Context, &userv1.GetProfileRequest{
		UserID: userID,
	})
	if err != nil {
		c.Log.Error("get profile resolver failed", zap.Error(err))
		return nil, fmt.Errorf("failed to get profile: %v", err)
	}

	prof := resp.Profile
	if prof == nil {
		return map[string]interface{}{
			"userId":                userID,
			"gender":                "",
			"dob":                   "",
			"whatsapp":              "",
			"phone":                 "",
			"experience":            "",
			"workExperience":        "",
			"careerGap":             "",
			"currentState":          "",
			"currentCity":           "",
			"preferredLocations":    []string{},
			"githubLink":            "",
			"linkedinLink":          "",
			"isWorkingProfessional": false,
			"resumeName":            "",
			"edu10SchoolName":       "",
			"edu10YearOfPassout":    "",
			"edu10MarksPercent":     "",
			"edu12SchoolName":       "",
			"edu12YearOfPassout":    "",
			"edu12MarksPercent":     "",
			"ugUniversityRollNo":    "",
			"ugCollegeName":         "",
			"ugCourseName":          "",
			"ugBranch":              "",
			"ugYearOfPassout":       "",
			"ugMarksPercent":        "",
			"ugCgpa":                "",
			"ugActiveBacklogs":      "",
			"pgHasCertificate":      false,
		}, nil
	}

	return map[string]interface{}{
		"userId":                prof.UserID,
		"gender":                prof.Gender,
		"dob":                   prof.Dob,
		"whatsapp":              prof.Whatsapp,
		"phone":                 prof.Phone,
		"experience":            prof.Experience,
		"workExperience":        prof.WorkExperience,
		"careerGap":             prof.CareerGap,
		"currentState":          prof.CurrentState,
		"currentCity":           prof.CurrentCity,
		"preferredLocations":    prof.PreferredLocations,
		"githubLink":            prof.GithubLink,
		"linkedinLink":          prof.LinkedinLink,
		"isWorkingProfessional": prof.IsWorkingProfessional,
		"resumeName":            prof.ResumeName,
		"edu10SchoolName":       prof.Edu10SchoolName,
		"edu10YearOfPassout":    prof.Edu10YearOfPassout,
		"edu10MarksPercent":     prof.Edu10MarksPercent,
		"edu12SchoolName":       prof.Edu12SchoolName,
		"edu12YearOfPassout":    prof.Edu12YearOfPassout,
		"edu12MarksPercent":     prof.Edu12MarksPercent,
		"ugUniversityRollNo":    prof.UGUniversityRollNo,
		"ugCollegeName":         prof.UGCollegeName,
		"ugCourseName":          prof.UGCourseName,
		"ugBranch":              prof.UGBranch,
		"ugYearOfPassout":       prof.UGYearOfPassout,
		"ugMarksPercent":        prof.UGMarksPercent,
		"ugCgpa":                prof.UGCGPA,
		"ugActiveBacklogs":      prof.UGActiveBacklogs,
		"pgHasCertificate":      prof.PGHasCertificate,
	}, nil
}

// UpsertProfile handles the upsertProfile GraphQL mutation.
func (c *UserClients) UpsertProfile(p graphql.ResolveParams) (interface{}, error) {
	userID := middleware.UserIDFromContext(p.Context)
	if userID == "" {
		return nil, fmt.Errorf("authentication required")
	}

	input, ok := p.Args["profile"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid profile input")
	}

	strVal := func(key string) string {
		v, _ := input[key].(string)
		return v
	}

	boolVal := func(key string) bool {
		v, _ := input[key].(bool)
		return v
	}

	strSliceVal := func(key string) []string {
		raw, ok := input[key].([]interface{})
		if !ok {
			return []string{}
		}
		res := make([]string, 0, len(raw))
		for _, item := range raw {
			if s, ok := item.(string); ok {
				res = append(res, s)
			}
		}
		return res
	}

	profile := &userv1.UserProfile{
		UserID:                userID,
		Gender:                strVal("gender"),
		Dob:                   strVal("dob"),
		Whatsapp:              strVal("whatsapp"),
		Phone:                 strVal("phone"),
		Experience:            strVal("experience"),
		WorkExperience:        strVal("workExperience"),
		CareerGap:             strVal("careerGap"),
		CurrentState:          strVal("currentState"),
		CurrentCity:           strVal("currentCity"),
		PreferredLocations:    strSliceVal("preferredLocations"),
		GithubLink:            strVal("githubLink"),
		LinkedinLink:          strVal("linkedinLink"),
		IsWorkingProfessional: boolVal("isWorkingProfessional"),
		ResumeName:            strVal("resumeName"),
		Edu10SchoolName:       strVal("edu10SchoolName"),
		Edu10YearOfPassout:    strVal("edu10YearOfPassout"),
		Edu10MarksPercent:     strVal("edu10MarksPercent"),
		Edu12SchoolName:       strVal("edu12SchoolName"),
		Edu12YearOfPassout:    strVal("edu12YearOfPassout"),
		Edu12MarksPercent:     strVal("edu12MarksPercent"),
		UGUniversityRollNo:    strVal("ugUniversityRollNo"),
		UGCollegeName:         strVal("ugCollegeName"),
		UGCourseName:          strVal("ugCourseName"),
		UGBranch:              strVal("ugBranch"),
		UGYearOfPassout:       strVal("ugYearOfPassout"),
		UGMarksPercent:        strVal("ugMarksPercent"),
		UGCGPA:                strVal("ugCgpa"),
		UGActiveBacklogs:      strVal("ugActiveBacklogs"),
		PGHasCertificate:      boolVal("pgHasCertificate"),
	}

	resp, err := c.UserSvc.UpsertProfile(p.Context, &userv1.UpsertProfileRequest{
		Profile: profile,
	})
	if err != nil {
		c.Log.Error("upsert profile resolver failed", zap.Error(err))
		return nil, fmt.Errorf("failed to save profile: %v", err)
	}

	return map[string]interface{}{
		"success": resp.Success,
		"message": resp.Message,
	}, nil
}

// programOrder is the order getMyCourses lists programmes in. Every key of
// programModules must appear here.
var programOrder = []string{"1", "2", "3", "4", "5", "genai", "seo", "digital-marketing", "testing"}

// programIncludes lists the programmes a bundle grants. Full Stack is an
// entitlement rather than a course of its own: the curriculum is these four.
var programIncludes = map[string][]string{
	"5": {"1", "2", "3", "genai"},
}

var programModules = map[string][]map[string]interface{}{
	"1": { // Java Dev
		{
			"id":        "mod-java",
			"title":     "Java Development",
			"mentor":    "Deeptanshu Kumar",
			"initial":   "J",
			"color":     "#6c5ce7",
			"classTime": "09:00 – 11:30 AM",
		},
	},
	"2": { // Frontend Technologies
		{
			"id":        "mod-fe",
			"title":     "Front-End Technologies",
			"mentor":    "Priya M. Khaisate",
			"initial":   "F",
			"color":     "#e05a36",
			"classTime": "11:15 – 01:15 PM",
		},
	},
	"3": { // Mastering SQL
		{
			"id":        "mod-sql",
			"title":     "Mastering SQL",
			"mentor":    "Ayush B",
			"initial":   "M",
			"color":     "#10ac84",
			"classTime": "11:30 – 12:45 PM",
		},
	},
	"4": { // Golang
		{
			"id":        "mod-golang",
			"title":     "Golang Engineering",
			"mentor":    "Deeptanshu Kumar",
			"initial":   "G",
			"color":     "#00d2d3",
			"classTime": "02:00 – 03:30 PM",
		},
	},
	"5": { // Fullstack
		{
			"id":        "mod-fullstack",
			"title":     "Full Stack Engineering",
			"mentor":    "Deeptanshu & Priya",
			"initial":   "FS",
			"color":     "#ff6b6b",
			"classTime": "04:00 – 06:00 PM",
		},
	},
	"seo": { // SEO
		{
			"id":        "mod-seo",
			"title":     "SEO Fundamentals",
			"mentor":    "Marketing Team",
			"initial":   "S",
			"color":     "#f39c12",
			"classTime": "10:00 – 11:00 AM",
		},
	},
	"digital-marketing": { // Digital Marketing
		{
			"id":        "mod-dm",
			"title":     "Digital Marketing Strategy",
			"mentor":    "Marketing Team",
			"initial":   "D",
			"color":     "#d35400",
			"classTime": "01:00 – 02:30 PM",
		},
	},
	"testing": { // Software Testing
		{
			"id":        "mod-testing",
			"title":     "Software Testing",
			"mentor":    "QA Tech Lead",
			"initial":   "QA",
			"color":     "#2ecc71",
			"classTime": "09:00 – 10:30 AM",
		},
	},
	"genai": { // GenAI & Forward Deployed Engineering
		{
			// The title must keep the "GenAI" prefix: the student sidebar and the
			// course-card route map both match on it to unlock /courses/genai.
			"id":        "mod-genai",
			"title":     "GenAI & Forward Deployed Engineering",
			"mentor":    "AI Engineering Team",
			"initial":   "AI",
			"color":     "#8e44ad",
			"classTime": "03:30 – 05:30 PM",
		},
	},
}

// GetMyCourses handles the getMyCourses GraphQL query.
func (c *UserClients) GetMyCourses(p graphql.ResolveParams) (interface{}, error) {
	userID := middleware.UserIDFromContext(p.Context)
	if userID == "" {
		return nil, fmt.Errorf("authentication required")
	}

	granted, err := c.grantedPrograms(p, userID)
	if err != nil {
		return nil, err
	}

	return coursesFor(granted), nil
}

// coursesFor lists the course records for the programmes a learner holds, in
// programOrder (not map order) so the list is stable between requests.
//
// A programme that bundles others (Full Stack) contributes the bundled courses
// first, in its own position, then its own record — the course page uses that
// record to open /courses/fullstack. A course reachable twice (bought directly
// and through Full Stack) is listed once.
func coursesFor(granted map[string]bool) []map[string]interface{} {
	var out []map[string]interface{}
	listed := map[string]bool{}
	add := func(programID string) {
		if listed[programID] {
			return
		}
		listed[programID] = true
		out = append(out, programModules[programID]...)
	}
	for _, programID := range programOrder {
		if !granted[programID] {
			continue
		}
		for _, included := range programIncludes[programID] {
			add(included)
		}
		add(programID)
	}
	return out
}

// grantedPrograms returns which programmes the user holds. With a pool it is
// one query; without one it falls back to asking user-service per programme,
// which is what this used to do on every dashboard load (nine sequential calls).
func (c *UserClients) grantedPrograms(p graphql.ResolveParams, userID string) (map[string]bool, error) {
	granted := map[string]bool{}
	if c.Pool != nil {
		rows, err := c.Pool.Query(p.Context, `SELECT course_id FROM user_courses WHERE user_id = $1`, userID)
		if err != nil {
			c.Log.Error("list user courses failed", zap.Error(err))
			return nil, fmt.Errorf("could not load your courses")
		}
		defer rows.Close()
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				return nil, fmt.Errorf("could not load your courses")
			}
			granted[id] = true
		}
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("could not load your courses")
		}
		return granted, nil
	}

	for _, programID := range programOrder {
		resp, err := c.UserSvc.CheckUserCourseAccess(p.Context, &userv1.CheckUserCourseAccessRequest{
			UserID:   userID,
			CourseID: programID,
		})
		if err != nil {
			c.Log.Error("check course access failed", zap.Error(err), zap.String("programId", programID))
			continue
		}
		granted[programID] = resp.HasAccess
		}
	return granted, nil
}

// SubmitQuiz handles the submitQuiz GraphQL mutation.
func (c *UserClients) SubmitQuiz(p graphql.ResolveParams) (interface{}, error) {
	userID := middleware.UserIDFromContext(p.Context)
	if userID == "" {
		return nil, fmt.Errorf("authentication required")
	}

	moduleID, _ := p.Args["moduleId"].(string)
	answersInput, ok := p.Args["answers"].([]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid answers input")
	}

	answers := make([]*userv1.QuizAnswer, 0, len(answersInput))
	for _, item := range answersInput {
		mapping, ok := item.(map[string]interface{})
		if !ok {
			c.Log.Warn("SubmitQuiz: invalid answers mapping", zap.Any("item", item))
			continue
		}
		
		var qID int
		switch v := mapping["questionId"].(type) {
		case int:
			qID = v
		case float64:
			qID = int(v)
		default:
			c.Log.Warn("SubmitQuiz: questionId invalid type", zap.Any("type", fmt.Sprintf("%T", mapping["questionId"])))
		}
		
		ans, _ := mapping["answer"].(string)
		answers = append(answers, &userv1.QuizAnswer{
			QuestionID: qID,
			Answer:     ans,
		})
	}
	// Answers are learner submissions, not secrets, but there is no reason to
	// dump the full payload into logs on every request.
	c.Log.Info("SubmitQuiz parsed answers",
		zap.String("module_id", moduleID),
		zap.Int("count", len(answers)))

	resp, err := c.UserSvc.SubmitQuiz(p.Context, &userv1.SubmitQuizRequest{
		UserID:   userID,
		ModuleID: moduleID,
		Answers:  answers,
	})
	if err != nil {
		c.Log.Error("submit quiz resolver failed", zap.Error(err))
		return nil, fmt.Errorf("failed to submit quiz: %v", err)
	}

	results := make([]map[string]interface{}, 0, len(resp.Results))
	for _, r := range resp.Results {
		results = append(results, map[string]interface{}{
			"questionId":    r.QuestionID,
			"correct":       r.Correct,
			"correctAnswer": r.CorrectAnswer,
		})
	}

	return map[string]interface{}{
		"success":        resp.Success,
		"score":          int(resp.Score),
		"totalQuestions": int(resp.TotalQuestions),
		"results":        results,
	}, nil
}

// GetQuizAttempts handles the getQuizAttempts GraphQL query.
func (c *UserClients) GetQuizAttempts(p graphql.ResolveParams) (interface{}, error) {
	userID := middleware.UserIDFromContext(p.Context)
	if userID == "" {
		return nil, fmt.Errorf("authentication required")
	}

	resp, err := c.UserSvc.GetQuizAttempts(p.Context, &userv1.GetQuizAttemptsRequest{
		UserID: userID,
	})
	if err != nil {
		c.Log.Error("get quiz attempts resolver failed", zap.Error(err))
		return nil, fmt.Errorf("failed to get quiz attempts: %v", err)
	}

	attempts := make([]map[string]interface{}, 0, len(resp.Attempts))
	for _, att := range resp.Attempts {
		attempts = append(attempts, map[string]interface{}{
			"moduleId":        att.ModuleID,
			"score":           int(att.Score),
			"totalQuestions":  int(att.TotalQuestions),
			"selectedAnswers": att.SelectedAnswers,
			"completedAt":     att.CompletedAt,
		})
	}

	return attempts, nil
}
