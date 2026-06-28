package auth

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// JWT 관련 에러
var (
	ErrInvalidToken   = errors.New("auth: 유효하지 않은 토큰")
	ErrExpiredToken   = errors.New("auth: 만료된 토큰")
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
	// jtiBlacklist 는 jti(토큰 식별자) 기반 폐기 목록이다(@SPEC:SPEC-REMOTE-001 M6,
	// REQ-F07). 원본 토큰 없이 jti 만으로 폐기할 수 있어, 서버 DB 에 원본 토큰을
	// 저장하지 않고도(노드 토큰 하드닝) 폐기가 가능하다.
	jtiBlacklist *jtiBlacklist
}

// TokenBlacklist 는 로그아웃된 토큰을 추적한다.
type TokenBlacklist struct {
	mu     sync.RWMutex
	tokens map[string]time.Time // token -> 만료 시각
}

// jtiBlacklist 는 폐기된 jti(토큰 식별자)를 추적한다(@SPEC:SPEC-REMOTE-001 M6).
type jtiBlacklist struct {
	mu  sync.RWMutex
	ids map[string]time.Time // jti -> 만료 시각(자동 정리용)
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
		jtiBlacklist: &jtiBlacklist{
			ids: make(map[string]time.Time),
		},
	}, nil
}

// GenerateTokens 는 액세스 토큰과 리프레시 토큰을 생성한다.
//
// 각 토큰은 고유한 jti(Claims.ID)를 포함한다(@SPEC:SPEC-REMOTE-001 M6, REQ-F07).
// jti 는 비밀이 아닌 랜덤 식별자로, jti 기반 폐기(BlacklistJTI)를 가능하게 한다.
// 기존 호출자는 jti 를 신경 쓸 필요가 없으며(반환 시그니처 불변), 하위 호환된다.
func (s *JWTService) GenerateTokens(username, role string) (accessToken, refreshToken string, expiresAt int64, err error) {
	accessToken, refreshToken, _, expiresAt, err = s.GenerateTokensWithID(username, role)
	return accessToken, refreshToken, expiresAt, err
}

// GenerateTokensWithID 는 액세스/리프레시 토큰을 생성하고 액세스 토큰의 jti 를 함께
// 반환한다(@SPEC:SPEC-REMOTE-001 M6, REQ-F07).
//
// 서버는 반환된 accessJTI 만 저장하면(원본 토큰 저장 불필요) jti 블랙리스트로 폐기할
// 수 있다. 이로써 DB 가 유출되어도 사용 가능한 토큰이 노출되지 않는다(노드 토큰 하드닝).
func (s *JWTService) GenerateTokensWithID(username, role string) (accessToken, refreshToken, accessJTI string, expiresAt int64, err error) {
	now := time.Now()

	accessJTI, err = newJTI()
	if err != nil {
		return "", "", "", 0, fmt.Errorf("auth: jti 생성 실패: %w", err)
	}
	refreshJTI, err := newJTI()
	if err != nil {
		return "", "", "", 0, fmt.Errorf("auth: jti 생성 실패: %w", err)
	}

	// 액세스 토큰
	accessExp := now.Add(s.accessExpiry)
	accessClaims := Claims{
		Username: username,
		Role:     role,
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        accessJTI,
			ExpiresAt: jwt.NewNumericDate(accessExp),
			IssuedAt:  jwt.NewNumericDate(now),
			Subject:   username,
		},
	}
	accessJWT := jwt.NewWithClaims(jwt.SigningMethodHS256, accessClaims)
	accessToken, err = accessJWT.SignedString(s.secret)
	if err != nil {
		return "", "", "", 0, fmt.Errorf("auth: 액세스 토큰 서명 실패: %w", err)
	}

	// 리프레시 토큰
	refreshExp := now.Add(s.refreshExpiry)
	refreshClaims := Claims{
		Username: username,
		Role:     role,
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        refreshJTI,
			ExpiresAt: jwt.NewNumericDate(refreshExp),
			IssuedAt:  jwt.NewNumericDate(now),
			Subject:   username,
		},
	}
	refreshJWT := jwt.NewWithClaims(jwt.SigningMethodHS256, refreshClaims)
	refreshToken, err = refreshJWT.SignedString(s.secret)
	if err != nil {
		return "", "", "", 0, fmt.Errorf("auth: 리프레시 토큰 서명 실패: %w", err)
	}

	return accessToken, refreshToken, accessJTI, accessExp.Unix(), nil
}

// newJTI 는 128비트 랜덤 jti(토큰 식별자) 16진 문자열을 생성한다.
func newJTI() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
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

	// jti 블랙리스트 검사(@SPEC:SPEC-REMOTE-001 M6, REQ-F07). jti 가 폐기되었으면
	// 거부한다. jti 가 빈 레거시 토큰(웹 어드민)은 이 검사를 건너뛴다(하위 호환).
	if claims.ID != "" && s.IsJTIBlacklisted(claims.ID) {
		return nil, ErrBlacklistToken
	}

	return claims, nil
}

// BlacklistJTI 는 jti(토큰 식별자)를 폐기 목록에 추가한다(@SPEC:SPEC-REMOTE-001 M6,
// REQ-F07). 원본 토큰 없이 jti 만으로 폐기할 수 있어, 서버 DB 가 jti 만 저장한 채
// 노드 토큰을 즉시 무효화할 수 있다.
//
// 만료 시각을 알 수 없으므로 refreshExpiry 기준의 보수적 보관 기간을 적용한다
// (그 이후 토큰은 어차피 만료되어 검증에서 거부됨).
func (s *JWTService) BlacklistJTI(jti string) {
	if jti == "" {
		return // 빈 jti 는 전체 레거시 토큰을 무효화할 위험이 있으므로 무시.
	}
	s.jtiBlacklist.mu.Lock()
	s.jtiBlacklist.ids[jti] = time.Now().Add(s.refreshExpiry)
	s.jtiBlacklist.mu.Unlock()
}

// IsJTIBlacklisted 는 jti 가 폐기되었는지 확인한다. 빈 jti 는 항상 false 이다
// (레거시 토큰 호환). 만료된 항목은 false 로 취급한다.
func (s *JWTService) IsJTIBlacklisted(jti string) bool {
	if jti == "" {
		return false
	}
	s.jtiBlacklist.mu.RLock()
	defer s.jtiBlacklist.mu.RUnlock()
	expiry, exists := s.jtiBlacklist.ids[jti]
	if !exists {
		return false
	}
	if time.Now().After(expiry) {
		return false
	}
	return true
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

	// jti 블랙리스트도 함께 정리한다(@SPEC:SPEC-REMOTE-001 M6).
	s.jtiBlacklist.mu.Lock()
	for jti, expiry := range s.jtiBlacklist.ids {
		if now.After(expiry) {
			delete(s.jtiBlacklist.ids, jti)
		}
	}
	s.jtiBlacklist.mu.Unlock()
}
