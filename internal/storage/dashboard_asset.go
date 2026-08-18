package storage

import (
	"context"
	"errors"
)

// DashboardAssetRepository 는 대시보드 패널이 참조하는 바이너리 자산(도면 이미지 등)의
// 영속 저장소 인터페이스이다.
//
// 왜 snapshot 과 분리하는가: 대시보드 snapshot 은 단일 JSON 으로 PUT/GET 되며 256KB
// 상한이 걸려 있다(handler.maxDashboardPayloadBytes). 도면 사진을 data-URL 로 config 에
// 박으면 그 한 장이 대시보드 전체 예산을 삼켜 **대시보드 저장 자체가 실패**한다. 자산을
// 별도 행으로 빼고 snapshot 에는 id 만 남기면 snapshot 은 작게 유지되고, 이미지는 자기
// 크기 상한(maxAssetBytes)만 지키면 된다.
//
// 내용 주소화(content-addressed): id 는 바이트 내용의 SHA-256 hex 이다. 같은 이미지를
// 여러 패널이 첨부해도 행은 하나이고, 업로드가 자연스럽게 멱등해진다.
type DashboardAssetRepository interface {
	// Put 은 자산을 저장하고 그 레코드를 반환한다. 같은 내용(id)이 이미 있으면 덮어쓰지 않고
	// 기존 레코드를 반환한다(멱등).
	Put(ctx context.Context, mime string, data []byte, nowMs int64) (*DashboardAsset, error)

	// Get 은 id 로 자산을 조회한다. 없으면 ErrDashboardAssetNotFound.
	Get(ctx context.Context, id string) (*DashboardAsset, error)
}

// DashboardAsset 은 저장된 자산 1건이다.
type DashboardAsset struct {
	// ID 는 내용의 SHA-256 hex(64자).
	ID string
	// MIME 은 Content-Type(예: image/png). 조회 응답 헤더에 그대로 쓰인다.
	MIME string
	// Size 는 바이트 크기.
	Size int64
	// CreatedAt 은 최초 저장 시각(epoch ms).
	CreatedAt int64
	// Data 는 원본 바이트. Get 에서만 채워진다(Put 응답은 메타데이터만 필요).
	Data []byte
}

// ErrDashboardAssetNotFound 는 id 에 해당하는 자산이 없을 때 반환된다.
var ErrDashboardAssetNotFound = errors.New("dashboard asset not found")
