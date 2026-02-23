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
	flowCmd.AddCommand(newFlowNodesCmd(client))
	flowCmd.AddCommand(newFlowNodeCmd(client))

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
// --name 플래그로 이름 부분 일치 필터링을 지원한다.
func newFlowListCmd(client **Client) *cobra.Command {
	var name string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "플로우 목록 조회",
		RunE: func(cmd *cobra.Command, args []string) error {
			var flows []map[string]any
			if err := (*client).Get("/api/v1/flows", &flows); err != nil {
				return err
			}

			flows = filterByName(flows, name, "name")

			format := getFormat(cmd)
			w := cmd.OutOrStdout()
			return PrintResult(w, format, flows, flowTableHeaders, flowRowFunc)
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "이름으로 필터링 (부분 일치)")

	return cmd
}

// newFlowGetCmd 는 flow get <id> 서브커맨드를 생성한다.
// GET /api/v1/flows/:id 로 플로우 상세 정보를 조회한다.
func newFlowGetCmd(client **Client) *cobra.Command {
	return &cobra.Command{
		Use:   "get <id|name>",
		Short: "플로우 상세 조회",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := resolveFlowID(*client, args[0])
			if err != nil {
				return err
			}
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
		Use:   "update <id|name>",
		Short: "파일에서 플로우 업데이트",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := resolveFlowID(*client, args[0])
			if err != nil {
				return err
			}

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
		Use:   "delete <id|name>",
		Short: "플로우 삭제",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := resolveFlowID(*client, args[0])
			if err != nil {
				return err
			}
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
		Use:   action + " <id|name>",
		Short: short,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := resolveFlowID(*client, args[0])
			if err != nil {
				return err
			}
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
		Use:   "export <id|name>",
		Short: "플로우를 파일로 내보내기",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if outputPath == "" {
				return fmt.Errorf("출력 파일 경로(-o)를 지정해야 합니다")
			}

			id, err := resolveFlowID(*client, args[0])
			if err != nil {
				return err
			}
			var flow map[string]any
			if err := (*client).Get("/api/v1/flows/"+id, &flow); err != nil {
				return err
			}

			// 파일 확장자로 형식 자동 감지
			fileFormat := detectFileFormat(outputPath)
			var data []byte

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

			// FlowCreateRequest 형식으로 래핑
			name, _ := body["name"].(string)
			desc, _ := body["description"].(string)
			request := map[string]any{
				"name":        name,
				"description": desc,
				"definition":  body,
			}

			var result map[string]any
			if err := (*client).Post("/api/v1/flows", request, &result); err != nil {
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
		Use:   "status <id|name>",
		Short: "플로우 런타임 상태 조회",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := resolveFlowID(*client, args[0])
			if err != nil {
				return err
			}
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

// flowNodeTableHeaders 는 플로우 노드 인스턴스 목록 테이블의 헤더이다.
var flowNodeTableHeaders = []string{"NODE_ID", "NAME", "TYPE", "STATE"}

// flowNodeRowFunc 는 플로우 노드 인스턴스 맵에서 테이블 행을 추출한다.
func flowNodeRowFunc(item any) []string {
	m, ok := item.(map[string]any)
	if !ok {
		return []string{"", "", "", ""}
	}
	return []string{
		fmt.Sprintf("%v", m["node_id"]),
		fmt.Sprintf("%v", m["name"]),
		fmt.Sprintf("%v", m["type"]),
		fmt.Sprintf("%v", m["state"]),
	}
}

// newFlowNodesCmd 는 flow nodes <id|name> 서브커맨드를 생성한다.
// GET /api/v1/flows/:id/nodes 로 배포된 플로우의 노드 인스턴스 목록을 조회한다.
func newFlowNodesCmd(client **Client) *cobra.Command {
	var name string

	cmd := &cobra.Command{
		Use:   "nodes <id|name>",
		Short: "플로우 노드 인스턴스 목록 조회",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := resolveFlowID(*client, args[0])
			if err != nil {
				return err
			}

			var nodes []map[string]any
			if err := (*client).Get("/api/v1/flows/"+id+"/nodes", &nodes); err != nil {
				return err
			}

			nodes = filterByName(nodes, name, "name")

			format := getFormat(cmd)
			w := cmd.OutOrStdout()
			return PrintResult(w, format, nodes, flowNodeTableHeaders, flowNodeRowFunc)
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "노드 이름으로 필터링 (부분 일치)")

	return cmd
}

// nodeDetailFieldOrder 는 flow node 상세 출력의 필드 순서이다.
var nodeDetailFieldOrder = []string{"node_id", "name", "type", "state", "config", "ports"}

// nodeDetailLabelMap 는 flow node 상세 출력의 필드 라벨 매핑이다.
var nodeDetailLabelMap = map[string]string{
	"node_id": "Node ID",
	"name":    "Name",
	"type":    "Type",
	"state":   "State",
	"config":  "Config",
	"ports":   "Ports",
}

// nodeDetailSectionKeys 는 별도 섹션으로 출력할 키 목록이다.
var nodeDetailSectionKeys = map[string]bool{
	"config": true,
	"ports":  true,
}

// newFlowNodeCmd 는 flow node <id|name> <nodeID> 서브커맨드를 생성한다.
// GET /api/v1/flows/:id/nodes/:nodeID 로 특정 노드 인스턴스의 상세 정보를 조회한다.
func newFlowNodeCmd(client **Client) *cobra.Command {
	return &cobra.Command{
		Use:   "node <id|name> <nodeID>",
		Short: "플로우 노드 인스턴스 상세 조회",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := resolveFlowID(*client, args[0])
			if err != nil {
				return err
			}

			var node map[string]any
			path := fmt.Sprintf("/api/v1/flows/%s/nodes/%s", id, args[1])
			if err := (*client).Get(path, &node); err != nil {
				return err
			}

			format := getFormat(cmd)
			w := cmd.OutOrStdout()

			// table/text 형식에서는 구조화된 상세 포맷 사용
			if format == "table" || format == "text" {
				df := NewDetailFormatter(nodeDetailFieldOrder, nodeDetailLabelMap, nodeDetailSectionKeys)
				return df.Format(node, w)
			}
			return PrintResult(w, format, node, nil, nil)
		},
	}
}

// resolveFlowID 는 인자를 플로우 ID로 해석한다.
// 플로우 목록에서 이름이 일치하는 항목을 찾아 ID를 반환한다.
// 이름이 중복되면 에러를 반환하고, 일치하는 이름이 없으면 원본을 그대로 반환한다.
func resolveFlowID(client *Client, idOrName string) (string, error) {
	// UUID 형식이면 바로 반환 (API 호출 불필요)
	if isUUID(idOrName) {
		return idOrName, nil
	}
	// 이름으로 검색
	var flows []map[string]any
	if err := client.Get("/api/v1/flows", &flows); err != nil {
		if client.verbose {
			fmt.Fprintf(os.Stderr, "[resolve] 플로우 목록 조회 실패: %v\n", err)
		}
		// 목록 조회 실패 시 원본 그대로 반환 (서버가 ID로 처리)
		return idOrName, nil
	}

	var matches []string
	for _, f := range flows {
		name, _ := f["name"].(string)
		if name == idOrName {
			id, _ := f["id"].(string)
			matches = append(matches, id)
		}
	}

	switch len(matches) {
	case 0:
		// 이름 매칭 없음 → 원본 그대로 반환 (서버가 ID로 처리 시도)
		return idOrName, nil
	case 1:
		return matches[0], nil
	default:
		return "", fmt.Errorf("동일한 이름의 플로우가 %d개 있습니다: %q (ID를 사용하세요)", len(matches), idOrName)
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
