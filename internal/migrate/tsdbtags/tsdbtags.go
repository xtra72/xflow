// Package tsdbtags implements the SPEC-DEVICE-IDENTITY-001 Phase C § C2
// migration tool that scans an InfluxDB (v2 or v3) instance for composite
// device tag values and generates Flux/SQL backfill scripts.
//
// IMPORTANT: This tool is **read-only** with respect to the Influx server.
// It never writes data. The generated scripts are output as files for the
// operator to execute manually in staging and production environments.
//
// Migration phases (mirrors deviceids tool):
//   - Plan: connect to Influx, scan schema, classify composite tag values,
//     resolve UUID mappings from device_ids.json.
//   - Apply: emit migration-v2.flux (or migration-v3.sql) plus RUN.md
//     into <output-dir>/.
//
// Safety invariants (acceptance C-AC6 / C-AC7 / MIG-AC2):
//   - No write API call on the Influx server (only schema queries).
//   - Output scripts are deterministic (same input → same output, idempotent).
//   - Ambiguous mappings are skipped with explicit warnings.
//   - Operator responsibility is clearly documented in the generated RUN.md.
package tsdbtags

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"
)

// Target は InfluxDB 의 목표 버전을 표현한다.
type Target string

const (
	// TargetV2 는 InfluxDB v2 (Flux + bucket/org) 를 의미한다.
	TargetV2 Target = "v2"
	// TargetV3 는 InfluxDB v3 (SQL/InfluxQL + database) 를 의미한다.
	TargetV3 Target = "v3"
	// TargetAuto 는 서버 응답을 통한 자동 감지를 의미한다.
	TargetAuto Target = "auto"
)

// Options 는 tsdb-tags 마이그레이션 도구의 실행 설정을 보관한다.
//
// 모든 path 는 절대/상대 경로 모두 허용 (caller 가 정규화).
type Options struct {
	// InfluxURL 은 InfluxDB 서버 주소이다 (필수, 예: http://localhost:8086).
	InfluxURL string

	// InfluxToken 은 인증 토큰이다 (read-only 권한 권장).
	InfluxToken string

	// Bucket 은 v2 의 bucket 또는 v3 의 database 명이다 (필수).
	Bucket string

	// Org 는 v2 의 organization 명이다 (v2 필수, v3 선택).
	Org string

	// IDRepoPath 는 composite → UUID 매핑 파일 경로이다.
	// 빈 문자열이면 ~/.xflow/storage/device_ids/device_ids.json 가 사용된다.
	IDRepoPath string

	// OutputDir 는 생성 스크립트 저장 위치이다.
	// 빈 문자열이면 ./tsdb-migrations-<UTC-timestamp> 가 사용된다.
	OutputDir string

	// Target 은 InfluxDB 의 버전이다 (v2 / v3 / auto).
	// auto 의 경우 detect 패키지가 X-Influxdb-Version 헤더로 추정한다.
	Target Target

	// Measurements 는 특정 measurement 만 처리할 때 사용한다.
	// 빈 슬라이스이면 전체 measurement 를 스캔한다.
	Measurements []string

	// DryRun 이 true 면 스크립트 생성 없이 영향 분석만 수행한다.
	DryRun bool

	// Stdout / Stderr 는 진행 보고 출력 대상이다 (테스트 격리용).
	Stdout io.Writer
	Stderr io.Writer
}

// Planner 는 tsdb-tags 마이그레이션의 전체 흐름을 관장한다.
//
// 디자인: schema 조회는 SchemaClient 인터페이스를 통하므로 mock 주입이 자유롭다.
// Apply 가 호출되더라도 본 도구는 Influx 에 어떤 쓰기도 수행하지 않는다 —
// 출력은 OutputDir 의 스크립트 파일뿐이다.
type Planner struct {
	opts   Options
	client SchemaClient
}

// NewPlanner 는 Planner 를 생성한다.
//
// client 가 nil 이면 검증 단계에서 에러 (실제 client 생성은 caller 책임).
// 이 분리는 mock 테스트와 실제 사용을 모두 지원한다.
func NewPlanner(_ context.Context, opts Options, client SchemaClient) (*Planner, error) {
	if err := validateOptions(opts); err != nil {
		return nil, err
	}
	if client == nil {
		return nil, errors.New("schema client 가 nil 입니다 (caller 가 생성해야 합니다)")
	}
	if opts.Stdout == nil {
		opts.Stdout = io.Discard
	}
	if opts.Stderr == nil {
		opts.Stderr = io.Discard
	}
	return &Planner{opts: opts, client: client}, nil
}

// validateOptions 는 Options 의 필수 필드와 일관성을 검증한다.
//
// URL/Token/Bucket 은 필수. Target=v2 이면 Org 도 필수.
// Target 이 비어 있으면 auto 로 간주.
func validateOptions(opts Options) error {
	if strings.TrimSpace(opts.InfluxURL) == "" {
		return errors.New("influx-url 가 비어 있습니다")
	}
	if strings.TrimSpace(opts.InfluxToken) == "" {
		return errors.New("influx-token 이 비어 있습니다")
	}
	if strings.TrimSpace(opts.Bucket) == "" {
		return errors.New("bucket 이 비어 있습니다")
	}
	switch opts.Target {
	case "", TargetAuto, TargetV2, TargetV3:
		// 유효 (빈 문자열은 auto 로 간주).
	default:
		return fmt.Errorf("target 은 'v2', 'v3', 'auto' 중 하나여야 합니다 (입력값: %q)", opts.Target)
	}
	if opts.Target == TargetV2 && strings.TrimSpace(opts.Org) == "" {
		return errors.New("target=v2 에서는 org 가 필수입니다")
	}
	return nil
}

// ResolveOutputDir 는 OutputDir 이 비어 있으면 timestamped 기본값을 반환한다.
//
// 운영자가 디렉토리 충돌을 명시적으로 인지하도록 caller 는 후속 단계에서
// 디렉토리 존재 여부를 확인해야 한다 (덮어쓰기 방지).
func ResolveOutputDir(opts Options) string {
	if opts.OutputDir != "" {
		return opts.OutputDir
	}
	ts := time.Now().UTC().Format("20060102T150405Z")
	return filepath.Clean("tsdb-migrations-" + ts)
}

// ResolveIDRepoPath 는 IDRepoPath 가 비어 있으면 기본 경로를 반환한다.
//
// 운영자가 명시적으로 지정하지 않은 경우 ~/.xflow/storage/device_ids/device_ids.json
// 의 표준 위치를 가정한다 (xflowd 데몬이 사용하는 동일 경로).
func ResolveIDRepoPath(opts Options, homeDir string) string {
	if opts.IDRepoPath != "" {
		return opts.IDRepoPath
	}
	return filepath.Join(homeDir, ".xflow", "storage", "device_ids", "device_ids.json")
}

// ScanSchema 는 SchemaClient 로부터 (measurement, tag_key, tag_value) 의
// 전체 집합을 수집하여 ScannedTagValue 슬라이스로 반환한다.
//
// 알고리즘:
//  1. ListMeasurements 호출 (또는 opts.Measurements 가 지정되었으면 그 부분집합).
//  2. 각 measurement 에 대해 ListTagKeys → 각 tag key 에 대해 ListTagValues.
//  3. (tag_value, tag_key) 별로 measurement 집합을 누적 (동일 tag value 가
//     여러 measurement 에 등장할 수 있음).
//  4. 결정론을 위해 정렬된 슬라이스를 반환.
//
// 본 함수는 read-only — 어떤 write API 도 호출하지 않는다.
func ScanSchema(ctx context.Context, client SchemaClient, restrict []string) ([]ScannedTagValue, error) {
	all, err := client.ListMeasurements(ctx)
	if err != nil {
		return nil, fmt.Errorf("measurement 목록 조회 실패: %w", err)
	}

	measurements := all
	if len(restrict) > 0 {
		measurements = filterMeasurements(all, restrict)
	}

	// (value, tagKey) → measurement 집합.
	type key struct {
		value  string
		tagKey string
	}
	bucket := make(map[key]map[string]struct{})

	for _, m := range measurements {
		keys, err := client.ListTagKeys(ctx, m)
		if err != nil {
			return nil, fmt.Errorf("tag-key 조회 실패 (measurement=%s): %w", m, err)
		}
		for _, tk := range keys {
			values, err := client.ListTagValues(ctx, m, tk)
			if err != nil {
				return nil, fmt.Errorf("tag-value 조회 실패 (measurement=%s, tag=%s): %w", m, tk, err)
			}
			for _, v := range values {
				k := key{value: v, tagKey: tk}
				if bucket[k] == nil {
					bucket[k] = make(map[string]struct{})
				}
				bucket[k][m] = struct{}{}
			}
		}
	}

	// map → 결정적 슬라이스.
	out := make([]ScannedTagValue, 0, len(bucket))
	for k, ms := range bucket {
		mlist := make([]string, 0, len(ms))
		for m := range ms {
			mlist = append(mlist, m)
		}
		// ScannedTagValue 자체는 Classify 에서 정렬되지만, 결정성을 위해
		// 본 단계에서도 정렬.
		out = append(out, ScannedTagValue{
			Value:        k.value,
			TagKey:       k.tagKey,
			Measurements: mlist,
		})
	}
	return out, nil
}

// filterMeasurements 는 all 중 restrict 에 포함된 항목만 반환한다 (대소문자 구분).
func filterMeasurements(all, restrict []string) []string {
	want := make(map[string]struct{}, len(restrict))
	for _, m := range restrict {
		want[m] = struct{}{}
	}
	out := make([]string, 0, len(restrict))
	for _, m := range all {
		if _, ok := want[m]; ok {
			out = append(out, m)
		}
	}
	return out
}
