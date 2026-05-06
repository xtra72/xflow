// @SPEC:SPEC-UPDATE-001 v0.1.0
// testutil_test.go — Phase B 테스트 전용 헬퍼 (production 코드에 포함되지 않음).
package updater

import "crypto/tls"

// insecureTLSConfig 는 다중 httptest.TLSServer 간 cross-server redirect 테스트 용도이다.
//
// production 경로에서는 절대 사용되지 않으며 (보안 critical: HTTPS + cert verify 강제),
// 단지 redirect follow 테스트를 위해 self-signed httptest cert 검증을 비활성화한다.
func insecureTLSConfig() *tls.Config {
	return &tls.Config{InsecureSkipVerify: true} // #nosec G402 — test-only
}
