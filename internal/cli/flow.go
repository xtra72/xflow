package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// newFlowCmd 는 플로우 관리 커맨드 그룹을 생성한다.
// 12개의 서브커맨드(list, get, create, update, delete,
// deploy, start, stop, restart, export, import, status)를 등록한다.
func newFlowCmd(client **Client, confirmFn func(string, io.Reader) bool) *cobra.Command {
	flowCmd := &cobra.Command{
		Use:   "flow",
		Short: "플로우 관리 명령어",
		Long:  "플로우의 생성, 조회, 수정, 삭제 및 라이프사이클 관리를 수행합니다.",
	}

	flowCmd.AddCommand(newFlowListCmd(client))
	flowCmd.AddCommand(newFlowGetCmd(client))
	flowCmd.AddCommand(newFlowCreateCmd(client))
	flowCmd.AddCommand(newFlowUpdateCmd(client))
	flowCmd.AddCommand(newFlowDeleteCmd(client, confirmFn))
	flowCmd.AddCommand(newFlowDeployCmd(client))
	flowCmd.AddCommand(newFlowStartCmd(client))
	flowCmd.AddCommand(newFlowStopCmd(client))
	flowCmd.AddCommand(newFlowRestartCmd(client))
	flowCmd.AddCommand(newFlowExportCmd(client))
	flowCmd.AddCommand(newFlowImportCmd(client))
	flowCmd.AddCommand(newFlowStatusCmd(client))

	return flowCmd
}

// getFormat 은 루트 커맨드에서 --format 플래그 값을 가져온다.
// 실패 시 기본값 "table" 을 반환한다.
func getFormat(cmd *cobra.Command) string {
	format, err := cmd.Root().PersistentFlags().GetString("format")
	if err != nil || format == "" {
		return "table"
	}
	return format
}

// flowTableHeaders 는 플로우 목록 테이블의 헤더이다.
var flowTableHeaders = []string{"ID", "NAME", "STATUS", "NODES", "CREATED"}

// flowRowFunc 는 플로우 맵에서 테이블 행을 추출한다.
func flowRowFunc(item any) []string {
	m, ok := item.(map[string]any)
	if !ok {
		return []string{"", "", "", "", ""}
	}
	return []string{
		fmt.Sprintf("%v", m["id"]),
		fmt.Sprintf("%v", m["name"]),
		fmt.Sprintf("%v", m["status"]),
		fmt.Sprintf("%v", m["node_count"]),
		fmt.Sprintf("%v", m["created_at"]),
	}
}

// newFlowListCmd 는 flow list 서브커맨드를 생성한다.
// GET /api/v1/flows 로 플로우 목록을 조회한다.
func newFlowListCmd(client **Client) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "플로우 목록 조회",
		RunE: func(cmd *cobra.Command, args []string) error {
			var flows []map[string]any
			if err := (*client).Get("/api/v1/flows", &flows); err != nil {
				return err
			}

			format := getFormat(cmd)
			w := cmd.OutOrStdout()
			return PrintResult(w, format, flows, flowTableHeaders, flowRowFunc)
		},
	}
}

// newFlowGetCmd 는 flow get <id> 서브커맨드를 생성한다.
// GET /api/v1/flows/:id 로 플로우 상세 정보를 조회한다.
func newFlowGetCmd(client **Client) *cobra.Command {
	return &cobra.Command{
		Use:   "get <id>",
		Short: "플로우 상세 조회",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id := args[0]
			var flow map[string]any
			if err := (*client).Get("/api/v1/flows/"+id, &flow); err != nil {
				return err
			}

			format := getFormat(cmd)
			w := cmd.OutOrStdout()

			// 단일 객체는 테이블 대신 text 로 표시
			if format == "table" {
				format = "text"
			}
			return PrintResult(w, format, flow, nil, nil)
		},
	}
}

// newFlowCreateCmd 는 flow create -f <file> 서브커맨드를 생성한다.
// 파일을 읽어 POST /api/v1/flows 로 플로우를 생성한다.
func newFlowCreateCmd(client **Client) *cobra.Command {
	var filePath string

	cmd := &cobra.Command{
		Use:   "create",
		Short: "파일에서 플로우 생성",
		RunE: func(cmd *cobra.Command, args []string) error {
			if filePath == "" {
				return fmt.Errorf("파일 경로(-f)를 지정해야 합니다")
			}

			body, err := loadFlowFile(filePath)
			if err != nil {
				return err
			}

			var result map[string]any
			if err := (*client).Post("/api/v1/flows", body, &result); err != nil {
				return err
			}

			format := getFormat(cmd)
			w := cmd.OutOrStdout()
			return PrintResult(w, format, result, nil, nil)
		},
	}

	cmd.Flags().StringVarP(&filePath, "file", "f", "", "플로우 정의 파일 경로 (JSON/YAML)")
	_ = cmd.MarkFlagRequired("file")

	return cmd
}

// newFlowUpdateCmd 는 flow update <id> -f <file> 서브커맨드를 생성한다.
// 파일을 읽어 PUT /api/v1/flows/:id 로 플로우를 업데이트한다.
func newFlowUpdateCmd(client **Client) *cobra.Command {
	var filePath string

	cmd := &cobra.Command{
		Use:   "update <id>",
		Short: "파일에서 플로우 업데이트",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id := args[0]

			body, err := loadFlowFile(filePath)
			if err != nil {
				return err
			}

			var result map[string]any
			if err := (*client).Put("/api/v1/flows/"+id, body, &result); err != nil {
				return err
			}

			format := getFormat(cmd)
			w := cmd.OutOrStdout()
			return PrintResult(w, format, result, nil, nil)
		},
	}

	cmd.Flags().StringVarP(&filePath, "file", "f", "", "플로우 정의 파일 경로 (JSON/YAML)")
	_ = cmd.MarkFlagRequired("file")

	return cmd
}

// newFlowDeleteCmd 는 flow delete <id> [--yes] 서브커맨드를 생성한다.
// 확인 프롬프트 후 DELETE /api/v1/flows/:id 로 플로우를 삭제한다.
func newFlowDeleteCmd(client **Client, confirmFn func(string, io.Reader) bool) *cobra.Command {
	var yes bool

	cmd := &cobra.Command{
		Use:   "delete <id>",
		Short: "플로우 삭제",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id := args[0]
			w := cmd.OutOrStdout()

			// --yes 플래그가 없으면 확인 요청
			if !yes {
				prompt := fmt.Sprintf("플로우 '%s' 를 삭제하시겠습니까?", id)
				if !confirmFn(prompt, os.Stdin) {
					fmt.Fprintln(w, "삭제가 취소되었습니다.")
					return nil
				}
			}

			var result map[string]any
			if err := (*client).Delete("/api/v1/flows/"+id, &result); err != nil {
				return err
			}

			format := getFormat(cmd)
			if format == "table" || format == "text" {
				fmt.Fprintf(w, "플로우 '%s' 가 삭제되었습니다.\n", id)
				return nil
			}
			return PrintResult(w, format, result, nil, nil)
		},
	}

	cmd.Flags().BoolVar(&yes, "yes", false, "확인 프롬프트 생략")

	return cmd
}

// newFlowActionCmd 는 deploy/start/stop/restart 같은 액션 커맨드를 생성하는 헬퍼이다.
// POST /api/v1/flows/:id/<action> 을 호출한다.
func newFlowActionCmd(client **Client, action, short string) *cobra.Command {
	return &cobra.Command{
		Use:   action + " <id>",
		Short: short,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id := args[0]
			path := fmt.Sprintf("/api/v1/flows/%s/%s", id, action)

			var result map[string]any
			if err := (*client).Post(path, nil, &result); err != nil {
				return err
			}

			format := getFormat(cmd)
			w := cmd.OutOrStdout()

			if format == "table" || format == "text" {
				fmt.Fprintf(w, "플로우 '%s' %s 완료.\n", id, action)
				return nil
			}
			return PrintResult(w, format, result, nil, nil)
		},
	}
}

// newFlowDeployCmd 는 flow deploy <id> 서브커맨드를 생성한다.
func newFlowDeployCmd(client **Client) *cobra.Command {
	return newFlowActionCmd(client, "deploy", "플로우 배포")
}

// newFlowStartCmd 는 flow start <id> 서브커맨드를 생성한다.
func newFlowStartCmd(client **Client) *cobra.Command {
	return newFlowActionCmd(client, "start", "플로우 시작")
}

// newFlowStopCmd 는 flow stop <id> 서브커맨드를 생성한다.
func newFlowStopCmd(client **Client) *cobra.Command {
	return newFlowActionCmd(client, "stop", "플로우 중지")
}

// newFlowRestartCmd 는 flow restart <id> 서브커맨드를 생성한다.
func newFlowRestartCmd(client **Client) *cobra.Command {
	return newFlowActionCmd(client, "restart", "플로우 재시작")
}

// newFlowExportCmd 는 flow export <id> -o <file> 서브커맨드를 생성한다.
// GET /api/v1/flows/:id 로 플로우를 조회하고 파일로 저장한다.
func newFlowExportCmd(client **Client) *cobra.Command {
	var outputPath string

	cmd := &cobra.Command{
		Use:   "export <id>",
		Short: "플로우를 파일로 내보내기",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if outputPath == "" {
				return fmt.Errorf("출력 파일 경로(-o)를 지정해야 합니다")
			}

			id := args[0]
			var flow map[string]any
			if err := (*client).Get("/api/v1/flows/"+id, &flow); err != nil {
				return err
			}

			// 파일 확장자로 형식 자동 감지
			fileFormat := detectFileFormat(outputPath)
			var data []byte
			var err error

			switch fileFormat {
			case "yaml":
				data, err = yaml.Marshal(flow)
			default:
				// 기본값: JSON
				data, err = json.MarshalIndent(flow, "", "  ")
				if err == nil {
					data = append(data, '\n')
				}
			}
			if err != nil {
				return fmt.Errorf("직렬화 실패: %w", err)
			}

			if err := os.WriteFile(outputPath, data, 0644); err != nil {
				return fmt.Errorf("파일 쓰기 실패: %w", err)
			}

			w := cmd.OutOrStdout()
			fmt.Fprintf(w, "플로우 '%s' 를 %s 로 내보냈습니다.\n", id, outputPath)
			return nil
		},
	}

	cmd.Flags().StringVarP(&outputPath, "output", "o", "", "출력 파일 경로 (.json 또는 .yaml)")
	_ = cmd.MarkFlagRequired("output")

	return cmd
}

// newFlowImportCmd 는 flow import -f <file> 서브커맨드를 생성한다.
// 파일을 읽어 POST /api/v1/flows 로 플로우를 생성한다 (create 와 동일).
func newFlowImportCmd(client **Client) *cobra.Command {
	var filePath string

	cmd := &cobra.Command{
		Use:   "import",
		Short: "파일에서 플로우 가져오기",
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := loadFlowFile(filePath)
			if err != nil {
				return err
			}

			var result map[string]any
			if err := (*client).Post("/api/v1/flows", body, &result); err != nil {
				return err
			}

			format := getFormat(cmd)
			w := cmd.OutOrStdout()
			return PrintResult(w, format, result, nil, nil)
		},
	}

	cmd.Flags().StringVarP(&filePath, "file", "f", "", "가져올 플로우 파일 경로 (JSON/YAML)")
	_ = cmd.MarkFlagRequired("file")

	return cmd
}

// newFlowStatusCmd 는 flow status <id> 서브커맨드를 생성한다.
// GET /api/v1/flows/:id/status 로 런타임 상태를 조회한다.
func newFlowStatusCmd(client **Client) *cobra.Command {
	return &cobra.Command{
		Use:   "status <id>",
		Short: "플로우 런타임 상태 조회",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id := args[0]
			var status map[string]any
			if err := (*client).Get("/api/v1/flows/"+id+"/status", &status); err != nil {
				return err
			}

			format := getFormat(cmd)
			w := cmd.OutOrStdout()

			// 단일 객체: 테이블 형식은 text 로 전환
			if format == "table" {
				format = "text"
			}
			return PrintResult(w, format, status, nil, nil)
		},
	}
}

// loadFlowFile 은 JSON 또는 YAML 파일을 읽어 맵으로 파싱한다.
// 파일 확장자를 기반으로 형식을 자동 감지한다.
func loadFlowFile(path string) (map[string]any, error) {
	data, err := readFile(path)
	if err != nil {
		return nil, err
	}

	fileFormat := detectFileFormat(path)
	var result map[string]any

	switch fileFormat {
	case "yaml":
		if err := yaml.Unmarshal(data, &result); err != nil {
			return nil, fmt.Errorf("YAML 파싱 실패: %w", err)
		}
	default:
		// 기본값: JSON
		if err := json.Unmarshal(data, &result); err != nil {
			return nil, fmt.Errorf("JSON 파싱 실패: %w", err)
		}
	}

	return result, nil
}
