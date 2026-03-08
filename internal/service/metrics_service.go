package service

import (
	"context"
	"time"

	"events/internal/dto"
	"events/internal/repository"
)

type MetricsService struct {
	repo repository.EventRepository
}

func NewMetricsService(repo repository.EventRepository) *MetricsService {
	return &MetricsService{repo: repo}
}

func (s *MetricsService) GetMetrics(ctx context.Context, eventName string, from, to int64, groupBy string) (*dto.MetricsResponse, error) {
	fromTime := time.Unix(from, 0).UTC()
	toTime := time.Unix(to, 0).UTC()

	result, err := s.repo.QueryMetrics(ctx, eventName, fromTime, toTime, groupBy)
	if err != nil {
		return nil, err
	}

	resp := &dto.MetricsResponse{
		EventName:   eventName,
		From:        from,
		To:          to,
		TotalCount:  result.TotalCount,
		UniqueUsers: result.UniqueUsers,
	}

	for _, b := range result.Breakdown {
		resp.Breakdown = append(resp.Breakdown, dto.MetricBreakdown{
			Key:         b.Key,
			TotalCount:  b.TotalCount,
			UniqueUsers: b.UniqueUsers,
		})
	}

	return resp, nil
}
