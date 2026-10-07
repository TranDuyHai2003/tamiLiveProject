package service

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"livestream-backend/internal/auth"
	"livestream-backend/internal/db"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

var usernamePattern = regexp.MustCompile(`^[a-zA-Z0-9_.]{3,50}$`)

// Hash giả dùng khi email không tồn tại, để thời gian phản hồi login không lộ email nào đã đăng ký.
var dummyPasswordHash, _ = bcrypt.GenerateFromPassword([]byte("dummy-password"), bcrypt.DefaultCost)

type UserService struct {
	pool    *pgxpool.Pool
	queries *db.Queries
	tokens  *auth.TokenIssuer
}

func NewUserService(pool *pgxpool.Pool, tokens *auth.TokenIssuer) *UserService {
	return &UserService{
		pool:    pool,
		queries: db.New(pool),
		tokens:  tokens,
	}
}

// Register tạo user và ví USER của họ trong cùng một transaction.
func (s *UserService) Register(ctx context.Context, username, email, password string) (db.CreateUserRow, error) {
	username = strings.TrimSpace(username)
	email = strings.ToLower(strings.TrimSpace(email))

	if !usernamePattern.MatchString(username) {
		return db.CreateUserRow{}, fmt.Errorf("%w: username must be 3-50 chars of letters, digits, '_' or '.'", ErrInvalidInput)
	}
	// bcrypt chỉ dùng 72 byte đầu tiên
	if len(password) < 8 || len(password) > 72 {
		return db.CreateUserRow{}, fmt.Errorf("%w: password must be 8-72 bytes", ErrInvalidInput)
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return db.CreateUserRow{}, fmt.Errorf("hash password: %w", err)
	}

	var user db.CreateUserRow
	err = withTx(ctx, s.pool, func(q *db.Queries) error {
		user, err = q.CreateUser(ctx, db.CreateUserParams{
			Username:     username,
			Email:        email,
			PasswordHash: string(hash),
			Role:         db.UserRoleVIEWER,
		})
		if err != nil {
			return err
		}
		_, err = q.CreateUserWallet(ctx, &user.ID)
		return err
	})
	if isUniqueViolation(err, "users_username_key") || isUniqueViolation(err, "idx_users_username_lower") {
		return db.CreateUserRow{}, fmt.Errorf("%w: username already exists", ErrConflict)
	}
	if isUniqueViolation(err, "users_email_key") || isUniqueViolation(err, "idx_users_email_lower") {
		return db.CreateUserRow{}, fmt.Errorf("%w: email already exists", ErrConflict)
	}
	if err != nil {
		return db.CreateUserRow{}, fmt.Errorf("register user: %w", err)
	}
	return user, nil
}

type LoginResult struct {
	AccessToken string    `json:"access_token"`
	ExpiresAt   time.Time `json:"expires_at"`
	UserID      uuid.UUID `json:"user_id"`
}

func (s *UserService) Login(ctx context.Context, email, password string) (LoginResult, error) {
	cred, err := s.queries.GetUserCredentialsByEmail(ctx, strings.TrimSpace(email))
	if isNoRows(err) {
		_ = bcrypt.CompareHashAndPassword(dummyPasswordHash, []byte(password))
		return LoginResult{}, fmt.Errorf("%w: invalid email or password", ErrUnauthorized)
	}
	if err != nil {
		return LoginResult{}, fmt.Errorf("get credentials: %w", err)
	}

	if err := bcrypt.CompareHashAndPassword([]byte(cred.PasswordHash), []byte(password)); err != nil {
		if errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) {
			return LoginResult{}, fmt.Errorf("%w: invalid email or password", ErrUnauthorized)
		}
		return LoginResult{}, fmt.Errorf("compare password: %w", err)
	}
	if cred.Status != db.UserStatusACTIVE {
		return LoginResult{}, fmt.Errorf("%w: account is %s", ErrForbidden, strings.ToLower(string(cred.Status)))
	}

	token, expiresAt, err := s.tokens.Issue(cred.ID)
	if err != nil {
		return LoginResult{}, err
	}
	return LoginResult{AccessToken: token, ExpiresAt: expiresAt, UserID: cred.ID}, nil
}

type Profile struct {
	db.GetUserByIDRow
	Coin int64 `json:"coin"`
	Gem  int64 `json:"gem"`
}

func (s *UserService) GetProfile(ctx context.Context, userID uuid.UUID) (Profile, error) {
	user, err := s.queries.GetUserByID(ctx, userID)
	if isNoRows(err) {
		return Profile{}, fmt.Errorf("%w: user", ErrNotFound)
	}
	if err != nil {
		return Profile{}, fmt.Errorf("get user: %w", err)
	}
	wallet, err := s.queries.GetUserWallet(ctx, &userID)
	if err != nil {
		return Profile{}, fmt.Errorf("get wallet: %w", err)
	}
	return Profile{GetUserByIDRow: user, Coin: wallet.Coin, Gem: wallet.Gem}, nil
}
