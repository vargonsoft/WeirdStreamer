package auth

import (
	"database/sql"
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/weirdstreamer/server/internal/config"
	"github.com/weirdstreamer/server/internal/database"
	"github.com/weirdstreamer/server/internal/logs"
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
		logs.Auth("Sign-in refused: %q from %s is locked out for %s", username, clientIP, wait.Round(time.Second))
		return nil, &LockedError{Wait: wait}
	}
	user, err := s.db.GetUserByUsername(username)
	if errors.Is(err, sql.ErrNoRows) {
		attempt.Failed()
		logs.Auth("Sign-in failed: no user %q, from %s", username, clientIP)
		return nil, ErrInvalidCredentials
	}
	if err != nil {
		attempt.Abandon()
		logs.Auth("Sign-in failed: %q from %s: %v", username, clientIP, err)
		return nil, err
	}
	if err := checkPassword(attempt, user, password); err != nil {
		logs.Auth("Sign-in failed: wrong password for %q, from %s", username, clientIP)
		return nil, err
	}
	logs.Auth("Signed in: %q from %s", username, clientIP)

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
		logs.Auth("Password check refused: %q from %s is locked out for %s", user.Username, clientIP, wait.Round(time.Second))
		return &LockedError{Wait: wait}
	}
	err := checkPassword(attempt, user, password)
	if err != nil {
		logs.Auth("Password check failed: wrong current password for %q, from %s", user.Username, clientIP)
	}
	return err
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

// GenerateToken signs a user in: it starts a session and returns its token. The token names the
// session (its ID) and the user; whether they're an admin is in it only for the apps' menus, and
// the server reads it from the database on every request.
func (s *Service) GenerateToken(user *models.User) (string, int64, error) {
	expiresAt := time.Now().Add(time.Hour * time.Duration(config.GlobalConfig.Auth.TokenExpiryHours))
	session, err := s.db.CreateSession(user.ID, expiresAt)
	if err != nil {
		return "", 0, err
	}
	claims := &Claims{
		UserID:   user.ID,
		Username: user.Username,
		IsAdmin:  user.IsAdmin,
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        session,
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

// ValidateToken checks a token's signature and expiry (not its session: UserForClaims does).
func (s *Service) ValidateToken(tokenString string) (*Claims, error) {
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
		return []byte(config.GlobalConfig.Auth.JWTSecret), nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}))

	if err != nil {
		return nil, err
	}

	if !token.Valid {
		return nil, errors.New("invalid token")
	}

	return claims, nil
}

// ErrSessionEnded refuses a token whose session was ended (signed out, password changed, user
// deleted) or that names none.
var ErrSessionEnded = errors.New("session ended")

// UserForClaims loads the user a validated token belongs to, if its session is still on, and
// brings the claims up to date from the database: a demoted admin, a deleted user or a group
// change take effect at once.
func (s *Service) UserForClaims(claims *Claims) (*models.User, error) {
	userID, ok, err := s.db.SessionUser(claims.ID)
	if err != nil {
		return nil, err
	}
	if !ok || userID != claims.UserID {
		return nil, ErrSessionEnded
	}
	user, err := s.db.GetUserByID(userID)
	if err != nil {
		return nil, err
	}
	claims.Username = user.Username
	claims.IsAdmin = user.IsAdmin
	return user, nil
}

// EndSession signs a token's session out.
func (s *Service) EndSession(claims *Claims) error {
	return s.db.EndSession(claims.ID)
}
