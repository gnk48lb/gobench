package service

import (
	"gobench/internal/model"
	"gobench/internal/repository"
	"gobench/pkg/apperrors"
	"gobench/pkg/jwt"

	"golang.org/x/crypto/bcrypt"
)

type AuthService interface {
	Register(username, password string) error
	Login(username, password string) (string, error)
}

type authService struct {
	userRepo repository.UserRepository
}

func NewAuthService(userRepo repository.UserRepository) AuthService {
	return &authService{userRepo: userRepo}
}

func (s *authService) Register(username, password string) error {
	// Check if user exists
	existingUser, err := s.userRepo.FindByUsername(username)
	if err != nil {
		return err
	}
	if existingUser != nil {
		return apperrors.ErrUsernameExists
	}

	// Hash password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	// Create user
	user := &model.User{
		Username: username,
		Password: string(hashedPassword),
		Role:     "user", // default role
	}

	return s.userRepo.Create(user)
}

func (s *authService) Login(username, password string) (string, error) {
	// Find user
	user, err := s.userRepo.FindByUsername(username)
	if err != nil {
		return "", err
	}
	if user == nil {
		return "", apperrors.ErrInvalidCredentials
	}

	// Verify password
	err = bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password))
	if err != nil {
		return "", apperrors.ErrInvalidCredentials
	}

	// Generate JWT
	token, err := jwt.GenerateToken(user.ID)
	if err != nil {
		return "", err
	}

	return token, nil
}
