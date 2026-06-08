// jwt_jti_test.go 는 M6 토큰 하드닝(jti 클레임 + jti 블랙리스트)을 검증한다
// (@SPEC:SPEC-REMOTE-001 M6, REQ-F02/F07).
//
// 핵심: DB 에는 원본 토큰이 아닌 jti(랜덤 식별자)만 저장되므로, 폐기는 원본 토큰
// 없이 jti 만으로 수행되어야 하고, 검증은 jti 블랙리스트를 거부해야 한다. 동시에
// jti 없는 기존(웹 어드민) 토큰은 그대로 검증되어야 한다(하위 호환).
package auth

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGenerateTokens_IncludesJTI 는 발급된 토큰이 비어있지 않은 jti(Claims.ID)를
// 포함하는지 검증한다(REQ-F07 — jti 기반 폐기 가능성의 토대).
func TestGenerateTokens_IncludesJTI(t *testing.T) {
	svc := newTestJWTService(t)

	access, refresh, _, err := svc.GenerateTokens("node-1", "node")
	require.NoError(t, err)

	ac, err := svc.ValidateToken(access)
	require.NoError(t, err)
	assert.NotEmpty(t, ac.ID, "access 토큰은 jti(Claims.ID)를 가져야 함")

	rc, err := svc.ValidateToken(refresh)
	require.NoError(t, err)
	assert.NotEmpty(t, rc.ID, "refresh 토큰은 jti(Claims.ID)를 가져야 함")

	assert.NotEqual(t, ac.ID, rc.ID, "access 와 refresh 의 jti 는 서로 달라야 함")
}

// TestGenerateTokensWithID_ReturnsJTI 는 GenerateTokensWithID 가 반환하는 jti 가
// 토큰 클레임의 jti 와 일치하는지 검증한다(서버가 jti 를 저장하기 위함).
func TestGenerateTokensWithID_ReturnsJTI(t *testing.T) {
	svc := newTestJWTService(t)

	access, _, jti, _, err := svc.GenerateTokensWithID("node-2", "node")
	require.NoError(t, err)
	require.NotEmpty(t, jti)

	claims, err := svc.ValidateToken(access)
	require.NoError(t, err)
	assert.Equal(t, jti, claims.ID, "반환된 jti 는 access 토큰 클레임의 jti 와 일치해야 함")
}

// TestBlacklistJTI_DeniesValidation 는 jti 를 블랙리스트하면 그 jti 를 가진 토큰의
// 검증이 거부되는지 검증한다(REQ-F07 — 원본 토큰 없이 폐기). 이것이 DB-안전 폐기의
// 핵심이다(서버는 jti 만 저장).
func TestBlacklistJTI_DeniesValidation(t *testing.T) {
	svc := newTestJWTService(t)

	access, _, jti, _, err := svc.GenerateTokensWithID("node-3", "node")
	require.NoError(t, err)

	// 폐기 전: 유효.
	_, err = svc.ValidateToken(access)
	require.NoError(t, err)
	assert.False(t, svc.IsJTIBlacklisted(jti))

	// jti 만으로 폐기(원본 토큰 불필요).
	svc.BlacklistJTI(jti)
	assert.True(t, svc.IsJTIBlacklisted(jti))

	// 폐기 후: 검증 거부.
	_, err = svc.ValidateToken(access)
	assert.ErrorIs(t, err, ErrBlacklistToken, "jti 블랙리스트 토큰은 검증에서 거부되어야 함")
}

// TestLegacyTokenWithoutJTI_StillValidates 는 jti 가 없는 기존 토큰(웹 어드민
// 호환 경로)이 여전히 검증되는지 확인한다(하위 호환 핵심).
func TestLegacyTokenWithoutJTI_StillValidates(t *testing.T) {
	svc := newTestJWTService(t)

	// jti 없는 레거시 토큰을 직접 서명한다(GenerateTokens 가 jti 를 채우기 전 형태).
	legacy := jwt.NewWithClaims(jwt.SigningMethodHS256, Claims{
		Username: "admin",
		Role:     "admin",
		// RegisteredClaims.ID(jti) 미설정 — 레거시 토큰.
	})
	signed, err := legacy.SignedString(svc.secret)
	require.NoError(t, err)

	claims, err := svc.ValidateToken(signed)
	require.NoError(t, err, "jti 없는 레거시 토큰은 여전히 유효해야 함")
	assert.Empty(t, claims.ID)
	assert.Equal(t, "admin", claims.Username)

	// 빈 jti 는 결코 블랙리스트로 간주되지 않아야 한다(전체 레거시 토큰 무효화 방지).
	assert.False(t, svc.IsJTIBlacklisted(""))
}

// TestTokenStringBlacklist_StillWorks 는 기존 토큰-문자열 블랙리스트(웹 어드민
// 로그아웃 경로)가 jti 도입 후에도 동작하는지 확인한다(하위 호환).
func TestTokenStringBlacklist_StillWorks(t *testing.T) {
	svc := newTestJWTService(t)

	access, _, _, err := svc.GenerateTokens("admin", "admin")
	require.NoError(t, err)

	svc.Blacklist(access)
	assert.True(t, svc.IsBlacklisted(access), "토큰-문자열 블랙리스트는 계속 동작해야 함")
}

// TestBlacklistJTI_EmptyIgnored 는 빈 jti 블랙리스트 시도가 무시되는지 확인한다
// (전체 레거시 토큰 무효화 방지).
func TestBlacklistJTI_EmptyIgnored(t *testing.T) {
	svc := newTestJWTService(t)
	svc.BlacklistJTI("")
	assert.False(t, svc.IsJTIBlacklisted(""), "빈 jti 는 결코 블랙리스트되지 않아야 함")
}

// TestCleanupBlacklist_RemovesExpiredJTI 는 CleanupBlacklist 가 만료된 jti 항목을
// 정리하는지 확인한다.
func TestCleanupBlacklist_RemovesExpiredJTI(t *testing.T) {
	// 매우 짧은 refresh 만료로 만료된 jti 항목을 만든다.
	svc, err := NewJWTService("test-secret-key", "15m", "1ns")
	require.NoError(t, err)

	svc.BlacklistJTI("jti-expired")
	time.Sleep(2 * time.Millisecond) // refreshExpiry(1ns) 경과.
	svc.CleanupBlacklist()
	assert.False(t, svc.IsJTIBlacklisted("jti-expired"),
		"만료된 jti 항목은 CleanupBlacklist 로 제거되어야 함")
}
