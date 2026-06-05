// jwt_issuer_test.go 는 JWTService 기반 TokenIssuer 어댑터를 검증한다
// (@SPEC:SPEC-REMOTE-001 M2, REQ-F02/F07 — JWTService 재사용).
package remote

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/auth"
)

// TestJWTTokenIssuer_IssueValidatesAsNodeRole 는 발급된 노드 토큰이 role=node,
// subject=instance_id 로 검증되는지 확인한다(REQ-F02).
func TestJWTTokenIssuer_IssueValidatesAsNodeRole(t *testing.T) {
	jwtSvc, err := auth.NewJWTService("test-secret", "1h", "24h")
	require.NoError(t, err)
	issuer := NewJWTTokenIssuer(jwtSvc)

	token, err := issuer.Issue("node-abc", "node")
	require.NoError(t, err)
	require.NotEmpty(t, token)

	subject, role, err := issuer.Validate(token)
	require.NoError(t, err)
	assert.Equal(t, "node-abc", subject)
	assert.Equal(t, "node", role)
}

// TestJWTTokenIssuer_RevokeDeniesValidation 는 폐기된 토큰이 검증에서 거부되는지
// 확인한다(REQ-F07 — blacklist 재사용).
func TestJWTTokenIssuer_RevokeDeniesValidation(t *testing.T) {
	jwtSvc, err := auth.NewJWTService("test-secret", "1h", "24h")
	require.NoError(t, err)
	issuer := NewJWTTokenIssuer(jwtSvc)

	token, err := issuer.Issue("node-xyz", "node")
	require.NoError(t, err)

	assert.False(t, issuer.IsRevoked(token))

	issuer.Revoke(token)
	assert.True(t, issuer.IsRevoked(token))

	// 폐기 후 검증은 거부된다.
	_, _, err = issuer.Validate(token)
	assert.Error(t, err, "폐기된 토큰은 검증에서 거부되어야 함")
}

// TestJWTTokenIssuer_InvalidTokenRejected 는 위조/임의 토큰이 거부되는지 확인한다.
func TestJWTTokenIssuer_InvalidTokenRejected(t *testing.T) {
	jwtSvc, err := auth.NewJWTService("test-secret", "1h", "24h")
	require.NoError(t, err)
	issuer := NewJWTTokenIssuer(jwtSvc)

	_, _, err = issuer.Validate("not-a-real-token")
	assert.Error(t, err)
}
