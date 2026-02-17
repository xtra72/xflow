package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// newAgentCmd 는 에이전트 관리 커맨드 그룹을 생성한다.
// 7개의 서브커맨드 (list, get, create, start, stop, restart, delete) 를 포함한다.
func newAgentCmd(client **Client, confirmFn func(string, io.Reader) bool) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "agent",
		Short: "에이전트 관리",
		Long:  "에이전트의 조회, 생성, 시작, 중지, 재시작, 삭제를 수행합니다.",
	}

	// 서브커맨드 등록
	cmd.AddCommand(newAgentListCmd(client))
	cmd.AddCommand(newAgentGetCmd(client))
	cmd.AddCommand(newAgentCreateCmd(client))
	cmd.AddCommand(newAgentStartCmd(client))
	cmd.AddCommand(newAgentStopCmd(client))
	cmd.AddCommand(newAgentRestartCmd(client))
	cmd.AddCommand(newAgentDeleteCmd(client, confirmFn))

	return cmd
}

// newAgentListCmd 는 에이전트 목록 조회 커맨드를 생성한다.
// GET /api/v1/agents
func newAgentListCmd(client **Client) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "에이전트 목록 조회",
		RunE: func(cmd *cobra.Command, args []string) error {
			var agents []map[string]any
			if err := (*client).Get("/api/v1/agents", &agents); err != nil {
				return err
			}

			format, _ := cmd.Flags().GetString("format")
			w := cmd.OutOrStdout()

			return PrintResult(w, format, agents,
				[]string{"ID", "NAME", "TYPE", "STATUS", "CONNECTED"},
				func(item any) []string {
					m, ok := item.(map[string]any)
					if !ok {
						return []string{"", "", "", "", ""}
					}
					return []string{
						fmt.Sprintf("%v", m["id"]),
						fmt.Sprintf("%v", m["name"]),
						fmt.Sprintf("%v", m["type"]),
						fmt.Sprintf("%v", m["status"]),
						fmt.Sprintf("%v", m["connected"]),
					}
				},
			)
		},
	}
}

// newAgentGetCmd 는 에이전트 상세 조회 커맨드를 생성한다.
// GET /api/v1/agents/:id
func newAgentGetCmd(client **Client) *cobra.Command {
	return &cobra.Command{
		Use:   "get <id>",
		Short: "에이전트 상세 조회",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id := args[0]

			var agent map[string]any
			if err := (*client).Get("/api/v1/agents/"+id, &agent); err != nil {
				return err
			}

			format, _ := cmd.Flags().GetString("format")
			w := cmd.OutOrStdout()

			return PrintResult(w, format, agent, nil, nil)
		},
	}
}

// newAgentCreateCmd 는 파일 기반 에이전트 생성 커맨드를 생성한다.
// POST /api/v1/agents
func newAgentCreateCmd(client **Client) *cobra.Command {
	var filePath string

	cmd := &cobra.Command{
		Use:   "create",
		Short: "에이전트 생성",
		RunE: func(cmd *cobra.Command, args []string) error {
			if filePath == "" {
				return ErrInvalidInput("파일 경로를 지정해주세요 (-f 플래그)")
			}

			// 파일 읽기
			data, err := readFile(filePath)
			if err != nil {
				return err
			}

			// 파일 형식에 따라 파싱
			var body map[string]any
			fileFormat := detectFileFormat(filePath)

			switch fileFormat {
			case "yaml":
				if err := yaml.Unmarshal(data, &body); err != nil {
					return ErrInvalidInput(fmt.Sprintf("YAML 파싱 실패: %v", err))
				}
			case "json":
				if err := json.Unmarshal(data, &body); err != nil {
					return ErrInvalidInput(fmt.Sprintf("JSON 파싱 실패: %v", err))
				}
			default:
				// 기본적으로 JSON 으로 시도
				if err := json.Unmarshal(data, &body); err != nil {
					return ErrInvalidInput(fmt.Sprintf("파일 형식을 인식할 수 없습니다: %s", filePath))
				}
			}

			// API 호출
			var result map[string]any
			if err := (*client).Post("/api/v1/agents", body, &result); err != nil {
				return err
			}

			format, _ := cmd.Flags().GetString("format")
			w := cmd.OutOrStdout()

			return PrintResult(w, format, result, nil, nil)
		},
	}

	cmd.Flags().StringVarP(&filePath, "file", "f", "", "에이전트 정의 파일 경로 (JSON/YAML)")

	return cmd
}

// newAgentStartCmd 는 에이전트 시작 커맨드를 생성한다.
// POST /api/v1/agents/:id/start
func newAgentStartCmd(client **Client) *cobra.Command {
	return newAgentLifecycleCmd(client, "start", "에이전트 시작")
}

// newAgentStopCmd 는 에이전트 중지 커맨드를 생성한다.
// POST /api/v1/agents/:id/stop
func newAgentStopCmd(client **Client) *cobra.Command {
	return newAgentLifecycleCmd(client, "stop", "에이전트 중지")
}

// newAgentRestartCmd 는 에이전트 재시작 커맨드를 생성한다.
// POST /api/v1/agents/:id/restart
func newAgentRestartCmd(client **Client) *cobra.Command {
	return newAgentLifecycleCmd(client, "restart", "에이전트 재시작")
}

// newAgentLifecycleCmd 는 에이전트 라이프사이클 (start/stop/restart) 커맨드의 공통 팩토리이다.
// POST /api/v1/agents/:id/{action}
func newAgentLifecycleCmd(client **Client, action, short string) *cobra.Command {
	return &cobra.Command{
		Use:   action + " <id>",
		Short: short,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id := args[0]

			var result map[string]any
			path := fmt.Sprintf("/api/v1/agents/%s/%s", id, action)
			if err := (*client).Post(path, nil, &result); err != nil {
				return err
			}

			w := cmd.OutOrStdout()
			fmt.Fprintf(w, "에이전트 %s: %s 완료\n", id, short)
			return nil
		},
	}
}

// newAgentDeleteCmd 는 에이전트 삭제 커맨드를 생성한다.
// DELETE /api/v1/agents/:id
// --yes 플래그로 확인 프롬프트를 건너뛸 수 있다.
func newAgentDeleteCmd(client **Client, confirmFn func(string, io.Reader) bool) *cobra.Command {
	var yes bool

	cmd := &cobra.Command{
		Use:   "delete <id>",
		Short: "에이전트 삭제",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id := args[0]
			w := cmd.OutOrStdout()

			// --yes 플래그가 없으면 확인 요청
			if !yes {
				prompt := fmt.Sprintf("에이전트 '%s' 를 삭제하시겠습니까?", id)
				if !confirmFn(prompt, os.Stdin) {
					fmt.Fprintln(w, "삭제가 취소되었습니다.")
					return nil
				}
			}

			// DELETE 요청
			var result map[string]any
			if err := (*client).Delete("/api/v1/agents/"+id, &result); err != nil {
				return err
			}

			fmt.Fprintf(w, "에이전트 '%s' 가 삭제되었습니다.\n", id)

			// 경고 메시지가 있으면 출력
			if result != nil {
				if warning, ok := result["warning"]; ok && warning != nil {
					fmt.Fprintf(w, "경고: %v\n", warning)
				}
			}

			return nil
		},
	}

	cmd.Flags().BoolVar(&yes, "yes", false, "확인 프롬프트 건너뛰기")

	return cmd
}
