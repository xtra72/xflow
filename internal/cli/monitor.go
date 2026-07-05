package cli

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

// validMonitorLogLevels 는 monitor 명령에서 허용하는 로그 레벨 목록이다.
// 백엔드(handler.validLogLevels)와 동일한 집합을 클라이언트 측에서도 검증하여
// 잘못된 값이 서버에 도달하기 전에 차단한다.
var validMonitorLogLevels = map[string]bool{
	"debug": true,
	"info":  true,
	"warn":  true,
	"error": true,
}

// validLogLevelHint 는 잘못된 레벨 입력 시 안내에 사용할 정렬된 레벨 목록 문자열이다.
func validLogLevelHint() string {
	levels := make([]string, 0, len(validMonitorLogLevels))
	for lvl := range validMonitorLogLevels {
		levels = append(levels, lvl)
	}
	sort.Strings(levels)
	return strings.Join(levels, ", ")
}

// normalizeLogLevel 은 로그 레벨 문자열을 정규화(trim + 소문자)한다.
func normalizeLogLevel(level string) string {
	return strings.ToLower(strings.TrimSpace(level))
}

// validateLogLevel 은 주어진 레벨이 허용 목록에 속하는지 검증한다.
// 정규화된 레벨과 에러를 반환한다. 유효하지 않으면 CLIError 를 반환한다.
func validateLogLevel(level string) (string, error) {
	normalized := normalizeLogLevel(level)
	if !validMonitorLogLevels[normalized] {
		return "", ErrInvalidInput(fmt.Sprintf(
			"유효하지 않은 로그 레벨: %q (허용: %s)", level, validLogLevelHint()))
	}
	return normalized, nil
}

// newMonitorCmd 는 모니터링 커맨드 그룹을 생성한다.
// 시스템 메트릭 조회와 로그 레벨 CRUD(전역/컴포넌트)를 노출한다.
//
// 서브커맨드:
//
//	monitor metrics                     -> GET    /api/v1/monitor/metrics
//	monitor loglevel [get]              -> GET    /api/v1/monitor/loglevel
//	monitor loglevel set <level>        -> PUT    /api/v1/monitor/loglevel
//	monitor loglevel set <comp> <level> -> PUT    /api/v1/monitor/loglevel/{component}
//	monitor loglevel reset <component>  -> DELETE /api/v1/monitor/loglevel/{component}
//
// confirmFn 은 향후 위험 작업 확인 프롬프트를 위한 시그니처 일관성 차원에서 받는다.
func newMonitorCmd(client **Client, confirmFn func(string, io.Reader) bool) *cobra.Command {
	_ = confirmFn // 현재 모니터 작업은 확인 프롬프트가 필요 없으나 시그니처 일관성을 위해 받는다.

	monitorCmd := &cobra.Command{
		Use:   "monitor",
		Short: "모니터링 및 로그 레벨 관리 명령어",
		Long:  "시스템 메트릭 조회와 전역/컴포넌트별 로그 레벨 관리를 수행합니다.",
	}

	monitorCmd.AddCommand(newMonitorMetricsCmd(client))
	monitorCmd.AddCommand(newMonitorLoglevelCmd(client))

	return monitorCmd
}

// monitorMetricsFieldOrder 는 메트릭 출력의 필드 순서이다.
var monitorMetricsFieldOrder = []string{
	"cpu_usage_percent", "memory_usage_percent",
	"go_routines", "go_mem_alloc_mb", "go_mem_sys_mb",
	"uptime_seconds",
}

// monitorMetricsLabelMap 는 메트릭 출력의 필드 라벨 매핑이다.
var monitorMetricsLabelMap = map[string]string{
	"cpu_usage_percent":    "CPU Usage (%)",
	"memory_usage_percent": "Memory Usage (%)",
	"go_routines":          "Goroutines",
	"go_mem_alloc_mb":      "Go Mem Alloc (MB)",
	"go_mem_sys_mb":        "Go Mem Sys (MB)",
	"uptime_seconds":       "Uptime (s)",
}

// newMonitorMetricsCmd 는 monitor metrics 서브커맨드를 생성한다.
// GET /api/v1/monitor/metrics 로 시스템 메트릭을 조회한다.
func newMonitorMetricsCmd(client **Client) *cobra.Command {
	return &cobra.Command{
		Use:   "metrics",
		Short: "시스템 메트릭 조회",
		Long:  "서버의 시스템 메트릭(CPU/메모리/goroutines/uptime)을 조회합니다.",
		RunE: func(cmd *cobra.Command, args []string) error {
			var metrics map[string]any
			if err := (*client).Get("/api/v1/monitor/metrics", &metrics); err != nil {
				return err
			}

			format := getFormat(cmd)
			w := cmd.OutOrStdout()

			// 기본(table/text) 형식은 DetailFormatter 로 가독성 있게 표시
			if format == "table" || format == "text" {
				df := NewDetailFormatter(monitorMetricsFieldOrder, monitorMetricsLabelMap, nil)
				return df.Format(metrics, w)
			}
			return PrintResult(w, format, metrics, nil, nil)
		},
	}
}

// monitorLoglevelFieldOrder 는 로그 레벨 목록 출력의 필드 순서이다.
var monitorLoglevelFieldOrder = []string{"default_level", "components"}

// monitorLoglevelLabelMap 는 로그 레벨 목록 출력의 필드 라벨 매핑이다.
var monitorLoglevelLabelMap = map[string]string{
	"default_level": "Default Level",
	"components":    "Components",
}

// monitorLoglevelSectionKeys 는 별도 섹션으로 출력할 키 목록이다.
var monitorLoglevelSectionKeys = map[string]bool{
	"components": true,
}

// newMonitorLoglevelCmd 는 monitor loglevel 커맨드 그룹을 생성한다.
// 인자 없이 호출하면 로그 레벨 목록을 조회하며(get 과 동일),
// get/set/reset 서브커맨드를 통해 전체 CRUD 를 제공한다.
func newMonitorLoglevelCmd(client **Client) *cobra.Command {
	loglevelCmd := &cobra.Command{
		Use:   "loglevel",
		Short: "로그 레벨 관리 (조회/변경/초기화)",
		Long: "전역 및 컴포넌트별 로그 레벨을 조회하고 변경합니다.\n" +
			"인자 없이 실행하면 현재 로그 레벨 목록을 조회합니다.",
		// 인자 없이 'monitor loglevel' 실행 시 목록 조회를 수행한다.
		RunE: func(cmd *cobra.Command, args []string) error {
			return runLoglevelList(*client, cmd)
		},
	}

	loglevelCmd.AddCommand(newMonitorLoglevelGetCmd(client))
	loglevelCmd.AddCommand(newMonitorLoglevelSetCmd(client))
	loglevelCmd.AddCommand(newMonitorLoglevelResetCmd(client))

	return loglevelCmd
}

// runLoglevelList 는 GET /api/v1/monitor/loglevel 로 전역/컴포넌트별 로그 레벨을 조회·출력한다.
func runLoglevelList(client *Client, cmd *cobra.Command) error {
	var levels map[string]any
	if err := client.Get("/api/v1/monitor/loglevel", &levels); err != nil {
		return err
	}

	format := getFormat(cmd)
	w := cmd.OutOrStdout()

	if format == "table" || format == "text" {
		df := NewDetailFormatter(monitorLoglevelFieldOrder, monitorLoglevelLabelMap, monitorLoglevelSectionKeys)
		return df.Format(levels, w)
	}
	return PrintResult(w, format, levels, nil, nil)
}

// newMonitorLoglevelGetCmd 는 monitor loglevel get 서브커맨드를 생성한다.
// GET /api/v1/monitor/loglevel 로 로그 레벨 목록을 조회한다.
func newMonitorLoglevelGetCmd(client **Client) *cobra.Command {
	return &cobra.Command{
		Use:   "get",
		Short: "로그 레벨 목록 조회",
		Long:  "전역 기본 레벨과 컴포넌트별 오버라이드 레벨을 조회합니다.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runLoglevelList(*client, cmd)
		},
	}
}

// newMonitorLoglevelSetCmd 는 monitor loglevel set 서브커맨드를 생성한다.
//
// 인자 개수로 전역/컴포넌트 변경을 분기한다:
//
//	set <level>             -> PUT /api/v1/monitor/loglevel            (전역)
//	set <component> <level> -> PUT /api/v1/monitor/loglevel/{component} (컴포넌트)
func newMonitorLoglevelSetCmd(client **Client) *cobra.Command {
	return &cobra.Command{
		Use:   "set <level> | <component> <level>",
		Short: "로그 레벨 변경 (전역 또는 컴포넌트)",
		Long: "로그 레벨을 변경합니다.\n" +
			"  인자 1개: 전역 로그 레벨을 변경합니다 (예: monitor loglevel set debug)\n" +
			"  인자 2개: 특정 컴포넌트의 로그 레벨을 변경합니다 (예: monitor loglevel set scheduler debug)\n" +
			"  허용 레벨: " + validLogLevelHint(),
		Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			// 인자 개수로 전역 vs 컴포넌트 분기
			if len(args) == 1 {
				return runLoglevelSetGlobal(*client, cmd, args[0])
			}
			return runLoglevelSetComponent(*client, cmd, args[0], args[1])
		},
	}
}

// runLoglevelSetGlobal 은 전역 로그 레벨을 변경한다.
// PUT /api/v1/monitor/loglevel 로 {"level": "<level>"} 본문을 전송한다.
func runLoglevelSetGlobal(client *Client, cmd *cobra.Command, level string) error {
	normalized, err := validateLogLevel(level)
	if err != nil {
		return err
	}

	body := map[string]string{"level": normalized}
	var result map[string]any
	if err := client.Put("/api/v1/monitor/loglevel", body, &result); err != nil {
		return err
	}

	format := getFormat(cmd)
	w := cmd.OutOrStdout()
	if format == "table" || format == "text" {
		fmt.Fprintf(w, "전역 로그 레벨이 '%s' 로 변경되었습니다.\n", normalized)
		return nil
	}
	return PrintResult(w, format, result, nil, nil)
}

// runLoglevelSetComponent 는 특정 컴포넌트의 로그 레벨을 변경한다.
// PUT /api/v1/monitor/loglevel/{component} 로 {"level": "<level>"} 본문을 전송한다.
func runLoglevelSetComponent(client *Client, cmd *cobra.Command, component, level string) error {
	component = strings.TrimSpace(component)
	if component == "" {
		return ErrInvalidInput("컴포넌트 이름이 비어 있습니다")
	}

	normalized, err := validateLogLevel(level)
	if err != nil {
		return err
	}

	body := map[string]string{"level": normalized}
	var result map[string]any
	if err := client.Put("/api/v1/monitor/loglevel/"+component, body, &result); err != nil {
		return err
	}

	format := getFormat(cmd)
	w := cmd.OutOrStdout()
	if format == "table" || format == "text" {
		fmt.Fprintf(w, "컴포넌트 '%s' 의 로그 레벨이 '%s' 로 변경되었습니다.\n", component, normalized)
		return nil
	}
	return PrintResult(w, format, result, nil, nil)
}

// newMonitorLoglevelResetCmd 는 monitor loglevel reset <component> 서브커맨드를 생성한다.
// DELETE /api/v1/monitor/loglevel/{component} 로 컴포넌트 로그 레벨을 초기화한다.
func newMonitorLoglevelResetCmd(client **Client) *cobra.Command {
	return &cobra.Command{
		Use:   "reset <component>",
		Short: "컴포넌트 로그 레벨 초기화",
		Long:  "특정 컴포넌트의 로그 레벨 오버라이드를 제거하여 전역 기본 레벨을 따르도록 초기화합니다.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			component := strings.TrimSpace(args[0])
			if component == "" {
				return ErrInvalidInput("컴포넌트 이름이 비어 있습니다")
			}

			var result map[string]any
			if err := (*client).Delete("/api/v1/monitor/loglevel/"+component, &result); err != nil {
				return err
			}

			format := getFormat(cmd)
			w := cmd.OutOrStdout()
			if format == "table" || format == "text" {
				fmt.Fprintf(w, "컴포넌트 '%s' 의 로그 레벨이 초기화되었습니다.\n", component)
				return nil
			}
			return PrintResult(w, format, result, nil, nil)
		},
	}
}
