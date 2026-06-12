// settings_repository.go 는 전역(앱 전체 공유) key-value 설정 영속 저장소
// 인터페이스를 정의한다.
//
// 디바이스 컬럼 구성 등 "전역 1벌"로 공유되는 UI/서버 설정을 저장한다. 서버는
// value 의 스키마를 강제하지 않고 불투명(opaque) JSON 문자열로 저장/반환만 한다.
// 키 네임스페이스는 프론트엔드가 정한다(예: "device-list-columns").
//
// 기존 FlowRepository/DashboardRepository 패턴(인터페이스 + sqlite 구현 + factory)을
// 준용한다. updated_at 은 epoch milliseconds(int64)이다(프로젝트 규약).
package storage

import (
	"context"
	"errors"
)

// ErrSettingNotFound 는 해당 key 의 설정이 없을 때 GetSetting 이 반환한다.
var ErrSettingNotFound = errors.New("setting not found")

// SettingsRepository 는 전역 key-value 설정의 영속 저장소 인터페이스이다.
//
// 전역 단일 네임스페이스이며 사용자/스코프 구분이 없다(전역 1벌). value 는 불투명
// JSON 문자열로 저장되며 서버는 스키마를 검증하지 않는다.
type SettingsRepository interface {
	// GetSetting 은 key 에 해당하는 value(JSON 문자열)를 반환한다.
	// 없으면 ErrSettingNotFound 를 반환한다.
	GetSetting(ctx context.Context, key string) (string, error)

	// SetSetting 은 key 에 value 를 저장한다(있으면 덮어쓴다 — UPSERT).
	SetSetting(ctx context.Context, key, value string) error

	// Close 는 저장소 리소스를 정리한다.
	Close() error
}

// NewSettingsRepository 는 storage type 에 따라 SettingsRepository 구현을 생성한다.
// 현재는 sqlite 만 지원한다(전역 서버 측 저장소). 알 수 없는 type 은 sqlite 로 폴백한다.
func NewSettingsRepository(ctx context.Context, storageType, sqlitePath string) (SettingsRepository, error) {
	switch storageType {
	case "sqlite", "file":
		return NewSettingsSQLiteRepository(ctx, sqlitePath)
	default:
		return NewSettingsSQLiteRepository(ctx, sqlitePath)
	}
}
