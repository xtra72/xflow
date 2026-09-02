// @spec SPEC-STORE-004 (O3, M6)
// Package storeseries 는 영속 store 백엔드의 "시리즈 도입 이전(bare key)" 엔트리를
// (key, "unknown", {}) 기본 시리즈 인코딩 키로 변환하는 일회성 마이그레이션 유틸이다.
//
// 배경 (SPEC §기존 데이터 마이그레이션, 가정 A1/A6):
//   - 인메모리(VolatileStore) 데이터는 재시작 시 휘발되므로 변환이 불필요하다(A6).
//     재시작 후 신규 쓰기가 시리즈 모델로 자연 흡수된다.
//   - 영속 백엔드(StoreRepository) 가 활성화된 경우에만, 기존 단일-키 엔트리를
//     기본 시리즈 인코딩 키로 재인코딩하는 명시적 변환이 필요하다.
//   - 현재 운영 store 에이전트는 VolatileStore 만 사용하며 활성 영속 백엔드가 없다(A1).
//     따라서 본 유틸은 "향후 영속 백엔드 활성 시" 를 위한 안전한 도구로 제공되며,
//     실제 라이브 데이터에 자동 실행되지 않는다 (cmd 진입점은 명시적 호출만 허용).
//
// 안전 불변식 (plan.md 리스크 "마이그레이션 데이터 손실" 대응):
//   - 변환 전 백업: 변환 대상 엔트리를 JSON manifest + sha256 으로 백업한다.
//   - dry-run 모드: 무엇이 어떻게 바뀌는지 출력만 하고 리포지토리를 변경하지 않는다.
//   - 멱등성(idempotency): 이미 시리즈 인코딩된 키(DecodeSeriesKey 성공 + 정규
//     형식 round-trip 일치)는 건너뛴다. 재실행해도 추가 변환이 없다.
//   - 변환 후 검증: 변환 건수/값 sha256 합산이 백업과 일치하는지 카운트·샘플 검증.
//   - all-or-nothing 에 가까운 절차: 각 키는 SetEntry(new) → DeleteEntry(old) 순서로
//     수행하여, 중간 실패 시에도 원본(old) 또는 변환본(new) 중 하나는 보존된다.
package storeseries

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"time"

	"github.com/xtra/xflow/internal/agent/system"
)

// Repository 는 본 마이그레이션이 의존하는 영속 백엔드 계약이다.
//
// system.StoreRepository 의 부분집합(ListKeys/GetEntry/SetEntry/DeleteEntry)만 사용한다.
// 어떤 system.StoreRepository 구현도 본 인터페이스를 만족하므로, 향후 영속 백엔드가
// 활성화되면 그 구현을 그대로 주입할 수 있다. 테스트는 in-memory fake 를 주입한다.
type Repository interface {
	ListKeys(ctx context.Context, pattern string) ([]string, error)
	GetEntry(ctx context.Context, key string) (*system.StoreEntry, error)
	SetEntry(ctx context.Context, key string, entry *system.StoreEntry) error
	DeleteEntry(ctx context.Context, key string) error
}

// 컴파일 타임: 어떤 system.StoreRepository 든 본 Repository 를 만족함을 문서화한다.
// (system.StoreRepository 는 DeleteExpired/ClearNamespace 를 더 가지므로 상위집합이다.)

// Options 는 마이그레이션 실행 설정이다.
type Options struct {
	// DryRun 이 true 면 리포지토리를 변경하지 않고 계획만 출력한다.
	DryRun bool

	// Backup 이 non-nil 이면 변환 직전 백업 manifest 를 이 writer 에 기록한다.
	// nil 이면 백업을 건너뛴다(dry-run 또는 외부 백업 보장 시). 운영 변환에서는
	// 반드시 non-nil 로 백업을 활성화할 것을 권장한다.
	Backup io.Writer

	// Stdout / Stderr 는 진행/경고 보고 출력 대상이다 (테스트 격리용).
	Stdout io.Writer
	Stderr io.Writer
}

// Planner 는 마이그레이션의 Plan/Apply 흐름을 관장한다.
type Planner struct {
	repo Repository
	opts Options

	// 캐시: Plan() 이 로드한 변환 계획. Apply() 가 재사용한다.
	plan *Plan
}

// NewPlanner 는 Repository 와 Options 로 Planner 를 생성한다.
func NewPlanner(repo Repository, opts Options) (*Planner, error) {
	if repo == nil {
		return nil, errors.New("storeseries: repository 가 nil 입니다")
	}
	if opts.Stdout == nil {
		opts.Stdout = io.Discard
	}
	if opts.Stderr == nil {
		opts.Stderr = io.Discard
	}
	return &Planner{repo: repo, opts: opts}, nil
}

// ConvertEntry 는 bare key → 시리즈 인코딩 키 변환 단일 항목이다.
type ConvertEntry struct {
	// OldKey 는 변환 전 키(시리즈 도입 이전 bare key 또는 namespace prefix 포함 키).
	OldKey string
	// NewKey 는 (OldKey, "unknown", {}) 기본 시리즈로 재인코딩한 키이다.
	NewKey string
}

// Plan 은 변환 계획이다.
//
// 각 키는 두 분류 중 하나에 속한다:
//   - convert: 시리즈 인코딩이 아닌 bare key. (key,"unknown",{}) 기본 시리즈로 변환.
//   - alreadySeries: 이미 정규 시리즈 인코딩 키. 멱등 — 건너뛴다.
type Plan struct {
	convert       []ConvertEntry
	alreadySeries []string
}

// ConvertCount 는 변환 대상 수를 반환한다.
func (p *Plan) ConvertCount() int { return len(p.convert) }

// AlreadySeriesCount 는 이미 시리즈 인코딩되어 건너뛸 키 수를 반환한다.
func (p *Plan) AlreadySeriesCount() int { return len(p.alreadySeries) }

// Convert 는 변환 대상 슬라이스의 복사본을 반환한다 (외부 검사/테스트용).
func (p *Plan) Convert() []ConvertEntry {
	out := make([]ConvertEntry, len(p.convert))
	copy(out, p.convert)
	return out
}

// Print 는 계획을 사람이 읽기 좋은 형태로 출력한다 (요약 + 상세, dry-run 핵심 출력).
func (p *Plan) Print(w io.Writer) error {
	if _, err := fmt.Fprintf(w,
		"Plan summary: convert=%d already-series=%d\n",
		p.ConvertCount(), p.AlreadySeriesCount()); err != nil {
		return err
	}
	if len(p.convert) > 0 {
		if _, err := fmt.Fprintln(w, "  [convert]"); err != nil {
			return err
		}
		for _, e := range p.convert {
			if _, err := fmt.Fprintf(w, "    %q -> %q\n", e.OldKey, e.NewKey); err != nil {
				return err
			}
		}
	}
	if len(p.alreadySeries) > 0 {
		if _, err := fmt.Fprintf(w,
			"  [already-series] %d keys (idempotent, will be left as-is)\n",
			len(p.alreadySeries)); err != nil {
			return err
		}
	}
	return nil
}

// Plan 은 리포지토리의 모든 키를 스캔하여 변환 계획을 생성한다 (read-only).
//
// 분류 규칙:
//   - 키가 isCanonicalSeriesKey 이면 alreadySeries (멱등 skip).
//   - 그 외(bare key)는 convert 대상. NewKey = (key,"unknown",{}) 기본 시리즈 인코딩.
//
// Apply 호출 전 반드시 한 번 실행되어야 한다 (계획이 캐시됨).
func (p *Planner) Plan(ctx context.Context) (*Plan, error) {
	keys, err := p.repo.ListKeys(ctx, "*")
	if err != nil {
		return nil, fmt.Errorf("storeseries: 키 목록 조회 실패: %w", err)
	}

	// 결정론적 출력을 위해 키를 정렬한다.
	sort.Strings(keys)

	plan := &Plan{}
	for _, k := range keys {
		if isCanonicalSeriesKey(k) {
			plan.alreadySeries = append(plan.alreadySeries, k)
			continue
		}
		plan.convert = append(plan.convert, ConvertEntry{
			OldKey: k,
			NewKey: defaultSeriesKey(k),
		})
	}

	p.plan = plan
	return plan, nil
}

// Result 는 Apply 실행 결과이다.
type Result struct {
	// Converted 는 실제로 변환된 키 수.
	Converted int
	// Skipped 는 이미 시리즈여서 건너뛴 키 수.
	Skipped int
	// Verified 는 변환 후 검증(건수 + value sha256 합산 일치)이 통과하면 true.
	Verified bool
	// DryRun 은 dry-run 모드로 실행되어 실제 변경이 없었으면 true.
	DryRun bool
}

// Print 는 결과를 사람이 읽기 좋은 형태로 출력한다.
func (r *Result) Print(w io.Writer) error {
	_, err := fmt.Fprintf(w,
		"Result: converted=%d skipped=%d verified=%t dry-run=%t\n",
		r.Converted, r.Skipped, r.Verified, r.DryRun)
	return err
}

// Apply 는 Plan 을 리포지토리에 반영한다.
//
// 절차:
//  1. (Backup 활성 시) 변환 대상 엔트리를 sha256 manifest 로 백업한다.
//  2. DryRun 이면 여기서 종료 — 리포지토리를 변경하지 않는다.
//  3. 각 변환 항목에 대해 SetEntry(NewKey, entry) → DeleteEntry(OldKey) 순서로 수행.
//     (new 를 먼저 쓰므로 중간 실패 시에도 데이터 손실이 없다.)
//  4. 변환 후 검증: 변환된 NewKey 들의 value sha256 합산이 백업과 일치하는지 확인.
//
// Plan 이 호출되지 않은 상태에서 Apply 가 호출되면 에러를 반환한다.
func (p *Planner) Apply(ctx context.Context) (*Result, error) {
	if p.plan == nil {
		return nil, errors.New("storeseries: Plan() 이 먼저 호출되어야 합니다")
	}
	plan := p.plan

	// 1. 변환 대상 엔트리 로드 + (선택) 백업 manifest 생성.
	preEntries := make(map[string]*system.StoreEntry, len(plan.convert))
	for _, e := range plan.convert {
		entry, err := p.repo.GetEntry(ctx, e.OldKey)
		if err != nil {
			return nil, fmt.Errorf("storeseries: 백업용 엔트리 조회 실패 (%s): %w", e.OldKey, err)
		}
		preEntries[e.OldKey] = entry
	}

	man := computeManifest(plan.convert, preEntries)
	if p.opts.Backup != nil {
		if err := writeManifest(p.opts.Backup, man); err != nil {
			return nil, fmt.Errorf("storeseries: 백업 manifest 기록 실패: %w", err)
		}
	}

	res := &Result{
		Skipped: plan.AlreadySeriesCount(),
		DryRun:  p.opts.DryRun,
	}

	// 2. dry-run 이면 실제 변경 없이 검증 생략하고 반환.
	if p.opts.DryRun {
		fmt.Fprintf(p.opts.Stdout, "dry-run: %d 개 키 변환이 예정됨 (실제 변경 없음)\n", plan.ConvertCount())
		return res, nil
	}

	// 3. 변환 수행 — new 먼저 쓰고 old 삭제 (데이터 손실 방지).
	for _, e := range plan.convert {
		entry := preEntries[e.OldKey]
		if err := p.repo.SetEntry(ctx, e.NewKey, entry); err != nil {
			return nil, fmt.Errorf("storeseries: 변환 키 쓰기 실패 (%s): %w", e.NewKey, err)
		}
		// new == old 인 경우(이론상 발생하지 않음)에는 삭제하지 않는다.
		if e.NewKey != e.OldKey {
			if err := p.repo.DeleteEntry(ctx, e.OldKey); err != nil {
				return nil, fmt.Errorf("storeseries: 원본 키 삭제 실패 (%s, 변환본 %s 는 기록됨): %w",
					e.OldKey, e.NewKey, err)
			}
		}
		res.Converted++
	}

	// 4. 변환 후 검증 — NewKey 들의 value sha256 합산이 백업과 일치해야 한다.
	postEntries := make(map[string]*system.StoreEntry, len(plan.convert))
	for _, e := range plan.convert {
		entry, err := p.repo.GetEntry(ctx, e.NewKey)
		if err != nil {
			return nil, fmt.Errorf("storeseries: 검증용 엔트리 조회 실패 (%s): %w", e.NewKey, err)
		}
		postEntries[e.NewKey] = entry
	}
	postSum := valueHashSum(postEntries)
	res.Verified = postSum == man.ValueSHA256Sum && len(postEntries) == man.EntryCount

	return res, nil
}

// isCanonicalSeriesKey 는 키가 이미 정규 시리즈 인코딩 형식인지 판정한다 (멱등성 근거).
//
// 판정: DecodeSeriesKey 가 성공하고, 디코딩 결과를 다시 EncodeSeriesKey 한 값이 원본과
// byte-identical 하면 "정규 시리즈 키" 로 간주한다. round-trip 일치를 요구함으로써
// 우연히 `|` 를 포함하는 bare key (예: "a|b|c") 가 시리즈로 오분류되어도, 그 재인코딩이
// 원본과 다르면 convert 대상으로 안전하게 처리되도록 한다.
//
// 예:
//   - "room"            → Decode 실패(`|` 0개) → false → convert.
//   - "unknown||room"   → Decode 성공 → Re-encode "unknown||room" == 원본 → true → skip.
//   - "temperature|area=a|room" → round-trip 일치 → true → skip.
func isCanonicalSeriesKey(key string) bool {
	sid, err := system.DecodeSeriesKey(key)
	if err != nil {
		return false
	}
	return system.EncodeSeriesKey(sid) == key
}

// defaultSeriesKey 는 bare key 를 (key, "unknown", {}) 기본 시리즈 인코딩 키로 변환한다.
func defaultSeriesKey(bareKey string) string {
	return system.EncodeSeriesKey(system.SeriesID{Measurement: bareKey})
}

// manifest 는 백업/검증용 sha256 합산 + per-key sha256 표현이다.
type manifest struct {
	// Generated 는 manifest 생성 시각 (RFC3339 UTC).
	Generated string `json:"generated"`
	// EntryCount 는 변환 대상 엔트리 수.
	EntryCount int `json:"entry_count"`
	// Conversions 는 OldKey → NewKey 매핑 목록 (복원/감사용).
	Conversions []ConvertEntry `json:"conversions"`
	// ValueSHA256Sum 은 모든 value 의 sha256 hex 를 정렬·결합 후 다시 sha256 한 값.
	// key 가 변환되어도 value 만 보존되면 변환 전후 동일하다 (검증 기준).
	ValueSHA256Sum string `json:"value_sha256_sum"`
	// ValueSHA256ByKey 는 진단용 OldKey → value sha256 매핑이다.
	ValueSHA256ByKey map[string]string `json:"value_sha256_by_key"`
}

// computeManifest 는 변환 계획과 변환 전 엔트리로부터 manifest 를 생성한다.
func computeManifest(convert []ConvertEntry, pre map[string]*system.StoreEntry) manifest {
	byKey := make(map[string]string, len(convert))
	for _, e := range convert {
		byKey[e.OldKey] = entryValueHash(pre[e.OldKey])
	}
	return manifest{
		Generated:        time.Now().UTC().Format(time.RFC3339),
		EntryCount:       len(convert),
		Conversions:      append([]ConvertEntry(nil), convert...),
		ValueSHA256Sum:   hashSumFromMap(byKey),
		ValueSHA256ByKey: byKey,
	}
}

// entryValueHash 는 엔트리의 value 를 JSON 직렬화한 byte 의 sha256 hex 를 반환한다.
// nil 엔트리는 빈 문자열 해시로 취급한다 (방어적).
func entryValueHash(entry *system.StoreEntry) string {
	var v any
	if entry != nil {
		v = entry.Value
	}
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// valueHashSum 은 변환 후 엔트리 맵의 value sha256 합산을 계산한다.
func valueHashSum(entries map[string]*system.StoreEntry) string {
	byKey := make(map[string]string, len(entries))
	for k, e := range entries {
		byKey[k] = entryValueHash(e)
	}
	return hashSumFromMap(byKey)
}

// hashSumFromMap 은 per-key sha256 hex 들을 정렬·결합 후 다시 sha256 하여 합산을 만든다.
// key 순서/이름과 무관하게 value 해시 집합이 같으면 동일한 합산을 보장한다.
func hashSumFromMap(byKey map[string]string) string {
	hashes := make([]string, 0, len(byKey))
	for _, v := range byKey {
		hashes = append(hashes, v)
	}
	sort.Strings(hashes)
	combined := sha256.New()
	for _, h := range hashes {
		_, _ = combined.Write([]byte(h))
	}
	return hex.EncodeToString(combined.Sum(nil))
}

// writeManifest 는 manifest 를 들여쓰기된 JSON 으로 writer 에 기록한다.
func writeManifest(w io.Writer, man manifest) error {
	b, err := json.MarshalIndent(man, "", "  ")
	if err != nil {
		return fmt.Errorf("manifest 직렬화 실패: %w", err)
	}
	if _, err := w.Write(b); err != nil {
		return err
	}
	_, err = w.Write([]byte("\n"))
	return err
}
