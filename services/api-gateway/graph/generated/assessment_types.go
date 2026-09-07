package generated

import (
	"github.com/graphql-go/graphql"

	"github.com/knovate211/api-gateway/graph/resolvers"
)

// resolversAssessment keeps the field-builder signatures below readable.
type resolversAssessment = resolvers.AssessmentClients

// ─── Placement assessment types ───────────────────────────────────────────────
//
// These back the student test player. Note what is absent from the student
// path: mcqOption.isCorrect is populated only on a revealed result, and hidden
// test-case content never appears at all.

var assessmentSummaryType = graphql.NewObject(graphql.ObjectConfig{
	Name: "AssessmentSummary",
	Fields: graphql.Fields{
		"id":              &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
		"title":           &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
		"description":     &graphql.Field{Type: graphql.String},
		"purpose":         &graphql.Field{Type: graphql.String},
		"companyName":     &graphql.Field{Type: graphql.String},
		"companyLogo":     &graphql.Field{Type: graphql.String},
		"durationMinutes": &graphql.Field{Type: graphql.Int},
		"totalMarks":      &graphql.Field{Type: graphql.Int},
		"questionCount":   &graphql.Field{Type: graphql.Int},
		"sectionSummary":  &graphql.Field{Type: graphql.String},
		"opensAt":         &graphql.Field{Type: graphql.String},
		"closesAt":        &graphql.Field{Type: graphql.String},
		"maxAttempts":     &graphql.Field{Type: graphql.Int},
		"attemptsUsed":    &graphql.Field{Type: graphql.Int},
		"liveAttemptId":   &graphql.Field{Type: graphql.String},
		"canStart":        &graphql.Field{Type: graphql.Boolean},
		"blockedReason":   &graphql.Field{Type: graphql.String},
		"proctoring":      &graphql.Field{Type: proctoringType},
	},
})

var mcqOptionType = graphql.NewObject(graphql.ObjectConfig{
	Name: "McqOption",
	Fields: graphql.Fields{
		"id":         &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
		"body":       &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
		"isCorrect":  &graphql.Field{Type: graphql.Boolean}, // only ever true on a revealed result
		"orderIndex": &graphql.Field{Type: graphql.Int},
	},
})

var attemptQuestionType = graphql.NewObject(graphql.ObjectConfig{
	Name: "AttemptQuestion",
	Fields: graphql.Fields{
		"id":                &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
		"sectionId":         &graphql.Field{Type: graphql.String},
		"kind":              &graphql.Field{Type: graphql.String},
		"orderIndex":        &graphql.Field{Type: graphql.Int},
		"marks":             &graphql.Field{Type: graphql.Float},
		"body":              &graphql.Field{Type: graphql.String},
		"mcqKind":           &graphql.Field{Type: graphql.String},
		"options":           &graphql.Field{Type: graphql.NewList(mcqOptionType)},
		"problemId":         &graphql.Field{Type: graphql.String},
		"problemTitle":      &graphql.Field{Type: graphql.String},
		"selectedOptionIds": &graphql.Field{Type: graphql.NewList(graphql.String)},
		"textAnswer":        &graphql.Field{Type: graphql.String},
		"submissionId":      &graphql.Field{Type: graphql.String},
		"language":          &graphql.Field{Type: graphql.String},
		"code":              &graphql.Field{Type: graphql.String},
		"gradingStatus":     &graphql.Field{Type: graphql.String},
		"visited":           &graphql.Field{Type: graphql.Boolean},
		"markedReview":      &graphql.Field{Type: graphql.Boolean},
		"timeSpentMs":       &graphql.Field{Type: graphql.Int},
		"awardedMarks":      &graphql.Field{Type: graphql.Float},
	},
})

var attemptSectionType = graphql.NewObject(graphql.ObjectConfig{
	Name: "AttemptSection",
	Fields: graphql.Fields{
		"id":              &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
		"title":           &graphql.Field{Type: graphql.String},
		"kind":            &graphql.Field{Type: graphql.String},
		"orderIndex":      &graphql.Field{Type: graphql.Int},
		"durationMinutes": &graphql.Field{Type: graphql.Int},
	},
})

var proctoringType = graphql.NewObject(graphql.ObjectConfig{
	Name: "Proctoring",
	Fields: graphql.Fields{
		"requireFullscreen": &graphql.Field{Type: graphql.Boolean},
		"tabSwitchLimit":    &graphql.Field{Type: graphql.Int},
		"blockCopyPaste":    &graphql.Field{Type: graphql.Boolean},
		"webcam":            &graphql.Field{Type: graphql.Boolean},
	},
})

var attemptStateType = graphql.NewObject(graphql.ObjectConfig{
	Name: "AttemptState",
	Fields: graphql.Fields{
		"attemptId":      &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
		"assessmentId":   &graphql.Field{Type: graphql.String},
		"title":          &graphql.Field{Type: graphql.String},
		"status":         &graphql.Field{Type: graphql.String},
		"allowBacktrack": &graphql.Field{Type: graphql.Boolean},
		"proctoring":     &graphql.Field{Type: proctoringType},
		// serverNow and secondsLeft are the authoritative clock; the client's
		// own time is display-only.
		"serverNow":       &graphql.Field{Type: graphql.String},
		"expiresAt":       &graphql.Field{Type: graphql.String},
		"secondsLeft":     &graphql.Field{Type: graphql.Int},
		"sections":        &graphql.Field{Type: graphql.NewList(attemptSectionType)},
		"questions":       &graphql.Field{Type: graphql.NewList(attemptQuestionType)},
		"maxScore":        &graphql.Field{Type: graphql.Float},
		"negativeMarking": &graphql.Field{Type: graphql.Float},
		// Lets the player end on "submitted" rather than a score screen,
		// without the client having to know what a scholarship is.
		"resultsWithheld": &graphql.Field{Type: graphql.Boolean},
	},
})

var attemptSummaryType = graphql.NewObject(graphql.ObjectConfig{
	Name: "AttemptSummary",
	Fields: graphql.Fields{
		"id":             &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
		"assessmentId":   &graphql.Field{Type: graphql.String},
		"assessmentName": &graphql.Field{Type: graphql.String},
		"attemptNo":      &graphql.Field{Type: graphql.Int},
		"status":         &graphql.Field{Type: graphql.String},
		"startedAt":      &graphql.Field{Type: graphql.String},
		"submittedAt":    &graphql.Field{Type: graphql.String},
		"evaluatedAt":    &graphql.Field{Type: graphql.String},
		"score":          &graphql.Field{Type: graphql.Float},
		"maxScore":       &graphql.Field{Type: graphql.Float},
		"percent":        &graphql.Field{Type: graphql.Float},
		"integrityScore": &graphql.Field{Type: graphql.Float},
		"passed":         &graphql.Field{Type: graphql.Boolean},
	},
})

var attemptResultType = graphql.NewObject(graphql.ObjectConfig{
	Name: "AttemptResult",
	Fields: graphql.Fields{
		"summary":   &graphql.Field{Type: attemptSummaryType},
		"questions": &graphql.Field{Type: graphql.NewList(attemptQuestionType)},
		"revealed":  &graphql.Field{Type: graphql.Boolean},
		"withheld":  &graphql.Field{Type: graphql.Boolean},
	},
})

var saveAnswerResultType = graphql.NewObject(graphql.ObjectConfig{
	Name: "SaveAnswerResult",
	Fields: graphql.Fields{
		"saved":       &graphql.Field{Type: graphql.Boolean},
		"secondsLeft": &graphql.Field{Type: graphql.Int},
	},
})

var attemptTestResultType = graphql.NewObject(graphql.ObjectConfig{
	Name: "AttemptTestResult",
	Fields: graphql.Fields{
		"input":          &graphql.Field{Type: graphql.String},
		"expectedOutput": &graphql.Field{Type: graphql.String},
		"actualOutput":   &graphql.Field{Type: graphql.String},
		"status":         &graphql.Field{Type: graphql.String},
		"executionMs":    &graphql.Field{Type: graphql.Int},
		"error":          &graphql.Field{Type: graphql.String},
	},
})

var runAttemptCodeResultType = graphql.NewObject(graphql.ObjectConfig{
	Name: "RunAttemptCodeResult",
	Fields: graphql.Fields{
		"overallStatus": &graphql.Field{Type: graphql.String},
		"testResults":   &graphql.Field{Type: graphql.NewList(attemptTestResultType)},
		"compileError":  &graphql.Field{Type: graphql.String},
		"runtimeMs":     &graphql.Field{Type: graphql.Int},
	},
})

var submitAttemptCodeResultType = graphql.NewObject(graphql.ObjectConfig{
	Name: "SubmitAttemptCodeResult",
	Fields: graphql.Fields{
		"submissionId": &graphql.Field{Type: graphql.String},
		"secondsLeft":  &graphql.Field{Type: graphql.Int},
	},
})

// attemptSubmissionType reports a judge verdict as counts only — hidden test
// cases are never exposed to a candidate mid-test.
var attemptSubmissionType = graphql.NewObject(graphql.ObjectConfig{
	Name: "AttemptSubmissionStatus",
	Fields: graphql.Fields{
		"submissionId": &graphql.Field{Type: graphql.String},
		"status":       &graphql.Field{Type: graphql.String},
		"passedCount":  &graphql.Field{Type: graphql.Int},
		"totalCount":   &graphql.Field{Type: graphql.Int},
		"compileError": &graphql.Field{Type: graphql.String},
		"runtimeMs":    &graphql.Field{Type: graphql.Int},
	},
})

var proctorEventResultType = graphql.NewObject(graphql.ObjectConfig{
	Name: "ProctorEventResult",
	Fields: graphql.Fields{
		"integrityScore": &graphql.Field{Type: graphql.Float},
		"terminated":     &graphql.Field{Type: graphql.Boolean},
		"warning":        &graphql.Field{Type: graphql.String},
	},
})

// assessmentQueryFields and assessmentMutationFields are merged into the root
// types by BuildSchema.
func assessmentQueryFields(c *resolversAssessment) graphql.Fields {
	return graphql.Fields{
		"listAssessments": {
			Type:    graphql.NewList(assessmentSummaryType),
			Args:    graphql.FieldConfigArgument{"scope": {Type: graphql.String}},
			Resolve: c.ListAssessments,
		},
		"getAttemptState": {
			Type:    attemptStateType,
			Args:    graphql.FieldConfigArgument{"attemptId": {Type: graphql.NewNonNull(graphql.String)}},
			Resolve: c.GetAttemptState,
		},
		"getMyAttempts": {
			Type:    graphql.NewList(attemptSummaryType),
			Resolve: c.GetMyAttempts,
		},
		"getAttemptResult": {
			Type:    attemptResultType,
			Args:    graphql.FieldConfigArgument{"attemptId": {Type: graphql.NewNonNull(graphql.String)}},
			Resolve: c.GetAttemptResult,
		},
		"getAttemptSubmission": {
			Type: attemptSubmissionType,
			Args: graphql.FieldConfigArgument{
				"attemptId":    {Type: graphql.NewNonNull(graphql.String)},
				"submissionId": {Type: graphql.NewNonNull(graphql.String)},
			},
			Resolve: c.GetAttemptSubmission,
		},
	}
}

func assessmentMutationFields(c *resolversAssessment) graphql.Fields {
	return graphql.Fields{
		"startAttempt": {
			Type: attemptStateType,
			Args: graphql.FieldConfigArgument{
				"assessmentId": {Type: graphql.NewNonNull(graphql.String)},
				"inviteToken":  {Type: graphql.String},
			},
			Resolve: c.StartAttempt,
		},
		"saveAnswer": {
			Type: saveAnswerResultType,
			Args: graphql.FieldConfigArgument{
				"attemptId":         {Type: graphql.NewNonNull(graphql.String)},
				"questionId":        {Type: graphql.NewNonNull(graphql.String)},
				"selectedOptionIds": {Type: graphql.NewList(graphql.NewNonNull(graphql.String))},
				"textAnswer":        {Type: graphql.String},
				"timeSpentMs":       {Type: graphql.Int},
				"markedReview":      {Type: graphql.Boolean},
				"clearAnswer":       {Type: graphql.Boolean},
				// Coding drafts ride the same autosave as every other answer.
				"language": {Type: graphql.String},
				"code":     {Type: graphql.String},
			},
			Resolve: c.SaveAnswer,
		},
		"runAttemptCode": {
			Type: runAttemptCodeResultType,
			Args: graphql.FieldConfigArgument{
				"attemptId":  {Type: graphql.NewNonNull(graphql.String)},
				"questionId": {Type: graphql.NewNonNull(graphql.String)},
				"language":   {Type: graphql.NewNonNull(graphql.String)},
				"code":       {Type: graphql.NewNonNull(graphql.String)},
			},
			Resolve: c.RunAttemptCode,
		},
		"submitAttemptCode": {
			Type: submitAttemptCodeResultType,
			Args: graphql.FieldConfigArgument{
				"attemptId":  {Type: graphql.NewNonNull(graphql.String)},
				"questionId": {Type: graphql.NewNonNull(graphql.String)},
				"language":   {Type: graphql.NewNonNull(graphql.String)},
				"code":       {Type: graphql.NewNonNull(graphql.String)},
			},
			Resolve: c.SubmitAttemptCode,
		},
		"submitAttempt": {
			Type:    attemptSummaryType,
			Args:    graphql.FieldConfigArgument{"attemptId": {Type: graphql.NewNonNull(graphql.String)}},
			Resolve: c.SubmitAttempt,
		},
		"recordProctorEvent": {
			Type: proctorEventResultType,
			Args: graphql.FieldConfigArgument{
				"attemptId": {Type: graphql.NewNonNull(graphql.String)},
				"kind":      {Type: graphql.NewNonNull(graphql.String)},
				"detail":    {Type: graphql.String},
			},
			Resolve: c.RecordProctorEvent,
		},
	}
}
