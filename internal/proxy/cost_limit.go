package proxy

import (
	"context"
	"fmt"
	"time"
)

const usdToEURRate = 0.855

// estimateRequestCost returns the estimated EUR cost for a completed request
// using the static model price table. Returns zero when the model is unknown.
func (s *Server) estimateRequestCost(modelID string, promptTokens, completionTokens, totalTokens int64) float64 {
	pricing, ok := staticModelPricing(modelID)
	if !ok {
		return 0
	}
	inputEUR := pricing.InputPer1MUSD * usdToEURRate
	outputEUR := pricing.OutputPer1MUSD * usdToEURRate
	return (float64(promptTokens)*inputEUR + float64(completionTokens)*outputEUR) / 1_000_000
}

type costLimitResult struct {
	Blocked       bool
	EstimatedCost float64
	LimitEUR      float64
}

func (s *Server) checkMonthlyCostLimit(userID string, ctx context.Context, now time.Time) (costLimitResult, error) {
	limitEUR := CostLimitEURFromContext(ctx)
	if limitEUR == nil {
		return costLimitResult{}, nil
	}

	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	monthEnd := monthStart.AddDate(0, 1, 0)

	totals, err := s.dashboard.usage(userID, dashboardQuery{
		From: &monthStart,
		To:   &monthEnd,
	})
	if err != nil {
		return costLimitResult{}, fmt.Errorf("cost limit: query monthly usage: %w", err)
	}

	estimated := totals.EstimatedCostEUR
	if estimated >= *limitEUR {
		return costLimitResult{
			Blocked:       true,
			EstimatedCost: estimated,
			LimitEUR:      *limitEUR,
		}, nil
	}
	return costLimitResult{
		Blocked:       false,
		EstimatedCost: estimated,
		LimitEUR:      *limitEUR,
	}, nil
}
