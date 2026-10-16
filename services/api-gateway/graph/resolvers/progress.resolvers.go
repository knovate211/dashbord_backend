package resolvers

import (
	"fmt"

	"github.com/graphql-go/graphql"
	"go.uber.org/zap"

	"github.com/knovate211/api-gateway/middleware"
	progressv1 "github.com/knovate211/proto/progress/v1"
)

// ProgressClients holds gRPC clients for progress resolvers.
type ProgressClients struct {
	ProgressSvc progressv1.ProgressServiceClient
	Log         *zap.Logger
}

// GetUserProgressResolver handles the getUserProgress GraphQL query.
func (c *ProgressClients) GetUserProgress(p graphql.ResolveParams) (interface{}, error) {
	userID := middleware.UserIDFromContext(p.Context)
	if userID == "" {
		return nil, fmt.Errorf("authentication required")
	}

	resp, err := c.ProgressSvc.GetUserProgress(p.Context, &progressv1.GetUserProgressRequest{
		UserId: userID,
	})
	if err != nil {
		c.Log.Error("get user progress resolver", zap.Error(err))
		return nil, fmt.Errorf("failed to get user progress: %v", err)
	}

	setProgress := make([]interface{}, 0, len(resp.SetProgress))
	for _, sp := range resp.SetProgress {
		setProgress = append(setProgress, map[string]interface{}{
			"setId":    mapUUIDToSimpleID(sp.SetId),
			"title":    sp.Title,
			"progress": sp.Progress,
			"solved":   sp.Solved,
			"total":    sp.Total,
		})
	}

	return map[string]interface{}{
		"userId":         resp.UserId,
		"totalSolved":    resp.TotalSolved,
		"totalAttempted": resp.TotalAttempted,
		"easySolved":     resp.EasySolved,
		"mediumSolved":   resp.MediumSolved,
		"hardSolved":     resp.HardSolved,
		"currentStreak":  resp.CurrentStreak,
		"longestStreak":  resp.LongestStreak,
		"totalXp":        resp.TotalXp,
		"setProgress":    setProgress,
	}, nil
}
