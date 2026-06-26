package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// statusFieldOrder 는 서버 상태 출력의 필드 순서이다.
var statusFieldOrder = []string{
	"version", "commit", "build_date", "go_version",
	"os", "arch", "hostname", "mode", "channel",
	"uptime_seconds", "memory_usage_percent", "go_routines",
	"total_flows", "total_agents", "connection",
}

// statusLabelMap 는 서버 상태 출력의 필드 라벨 매핑이다.
var statusLabelMap = map[string]string{
	"version":              "Version",
	"commit":               "Commit",
	"build_date":           "Build Date",
	"go_version":           "Go Version",
	"os":                   "OS",
	"arch":                 "Arch",
	"hostname":             "Hostname",
	"mode":                 "Mode",
	"channel":              "Channel",
	"uptime_seconds":       "Uptime (s)",
	"memory_usage_percent": "Memory Usage (%)",
	"go_routines":          "Goroutines",
	"total_flows":          "Total Flows",
	"total_agents":         "Total Agents",
	"connection":           "Connection",
}

// newStatusCmd 는 서버 상태 관련 커맨드를 생성한다.
// xflow status 는 서버 상태를 조회하고, logs 와 metrics 서브커맨드를 포함한다.
// 모든 상태 커맨드는 읽기 전용이므로 confirmFn 이 필요 없다.
//
// 재배선(SPEC-CLI-004): 기존 GET /api/v1/status 단일 엔드포인트는 백엔드에 존재하지 않으므로,
// 실제 가용한 엔드포인트(/system/version, /monitor/metrics, /flows, /agents, /health)를
// 조합하여 서버 상태 요약을 구성한다.
func newStatusCmd(client **Client) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "서버 상태 조회",
		Long:  "xflowd 서버의 버전, 업타임, 리소스, 플로우/에이전트 수, 연결 상태를 조회합니다.",
		RunE: func(cmd *cobra.Command, args []string) error {
			status := collectStatus(*client)

			format := getFormat(cmd)
			w := cmd.OutOrStdout()

			// 기본(table/text) 형식은 DetailFormatter 로 가독성 있게 표시
			if format == "table" || format == "text" {
				df := NewDetailFormatter(statusFieldOrder, statusLabelMap, nil)
				return df.Format(status, w)
			}
			return PrintResult(w, format, status, nil, nil)
		},
	}

	// 서브커맨드 등록
	cmd.AddCommand(newStatusLogsCmd(client))
	cmd.AddCommand(newStatusMetricsCmd(client))

	return cmd
}

// collectStatus 는 여러 실제 엔드포인트를 조합하여 서버 상태 요약 맵을 만든다.
//
// 조합 방식:
//   - 버전/빌드/식별/uptime: GET /api/v1/system/version
//   - 리소스(메모리/goroutines)/uptime 보강: GET /api/v1/monitor/metrics
//   - 플로우/에이전트 총 개수: GET /api/v1/flows, /api/v1/agents 의 meta.pagination.total
//   - 연결성: /health (Ping)
//
// 개별 엔드포인트 조회가 실패해도 전체 상태 조회는 계속 진행하며,
// 얻지 못한 필드는 결과 맵에서 생략된다(부분 실패 허용).
func collectStatus(client *Client) map[string]any {
	status := make(map[string]any)

	// 1) 버전/식별/uptime — system/version
	var version map[string]any
	if err := client.Get("/api/v1/system/version", &version); err == nil {
		for _, k := range []string{
			"version", "commit", "build_date", "go_version",
			"channel", "os", "arch", "hostname", "mode", "uptime_seconds",
		} {
			if v, ok := version[k]; ok {
				status[k] = v
			}
		}
	}

	// 2) 리소스 메트릭 — monitor/metrics (uptime_seconds 는 version 우선)
	var metrics map[string]any
	if err := client.Get("/api/v1/monitor/metrics", &metrics); err == nil {
		for _, k := range []string{"memory_usage_percent", "go_routines"} {
			if v, ok := metrics[k]; ok {
				status[k] = v
			}
		}
		// version 응답에 uptime 이 없을 때만 metrics 의 uptime 사용
		if _, ok := status["uptime_seconds"]; !ok {
			if v, ok := metrics["uptime_seconds"]; ok {
				status["uptime_seconds"] = v
			}
		}
	}

	// 3) 플로우/에이전트 총 개수 — 목록 응답의 meta.pagination.total 사용
	var flows []map[string]any
	if meta, err := client.GetWithMeta("/api/v1/flows", &flows); err == nil {
		status["total_flows"] = countFromMeta(meta, len(flows))
	}

	var agents []map[string]any
	if meta, err := client.GetWithMeta("/api/v1/agents", &agents); err == nil {
		status["total_agents"] = countFromMeta(meta, len(agents))
	}

	// 4) 연결성 — /health
	if err := client.Ping(); err == nil {
		status["connection"] = "ok"
	} else {
		status["connection"] = "unreachable"
	}

	return status
}

// countFromMeta 는 페이지네이션 메타가 있으면 total 을, 없으면 fallback(현재 페이지 길이)을 반환한다.
func countFromMeta(meta *PaginationMeta, fallback int) int64 {
	if meta != nil {
		return meta.Total
	}
	return int64(fallback)
}

// newStatusLogsCmd 는 status logs [component] 서브커맨드를 생성한다.
//
// 재배선(SPEC-CLI-004): 실시간 로그 조회 API(GET /api/v1/logs)는 백엔드에 존재하지 않는다.
// 실시간 로그 스트림은 remote 전용(SSE)으로 후속(P4)에서 다룬다.
// 본 서브커맨드는 현재 가용한 GET /api/v1/monitor/loglevel 을 호출하여
// 전역 및 컴포넌트별 로그 레벨을 표시하도록 재정의되었다.
// component 인자를 주면 해당 컴포넌트의 레벨만 필터링하여 표시한다.
func newStatusLogsCmd(client **Client) *cobra.Command {
	var follow bool

	cmd := &cobra.Command{
		Use:   "logs [component]",
		Short: "로그 레벨 조회",
		Long: "전역 및 컴포넌트별 로그 레벨을 조회합니다.\n" +
			"실시간 로그 스트리밍은 아직 지원되지 않습니다(향후 SSE 기반 지원 예정).",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			w := cmd.OutOrStdout()

			// --follow 플래그는 실시간 스트리밍 의도이나 미지원이므로 안내만 출력
			if follow {
				fmt.Fprintln(w, "실시간 로그 스트리밍은 아직 지원되지 않습니다(향후 SSE 기반 지원 예정)")
			}

			// 현재 로그 레벨 조회 (GET /api/v1/monitor/loglevel)
			var levels map[string]any
			if err := (*client).Get("/api/v1/monitor/loglevel", &levels); err != nil {
				return err
			}

			// component 인자가 주어지면 해당 컴포넌트 레벨만 필터링
			if len(args) == 1 {
				levels = filterComponentLevel(levels, args[0])
			}

			format := getFormat(cmd)
			if format == "table" {
				format = "text"
			}
			return PrintResult(w, format, levels, nil, nil)
		},
	}

	cmd.Flags().BoolVar(&follow, "follow", false, "실시간 로그 스트리밍 (미지원)")

	return cmd
}

// filterComponentLevel 은 로그 레벨 응답에서 특정 컴포넌트의 레벨만 추출한다.
// 응답 구조: {"default_level": "info", "components": {"<comp>": "<level>", ...}}
// 컴포넌트가 존재하지 않으면 default_level 로 동작함을 안내한다.
func filterComponentLevel(levels map[string]any, component string) map[string]any {
	result := map[string]any{"component": component}

	defaultLevel, _ := levels["default_level"]

	if comps, ok := levels["components"].(map[string]any); ok {
		if lvl, ok := comps[component]; ok {
			result["level"] = lvl
			return result
		}
	}

	// 컴포넌트별 오버라이드가 없으면 기본 레벨을 따른다
	result["level"] = defaultLevel
	result["note"] = "컴포넌트별 오버라이드 없음 (기본 레벨 적용)"
	return result
}

// metricsFieldOrder 는 메트릭스 출력의 필드 순서이다.
var metricsFieldOrder = []string{
	"cpu_usage_percent", "memory_usage_percent",
	"go_routines", "go_mem_alloc_mb", "go_mem_sys_mb",
	"uptime_seconds",
}

// metricsLabelMap 는 메트릭스 출력의 필드 라벨 매핑이다.
var metricsLabelMap = map[string]string{
	"cpu_usage_percent":    "CPU Usage (%)",
	"memory_usage_percent": "Memory Usage (%)",
	"go_routines":          "Goroutines",
	"go_mem_alloc_mb":      "Go Mem Alloc (MB)",
	"go_mem_sys_mb":        "Go Mem Sys (MB)",
	"uptime_seconds":       "Uptime (s)",
}

// newStatusMetricsCmd 는 status metrics 서브커맨드를 생성한다.
//
// 재배선(SPEC-CLI-004): 기존 GET /api/v1/metrics 는 백엔드에 존재하지 않으므로
// 실제 엔드포인트인 GET /api/v1/monitor/metrics 로 재배선한다.
func newStatusMetricsCmd(client **Client) *cobra.Command {
	return &cobra.Command{
		Use:   "metrics",
		Short: "메트릭스 요약 조회",
		Long:  "서버의 시스템 메트릭스(CPU/메모리/goroutines/uptime)를 조회합니다.",
		RunE: func(cmd *cobra.Command, args []string) error {
			var metrics map[string]any
			if err := (*client).Get("/api/v1/monitor/metrics", &metrics); err != nil {
				return err
			}

			format := getFormat(cmd)
			w := cmd.OutOrStdout()

			// 기본(table/text) 형식은 DetailFormatter 로 가독성 있게 표시
			if format == "table" || format == "text" {
				df := NewDetailFormatter(metricsFieldOrder, metricsLabelMap, nil)
				return df.Format(metrics, w)
			}
			return PrintResult(w, format, metrics, nil, nil)
		},
	}
}
