package cli

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

// newPluginCmd 는 플러그인 관리 커맨드 그룹을 생성한다.
// 4개의 서브커맨드 (list, install, remove, update) 를 등록한다.
func newPluginCmd(client **Client, confirmFn func(string, io.Reader) bool) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "plugin",
		Short: "플러그인 관리",
		Long:  "플러그인의 조회, 설치, 제거, 업데이트를 수행합니다.",
	}

	// 서브커맨드 등록
	cmd.AddCommand(newPluginListCmd(client))
	cmd.AddCommand(newPluginInstallCmd(client))
	cmd.AddCommand(newPluginRemoveCmd(client, confirmFn))
	cmd.AddCommand(newPluginUpdateCmd(client))

	return cmd
}

// pluginTableHeaders 는 플러그인 목록 테이블의 헤더이다.
var pluginTableHeaders = []string{"NAME", "VERSION", "TYPE", "STATUS", "NODES"}

// pluginRowFunc 는 플러그인 맵에서 테이블 행을 추출한다.
func pluginRowFunc(item any) []string {
	m, ok := item.(map[string]any)
	if !ok {
		return []string{"", "", "", "", ""}
	}
	return []string{
		fmt.Sprintf("%v", m["name"]),
		fmt.Sprintf("%v", m["version"]),
		fmt.Sprintf("%v", m["type"]),
		fmt.Sprintf("%v", m["status"]),
		fmt.Sprintf("%v", m["nodes"]),
	}
}

// newPluginListCmd 는 plugin list 서브커맨드를 생성한다.
// GET /api/v1/plugins 로 플러그인 목록을 조회한다.
func newPluginListCmd(client **Client) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "설치된 플러그인 목록 조회",
		RunE: func(cmd *cobra.Command, args []string) error {
			var plugins []map[string]any
			if err := (*client).Get("/api/v1/plugins", &plugins); err != nil {
				return err
			}

			format := getFormat(cmd)
			w := cmd.OutOrStdout()
			return PrintResult(w, format, plugins, pluginTableHeaders, pluginRowFunc)
		},
	}
}

// isPath 는 인자가 파일 경로인지 판별한다.
// / 또는 . 으로 시작하면 경로로 간주한다.
func isPath(arg string) bool {
	return strings.HasPrefix(arg, "/") || strings.HasPrefix(arg, ".")
}

// newPluginInstallCmd 는 plugin install <name|path> 서브커맨드를 생성한다.
// POST /api/v1/plugins 로 플러그인을 설치한다.
// 경로(/. 으로 시작)이면 {"path": "..."}, 아니면 {"name": "..."} 으로 전송한다.
func newPluginInstallCmd(client **Client) *cobra.Command {
	return &cobra.Command{
		Use:   "install <name|path>",
		Short: "플러그인 설치",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			nameOrPath := args[0]

			// 경로인지 이름인지 판별하여 요청 바디 구성
			body := make(map[string]any)
			if isPath(nameOrPath) {
				body["path"] = nameOrPath
			} else {
				body["name"] = nameOrPath
			}

			var result map[string]any
			if err := (*client).Post("/api/v1/plugins", body, &result); err != nil {
				return err
			}

			format := getFormat(cmd)
			w := cmd.OutOrStdout()

			if format == "table" || format == "text" {
				name := fmt.Sprintf("%v", result["name"])
				fmt.Fprintf(w, "플러그인 '%s' 가 설치되었습니다.\n", name)
				return nil
			}
			return PrintResult(w, format, result, nil, nil)
		},
	}
}

// newPluginRemoveCmd 는 plugin remove <name> [--yes] 서브커맨드를 생성한다.
// 확인 프롬프트 후 DELETE /api/v1/plugins/:name 으로 플러그인을 제거한다.
func newPluginRemoveCmd(client **Client, confirmFn func(string, io.Reader) bool) *cobra.Command {
	var yes bool

	cmd := &cobra.Command{
		Use:   "remove <name>",
		Short: "플러그인 제거",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			w := cmd.OutOrStdout()

			// --yes 플래그가 없으면 확인 요청
			if !yes {
				prompt := fmt.Sprintf("플러그인 '%s' 를 삭제하시겠습니까?", name)
				if !confirmFn(prompt, os.Stdin) {
					fmt.Fprintln(w, "삭제가 취소되었습니다.")
					return nil
				}
			}

			var result map[string]any
			if err := (*client).Delete("/api/v1/plugins/"+name, &result); err != nil {
				return err
			}

			fmt.Fprintf(w, "플러그인 '%s' 가 삭제되었습니다.\n", name)

			// 경고 메시지가 있으면 출력
			if result != nil {
				if warning, ok := result["warning"]; ok && warning != nil {
					fmt.Fprintf(w, "경고: %v\n", warning)
				}
			}

			return nil
		},
	}

	cmd.Flags().BoolVar(&yes, "yes", false, "확인 프롬프트 생략")

	return cmd
}

// newPluginUpdateCmd 는 plugin update <name> 서브커맨드를 생성한다.
// PUT /api/v1/plugins/:name 으로 플러그인을 업데이트한다.
func newPluginUpdateCmd(client **Client) *cobra.Command {
	return &cobra.Command{
		Use:   "update <name>",
		Short: "플러그인 업데이트",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]

			var result map[string]any
			if err := (*client).Put("/api/v1/plugins/"+name, nil, &result); err != nil {
				return err
			}

			format := getFormat(cmd)
			w := cmd.OutOrStdout()

			if format == "table" || format == "text" {
				version := fmt.Sprintf("%v", result["version"])
				fmt.Fprintf(w, "플러그인 '%s' 가 %s 로 업데이트되었습니다.\n", name, version)
				return nil
			}
			return PrintResult(w, format, result, nil, nil)
		},
	}
}
