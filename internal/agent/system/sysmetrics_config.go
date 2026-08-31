package system

import (
	"fmt"
	"strings"
	"time"

	"github.com/xtra/xflow/internal/agent"
)

// 시스템 모니터링 에이전트 설정.

const (
	// SysMetricsAgentType 은 에이전트 타입 식별자이다.
	SysMetricsAgentType = "sysmetrics"

	// defaultSysMetricsInterval 은 기본 표본 주기이다.
	defaultSysMetricsInterval = 5 * time.Second
	// minSysMetricsInterval 은 표본 주기 하한이다. 이보다 짧으면 수집 자체가
	// 관측 대상 부하가 된다(디스크 파티션 순회가 특히 무겁다).
	minSysMetricsInterval = time.Second
	// maxSysMetricsInterval 은 표본 주기 상한이다(1시간).
	maxSysMetricsInterval = time.Hour

	// defaultSysMetricsHistory 는 기본 이력 보관 기간이다.
	//
	// 대시보드가 최근 구간을 보는 용도이므로 1시간이면 대부분의 화면을 덮는다. 더
	// 긴 이력이 필요하면 `sysmetrics-in` 노드로 Store·TSDB 에 쌓는 것이 옳은 해법이다 —
	// 이 버퍼는 프로세스 메모리이고 재시작하면 사라진다.
	defaultSysMetricsHistory = time.Hour
	// minSysMetricsHistory 는 보관 기간 하한이다. 표본 두 개는 있어야 증가량이 나온다.
	minSysMetricsHistory = time.Minute
	// maxSysMetricsHistory 는 보관 기간 상한이다(24시간).
	maxSysMetricsHistory = 24 * time.Hour

	// maxSysMetricsHistorySamples 는 보관 표본 수 상한이다.
	//
	// 기간만으로 자르면 주기를 1초로 낮췄을 때 24시간 = 86,400 표본이 되고, 표본
	// 하나에 인터페이스·마운트·장치별 값이 모두 들어가 수백 MB 를 먹는다. 기간과
	// 개수 **두 축**으로 함께 죈다 — 개수 상한에 걸리면 실제 보관 기간이 설정보다
	// 짧아지며, 그 사실은 `State()` 가 드러낸다.
	maxSysMetricsHistorySamples = 10_000
)

// SysMetricsConfig 는 시스템 모니터링 에이전트 설정이다.
type SysMetricsConfig struct {
	// Interval 은 표본 주기이다.
	Interval time.Duration
	// CollectCPU 는 CPU 사용률 수집 여부이다.
	CollectCPU bool
	// CollectMemory 는 호스트 메모리 수집 여부이다.
	CollectMemory bool
	// CollectStorage 는 디스크 사용량 수집 여부이다.
	CollectStorage bool
	// CollectDiskIO 는 디스크 I/O 수집 여부이다.
	CollectDiskIO bool
	// CollectNetwork 는 네트워크 수집 여부이다.
	CollectNetwork bool
	// Mountpoints 는 관측할 마운트 목록이다. 비어 있으면 물리 파티션 전체.
	Mountpoints []string
	// Devices 는 관측할 디스크 장치 목록이다. 비어 있으면 전체.
	Devices []string
	// Interfaces 는 관측할 네트워크 인터페이스 목록이다. 비어 있으면 전체.
	Interfaces []string
	// History 는 표본을 메모리에 보관할 기간이다.
	//
	// 대시보드 패널이 이 버퍼를 질의해 시계열을 그린다. 프로세스 메모리이므로
	// 재시작하면 사라진다 — 영속 이력은 저장 경로(storage-write)의 몫이다.
	History time.Duration
}

// defaultSysMetricsConfig 는 설정이 비었을 때 쓰는 기본값이다.
//
// 기본으로 다섯 지표를 모두 켠다. 시스템 모니터링 에이전트를 만드는 사람은 "무엇이
// 도는지 보고 싶은" 것이고, 무엇을 끌지는 화면을 본 뒤에 정하는 편이 자연스럽다.
func defaultSysMetricsConfig() SysMetricsConfig {
	return SysMetricsConfig{
		Interval:       defaultSysMetricsInterval,
		CollectCPU:     true,
		CollectMemory:  true,
		CollectStorage: true,
		CollectDiskIO:  true,
		CollectNetwork: true,
		History:        defaultSysMetricsHistory,
	}
}

// boolSetting 은 설정에서 bool 을 읽는다. 값이 없으면 기본값을 그대로 둔다.
func boolSetting(opts map[string]any, key string, fallback bool) bool {
	raw, ok := opts[key]
	if !ok || raw == nil {
		return fallback
	}
	if b, ok := raw.(bool); ok {
		return b
	}
	return fallback
}

// stringListSetting 은 설정에서 문자열 목록을 읽는다.
//
// 세 가지 형태를 모두 받는다:
//   - `[]any`    : JSON 역직렬화 경로(설정 UI → API)
//   - `[]string` : Go 설정 파일 경로
//   - `string`   : 쉼표 구분 문자열. 목록 필드가 자유 입력이던 시절에 저장된 값이며,
//     선택 위젯으로 바꾼 뒤에도 기존 에이전트 config 에 그대로 남아 있다.
//
// 해석할 수 없는 타입은 **오류로 거부한다.** 조용히 nil 을 돌려주면 빈 목록이 되고,
// 빈 목록은 "전체"를 뜻하므로 필터가 통째로 풀린다 — 사용자가 인터페이스를 골라
// 두었는데 모든 인터페이스 데이터가 올라오는, 추적하기 어려운 증상이 된다.
func stringListSetting(opts map[string]any, key string) ([]string, error) {
	raw, ok := opts[key]
	if !ok || raw == nil {
		return nil, nil
	}

	switch v := raw.(type) {
	case []string:
		return filterNonEmpty(v), nil
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			s, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("%s 목록에 문자열이 아닌 항목이 있습니다: %T", key, item)
			}
			out = append(out, s)
		}
		return filterNonEmpty(out), nil
	case string:
		return filterNonEmpty(splitCommaList(v)), nil
	default:
		return nil, fmt.Errorf("%s 를 목록으로 해석할 수 없습니다: %T", key, raw)
	}
}

// splitCommaList 는 쉼표 구분 문자열을 항목으로 나눈다. 각 항목의 앞뒤 공백은 버린다.
func splitCommaList(v string) []string {
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		out = append(out, strings.TrimSpace(p))
	}
	return out
}

// filterNonEmpty 는 빈 문자열을 걷어낸다.
func filterNonEmpty(in []string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s != "" {
			out = append(out, s)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// parseSysMetricsConfig 는 AgentConfig 에서 설정을 읽는다.
//
// 알 수 없는 값이나 범위를 벗어난 주기는 오류로 돌려준다 — 조용히 기본값으로
// 떨어지면 사용자가 "1ms 로 넣었는데 왜 5초마다 오지?" 를 추적할 방법이 없다.
func parseSysMetricsConfig(cfg agent.AgentConfig) (SysMetricsConfig, error) {
	out := defaultSysMetricsConfig()
	opts := cfg.Transport.Options

	if raw, ok := opts["interval"]; ok && raw != nil {
		d, err := parseIntervalSetting(raw)
		if err != nil {
			return SysMetricsConfig{}, err
		}
		if d < minSysMetricsInterval || d > maxSysMetricsInterval {
			return SysMetricsConfig{}, fmt.Errorf(
				"interval 은 %s ~ %s 범위여야 합니다: %s",
				minSysMetricsInterval, maxSysMetricsInterval, d)
		}
		out.Interval = d
	}

	if raw, ok := opts["history"]; ok && raw != nil {
		d, err := parseIntervalSetting(raw)
		if err != nil {
			return SysMetricsConfig{}, err
		}
		if d < minSysMetricsHistory || d > maxSysMetricsHistory {
			return SysMetricsConfig{}, fmt.Errorf(
				"history 는 %s ~ %s 범위여야 합니다: %s",
				minSysMetricsHistory, maxSysMetricsHistory, d)
		}
		out.History = d
	}

	out.CollectCPU = boolSetting(opts, "collect_cpu", out.CollectCPU)
	out.CollectMemory = boolSetting(opts, "collect_memory", out.CollectMemory)
	out.CollectStorage = boolSetting(opts, "collect_storage", out.CollectStorage)
	out.CollectDiskIO = boolSetting(opts, "collect_disk_io", out.CollectDiskIO)
	out.CollectNetwork = boolSetting(opts, "collect_network", out.CollectNetwork)

	var err error
	if out.Mountpoints, err = stringListSetting(opts, "mountpoints"); err != nil {
		return SysMetricsConfig{}, err
	}
	if out.Devices, err = stringListSetting(opts, "devices"); err != nil {
		return SysMetricsConfig{}, err
	}
	if out.Interfaces, err = stringListSetting(opts, "interfaces"); err != nil {
		return SysMetricsConfig{}, err
	}

	if !out.CollectCPU && !out.CollectMemory && !out.CollectStorage &&
		!out.CollectDiskIO && !out.CollectNetwork {
		return SysMetricsConfig{}, fmt.Errorf("수집할 지표를 하나 이상 켜야 합니다")
	}

	return out, nil
}

// parseIntervalSetting 은 주기 설정값을 Duration 으로 바꾼다.
//
// 문자열("5s")과 숫자(초)를 모두 받는다 — 설정 UI 는 숫자를 보내고, 손으로 쓴
// 설정 파일은 기간 문자열을 쓰는 편이 읽기 좋다.
func parseIntervalSetting(raw any) (time.Duration, error) {
	switch v := raw.(type) {
	case string:
		d, err := time.ParseDuration(v)
		if err != nil {
			return 0, fmt.Errorf("interval 형식이 올바르지 않습니다(%q): %w", v, err)
		}
		return d, nil
	case float64:
		return time.Duration(v * float64(time.Second)), nil
	case int:
		return time.Duration(v) * time.Second, nil
	case int64:
		return time.Duration(v) * time.Second, nil
	default:
		return 0, fmt.Errorf("interval 타입을 해석할 수 없습니다: %T", raw)
	}
}
