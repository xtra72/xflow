// device_id_overwrite_test.go 는 Set 이 "같은 키 + 다른 값" 재지정 시 기존 매핑을
// 실제로 덮어쓰는지 검증한다.
//
// 기존 테스트가 덮는 것은 (a) 신규 지정, (b) 같은 키 + 같은 값 idempotent,
// (c) 다른 키 + 같은 값 충돌 세 가지뿐이라, 정작 "이미 값이 있는 키를 다른 값으로
// 갱신" 경로는 비어 있었다. ChirpStack 이 기존 UUID 를 devEui 로 전환하는 동작이
// 전적으로 이 경로에 의존하므로, 그 전제를 프로덕션에 배선된 파일 저장소
// (cmd/xflowd/main.go 가 NewDeviceIDFileRepository 를 주입) 로 직접 확인한다.

package storage

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDeviceIDFileRepository_Set_OverwritesExistingValue 는 파일 저장소가 기존
// 매핑을 새 값으로 갱신하고, 그 결과가 파일에 영속되어 재시작 후에도 유지되는지
// 검증한다.
func TestDeviceIDFileRepository_Set_OverwritesExistingValue(t *testing.T) {
	dir := t.TempDir()
	r, err := NewDeviceIDFileRepository(dir)
	require.NoError(t, err)

	ctx := context.Background()

	// 기존 상태: 자동 발급 UUID 가 배정돼 있다.
	require.NoError(t, r.Set(ctx, "chirpstack-1", "24e124141d180806", "550e8400-e29b-41d4-a716-446655440000"))

	// 같은 키에 다른 값을 지정하면 거부가 아니라 갱신이어야 한다.
	require.NoError(t, r.Set(ctx, "chirpstack-1", "24e124141d180806", "24e124141d180806"),
		"같은 키에 다른 값 재지정은 갱신이어야 한다(거부 아님)")

	got, err := r.Get(ctx, "chirpstack-1", "24e124141d180806")
	require.NoError(t, err)
	assert.Equal(t, "24e124141d180806", got, "Set 은 기존 값을 덮어써야 한다")

	// GetOrCreate 도 갱신된 값을 반환해야 한다(새 UUID 생성 금지).
	goc, err := r.GetOrCreate(ctx, "chirpstack-1", "24e124141d180806")
	require.NoError(t, err)
	assert.Equal(t, "24e124141d180806", goc)

	// 덮어쓴 값이 파일에 영속되어 재시작 후에도 유지된다.
	require.NoError(t, r.Close())
	r2, err := NewDeviceIDFileRepository(dir)
	require.NoError(t, err)
	defer r2.Close()

	reloaded, err := r2.Get(ctx, "chirpstack-1", "24e124141d180806")
	require.NoError(t, err)
	assert.Equal(t, "24e124141d180806", reloaded, "덮어쓴 값이 재시작 후에도 유지되어야 한다")

	// 옛 UUID 는 어떤 키에도 남아 있지 않아야 한다(유령 매핑 금지).
	all, err := r2.List(ctx)
	require.NoError(t, err)
	for k, v := range all {
		assert.NotEqual(t, "550e8400-e29b-41d4-a716-446655440000", v,
			"덮어쓰기 후에도 옛 UUID 가 키 %q 에 남아 있다", k)
	}
}

// TestDeviceIDMemoryRepository_Set_OverwritesExistingValue 는 메모리 저장소도
// 동일한 갱신 의미론을 갖는지 검증한다(두 구현의 의미론 드리프트 방지).
func TestDeviceIDMemoryRepository_Set_OverwritesExistingValue(t *testing.T) {
	r := NewDeviceIDMemoryRepository()
	ctx := context.Background()

	require.NoError(t, r.Set(ctx, "chirpstack-1", "24e124141d180806", "550e8400-e29b-41d4-a716-446655440000"))
	require.NoError(t, r.Set(ctx, "chirpstack-1", "24e124141d180806", "24e124141d180806"))

	got, err := r.Get(ctx, "chirpstack-1", "24e124141d180806")
	require.NoError(t, err)
	assert.Equal(t, "24e124141d180806", got)
}
