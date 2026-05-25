// Package deviceids implements the SPEC-DEVICE-IDENTITY-001 Phase C § C1
// migration tool that converts device_metadata.json map keys from composite
// (agent:unit_id) to UUID.
//
// The migration is split into three phases:
//   - Plan: scan metadata + ID repository, classify entries into convert /
//     ambiguous / orphan / already-uuid.
//   - Backup: copy the original file + sha256 manifest to backup directory.
//   - Apply: rewrite the file in-place with UUID keys using atomic rename.
//
// Safety invariants (acceptance C-AC2 / MIG-AC1..5):
//   - All-or-nothing: if any step fails, the original file is preserved.
//   - Idempotency: re-running on a UUID-keyed file is a no-op.
//   - Hash verification: post-migration metadata values match pre-migration
//     entry-by-entry (entry count + per-key sha256 stable).
package deviceids

import (
	"context"
	"errors"
	"fmt"
	"io"
)

// Options 는 마이그레이션 실행에 필요한 설정을 보관한다.
//
// 모든 경로는 절대 또는 상대 경로 모두 허용된다 (caller 가 정규화).
type Options struct {
	// MetadataDir 는 device_metadata.json 가 위치한 디렉토리이다 (필수).
	MetadataDir string

	// IDRepoDir 는 device_ids.json 이 위치한 디렉토리이다 (필수).
	// 이 저장소가 composite → UUID 매핑의 권위 (source of truth) 이다.
	IDRepoDir string

	// BackupDir 가 빈 문자열이면 <MetadataDir>/.backup-<timestamp> 가 사용된다.
	BackupDir string

	// DryRun 이 true 면 Apply 가 호출되지 않고 계획만 출력된다.
	DryRun bool

	// Strict 가 true 면 ambiguous / orphan mapping 발견 시 Plan 단계에서 abort.
	Strict bool

	// Stdout / Stderr 는 진행 보고 출력 대상이다 (테스트 격리용).
	Stdout io.Writer
	Stderr io.Writer
}

// Planner 는 마이그레이션의 전체 흐름을 관장한다.
//
// 본 골격 단계 (commit 1) 에서는 시그니처만 노출하고 후속 커밋에서 채워진다.
type Planner struct {
	opts Options
}

// NewPlanner 는 Planner 를 생성한다.
//
// opts 의 필수 필드 (MetadataDir, IDRepoDir) 를 검증하고 실패 시 즉시 에러를 반환한다.
func NewPlanner(_ context.Context, opts Options) (*Planner, error) {
	if opts.MetadataDir == "" {
		return nil, errors.New("metadata-dir 가 비어 있습니다")
	}
	if opts.IDRepoDir == "" {
		return nil, errors.New("id-repo 가 비어 있습니다")
	}
	return &Planner{opts: opts}, nil
}

// Plan 은 메타데이터와 ID repository 를 읽고 변환 계획을 생성한다.
//
// 본 골격 단계에서는 빈 Plan 을 반환한다. 후속 커밋 (core 구현) 에서 실제
// 메타데이터 스캔과 분류 로직이 채워진다.
func (p *Planner) Plan(_ context.Context) (*Plan, error) {
	return &Plan{}, nil
}

// Apply 는 Plan 을 실제 파일 시스템에 반영한다.
//
// 본 골격 단계에서는 dry-run-only stub 으로 동작한다. 후속 커밋에서 백업 +
// atomic rename + 검증 로직이 채워진다.
func (p *Planner) Apply(_ context.Context, _ *Plan) (*Result, error) {
	return nil, errors.New("apply 는 아직 구현되지 않았습니다 (commit 2 에서 채워질 예정)")
}

// Plan 은 변환 계획을 표현한다.
//
// 각 entry 는 4 가지 카테고리 중 하나에 속한다:
//   - Convert: composite key → UUID 매핑이 존재하고 UUID 가 메타데이터에 아직 없음.
//   - AlreadyUUID: key 가 이미 UUID 형식이거나 동일 UUID 가 메타데이터에 존재 (idempotent).
//   - Orphan: composite key 인데 ID repository 에 매핑이 없음.
//   - Ambiguous: composite key 가 ID repository 에 매핑되었으나 동일 UUID 가
//     다른 composite 와도 매핑된 경우 (다대일 매핑).
type Plan struct {
	convert     []ConvertEntry
	alreadyUUID []string
	orphan      []string
	ambiguous   []AmbiguousEntry
}

// ConvertEntry 는 composite 에서 UUID 로 변환될 단일 엔트리를 나타낸다.
type ConvertEntry struct {
	Composite string
	UUID      string
}

// AmbiguousEntry 는 모호한 매핑을 나타낸다.
type AmbiguousEntry struct {
	Composite string
	UUID      string
	Conflicts []string // 동일 UUID 와 매핑된 다른 composite 들
}

// ConvertCount 는 변환 대상 엔트리 수를 반환한다.
func (p *Plan) ConvertCount() int { return len(p.convert) }

// AmbiguousCount 는 모호한 매핑 수를 반환한다.
func (p *Plan) AmbiguousCount() int { return len(p.ambiguous) }

// OrphanCount 는 매핑 없는 composite 수를 반환한다.
func (p *Plan) OrphanCount() int { return len(p.orphan) }

// AlreadyUUIDCount 는 이미 UUID 명명된 엔트리 수를 반환한다 (idempotent).
func (p *Plan) AlreadyUUIDCount() int { return len(p.alreadyUUID) }

// HasAmbiguous 는 ambiguous 가 1건 이상이면 true 를 반환한다.
func (p *Plan) HasAmbiguous() bool { return len(p.ambiguous) > 0 }

// Print 는 계획을 사람이 읽기 좋은 형태로 출력한다.
//
// 본 골격 단계에서는 기본 카운터만 출력한다. 후속 커밋에서 상세 매핑 목록이 추가된다.
func (p *Plan) Print(w io.Writer) error {
	_, err := fmt.Fprintf(w,
		"Plan: convert=%d ambiguous=%d orphan=%d already-uuid=%d\n",
		p.ConvertCount(), p.AmbiguousCount(), p.OrphanCount(), p.AlreadyUUIDCount())
	return err
}

// Result 는 Apply 의 실행 결과를 보관한다.
type Result struct {
	// Verified 는 변환 후 검증이 통과하면 true.
	Verified bool

	// Converted 는 실제로 변환된 entry 수.
	Converted int

	// Skipped 는 ambiguous/orphan 으로 건너뛴 entry 수.
	Skipped int

	// BackupPath 는 백업 디렉토리의 절대 경로.
	BackupPath string
}

// Print 는 결과를 사람이 읽기 좋은 형태로 출력한다.
func (r *Result) Print(w io.Writer) error {
	_, err := fmt.Fprintf(w,
		"Result: converted=%d skipped=%d verified=%t backup=%s\n",
		r.Converted, r.Skipped, r.Verified, r.BackupPath)
	return err
}
