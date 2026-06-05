// jwt_issuer.go 는 internal/auth.JWTService 를 TokenIssuer 로 어댑트한다
// (@SPEC:SPEC-REMOTE-001 M2, REQ-C04/C05/C07/F02/F07 — 기존 JWTService 재사용,
// 재발명 금지).
//
// 노드 토큰은 access 토큰(role="node", subject=instance_id)을 사용한다. 폐기는
// JWTService.Blacklist(token) 로, 검증은 ValidateToken 으로, blacklist 확인은
// IsBlacklisted 로 처리한다.
package remote

import (
	"github.com/xtra/xflow/internal/auth"
)

// jwtTokenIssuer 는 *auth.JWTService 기반 TokenIssuer 구현이다.
type jwtTokenIssuer struct {
	svc *auth.JWTService
}

var _ TokenIssuer = (*jwtTokenIssuer)(nil)

// NewJWTTokenIssuer 는 JWTService 를 래핑한 TokenIssuer 를 생성한다.
func NewJWTTokenIssuer(svc *auth.JWTService) TokenIssuer {
	return &jwtTokenIssuer{svc: svc}
}

// Issue 는 subject(=instance_id) + role 로 노드 토큰을 발급한다. access 토큰을
// 노드 토큰으로 사용한다(refresh 토큰은 노드 시나리오에서 불필요).
func (j *jwtTokenIssuer) Issue(subject, role string) (string, error) {
	accessToken, _, _, err := j.svc.GenerateTokens(subject, role)
	if err != nil {
		return "", err
	}
	return accessToken, nil
}

// Validate 는 토큰을 검증하고 subject(username)/role 을 반환한다. blacklist 토큰은
// 거부한다(REQ-F07).
func (j *jwtTokenIssuer) Validate(token string) (string, string, error) {
	if j.svc.IsBlacklisted(token) {
		return "", "", auth.ErrBlacklistToken
	}
	claims, err := j.svc.ValidateToken(token)
	if err != nil {
		return "", "", err
	}
	return claims.Username, claims.Role, nil
}

// Revoke 는 토큰을 blacklist 에 추가하여 즉시 무효화한다(REQ-F07).
func (j *jwtTokenIssuer) Revoke(token string) {
	j.svc.Blacklist(token)
}

// IsRevoked 는 토큰이 blacklist 되었는지 확인한다.
func (j *jwtTokenIssuer) IsRevoked(token string) bool {
	return j.svc.IsBlacklisted(token)
}
