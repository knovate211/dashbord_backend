// Package generated contains the programmatically built GraphQL schema for the API gateway.
// This replaces what gqlgen would normally generate — no codegen tool required.
package generated

import (
	"github.com/graphql-go/graphql"

	"github.com/knovate211/api-gateway/graph/resolvers"
)

// ─── Shared GraphQL Types ─────────────────────────────────────────────────────

var exampleType = graphql.NewObject(graphql.ObjectConfig{
	Name: "Example",
	Fields: graphql.Fields{
		"input":       &graphql.Field{Type: graphql.String},
		"output":      &graphql.Field{Type: graphql.String},
		"explanation": &graphql.Field{Type: graphql.String},
	},
})

var hintType = graphql.NewObject(graphql.ObjectConfig{
	Name: "Hint",
	Fields: graphql.Fields{
		"order": &graphql.Field{Type: graphql.Int},
		"title": &graphql.Field{Type: graphql.String},
		"body":  &graphql.Field{Type: graphql.String},
	},
})

var starterCodesType = graphql.NewObject(graphql.ObjectConfig{
	Name: "StarterCodes",
	Fields: graphql.Fields{
		"javascript": &graphql.Field{Type: graphql.String},
		"python":     &graphql.Field{Type: graphql.String},
		"java":       &graphql.Field{Type: graphql.String},
		"cpp":        &graphql.Field{Type: graphql.String},
		"go":         &graphql.Field{Type: graphql.String},
	},
})

var problemType = graphql.NewObject(graphql.ObjectConfig{
	Name: "Problem",
	Fields: graphql.Fields{
		"id":           &graphql.Field{Type: graphql.ID},
		"slug":         &graphql.Field{Type: graphql.String},
		"title":        &graphql.Field{Type: graphql.String},
		"difficulty":   &graphql.Field{Type: graphql.String},
		"topic":        &graphql.Field{Type: graphql.String},
		"xp":           &graphql.Field{Type: graphql.Int},
		"statement":    &graphql.Field{Type: graphql.String},
		"constraints":  &graphql.Field{Type: graphql.NewList(graphql.String)},
		"tags":         &graphql.Field{Type: graphql.NewList(graphql.String)},
		"examples":     &graphql.Field{Type: graphql.NewList(exampleType)},
		"hints":        &graphql.Field{Type: graphql.NewList(hintType)},
		"starterCodes": &graphql.Field{Type: starterCodesType},
		// Languages this problem can actually be solved in. The editor offered
		// all five on every problem, so the 412 stdio problems opened by
		// default on a placeholder starter no submission could fix.
		"supportedLanguages": &graphql.Field{Type: graphql.NewList(graphql.String)},
		"setId":              &graphql.Field{Type: graphql.String},
		"userStatus":         &graphql.Field{Type: graphql.String},
	},
})

var courseType = graphql.NewObject(graphql.ObjectConfig{
	Name: "Course",
	Fields: graphql.Fields{
		"id":        &graphql.Field{Type: graphql.ID},
		"title":     &graphql.Field{Type: graphql.String},
		"mentor":    &graphql.Field{Type: graphql.String},
		"initial":   &graphql.Field{Type: graphql.String},
		"color":     &graphql.Field{Type: graphql.String},
		"classTime": &graphql.Field{Type: graphql.String},
	},
})

var practiceSetType = graphql.NewObject(graphql.ObjectConfig{
	Name: "PracticeSet",
	Fields: graphql.Fields{
		"id":            &graphql.Field{Type: graphql.ID},
		"title":         &graphql.Field{Type: graphql.String},
		"level":         &graphql.Field{Type: graphql.String},
		"levelColor":    &graphql.Field{Type: graphql.String},
		"bgColor":       &graphql.Field{Type: graphql.String},
		"totalProblems": &graphql.Field{Type: graphql.Int},
		"progress":      &graphql.Field{Type: graphql.Float},
	},
})

var listProblemsResultType = graphql.NewObject(graphql.ObjectConfig{
	Name: "ListProblemsResult",
	Fields: graphql.Fields{
		"problems": &graphql.Field{Type: graphql.NewList(problemType)},
		"total":    &graphql.Field{Type: graphql.Int},
		"page":     &graphql.Field{Type: graphql.Int},
		"pageSize": &graphql.Field{Type: graphql.Int},
	},
})

var testResultType = graphql.NewObject(graphql.ObjectConfig{
	Name: "TestResult",
	Fields: graphql.Fields{
		"testCaseId":     &graphql.Field{Type: graphql.String},
		"input":          &graphql.Field{Type: graphql.String},
		"expectedOutput": &graphql.Field{Type: graphql.String},
		"actualOutput":   &graphql.Field{Type: graphql.String},
		"status":         &graphql.Field{Type: graphql.String},
		"executionMs":    &graphql.Field{Type: graphql.Int},
		"error":          &graphql.Field{Type: graphql.String},
	},
})

var submissionType = graphql.NewObject(graphql.ObjectConfig{
	Name: "Submission",
	Fields: graphql.Fields{
		"id":          &graphql.Field{Type: graphql.ID},
		"userId":      &graphql.Field{Type: graphql.String},
		"problemId":   &graphql.Field{Type: graphql.String},
		"language":    &graphql.Field{Type: graphql.String},
		"status":      &graphql.Field{Type: graphql.String},
		"runtimeMs":   &graphql.Field{Type: graphql.Int},
		"memoryKb":    &graphql.Field{Type: graphql.Int},
		"testResults": &graphql.Field{Type: graphql.NewList(testResultType)},
		"submittedAt": &graphql.Field{Type: graphql.String},
		"completedAt": &graphql.Field{Type: graphql.String},
	},
})

var listSubmissionsResultType = graphql.NewObject(graphql.ObjectConfig{
	Name: "ListSubmissionsResult",
	Fields: graphql.Fields{
		"submissions": &graphql.Field{Type: graphql.NewList(submissionType)},
		"total":       &graphql.Field{Type: graphql.Int},
		"page":        &graphql.Field{Type: graphql.Int},
		"pageSize":    &graphql.Field{Type: graphql.Int},
	},
})

var setProgressType = graphql.NewObject(graphql.ObjectConfig{
	Name: "SetProgress",
	Fields: graphql.Fields{
		"setId":    &graphql.Field{Type: graphql.String},
		"title":    &graphql.Field{Type: graphql.String},
		"progress": &graphql.Field{Type: graphql.Float},
		"solved":   &graphql.Field{Type: graphql.Int},
		"total":    &graphql.Field{Type: graphql.Int},
	},
})

var userProgressType = graphql.NewObject(graphql.ObjectConfig{
	Name: "UserProgress",
	Fields: graphql.Fields{
		"userId":         &graphql.Field{Type: graphql.String},
		"totalSolved":    &graphql.Field{Type: graphql.Int},
		"totalAttempted": &graphql.Field{Type: graphql.Int},
		"easySolved":     &graphql.Field{Type: graphql.Int},
		"mediumSolved":   &graphql.Field{Type: graphql.Int},
		"hardSolved":     &graphql.Field{Type: graphql.Int},
		"currentStreak":  &graphql.Field{Type: graphql.Int},
		"longestStreak":  &graphql.Field{Type: graphql.Int},
		"totalXp":        &graphql.Field{Type: graphql.Int},
		"setProgress":    &graphql.Field{Type: graphql.NewList(setProgressType)},
	},
})

var scratchpadResultType = graphql.NewObject(graphql.ObjectConfig{
	Name: "ScratchpadResult",
	Fields: graphql.Fields{
		"stdout":      &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
		"stderr":      &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
		"exitCode":    &graphql.Field{Type: graphql.NewNonNull(graphql.Int)},
		"executionMs": &graphql.Field{Type: graphql.NewNonNull(graphql.Int)},
		"timedOut":    &graphql.Field{Type: graphql.NewNonNull(graphql.Boolean)},
	},
})

var runCodeResultType = graphql.NewObject(graphql.ObjectConfig{
	Name: "RunCodeResult",
	Fields: graphql.Fields{
		"jobId":         &graphql.Field{Type: graphql.String},
		"overallStatus": &graphql.Field{Type: graphql.String},
		"testResults":   &graphql.Field{Type: graphql.NewList(testResultType)},
		"compileError":  &graphql.Field{Type: graphql.String},
		"runtimeMs":     &graphql.Field{Type: graphql.Int},
	},
})

var submitResultType = graphql.NewObject(graphql.ObjectConfig{
	Name: "SubmitResult",
	Fields: graphql.Fields{
		"submissionId": &graphql.Field{Type: graphql.String},
	},
})

var userProfileType = graphql.NewObject(graphql.ObjectConfig{
	Name: "UserProfile",
	Fields: graphql.Fields{
		"userId":                &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
		"gender":                &graphql.Field{Type: graphql.String},
		"dob":                   &graphql.Field{Type: graphql.String},
		"whatsapp":              &graphql.Field{Type: graphql.String},
		"phone":                 &graphql.Field{Type: graphql.String},
		"experience":            &graphql.Field{Type: graphql.String},
		"workExperience":        &graphql.Field{Type: graphql.String},
		"careerGap":             &graphql.Field{Type: graphql.String},
		"currentState":          &graphql.Field{Type: graphql.String},
		"currentCity":           &graphql.Field{Type: graphql.String},
		"preferredLocations":    &graphql.Field{Type: graphql.NewList(graphql.String)},
		"githubLink":            &graphql.Field{Type: graphql.String},
		"linkedinLink":          &graphql.Field{Type: graphql.String},
		"isWorkingProfessional": &graphql.Field{Type: graphql.Boolean},
		"resumeName":            &graphql.Field{Type: graphql.String},
		"edu10SchoolName":       &graphql.Field{Type: graphql.String},
		"edu10YearOfPassout":    &graphql.Field{Type: graphql.String},
		"edu10MarksPercent":     &graphql.Field{Type: graphql.String},
		"edu12SchoolName":       &graphql.Field{Type: graphql.String},
		"edu12YearOfPassout":    &graphql.Field{Type: graphql.String},
		"edu12MarksPercent":     &graphql.Field{Type: graphql.String},
		"ugUniversityRollNo":    &graphql.Field{Type: graphql.String},
		"ugCollegeName":         &graphql.Field{Type: graphql.String},
		"ugCourseName":          &graphql.Field{Type: graphql.String},
		"ugBranch":              &graphql.Field{Type: graphql.String},
		"ugYearOfPassout":       &graphql.Field{Type: graphql.String},
		"ugMarksPercent":        &graphql.Field{Type: graphql.String},
		"ugCgpa":                &graphql.Field{Type: graphql.String},
		"ugActiveBacklogs":      &graphql.Field{Type: graphql.String},
		"pgHasCertificate":      &graphql.Field{Type: graphql.Boolean},
	},
})

var userProfileInputType = graphql.NewInputObject(graphql.InputObjectConfig{
	Name: "UserProfileInput",
	Fields: graphql.InputObjectConfigFieldMap{
		"gender":                &graphql.InputObjectFieldConfig{Type: graphql.String},
		"dob":                   &graphql.InputObjectFieldConfig{Type: graphql.String},
		"whatsapp":              &graphql.InputObjectFieldConfig{Type: graphql.String},
		"phone":                 &graphql.InputObjectFieldConfig{Type: graphql.String},
		"experience":            &graphql.InputObjectFieldConfig{Type: graphql.String},
		"workExperience":        &graphql.InputObjectFieldConfig{Type: graphql.String},
		"careerGap":             &graphql.InputObjectFieldConfig{Type: graphql.String},
		"currentState":          &graphql.InputObjectFieldConfig{Type: graphql.String},
		"currentCity":           &graphql.InputObjectFieldConfig{Type: graphql.String},
		"preferredLocations":    &graphql.InputObjectFieldConfig{Type: graphql.NewList(graphql.String)},
		"githubLink":            &graphql.InputObjectFieldConfig{Type: graphql.String},
		"linkedinLink":          &graphql.InputObjectFieldConfig{Type: graphql.String},
		"isWorkingProfessional": &graphql.InputObjectFieldConfig{Type: graphql.Boolean},
		"resumeName":            &graphql.InputObjectFieldConfig{Type: graphql.String},
		"edu10SchoolName":       &graphql.InputObjectFieldConfig{Type: graphql.String},
		"edu10YearOfPassout":    &graphql.InputObjectFieldConfig{Type: graphql.String},
		"edu10MarksPercent":     &graphql.InputObjectFieldConfig{Type: graphql.String},
		"edu12SchoolName":       &graphql.InputObjectFieldConfig{Type: graphql.String},
		"edu12YearOfPassout":    &graphql.InputObjectFieldConfig{Type: graphql.String},
		"edu12MarksPercent":     &graphql.InputObjectFieldConfig{Type: graphql.String},
		"ugUniversityRollNo":    &graphql.InputObjectFieldConfig{Type: graphql.String},
		"ugCollegeName":         &graphql.InputObjectFieldConfig{Type: graphql.String},
		"ugCourseName":          &graphql.InputObjectFieldConfig{Type: graphql.String},
		"ugBranch":              &graphql.InputObjectFieldConfig{Type: graphql.String},
		"ugYearOfPassout":       &graphql.InputObjectFieldConfig{Type: graphql.String},
		"ugMarksPercent":        &graphql.InputObjectFieldConfig{Type: graphql.String},
		"ugCgpa":                &graphql.InputObjectFieldConfig{Type: graphql.String},
		"ugActiveBacklogs":      &graphql.InputObjectFieldConfig{Type: graphql.String},
		"pgHasCertificate":      &graphql.InputObjectFieldConfig{Type: graphql.Boolean},
	},
})

var upsertProfileResultType = graphql.NewObject(graphql.ObjectConfig{
	Name: "UpsertProfileResult",
	Fields: graphql.Fields{
		"success": &graphql.Field{Type: graphql.NewNonNull(graphql.Boolean)},
		"message": &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
	},
})

var quizAnswerInputType = graphql.NewInputObject(graphql.InputObjectConfig{
	Name: "QuizAnswerInput",
	Fields: graphql.InputObjectConfigFieldMap{
		"questionId": &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(graphql.Int)},
		"answer":     &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(graphql.String)},
	},
})

var quizQuestionResultType = graphql.NewObject(graphql.ObjectConfig{
	Name: "QuizQuestionResult",
	Fields: graphql.Fields{
		"questionId":    &graphql.Field{Type: graphql.NewNonNull(graphql.Int)},
		"correct":       &graphql.Field{Type: graphql.NewNonNull(graphql.Boolean)},
		"correctAnswer": &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
	},
})

var submitQuizResultType = graphql.NewObject(graphql.ObjectConfig{
	Name: "SubmitQuizResult",
	Fields: graphql.Fields{
		"success":        &graphql.Field{Type: graphql.NewNonNull(graphql.Boolean)},
		"score":          &graphql.Field{Type: graphql.NewNonNull(graphql.Int)},
		"totalQuestions": &graphql.Field{Type: graphql.NewNonNull(graphql.Int)},
		"results":        &graphql.Field{Type: graphql.NewNonNull(graphql.NewList(graphql.NewNonNull(quizQuestionResultType)))},
	},
})

var quizAttemptType = graphql.NewObject(graphql.ObjectConfig{
	Name: "QuizAttempt",
	Fields: graphql.Fields{
		"moduleId":        &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
		"score":           &graphql.Field{Type: graphql.NewNonNull(graphql.Int)},
		"totalQuestions":  &graphql.Field{Type: graphql.NewNonNull(graphql.Int)},
		"selectedAnswers": &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
		"completedAt":     &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
	},
})

var jobType = graphql.NewObject(graphql.ObjectConfig{
	Name: "Job",
	Fields: graphql.Fields{
		"url":         &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
		"title":       &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
		"company":     &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
		"locations":   &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
		"description": &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
		"salary":      &graphql.Field{Type: graphql.String},
		"date":        &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
		"site":        &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
		// Extras JSearch provides that Careerjet did not.
		"employerLogo":   &graphql.Field{Type: graphql.String},
		"employmentType": &graphql.Field{Type: graphql.String},
		"isRemote":       &graphql.Field{Type: graphql.NewNonNull(graphql.Boolean)},
	},
})

var jobSearchResultType = graphql.NewObject(graphql.ObjectConfig{
	Name: "JobSearchResult",
	Fields: graphql.Fields{
		"jobs":  &graphql.Field{Type: graphql.NewNonNull(graphql.NewList(graphql.NewNonNull(jobType)))},
		"total": &graphql.Field{Type: graphql.NewNonNull(graphql.Int)},
		"pages": &graphql.Field{Type: graphql.NewNonNull(graphql.Int)},
	},
})

// ─── Schema Builder ───────────────────────────────────────────────────────────

// Clients bundles all service clients needed by the schema.
type Clients struct {
	Problems    *resolvers.ProblemClients
	Submissions *resolvers.SubmissionClients
	Progress    *resolvers.ProgressClients
	User        *resolvers.UserClients
	Jobs        *resolvers.JobClients
	Assessments *resolvers.AssessmentClients
}

// merge folds a feature's fields into a root type's field map. It panics on a
// duplicate field name rather than silently letting one resolver shadow
// another — that would be a wiring bug, and a startup panic is where you want
// to find it.
func merge(dst, src graphql.Fields) {
	for name, field := range src {
		if _, exists := dst[name]; exists {
			panic("duplicate GraphQL field: " + name)
		}
		dst[name] = field
	}
}

// BuildSchema constructs the full GraphQL schema wiring resolvers to types.
func BuildSchema(clients *Clients) (graphql.Schema, error) {
	queryFields := graphql.Fields{
		"listProblems": {
			Type: listProblemsResultType,
			Args: graphql.FieldConfigArgument{
				"setId":      {Type: graphql.String},
				"topic":      {Type: graphql.String},
				"difficulty": {Type: graphql.String},
				"page":       {Type: graphql.Int},
				"pageSize":   {Type: graphql.Int},
			},
			Resolve: clients.Problems.ListProblems,
		},
		"getProblem": {
			Type: problemType,
			Args: graphql.FieldConfigArgument{
				"id": {Type: graphql.NewNonNull(graphql.String)},
			},
			Resolve: clients.Problems.GetProblem,
		},
		"listPracticeSets": {
			Type:    graphql.NewList(practiceSetType),
			Resolve: clients.Problems.ListPracticeSets,
		},
		"getSubmission": {
			Type: submissionType,
			Args: graphql.FieldConfigArgument{
				"id": {Type: graphql.NewNonNull(graphql.String)},
			},
			Resolve: clients.Submissions.GetSubmission,
		},
		"listSubmissions": {
			Type: listSubmissionsResultType,
			Args: graphql.FieldConfigArgument{
				"problemId": {Type: graphql.String},
				"page":      {Type: graphql.Int},
				"pageSize":  {Type: graphql.Int},
			},
			Resolve: clients.Submissions.ListSubmissions,
		},
		"getUserProgress": {
			Type:    userProgressType,
			Resolve: clients.Progress.GetUserProgress,
		},
		"getProfile": {
			Type:    userProfileType,
			Resolve: clients.User.GetProfile,
		},
		"getMyCourses": {
			Type:    graphql.NewList(courseType),
			Resolve: clients.User.GetMyCourses,
		},
		"getQuizAttempts": {
			Type:    graphql.NewList(quizAttemptType),
			Resolve: clients.User.GetQuizAttempts,
		},
		"searchJobs": {
			Type: graphql.NewNonNull(jobSearchResultType),
			Args: graphql.FieldConfigArgument{
				"keywords": {Type: graphql.String},
				"location": {Type: graphql.String},
				"page":     {Type: graphql.Int},
			},
			Resolve: clients.Jobs.SearchJobs,
		},
	}

	mutationFields := graphql.Fields{
		"submitCode": {
			Type: submitResultType,
			Args: graphql.FieldConfigArgument{
				"problemId": {Type: graphql.NewNonNull(graphql.String)},
				"language":  {Type: graphql.NewNonNull(graphql.String)},
				"code":      {Type: graphql.NewNonNull(graphql.String)},
			},
			Resolve: clients.Submissions.SubmitCode,
		},
		"runScratchpad": {
			Type: scratchpadResultType,
			Args: graphql.FieldConfigArgument{
				"language": {Type: graphql.NewNonNull(graphql.String)},
				"code":     {Type: graphql.NewNonNull(graphql.String)},
				"stdin":    {Type: graphql.String},
			},
			Resolve: clients.Submissions.RunScratchpad,
		},
		"runCode": {
			Type: runCodeResultType,
			Args: graphql.FieldConfigArgument{
				"problemId": {Type: graphql.NewNonNull(graphql.String)},
				"language":  {Type: graphql.NewNonNull(graphql.String)},
				"code":      {Type: graphql.NewNonNull(graphql.String)},
			},
			Resolve: clients.Submissions.RunCode,
		},
		"upsertProfile": {
			Type: upsertProfileResultType,
			Args: graphql.FieldConfigArgument{
				"profile": {Type: graphql.NewNonNull(userProfileInputType)},
			},
			Resolve: clients.User.UpsertProfile,
		},
		"submitQuiz": {
			Type: submitQuizResultType,
			Args: graphql.FieldConfigArgument{
				"moduleId": {Type: graphql.NewNonNull(graphql.String)},
				"answers":  {Type: graphql.NewNonNull(graphql.NewList(graphql.NewNonNull(quizAnswerInputType)))},
			},
			Resolve: clients.User.SubmitQuiz,
		},
	}

	// Placement assessments live in their own file to keep this one readable.
	merge(queryFields, assessmentQueryFields(clients.Assessments))
	merge(mutationFields, assessmentMutationFields(clients.Assessments))

	return graphql.NewSchema(graphql.SchemaConfig{
		Query:    graphql.NewObject(graphql.ObjectConfig{Name: "Query", Fields: queryFields}),
		Mutation: graphql.NewObject(graphql.ObjectConfig{Name: "Mutation", Fields: mutationFields}),
	})
}
