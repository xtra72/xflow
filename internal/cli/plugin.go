package cli

import (
	"io"

	"github.com/spf13/cobra"
)

// pluginNotSupportedMessage 는 플러그인 기능 미지원 안내 메시지이다.
// 백엔드 plugin 시스템은 별도 SPEC(SPEC-PLUGIN-001)으로 분리되어 아직 구현되지 않았다.
const pluginNotSupportedMessage = "플러그인 기능은 아직 지원되지 않습니다 (SPEC-PLUGIN-001 예정)"

// newPluginNotSupportedError 는 플러그인 명령 실행 시 반환할 안내 에러를 생성한다.
// 실제 백엔드 호출 없이 즉시 반환되어 불필요한 404 를 방지한다.
func newPluginNotSupportedError() *CLIError {
	return &CLIError{
		Message:  pluginNotSupportedMessage,
		Hint:     "플러그인 시스템은 향후 릴리스에서 제공될 예정입니다",
		ExitCode: 1,
	}
}

// newPluginCmd 는 플러그인 관리 커맨드 그룹을 생성한다.
//
// 재배선(SPEC-CLI-004): 백엔드 plugin 시스템(/api/v1/plugins)이 미구현이므로
// 본 명령 그룹과 모든 하위 명령은 Hidden 처리되어 도움말/자동완성에서 숨겨진다.
// 각 하위 명령은 실제 API 호출 대신 미지원 안내 메시지를 출력하고 non-zero exit 한다.
// 코드는 삭제하지 않고 보존하여 향후 SPEC-PLUGIN-001 구현 시 재활성화한다.
//
// confirmFn 은 시그니처 호환을 위해 유지하되 현재는 사용하지 않는다(미지원 안내로 단락).
func newPluginCmd(client **Client, confirmFn func(string, io.Reader) bool) *cobra.Command {
	_ = client
	_ = confirmFn

	cmd := &cobra.Command{
		Use:    "plugin",
		Short:  "플러그인 관리 (미지원)",
		Long:   "플러그인 관리 기능입니다. " + pluginNotSupportedMessage,
		Hidden: true,
	}

	// 서브커맨드 등록 (모두 Hidden + 미지원 안내)
	cmd.AddCommand(newPluginListCmd())
	cmd.AddCommand(newPluginInstallCmd())
	cmd.AddCommand(newPluginRemoveCmd())
	cmd.AddCommand(newPluginUpdateCmd())

	return cmd
}

// newPluginListCmd 는 plugin list 서브커맨드를 생성한다 (미지원).
func newPluginListCmd() *cobra.Command {
	return &cobra.Command{
		Use:    "list",
		Short:  "설치된 플러그인 목록 조회 (미지원)",
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return newPluginNotSupportedError()
		},
	}
}

// newPluginInstallCmd 는 plugin install <name|path> 서브커맨드를 생성한다 (미지원).
func newPluginInstallCmd() *cobra.Command {
	return &cobra.Command{
		Use:    "install <name|path>",
		Short:  "플러그인 설치 (미지원)",
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return newPluginNotSupportedError()
		},
	}
}

// newPluginRemoveCmd 는 plugin remove <name> 서브커맨드를 생성한다 (미지원).
func newPluginRemoveCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:    "remove <name>",
		Short:  "플러그인 제거 (미지원)",
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return newPluginNotSupportedError()
		},
	}

	// --yes 플래그는 기존 인터페이스 호환을 위해 유지(동작은 미지원 안내로 단락).
	cmd.Flags().Bool("yes", false, "확인 프롬프트 생략 (미지원)")

	return cmd
}

// newPluginUpdateCmd 는 plugin update <name> 서브커맨드를 생성한다 (미지원).
func newPluginUpdateCmd() *cobra.Command {
	return &cobra.Command{
		Use:    "update <name>",
		Short:  "플러그인 업데이트 (미지원)",
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return newPluginNotSupportedError()
		},
	}
}
