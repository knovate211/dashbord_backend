// Package resolvers contains the GraphQL resolvers for the API gateway.
// Each resolver calls the appropriate downstream gRPC service.
package resolvers

import (
	"context"
	"fmt"

	"github.com/graphql-go/graphql"
	"go.uber.org/zap"

	"github.com/knovate211/api-gateway/middleware"
	problemv1 "github.com/knovate211/proto/problem/v1"
	userv1 "github.com/knovate211/proto/user/v1"
)

// ProblemClients holds all gRPC clients needed for problem resolvers.
type ProblemClients struct {
	ProblemSvc problemv1.ProblemServiceClient
	UserSvc    userv1.UserServiceClient
	Log        *zap.Logger
}

// ListProblemsResolver handles the listProblems GraphQL query.
func (c *ProblemClients) ListProblems(p graphql.ResolveParams) (interface{}, error) {
	req := &problemv1.ListProblemsRequest{
		SetId:      mapSimpleIDToUUID(stringArg(p, "setId")),
		Topic:      stringArg(p, "topic"),
		Difficulty: stringArg(p, "difficulty"),
		Page:       int32Arg(p, "page", 1),
		PageSize:   int32Arg(p, "pageSize", 50),
	}

	// Inject user ID for user-specific status
	if uid := middleware.UserIDFromContext(p.Context); uid != "" {
		req.UserId = uid
	}

	resp, err := c.ProblemSvc.ListProblems(p.Context, req)
	if err != nil {
		c.Log.Error("listProblems resolver", zap.Error(err))
		return nil, fmt.Errorf("failed to list problems: %v", err)
	}

	return problemListToMap(resp), nil
}

// GetProblemResolver handles the getProblem GraphQL query.
func (c *ProblemClients) GetProblem(p graphql.ResolveParams) (interface{}, error) {
	id, ok := p.Args["id"].(string)
	if !ok || id == "" {
		return nil, fmt.Errorf("id is required")
	}

	req := &problemv1.GetProblemRequest{Id: id}

	// Enrich with user status if authenticated
	userID := middleware.UserIDFromContext(p.Context)

	prob, err := c.ProblemSvc.GetProblem(p.Context, req)
	if err != nil {
		return nil, fmt.Errorf("problem not found: %s", id)
	}

	// Fetch user-specific status if we have a user ID
	if userID != "" {
		statusResp, err := c.ProblemSvc.GetProblemUserStatus(p.Context, &problemv1.GetProblemUserStatusRequest{
			UserId:    userID,
			ProblemId: prob.Id,
		})
		if err == nil {
			prob.UserStatus = statusResp.Status
		}
	}

	return problemToMap(prob), nil
}

// ListPracticeSetsResolver handles the listPracticeSets GraphQL query.
func (c *ProblemClients) ListPracticeSets(p graphql.ResolveParams) (interface{}, error) {
	userID := middleware.UserIDFromContext(p.Context)
	if userID == "" {
		return []interface{}{}, nil
	}

	// No restriction on loading practice sets for authenticated users.
	// Filter logic is enforced down in problem-service or handled via enrollment cards mapping.

	resp, err := c.ProblemSvc.ListPracticeSets(p.Context, &problemv1.ListPracticeSetsRequest{
		UserId: userID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list practice sets: %v", err)
	}

	c.Log.Info("ListPracticeSets called", zap.String("userId", userID), zap.Int("respCount", len(resp.PracticeSets)))

	result := make([]interface{}, 0, len(resp.PracticeSets))
	for _, s := range resp.PracticeSets {
		c.Log.Info("Mapping set item", zap.String("id", s.Id), zap.String("title", s.Title))
		result = append(result, map[string]interface{}{
			"id":            mapUUIDToSimpleID(s.Id),
			"title":         s.Title,
			"level":         s.Level,
			"levelColor":    s.LevelColor,
			"bgColor":       s.BgColor,
			"totalProblems": s.TotalProblems,
			"progress":      s.Progress,
		})
	}
	return result, nil
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func mapSimpleIDToUUID(id string) string {
	switch id {
	case "1":
		return "54574a34-9a68-4e65-ab9a-af05db4ca003" // Masters of Algorithms (Advanced)
	case "2":
		return "54574a34-9a68-4e65-ab9a-af05db4ca002" // Path to Proficiency (Intermediate)
	case "3":
		return "54574a34-9a68-4e65-ab9a-af05db4ca001" // Foundational Basics (Beginner)
	case "4":
		return "54574a34-9a68-4e65-ab9a-af05db4ca004" // SQL Mastery Challenge
	default:
		return id
	}
}

func mapUUIDToSimpleID(uuid string) string {
	switch uuid {
	case "54574a34-9a68-4e65-ab9a-af05db4ca003":
		return "1"
	case "54574a34-9a68-4e65-ab9a-af05db4ca002":
		return "2"
	case "54574a34-9a68-4e65-ab9a-af05db4ca001":
		return "3"
	case "54574a34-9a68-4e65-ab9a-af05db4ca004":
		return "4"
	default:
		return uuid
	}
}

func problemListToMap(resp *problemv1.ListProblemsResponse) map[string]interface{} {
	problems := make([]interface{}, 0, len(resp.Problems))
	for _, p := range resp.Problems {
		problems = append(problems, problemSummaryToMap(p))
	}
	return map[string]interface{}{
		"problems": problems,
		"total":    resp.Total,
		"page":     resp.Page,
		"pageSize": resp.PageSize,
	}
}

func problemSummaryToMap(p *problemv1.Problem) map[string]interface{} {
	return map[string]interface{}{
		"id":                 p.Id,
		"slug":               p.Slug,
		"title":              p.Title,
		"difficulty":         p.Difficulty,
		"topic":              p.Topic,
		"xp":                 p.Xp,
		"statement":          p.Statement,
		"constraints":        p.Constraints,
		"tags":               p.Tags,
		"examples":           examplesToMap(p.Examples),
		"hints":              hintsToMap(p.Hints),
		"starterCodes":       starterCodesToMap(p.StarterCodes),
		"supportedLanguages": supportedLanguagesOrDefault(p.SupportedLanguages),
		"setId":              mapUUIDToSimpleID(p.SetId),
		"userStatus":         p.UserStatus,
	}
}

func problemToMap(p *problemv1.Problem) map[string]interface{} {
	return problemSummaryToMap(p) // full detail uses same structure
}

func examplesToMap(examples []*problemv1.Example) []interface{} {
	result := make([]interface{}, 0, len(examples))
	for _, e := range examples {
		result = append(result, map[string]interface{}{
			"input":       e.Input,
			"output":      e.Output,
			"explanation": e.Explanation,
		})
	}
	return result
}

func hintsToMap(hints []*problemv1.Hint) []interface{} {
	result := make([]interface{}, 0, len(hints))
	for _, h := range hints {
		result = append(result, map[string]interface{}{
			"order": h.Order,
			"title": h.Title,
			"body":  h.Body,
		})
	}
	return result
}

// supportedLanguagesOrDefault keeps the field non-null for problems served by
// an older problem-service that does not populate it yet.
func supportedLanguagesOrDefault(langs []string) []string {
	if len(langs) == 0 {
		return []string{"javascript", "python", "java", "cpp", "go"}
	}
	return langs
}

func starterCodesToMap(sc *problemv1.StarterCodes) interface{} {
	if sc == nil {
		return nil
	}
	return map[string]interface{}{
		"javascript": sc.Javascript,
		"python":     sc.Python,
		"java":       sc.Java,
		"cpp":        sc.Cpp,
		"go":         sc.Go,
	}
}

func stringArg(p graphql.ResolveParams, key string) string {
	v, _ := p.Args[key].(string)
	return v
}

func int32Arg(p graphql.ResolveParams, key string, def int32) int32 {
	v, ok := p.Args[key].(int)
	if !ok {
		return def
	}
	return int32(v)
}

// ensure context import is used
var _ context.Context
