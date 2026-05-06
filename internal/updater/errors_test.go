// @SPEC:SPEC-UPDATE-001 v0.1.0
// errors_test.go — Phase A 단위 테스트: sentinel error 식별/래핑 검증
package updater

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestErrors_AreSentinels 는 9종 SPEC 에러 + 보조 에러가 non-nil 이고 서로 구별됨을 검증한다.
func TestErrors_AreSentinels(t *testing.T) {
	all := []error{
		ErrUpdateChannelInvalid,
		ErrUpdateDownloadFailed,
		ErrUpdateInsufficientDiskSpace,
		ErrUpdateChecksumMismatch,
		ErrUpdateSignatureInvalid,
		ErrUpdateApplyFailed,
		ErrUpdateRollbackFailed,
		ErrDowngradeRequiresForce,
		ErrUpdateInProgress,
		ErrUpdateInvalidInput, // 보조: 입력 검증 실패용 (verifier 등에서 사용)
	}

	for i, err := range all {
		assert.NotNil(t, err, "에러[%d] 가 nil", i)
	}

	// 모두 distinct 한지: pair-wise 비교 (errors.Is(a, b) 는 a != b 일 때 false)
	for i := 0; i < len(all); i++ {
		for j := 0; j < len(all); j++ {
			if i == j {
				assert.True(t, errors.Is(all[i], all[j]), "[%d]==[%d] should be Is", i, j)
			} else {
				assert.False(t, errors.Is(all[i], all[j]), "[%d]!=[%d] should not be Is", i, j)
			}
		}
	}
}

// TestErrors_Identity 는 wrapping 후에도 errors.Is 가 동작함을 검증한다 (sentinel 패턴 핵심).
func TestErrors_Identity(t *testing.T) {
	wrapped := fmt.Errorf("verifier: failure detail: %w", ErrUpdateSignatureInvalid)

	assert.True(t, errors.Is(wrapped, ErrUpdateSignatureInvalid))
	assert.False(t, errors.Is(wrapped, ErrUpdateChecksumMismatch))
}

// TestErrors_DoubleWrap 는 다중 래핑에서도 sentinel 식별을 보장한다.
func TestErrors_DoubleWrap(t *testing.T) {
	inner := fmt.Errorf("download phase: %w", ErrUpdateDownloadFailed)
	outer := fmt.Errorf("apply phase: %w", inner)
	assert.True(t, errors.Is(outer, ErrUpdateDownloadFailed))
}

// TestErrors_Messages 는 운영자/감사 로그용 메시지가 prefix("updater:") 를 갖는지 검증한다.
func TestErrors_Messages(t *testing.T) {
	type pair struct {
		err     error
		keyword string
	}
	cases := []pair{
		{ErrUpdateChannelInvalid, "channel"},
		{ErrUpdateDownloadFailed, "download"},
		{ErrUpdateInsufficientDiskSpace, "disk"},
		{ErrUpdateChecksumMismatch, "checksum"},
		{ErrUpdateSignatureInvalid, "signature"},
		{ErrUpdateApplyFailed, "apply"},
		{ErrUpdateRollbackFailed, "rollback"},
		{ErrDowngradeRequiresForce, "downgrade"},
		{ErrUpdateInProgress, "progress"},
		{ErrUpdateInvalidInput, "input"},
	}
	for _, c := range cases {
		t.Run(c.err.Error(), func(t *testing.T) {
			msg := c.err.Error()
			assert.True(t, strings.HasPrefix(msg, "updater:"), "메시지가 updater: prefix 를 가져야 함, got=%q", msg)
			assert.Contains(t, strings.ToLower(msg), strings.ToLower(c.keyword), "메시지가 키워드를 포함해야 함")
		})
	}
}
