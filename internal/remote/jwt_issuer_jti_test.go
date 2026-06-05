// jwt_issuer_jti_test.go 는 M6 토큰 하드닝(jti 발급/저장/폐기)을 검증한다
// (@SPEC:SPEC-REMOTE-001 M6, REQ-F02/F07).
//
// 서버는 원본 토큰이 아닌 jti 만 저장하므로, jti 기반 폐기(RevokeID)가 원본 토큰
// 없이 동작하고, 폐기된 jti 토큰의 재검증이 거부되어야 한다.
package remote

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/auth"
)

// TestJWTTokenIssuer_IssueWithIDReturnsJTI 는 IssueWithID 가 토큰과 그 jti 를
// 함께 반환하는지 확인한다(서버가 jti 만 저장하기 위함).
func TestJWTTokenIssuer_IssueWithIDReturnsJTI(t *testing.T) {
	jwtSvc, err := auth.NewJWTService("test-secret", "1h", "24h")
	require.NoError(t, err)
	issuer := NewJWTTokenIssuer(jwtSvc)

	token, jti, err := issuer.IssueWithID("node-jti", "node")
	require.NoError(t, err)
	require.NotEmpty(t, token)
	require.NotEmpty(t, jti)
	assert.NotEqual(t, token, jti, "jti 는 원본 토큰과 달라야 한다(DB 안전)")

	subject, role, err := issuer.Validate(token)
	require.NoError(t, err)
	assert.Equal(t, "node-jti", subject)
	assert.Equal(t, "node", role)
}

// TestJWTTokenIssuer_RevokeIDDeniesValidation 는 jti 만으로 폐기하면(원본 토큰
// 불필요) 그 토큰의 재검증이 거부되는지 확인한다(REQ-F07 — DB-안전 폐기).
func TestJWTTokenIssuer_RevokeIDDeniesValidation(t *testing.T) {
	jwtSvc, err := auth.NewJWTService("test-secret", "1h", "24h")
	require.NoError(t, err)
	issuer := NewJWTTokenIssuer(jwtSvc)

	token, jti, err := issuer.IssueWithID("node-rev", "node")
	require.NoError(t, err)

	// 폐기 전: 유효.
	_, _, err = issuer.Validate(token)
	require.NoError(t, err)
	assert.False(t, issuer.IsIDRevoked(jti))

	// jti 만으로 폐기(서버 DB 는 jti 만 보유).
	issuer.RevokeID(jti)
	assert.True(t, issuer.IsIDRevoked(jti))

	// 폐기 후: 원본 토큰 재검증 거부.
	_, _, err = issuer.Validate(token)
	assert.Error(t, err, "jti 폐기 후 원본 토큰 재검증은 거부되어야 함")
}
