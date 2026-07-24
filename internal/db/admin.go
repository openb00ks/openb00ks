package db

import "context"

type AdminChecker interface {
	IsAdmin(ctx context.Context, userID string) (bool, error)
}

func (s *UserStore) IsAdmin(ctx context.Context, userID string) (bool, error) {
	user, err := s.GetByID(ctx, userID)
	if err != nil {
		return false, err
	}
	return user.IsAdmin, nil
}
