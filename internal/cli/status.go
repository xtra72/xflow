package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// newStatusCmd 는 서버 상태 관련 커맨드를 생성한다.
// xflow status 는 서버 상태를 조회하고, logs 와 metrics 서브커맨드를 포함한다.
// 모든 상태 커맨드는 읽기 전용이므로 confirmFn 이 필요 없다.
func newStatusCmd(client **Client) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "서버 상태 조회",
		Long:  "xflowd 서버의 상태, 로그, 메트릭스를 조회합니다.",
		// xflow status → GET /api/v1/status
		RunE: func(cmd *cobra.Command, args []string) error {
			var status map[string]any
			if err := (*client).Get("/api/v1/status", &status); err != nil {
				return err
			}

			format := getFormat(cmd)
			w := cmd.OutOrStdout()

			// 기본(table) 형식은 텍스트 키-값 쌍으로 표시
			if format == "table" {
				format = "text"
			}
			return PrintResult(w, format, status, nil, nil)
		},
	}

	// 서브커맨드 등록
	cmd.AddCommand(newStatusLogsCmd(client))
	cmd.AddCommand(newStatusMetricsCmd(client))

	return cmd
}

// newStatusLogsCmd 는 status logs <component> 서브커맨드를 생성한다.
// GET /api/v1/logs?component=<name>&lines=<N> 로 컴포넌트 로그를 조회한다.
// --follow 플래그는 문서화되어 있으나 아직 미지원 (경고 메시지 출력).
// --lines 플래그로 조회할 로그 줄 수를 지정한다 (기본값: 50).
func newStatusLogsCmd(client **Client) *cobra.Command {
	var follow bool
	var lines int

	cmd := &cobra.Command{
		Use:   "logs <component>",
		Short: "컴포넌트 로그 조회",
		Long:  "지정된 컴포넌트의 로그를 조회합니다.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			component := args[0]
			w := cmd.OutOrStdout()

			// --follow 플래그 경고 메시지
			if follow {
				fmt.Fprintln(w, "실시간 스트리밍은 아직 지원되지 않습니다")
			}

			// GET /api/v1/logs?component=<name>&lines=<N>
			path := fmt.Sprintf("/api/v1/logs?component=%s&lines=%d", component, lines)

			var logResult map[string]any
			if err := (*client).Get(path, &logResult); err != nil {
				return err
			}

			format := getFormat(cmd)

			// 기본(table) 형식은 텍스트로 표시
			if format == "table" {
				format = "text"
			}
			return PrintResult(w, format, logResult, nil, nil)
		},
	}

	cmd.Flags().BoolVar(&follow, "follow", false, "실시간 로그 스트리밍 (미지원)")
	cmd.Flags().IntVar(&lines, "lines", 50, "조회할 로그 줄 수")

	return cmd
}

// newStatusMetricsCmd 는 status metrics 서브커맨드를 생성한다.
// GET /api/v1/metrics 로 메트릭스 요약 정보를 조회한다.
func newStatusMetricsCmd(client **Client) *cobra.Command {
	return &cobra.Command{
		Use:   "metrics",
		Short: "메트릭스 요약 조회",
		Long:  "서버의 메트릭스 요약 정보를 조회합니다.",
		RunE: func(cmd *cobra.Command, args []string) error {
			var metrics map[string]any
			if err := (*client).Get("/api/v1/metrics", &metrics); err != nil {
				return err
			}

			format := getFormat(cmd)
			w := cmd.OutOrStdout()

			// 기본(table) 형식은 텍스트 키-값 쌍으로 표시
			if format == "table" {
				format = "text"
			}
			return PrintResult(w, format, metrics, nil, nil)
		},
	}
}
