package auth

import (
	"crypto/rand"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// JWT 관련 에러
var (
	ErrInvalidToken  = errors.New("auth: 유효하지 않은 토큰")
	ErrExpiredToken  = errors.New("auth: 만료된 토큰")
	ErrBlacklistToken = errors.New("auth: 블랙리스트 토큰")
)

// Claims 는 JWT 토큰의 클레임이다.
type Claims struct {
	Username string `json:"username"`
	Role     string `json:"role"`
	jwt.RegisteredClaims
}

// JWTService 는 JWT 토큰 생성/검증 서비스이다.
type JWTService struct {
	secret        []byte
	accessExpiry  time.Duration
	refreshExpiry time.Duration
	blacklist     *TokenBlacklist
}

// TokenBlacklist 는 로그아웃된 토큰을 추적한다.
type TokenBlacklist struct {
	mu     sync.RWMutex
	tokens map[string]time.Time // token -> 만료 시각
}

// NewJWTService 는 새 JWTService를 생성한다.
// secret이 빈 문자열이면 32바이트 랜덤 시크릿을 자동 생성한다.
func NewJWTService(secret string, accessExpiry, refreshExpiry string) (*JWTService, error) {
	var secretBytes []byte
	if secret == "" {
		secretBytes = make([]byte, 32)
		if _, err := rand.Read(secretBytes); err != nil {
			return nil, fmt.Errorf("auth: 랜덤 시크릿 생성 실패: %w", err)
		}
	} else {
		secretBytes = []byte(secret)
	}

	accessDur, err := time.ParseDuration(accessExpiry)
	if err != nil {
		return nil, fmt.Errorf("auth: access_expiry 파싱 실패: %w", err)
	}

	refreshDur, err := time.ParseDuration(refreshExpiry)
	if err != nil {
		return nil, fmt.Errorf("auth: refresh_expiry 파싱 실패: %w", err)
	}

	return &JWTService{
		secret:        secretBytes,
		accessExpiry:  accessDur,
		refreshExpiry: refreshDur,
		blacklist: &TokenBlacklist{
			tokens: make(map[string]time.Time),
		},
	}, nil
}

// GenerateTokens 는 액세스 토큰과 리프레시 토큰을 생성한다.
func (s *JWTService) GenerateTokens(username, role string) (accessToken, refreshToken string, expiresAt int64, err error) {
	now := time.Now()

	// 액세스 토큰
	accessExp := now.Add(s.accessExpiry)
	accessClaims := Claims{
		Username: username,
		Role:     role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(accessExp),
			IssuedAt:  jwt.NewNumericDate(now),
			Subject:   username,
		},
	}
	accessJWT := jwt.NewWithClaims(jwt.SigningMethodHS256, accessClaims)
	accessToken, err = accessJWT.SignedString(s.secret)
	if err != nil {
		return "", "", 0, fmt.Errorf("auth: 액세스 토큰 서명 실패: %w", err)
	}

	// 리프레시 토큰
	refreshExp := now.Add(s.refreshExpiry)
	refreshClaims := Claims{
		Username: username,
		Role:     role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(refreshExp),
			IssuedAt:  jwt.NewNumericDate(now),
			Subject:   username,
		},
	}
	refreshJWT := jwt.NewWithClaims(jwt.SigningMethodHS256, refreshClaims)
	refreshToken, err = refreshJWT.SignedString(s.secret)
	if err != nil {
		return "", "", 0, fmt.Errorf("auth: 리프레시 토큰 서명 실패: %w", err)
	}

	return accessToken, refreshToken, accessExp.Unix(), nil
}

// ValidateToken 는 토큰을 검증하고 클레임을 반환한다.
func (s *JWTService) ValidateToken(tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("auth: 예상하지 못한 서명 방식: %v", token.Header["alg"])
		}
		return s.secret, nil
	})
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrExpiredToken
		}
		return nil, ErrInvalidToken
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, ErrInvalidToken
	}

	return claims, nil
}

// Blacklist 는 토큰을 블랙리스트에 추가한다.
func (s *JWTService) Blacklist(token string) {
	// 토큰에서 만료 시각 추출 (블랙리스트 자동 정리용)
	claims, err := s.ValidateToken(token)
	expiry := time.Now().Add(s.refreshExpiry) // 기본 만료
	if err == nil && claims.ExpiresAt != nil {
		expiry = claims.ExpiresAt.Time
	}

	s.blacklist.mu.Lock()
	s.blacklist.tokens[token] = expiry
	s.blacklist.mu.Unlock()
}

// IsBlacklisted 는 토큰이 블랙리스트에 있는지 확인한다.
func (s *JWTService) IsBlacklisted(token string) bool {
	s.blacklist.mu.RLock()
	defer s.blacklist.mu.RUnlock()

	expiry, exists := s.blacklist.tokens[token]
	if !exists {
		return false
	}

	// 만료된 블랙리스트 항목 자동 정리
	if time.Now().After(expiry) {
		// 읽기 락에서 쓰기는 불가하므로, 그냥 false 반환
		// 다음 쓰기 시 정리됨
		return false
	}

	return true
}

// CleanupBlacklist 는 만료된 블랙리스트 항목을 정리한다.
func (s *JWTService) CleanupBlacklist() {
	s.blacklist.mu.Lock()
	defer s.blacklist.mu.Unlock()

	now := time.Now()
	for token, expiry := range s.blacklist.tokens {
		if now.After(expiry) {
			delete(s.blacklist.tokens, token)
		}
	}
}
