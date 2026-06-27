package cli

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
)

// validChannels 는 업데이트 채널의 허용 값 셋이다.
// system channel set 의 인자 검증과 도움말 표기에 사용된다.
var validChannels = []string{"stable", "beta", "nightly"}

// isValidChannel 은 주어진 채널 값이 허용 목록에 속하는지 검증한다.
func isValidChannel(channel string) bool {
	for _, c := range validChannels {
		if c == channel {
			return true
		}
	}
	return false
}

// newSystemCmd 는 시스템(원격 서버) 버전/업데이트 관리 커맨드 그룹을 생성한다.
//
// 본 명령은 CLI 가 접속한 "원격 xflowd 서버"를 REST API 로 제어한다.
// 로컬 데몬 자체의 자가 업데이트(`xflowd update`)와는 별개이며, 대상이 다르다:
//   - xflow system ...  → 접속 중인 원격 서버의 /api/v1/system/* API 호출
//   - xflowd update ...  → 로컬에서 실행 중인 데몬 프로세스의 자가 업데이트
//
// rollback 같은 위험 작업은 confirmFn 으로 사용자 확인을 받는다.
func newSystemCmd(client **Client, confirmFn func(string, io.Reader) bool) *cobra.Command {
	systemCmd := &cobra.Command{
		Use:   "system",
		Short: "시스템(원격 서버) 버전/업데이트 관리 명령어",
		Long: "접속한 원격 xflowd 서버의 버전 정보 조회 및 업데이트 작업을 수행합니다.\n\n" +
			"주의: 이 명령은 '원격 서버'를 API 로 제어합니다.\n" +
			"로컬 데몬 자체의 자가 업데이트(`xflowd update`)와는 다릅니다.",
	}

	systemCmd.AddCommand(newSystemVersionCmd(client))
	systemCmd.AddCommand(newSystemUpdateCmd(client, confirmFn))
	systemCmd.AddCommand(newSystemChannelCmd(client))

	return systemCmd
}

// systemVersionFieldOrder 는 버전 정보 출력의 필드 순서이다.
var systemVersionFieldOrder = []string{
	"version", "commit", "build_date", "go_version",
	"os", "arch", "hostname", "mode", "channel",
	"uptime_seconds", "update_available", "latest_version",
}

// systemVersionLabelMap 는 버전 정보 출력의 필드 라벨 매핑이다.
var systemVersionLabelMap = map[string]string{
	"version":          "Version",
	"commit":           "Commit",
	"build_date":       "Build Date",
	"go_version":       "Go Version",
	"os":               "OS",
	"arch":             "Arch",
	"hostname":         "Hostname",
	"mode":             "Mode",
	"channel":          "Channel",
	"uptime_seconds":   "Uptime (s)",
	"update_available": "Update Available",
	"latest_version":   "Latest Version",
}

// newSystemVersionCmd 는 system version 서브커맨드를 생성한다.
// GET /api/v1/system/version 으로 원격 서버의 버전/빌드/식별 정보와
// 마지막 업데이트 체크 결과(update_available, latest_version)를 조회한다.
func newSystemVersionCmd(client **Client) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "원격 서버 버전 정보 조회",
		Long:  "접속한 원격 xflowd 서버의 버전, 빌드, OS/Arch, 호스트명, 모드, 업타임 및 마지막 업데이트 체크 결과를 조회합니다.",
		RunE: func(cmd *cobra.Command, args []string) error {
			var version map[string]any
			if err := (*client).Get("/api/v1/system/version", &version); err != nil {
				return err
			}

			format := getFormat(cmd)
			w := cmd.OutOrStdout()

			if format == "table" || format == "text" {
				df := NewDetailFormatter(systemVersionFieldOrder, systemVersionLabelMap, nil)
				return df.Format(version, w)
			}
			return PrintResult(w, format, version, nil, nil)
		},
	}
}

// newSystemUpdateCmd 는 system update 서브커맨드 그룹을 생성한다.
// check / apply / status / rollback 하위 명령을 등록한다.
func newSystemUpdateCmd(client **Client, confirmFn func(string, io.Reader) bool) *cobra.Command {
	updateCmd := &cobra.Command{
		Use:   "update",
		Short: "원격 서버 업데이트 작업",
		Long: "원격 xflowd 서버의 업데이트 확인/적용/상태/롤백을 수행합니다.\n\n" +
			"주의: 대상은 '원격 서버'입니다. 로컬 데몬 자가 업데이트(`xflowd update`)와 다릅니다.",
	}

	updateCmd.AddCommand(newSystemUpdateCheckCmd(client))
	updateCmd.AddCommand(newSystemUpdateApplyCmd(client))
	updateCmd.AddCommand(newSystemUpdateStatusCmd(client))
	updateCmd.AddCommand(newSystemUpdateRollbackCmd(client, confirmFn))

	return updateCmd
}

// systemCheckFieldOrder 는 업데이트 확인 결과 출력의 필드 순서이다.
var systemCheckFieldOrder = []string{
	"current", "latest", "available", "channel", "release_url", "published_at",
}

// systemCheckLabelMap 는 업데이트 확인 결과 출력의 필드 라벨 매핑이다.
var systemCheckLabelMap = map[string]string{
	"current":      "Current",
	"latest":       "Latest",
	"available":    "Available",
	"channel":      "Channel",
	"release_url":  "Release URL",
	"published_at": "Published At",
}

// newSystemUpdateCheckCmd 는 system update check 서브커맨드를 생성한다.
// POST /api/v1/system/update/check 로 채널에서 신규 버전을 조회한다.
func newSystemUpdateCheckCmd(client **Client) *cobra.Command {
	return &cobra.Command{
		Use:   "check",
		Short: "원격 서버 신규 버전 확인",
		Long:  "원격 서버가 설정된 채널에서 신규 버전이 있는지 확인합니다.",
		RunE: func(cmd *cobra.Command, args []string) error {
			var result map[string]any
			if err := (*client).Post("/api/v1/system/update/check", nil, &result); err != nil {
				return err
			}

			format := getFormat(cmd)
			w := cmd.OutOrStdout()

			if format == "table" || format == "text" {
				df := NewDetailFormatter(systemCheckFieldOrder, systemCheckLabelMap, nil)
				return df.Format(result, w)
			}
			return PrintResult(w, format, result, nil, nil)
		},
	}
}

// systemApplyFieldOrder 는 업데이트 적용 응답 출력의 필드 순서이다.
var systemApplyFieldOrder = []string{
	"operation_id", "status", "from_version", "to_version",
}

// systemApplyLabelMap 는 업데이트 적용 응답 출력의 필드 라벨 매핑이다.
var systemApplyLabelMap = map[string]string{
	"operation_id": "Operation ID",
	"status":       "Status",
	"from_version": "From Version",
	"to_version":   "To Version",
}

// newSystemUpdateApplyCmd 는 system update apply [--version] [--force] 서브커맨드를 생성한다.
// POST /api/v1/system/update/apply 로 비동기 업데이트 작업을 시작한다.
//
// --version: 적용 대상 버전 (비우면 채널 latest 자동 선택).
// --force:   다운그레이드(현재보다 낮은 버전) 적용을 명시적으로 허용.
//
// 응답은 즉시 반환되며 operation_id 와 초기 상태(starting)를 포함한다.
// 진행 상황은 `system update status` 로 확인한다.
func newSystemUpdateApplyCmd(client **Client) *cobra.Command {
	var (
		version string
		force   bool
	)

	cmd := &cobra.Command{
		Use:   "apply",
		Short: "원격 서버 업데이트 적용(비동기)",
		Long: "원격 서버에 업데이트를 적용하는 비동기 작업을 시작합니다.\n" +
			"진행 상황은 'xflow system update status' 로 확인하세요.",
		RunE: func(cmd *cobra.Command, args []string) error {
			// 요청 바디 구성: 빈 값 필드는 서버 기본값(latest/false)으로 처리된다.
			// omitempty 동작과 동일하게, 지정된 필드만 바디에 포함한다.
			request := map[string]any{}
			if version != "" {
				request["version"] = version
			}
			if force {
				request["force"] = true
			}

			var result map[string]any
			if err := (*client).Post("/api/v1/system/update/apply", request, &result); err != nil {
				return err
			}

			format := getFormat(cmd)
			w := cmd.OutOrStdout()

			if format == "table" || format == "text" {
				df := NewDetailFormatter(systemApplyFieldOrder, systemApplyLabelMap, nil)
				return df.Format(result, w)
			}
			return PrintResult(w, format, result, nil, nil)
		},
	}

	cmd.Flags().StringVar(&version, "version", "", "적용 대상 버전 (예: v0.4.0). 비우면 채널 latest 사용")
	cmd.Flags().BoolVar(&force, "force", false, "다운그레이드(현재보다 낮은 버전) 적용 허용")

	return cmd
}

// systemStatusFieldOrder 는 업데이트 상태 출력의 필드 순서이다.
var systemStatusFieldOrder = []string{
	"operation_id", "status", "from_version", "to_version",
	"started_at", "completed_at", "error",
}

// systemStatusLabelMap 는 업데이트 상태 출력의 필드 라벨 매핑이다.
var systemStatusLabelMap = map[string]string{
	"operation_id": "Operation ID",
	"status":       "Status",
	"from_version": "From Version",
	"to_version":   "To Version",
	"started_at":   "Started At",
	"completed_at": "Completed At",
	"error":        "Error",
}

// newSystemUpdateStatusCmd 는 system update status 서브커맨드를 생성한다.
// GET /api/v1/system/update/status 로 현재(또는 마지막) 업데이트 작업의 상태를 조회한다.
func newSystemUpdateStatusCmd(client **Client) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "원격 서버 업데이트 작업 상태 조회",
		Long:  "현재 진행 중이거나 마지막으로 시도된 업데이트 작업의 상태를 조회합니다.",
		RunE: func(cmd *cobra.Command, args []string) error {
			var result map[string]any
			if err := (*client).Get("/api/v1/system/update/status", &result); err != nil {
				return err
			}

			format := getFormat(cmd)
			w := cmd.OutOrStdout()

			if format == "table" || format == "text" {
				df := NewDetailFormatter(systemStatusFieldOrder, systemStatusLabelMap, nil)
				return df.Format(result, w)
			}
			return PrintResult(w, format, result, nil, nil)
		},
	}
}

// systemRollbackFieldOrder 는 롤백 응답 출력의 필드 순서이다.
var systemRollbackFieldOrder = []string{
	"from_version", "to_version", "rolled_back_at",
}

// systemRollbackLabelMap 는 롤백 응답 출력의 필드 라벨 매핑이다.
var systemRollbackLabelMap = map[string]string{
	"from_version":   "From Version",
	"to_version":     "To Version",
	"rolled_back_at": "Rolled Back At",
}

// newSystemUpdateRollbackCmd 는 system update rollback [--yes] 서브커맨드를 생성한다.
// POST /api/v1/system/update/rollback 으로 이전 버전(백업)으로 롤백한다.
//
// 위험 작업이므로 --yes 가 없으면 confirmFn 으로 사용자 확인을 받는다.
func newSystemUpdateRollbackCmd(client **Client, confirmFn func(string, io.Reader) bool) *cobra.Command {
	var yes bool

	cmd := &cobra.Command{
		Use:   "rollback",
		Short: "원격 서버 이전 버전으로 롤백",
		Long:  "원격 서버를 백업된 이전 버전 바이너리로 복원합니다. 위험 작업이므로 확인이 필요합니다.",
		RunE: func(cmd *cobra.Command, args []string) error {
			w := cmd.OutOrStdout()

			// --yes 플래그가 없으면 확인 요청
			if !yes {
				prompt := "원격 서버를 이전 버전으로 롤백하시겠습니까?"
				if !confirmFn(prompt, os.Stdin) {
					fmt.Fprintln(w, "롤백이 취소되었습니다.")
					return nil
				}
			}

			var result map[string]any
			if err := (*client).Post("/api/v1/system/update/rollback", nil, &result); err != nil {
				return err
			}

			format := getFormat(cmd)
			if format == "table" || format == "text" {
				df := NewDetailFormatter(systemRollbackFieldOrder, systemRollbackLabelMap, nil)
				return df.Format(result, w)
			}
			return PrintResult(w, format, result, nil, nil)
		},
	}

	cmd.Flags().BoolVar(&yes, "yes", false, "확인 프롬프트 생략")

	return cmd
}

// systemChannelFieldOrder 는 채널 정보 출력의 필드 순서이다.
var systemChannelFieldOrder = []string{"current", "available"}

// systemChannelLabelMap 는 채널 정보 출력의 필드 라벨 매핑이다.
var systemChannelLabelMap = map[string]string{
	"current":   "Current",
	"available": "Available",
}

// newSystemChannelCmd 는 system channel 서브커맨드 그룹을 생성한다.
//
// 인자 없이 `system channel` 을 실행하면 현재 채널을 조회한다(get 과 동일).
// get / set 하위 명령을 추가로 등록한다.
func newSystemChannelCmd(client **Client) *cobra.Command {
	channelCmd := &cobra.Command{
		Use:   "channel",
		Short: "원격 서버 업데이트 채널 조회/변경",
		Long: "원격 서버의 업데이트 채널(stable/beta/nightly)을 조회하거나 변경합니다.\n" +
			"인자 없이 실행하면 현재 채널을 조회합니다('channel get' 과 동일).",
		// 인자 없이 호출 시 현재 채널 조회 (get 과 동일 동작)
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSystemChannelGet(cmd, client)
		},
	}

	channelCmd.AddCommand(newSystemChannelGetCmd(client))
	channelCmd.AddCommand(newSystemChannelSetCmd(client))

	return channelCmd
}

// runSystemChannelGet 은 GET /api/v1/system/update/channel 로 현재 채널을 조회하고 출력한다.
// `system channel` 과 `system channel get` 이 공유하는 핸들러이다.
func runSystemChannelGet(cmd *cobra.Command, client **Client) error {
	var result map[string]any
	if err := (*client).Get("/api/v1/system/update/channel", &result); err != nil {
		return err
	}

	format := getFormat(cmd)
	w := cmd.OutOrStdout()

	if format == "table" || format == "text" {
		df := NewDetailFormatter(systemChannelFieldOrder, systemChannelLabelMap, nil)
		return df.Format(result, w)
	}
	return PrintResult(w, format, result, nil, nil)
}

// newSystemChannelGetCmd 는 system channel get 서브커맨드를 생성한다.
// GET /api/v1/system/update/channel 로 현재 채널과 선택 가능한 채널 목록을 조회한다.
func newSystemChannelGetCmd(client **Client) *cobra.Command {
	return &cobra.Command{
		Use:   "get",
		Short: "원격 서버 현재 채널 조회",
		Long:  "원격 서버의 현재 업데이트 채널과 선택 가능한 채널 목록을 조회합니다.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSystemChannelGet(cmd, client)
		},
	}
}

// newSystemChannelSetCmd 는 system channel set <stable|beta|nightly> 서브커맨드를 생성한다.
// PUT /api/v1/system/update/channel 로 채널을 변경한다.
//
// 채널 값은 클라이언트에서 먼저 검증한다(stable/beta/nightly).
// 변경 직후 서버가 새 채널로 즉시 check 를 수행하고, 그 결과를 응답에 포함한다.
func newSystemChannelSetCmd(client **Client) *cobra.Command {
	return &cobra.Command{
		Use:   "set <stable|beta|nightly>",
		Short: "원격 서버 채널 변경",
		Long:  "원격 서버의 업데이트 채널을 변경합니다. 허용 값: stable, beta, nightly.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			channel := args[0]

			// 클라이언트 측 채널 값 검증 (불필요한 서버 왕복 방지).
			if !isValidChannel(channel) {
				return fmt.Errorf("유효하지 않은 채널입니다: %q (허용 값: stable, beta, nightly)", channel)
			}

			request := map[string]any{"channel": channel}

			var result map[string]any
			if err := (*client).Put("/api/v1/system/update/channel", request, &result); err != nil {
				return err
			}

			format := getFormat(cmd)
			w := cmd.OutOrStdout()
			return PrintResult(w, format, result, nil, nil)
		},
	}
}
