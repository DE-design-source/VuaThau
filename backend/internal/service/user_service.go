package service

import (
	"context"
	"errors"

	"vuathau/backend/internal/model"
	"vuathau/backend/internal/repository"
)

type UserService interface {
	GetByID(ctx context.Context, id int64) (*model.User, error)
	List(ctx context.Context, limit, offset int) ([]model.User, error)
}

type userService struct {
	repo repository.UserRepository
}

func NewUserService(repo repository.UserRepository) UserService {
	return &userService{repo: repo}
}

func (s *userService) GetByID(ctx context.Context, id int64) (*model.User, error) {
	u, err := s.repo.GetByID(ctx, id)
	if errors.Is(err, repository.ErrNotFound) {
		return nil, ErrNotFound
	}
	return u, err
}

func (s *userService) List(ctx context.Context, limit, offset int) ([]model.User, error) {
	limit, offset = normalizePage(limit, offset)
	return s.repo.List(ctx, limit, offset)
}
