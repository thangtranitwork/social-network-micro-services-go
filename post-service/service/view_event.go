package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"social-network-go/logger"
	"social-network-go/post-service/dto"
)

var (
	ErrUnauthorized       = errors.New("UNAUTHORIZED")
	ErrBadRequest         = errors.New("BAD_REQUEST")
	ErrServiceUnavailable = errors.New("SERVICE_UNAVAILABLE")
)

// ProcessViewEvents validates the incoming batch of qualified view events and publishes them to Kafka with broker acknowledgement.
func (s *PostService) ProcessViewEvents(ctx context.Context, currentUserID string, req *dto.ViewEventsBatchRequest) (*dto.ViewEventsBatchResponse, error) {
	if currentUserID == "" {
		return nil, ErrUnauthorized
	}
	if req == nil {
		return nil, fmt.Errorf("%w: request cannot be nil", ErrBadRequest)
	}

	now := time.Now().UTC()
	if err := req.Validate(now); err != nil {
		logger.WithContext(ctx).Warn("invalid view events payload: %v", err)
		return nil, fmt.Errorf("%w: %v", ErrBadRequest, err)
	}

	if s.ViewPublisher != nil {
		if err := s.ViewPublisher.PublishViewEvents(ctx, currentUserID, req.Events); err != nil {
			logger.WithContext(ctx).Err(err).Error("view publisher failed to publish events with ack")
			return nil, fmt.Errorf("%w: %v", ErrServiceUnavailable, err)
		}
	}

	return &dto.ViewEventsBatchResponse{
		AcceptedCount: len(req.Events),
	}, nil
}
