package system

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestStoreErrors_NonNil 은 모든 센티넬 에러가 nil이 아닌지 확인한다.
func TestStoreErrors_NonNil(t *testing.T) {
	sentinels := []struct {
		name string
		err  error
	}{
		{"ErrKeyNotFound", ErrKeyNotFound},
		{"ErrNamespaceNotAllowed", ErrNamespaceNotAllowed},
		{"ErrStoreClosed", ErrStoreClosed},
		{"ErrStorePaused", ErrStorePaused},
		{"ErrNilValue", ErrNilValue},
		{"ErrInvalidTTL", ErrInvalidTTL},
		{"ErrNotSerializable", ErrNotSerializable},
		{"ErrKeyTooLong", ErrKeyTooLong},
	}

	for _, tc := range sentinels {
		t.Run(tc.name, func(t *testing.T) {
			require.NotNil(t, tc.err, "%s 는 nil이 아니어야 한다", tc.name)
		})
	}
}

// TestStoreErrors_UniqueMessages 는 모든 센티넬 에러의 메시지가 서로 다른지 확인한다.
func TestStoreErrors_UniqueMessages(t *testing.T) {
	sentinels := []error{
		ErrKeyNotFound,
		ErrNamespaceNotAllowed,
		ErrStoreClosed,
		ErrStorePaused,
		ErrNilValue,
		ErrInvalidTTL,
		ErrNotSerializable,
		ErrKeyTooLong,
	}

	seen := make(map[string]bool)
	for _, err := range sentinels {
		msg := err.Error()
		assert.False(t, seen[msg], "중복된 에러 메시지 발견: %s", msg)
		seen[msg] = true
	}
}

// TestStoreErrors_ErrorsIs 는 fmt.Errorf로 래핑한 에러가 errors.Is로 식별되는지 확인한다.
func TestStoreErrors_ErrorsIs(t *testing.T) {
	sentinels := []struct {
		name string
		err  error
	}{
		{"ErrKeyNotFound", ErrKeyNotFound},
		{"ErrNamespaceNotAllowed", ErrNamespaceNotAllowed},
		{"ErrStoreClosed", ErrStoreClosed},
		{"ErrStorePaused", ErrStorePaused},
		{"ErrNilValue", ErrNilValue},
		{"ErrInvalidTTL", ErrInvalidTTL},
		{"ErrNotSerializable", ErrNotSerializable},
		{"ErrKeyTooLong", ErrKeyTooLong},
	}

	for _, tc := range sentinels {
		t.Run(tc.name, func(t *testing.T) {
			wrapped := fmt.Errorf("추가 컨텍스트: %w", tc.err)
			assert.True(t, errors.Is(wrapped, tc.err),
				"래핑된 에러에서 %s를 식별할 수 있어야 한다", tc.name)
		})
	}
}

// TestStoreErrors_MessagePrefix 는 모든 에러 메시지가 "store:" 접두사를 갖는지 확인한다.
func TestStoreErrors_MessagePrefix(t *testing.T) {
	sentinels := []error{
		ErrKeyNotFound,
		ErrNamespaceNotAllowed,
		ErrStoreClosed,
		ErrStorePaused,
		ErrNilValue,
		ErrInvalidTTL,
		ErrNotSerializable,
		ErrKeyTooLong,
	}

	for _, err := range sentinels {
		assert.Contains(t, err.Error(), "store:",
			"에러 메시지는 'store:' 접두사를 포함해야 한다: %s", err.Error())
	}
}
