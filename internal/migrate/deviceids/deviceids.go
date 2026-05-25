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
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
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
type Planner struct {
	opts         Options
	metadataPath string
	idRepoPath   string

	// 캐시: Plan() 에서 로드된 원본 byte / metadata / mapping. Apply() 가 재사용.
	sourceBytes []byte
	metadata    rawMetadata
	idMapping   idMapping
}

// NewPlanner 는 Planner 를 생성한다.
//
// 디렉토리 존재 여부 검증은 본 단계에서 수행하지 않는다 (Plan() 에서 파일을
// 실제로 읽을 때 자연스럽게 발생). 빈 디렉토리는 신규 환경으로 간주.
func NewPlanner(_ context.Context, opts Options) (*Planner, error) {
	if opts.MetadataDir == "" {
		return nil, errors.New("metadata-dir 가 비어 있습니다")
	}
	if opts.IDRepoDir == "" {
		return nil, errors.New("id-repo 가 비어 있습니다")
	}
	if opts.Stdout == nil {
		opts.Stdout = io.Discard
	}
	if opts.Stderr == nil {
		opts.Stderr = io.Discard
	}
	return &Planner{
		opts:         opts,
		metadataPath: filepath.Join(opts.MetadataDir, metadataFileName),
		idRepoPath:   filepath.Join(opts.IDRepoDir, idRepoFileName),
	}, nil
}

// Plan 은 메타데이터와 ID repository 를 읽고 변환 계획을 생성한다.
//
// 본 메서드는 read-only 이며 파일을 변경하지 않는다. Apply 호출 전에 반드시
// 한 번 실행되어야 한다 (캐시된 metadata 가 Apply 에서 사용됨).
func (p *Planner) Plan(_ context.Context) (*Plan, error) {
	// 원본 byte 를 보관해야 백업이 byte-perfect 가능.
	sourceBytes, err := readFileOrEmpty(p.metadataPath)
	if err != nil {
		return nil, err
	}
	p.sourceBytes = sourceBytes

	meta, err := loadMetadata(p.metadataPath)
	if err != nil {
		return nil, err
	}
	p.metadata = meta

	ids, err := loadIDMapping(p.idRepoPath)
	if err != nil {
		return nil, err
	}
	p.idMapping = ids

	res := classify(meta, ids)
	return &Plan{
		convert:     res.convert,
		alreadyUUID: res.alreadyUUID,
		orphan:      res.orphan,
		ambiguous:   res.ambiguous,
	}, nil
}

// Apply 는 Plan 을 실제 파일 시스템에 반영한다.
//
// 절차:
//  1. 백업 생성 (metadataDir 또는 BackupDir).
//  2. metadata map 의 composite key 를 UUID 로 교체.
//  3. atomic rename 으로 device_metadata.json 갱신.
//  4. 변환 후 value sha256 합산이 백업 manifest 와 일치하는지 검증.
//
// Plan 이 호출되지 않은 상태에서 Apply 가 호출되면 에러.
func (p *Planner) Apply(_ context.Context, plan *Plan) (*Result, error) {
	if p.metadata == nil {
		return nil, errors.New("Plan() 이 먼저 호출되어야 합니다")
	}
	if plan == nil {
		return nil, errors.New("plan 이 nil 입니다")
	}

	// 1. 백업.
	backupDir, man, err := performBackup(p.opts.MetadataDir, p.opts.BackupDir, p.sourceBytes, p.metadata)
	if err != nil {
		return nil, err
	}

	// 2. metadata 의 key 변환 — value 는 byte-perfect 보존.
	converted := make(rawMetadata, len(p.metadata))
	for k, v := range p.metadata {
		converted[k] = v
	}
	for _, e := range plan.convert {
		// composite key 의 value 를 UUID key 로 옮기고 composite 삭제.
		// 이미 UUID 가 존재하면 안 됨 (classify 단계에서 ambiguous 로 걸렀음).
		converted[e.UUID] = converted[e.Composite]
		delete(converted, e.Composite)
	}

	// 3. atomic rename.
	if err := saveMetadataAtomic(p.metadataPath, converted); err != nil {
		return nil, fmt.Errorf("metadata 쓰기 실패 (백업: %s): %w", backupDir, err)
	}

	// 4. 검증 — 변환 후 value sha256 합산이 백업과 일치해야 한다.
	postSum := computeValueHashSum(converted)
	preSum := computeValueHashSumFromManifest(man)
	verified := postSum == preSum && len(converted) == man.EntryCount

	return &Result{
		Verified:   verified,
		Converted:  len(plan.convert),
		Skipped:    len(plan.orphan) + len(plan.ambiguous),
		BackupPath: backupDir,
	}, nil
}

// readFileOrEmpty 는 path 의 파일을 읽고, 파일이 없으면 빈 byte slice 와 nil 에러를 반환한다.
//
// 신규 환경 (메타데이터 파일이 아직 생성되지 않음) 에서 도구를 실행해도 graceful.
func readFileOrEmpty(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []byte{}, nil
		}
		return nil, fmt.Errorf("파일 읽기 실패 (%s): %w", path, err)
	}
	return data, nil
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
	Conflicts []string // 동일 UUID 와 매핑된 다른 composite 들 (또는 충돌 사유 설명)
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

// Convert 는 변환 대상 슬라이스의 복사본을 반환한다 (외부 검사용).
func (p *Plan) Convert() []ConvertEntry {
	out := make([]ConvertEntry, len(p.convert))
	copy(out, p.convert)
	return out
}

// Print 는 계획을 사람이 읽기 좋은 형태로 출력한다 (요약 + 상세).
//
// 출력 형식 (acceptance C-AC1 dry-run 출력 예시 준수):
//
//	Plan summary: convert=N ambiguous=M orphan=O already-uuid=P
//	  [convert]
//	    "lgcnp:81" -> "a58ba668-..."
//	    ...
//	  [ambiguous]
//	    "lgcnp:99" -> "uuid-x" (conflicts: lgcp:1)
//	  [orphan]
//	    "samsung:0.0.16"
func (p *Plan) Print(w io.Writer) error {
	if _, err := fmt.Fprintf(w,
		"Plan summary: convert=%d ambiguous=%d orphan=%d already-uuid=%d\n",
		p.ConvertCount(), p.AmbiguousCount(), p.OrphanCount(), p.AlreadyUUIDCount()); err != nil {
		return err
	}
	if len(p.convert) > 0 {
		if _, err := fmt.Fprintln(w, "  [convert]"); err != nil {
			return err
		}
		for _, e := range p.convert {
			if _, err := fmt.Fprintf(w, "    %q -> %q\n", e.Composite, e.UUID); err != nil {
				return err
			}
		}
	}
	if len(p.ambiguous) > 0 {
		if _, err := fmt.Fprintln(w, "  [ambiguous]"); err != nil {
			return err
		}
		for _, e := range p.ambiguous {
			if _, err := fmt.Fprintf(w, "    %q -> %q (conflicts: %v)\n",
				e.Composite, e.UUID, e.Conflicts); err != nil {
				return err
			}
		}
	}
	if len(p.orphan) > 0 {
		if _, err := fmt.Fprintln(w, "  [orphan]"); err != nil {
			return err
		}
		for _, c := range p.orphan {
			if _, err := fmt.Fprintf(w, "    %q\n", c); err != nil {
				return err
			}
		}
	}
	if len(p.alreadyUUID) > 0 {
		if _, err := fmt.Fprintf(w, "  [already-uuid] %d entries (idempotent, will be left as-is)\n",
			len(p.alreadyUUID)); err != nil {
			return err
		}
	}
	return nil
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

// Ensure imported packages are used in deviceids.go to keep the file compilable
// when others use them.
var _ = json.Marshal
