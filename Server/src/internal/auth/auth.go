package auth

import (
	"database/sql"
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/weirdstreamer/server/internal/config"
	"github.com/weirdstreamer/server/internal/database"
	"github.com/weirdstreamer/server/internal/models"
	"golang.org/x/crypto/bcrypt"
)

type Claims struct {
	UserID   int64  `json:"user_id"`
	Username string `json:"username"`
	IsAdmin  bool   `json:"is_admin"`
	jwt.RegisteredClaims
}

type Service struct {
	db      *database.DB
	limiter *Limiter
}

func NewService(db *database.DB) *Service {
	return &Service{db: db, limiter: NewLimiter()}
}

// ErrInvalidCredentials is a wrong password or an unknown username.
var ErrInvalidCredentials = errors.New("invalid credentials")

// LockedError refuses a password check while the limiter's lock is in force.
type LockedError struct {
	Wait time.Duration
}

func (e *LockedError) Error() string {
	return "too many failed attempts"
}

// Authenticate signs a user in from the client address clientIP. Wrong passwords are limited per
// username and address (a *LockedError while locked out); ErrInvalidCredentials is a wrong
// password or unknown user, and any other error is the server's.
func (s *Service) Authenticate(username, password, clientIP string) (*models.LoginResponse, error) {
	attempt, wait := s.limiter.Begin(username, clientIP)
	if attempt == nil {
		return nil, &LockedError{Wait: wait}
	}
	user, err := s.db.GetUserByUsername(username)
	if errors.Is(err, sql.ErrNoRows) {
		attempt.Failed()
		return nil, ErrInvalidCredentials
	}
	if err != nil {
		attempt.Abandon()
		return nil, err
	}
	if err := checkPassword(attempt, user, password); err != nil {
		return nil, err
	}

	token, expiresAt, err := s.GenerateToken(user)
	if err != nil {
		return nil, err
	}

	return &models.LoginResponse{
		Token:     token,
		ExpiresAt: expiresAt,
		User:      *user,
	}, nil
}

// VerifyPassword checks a signed-in user's password (to change it), limited like signing in.
func (s *Service) VerifyPassword(user *models.User, password, clientIP string) error {
	attempt, wait := s.limiter.Begin(user.Username, clientIP)
	if attempt == nil {
		return &LockedError{Wait: wait}
	}
	return checkPassword(attempt, user, password)
}

// checkPassword compares password with the user's hash and ends the attempt with the result.
func checkPassword(attempt *Attempt, user *models.User, password string) error {
	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)) != nil {
		attempt.Failed()
		return ErrInvalidCredentials
	}
	attempt.Passed()
	return nil
}

func (s *Service) GenerateToken(user *models.User) (string, int64, error) {
	expiresAt := time.Now().Add(time.Hour * time.Duration(config.GlobalConfig.Auth.TokenExpiryHours))
	claims := &Claims{
		UserID:   user.ID,
		Username: user.Username,
		IsAdmin:  user.IsAdmin,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(config.GlobalConfig.Auth.JWTSecret))
	if err != nil {
		return "", 0, err
	}

	return tokenString, expiresAt.Unix(), nil
}

func (s *Service) ValidateToken(tokenString string) (*Claims, error) {
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
		return []byte(config.GlobalConfig.Auth.JWTSecret), nil
	})

	if err != nil {
		return nil, err
	}

	if !token.Valid {
		return nil, errors.New("invalid token")
	}

	return claims, nil
}

// UserForClaims loads the user a validated token belongs to, so deleted users lose access
// and group changes apply without logging in again.
func (s *Service) UserForClaims(claims *Claims) (*models.User, error) {
	return s.db.GetUserByUsername(claims.Username)
}
