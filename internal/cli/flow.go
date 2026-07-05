package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// newFlowCmd 는 플로우 관리 커맨드 그룹을 생성한다.
// 18개의 서브커맨드(list, get, create, update, delete,
// deploy, start, stop, restart, undeploy, export, import, status,
// config, subflow-stats, tap, taps, node-configure)를 등록한다.
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
	flowCmd.AddCommand(newFlowUndeployCmd(client))
	flowCmd.AddCommand(newFlowExportCmd(client))
	flowCmd.AddCommand(newFlowImportCmd(client))
	flowCmd.AddCommand(newFlowStatusCmd(client))
	flowCmd.AddCommand(newFlowConfigCmd(client))
	flowCmd.AddCommand(newFlowSubflowStatsCmd(client))
	flowCmd.AddCommand(newFlowTapCmd(client))
	flowCmd.AddCommand(newFlowTapsCmd(client))
	flowCmd.AddCommand(newFlowNodeConfigureCmd(client))

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

			// 단일 객체는 DetailFormatter 로 가독성 있게 표시
			if format == "table" || format == "text" {
				displayData := prepareFlowDetail(flow)
				df := NewDetailFormatter(flowDetailFieldOrder, flowDetailLabelMap, flowDetailSectionKeys)
				return df.Format(displayData, w)
			}
			return PrintResult(w, format, flow, nil, nil)
		},
	}
}

// newFlowCreateCmd 는 flow create -f <file> 서브커맨드를 생성한다.
// 파일을 읽어 FlowCreateRequest 형식으로 래핑한 후 POST /api/v1/flows 로 플로우를 생성한다.
func newFlowCreateCmd(client **Client) *cobra.Command {
	var (
		filePath     string
		skipExisting bool
	)

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

			name, desc, definition := extractDefinition(body)

			// --skip-existing: 동일 이름의 플로우가 있으면 건너뛴다
			if skipExisting && name != "" {
				if exists, id := flowExistsByName(*client, name); exists {
					w := cmd.OutOrStdout()
					fmt.Fprintf(w, "플로우 '%s' 가 이미 존재합니다 (ID: %s). 건너뜁니다.\n", name, id)
					return nil
				}
			}

			// FlowCreateRequest 형식으로 래핑
			request := map[string]any{
				"name":        name,
				"description": desc,
				"definition":  definition,
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

	cmd.Flags().StringVarP(&filePath, "file", "f", "", "플로우 정의 파일 경로 (JSON/YAML)")
	cmd.Flags().BoolVar(&skipExisting, "skip-existing", false, "동일 이름의 플로우가 있으면 건너뛰기")
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

// newFlowUndeployCmd 는 flow undeploy <id> 서브커맨드를 생성한다.
// POST /api/v1/flows/:id/undeploy 로 플로우 배포를 해제한다.
func newFlowUndeployCmd(client **Client) *cobra.Command {
	return newFlowActionCmd(client, "undeploy", "플로우 배포 해제")
}

// newFlowConfigCmd 는 flow config <id|name> [-f file | key=value ...] 서브커맨드를 생성한다.
// PUT /api/v1/flows/:id/config 로 플로우 구성(런타임 설정값)을 수정한다.
// 요청 본문은 {"config": {...}} 형식이다 (dto.ConfigUpdateRequest).
//
// 설정값은 파일(-f) 또는 key=value 인자로 전달할 수 있다.
//
//	xflow flow config <id|name> -f config.json
//	xflow flow config <id|name> key=value [key=value ...]
func newFlowConfigCmd(client **Client) *cobra.Command {
	var filePath string

	cmd := &cobra.Command{
		Use:   "config <id|name> [key=value ...]",
		Short: "플로우 구성 수정",
		Long: `플로우의 구성(설정값)을 수정합니다.

설정값은 파일(-f) 또는 key=value 인자로 전달할 수 있습니다.

예시:
  xflow flow config my-flow -f config.json
  xflow flow config my-flow log_level=debug max_retries=3`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := resolveFlowID(*client, args[0])
			if err != nil {
				return err
			}

			config, err := buildConfigFromArgs(filePath, args[1:])
			if err != nil {
				return err
			}

			// 요청 본문: ConfigUpdateRequest = {"config": {...}}
			body := map[string]any{"config": config}

			var result map[string]any
			path := fmt.Sprintf("/api/v1/flows/%s/config", id)
			if err := (*client).Put(path, body, &result); err != nil {
				return err
			}

			format := getFormat(cmd)
			w := cmd.OutOrStdout()
			if format == "table" || format == "text" {
				fmt.Fprintf(w, "플로우 '%s' 구성이 수정되었습니다.\n", id)
				return nil
			}
			return PrintResult(w, format, result, nil, nil)
		},
	}

	cmd.Flags().StringVarP(&filePath, "file", "f", "", "구성 정의 파일 경로 (JSON/YAML)")

	return cmd
}

// newFlowSubflowStatsCmd 는 flow subflow-stats <id|name> 서브커맨드를 생성한다.
// GET /api/v1/flows/:id/subflow-stats 로 서브플로우의 LIVE per-original-node 통계를 조회한다.
func newFlowSubflowStatsCmd(client **Client) *cobra.Command {
	return &cobra.Command{
		Use:   "subflow-stats <id|name>",
		Short: "서브플로우 통계 조회",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := resolveFlowID(*client, args[0])
			if err != nil {
				return err
			}

			var stats map[string]any
			path := fmt.Sprintf("/api/v1/flows/%s/subflow-stats", id)
			if err := (*client).Get(path, &stats); err != nil {
				return err
			}

			format := getFormat(cmd)
			w := cmd.OutOrStdout()

			if format == "table" || format == "text" {
				df := NewDetailFormatter(subflowStatsFieldOrder, subflowStatsLabelMap, subflowStatsSectionKeys)
				return df.Format(stats, w)
			}
			return PrintResult(w, format, stats, nil, nil)
		},
	}
}

// newFlowTapCmd 는 flow tap <id|name> <nodeID> [--disable] 서브커맨드를 생성한다.
// POST /api/v1/flows/:id/nodes/:nodeID/tap 으로 노드 출력 tap(라이브 데이터 캡처)을 토글한다.
// 요청 본문은 {"enabled": bool} 형식이다 (dto.NodeTapRequest).
//
// 기본값은 tap 활성화(enabled=true)이며, --disable 플래그로 비활성화한다.
func newFlowTapCmd(client **Client) *cobra.Command {
	var disable bool

	cmd := &cobra.Command{
		Use:   "tap <id|name> <nodeID>",
		Short: "노드 출력 탭(라이브 데이터 캡처)",
		Long: `노드 출력 tap 을 토글하여 라이브 데이터 캡처를 시작/중지합니다.

기본값은 tap 활성화이며, --disable 플래그로 비활성화합니다.

예시:
  xflow flow tap my-flow node-123
  xflow flow tap my-flow node-123 --disable`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := resolveFlowID(*client, args[0])
			if err != nil {
				return err
			}
			nodeID := args[1]

			// 요청 본문: NodeTapRequest = {"enabled": bool}
			body := map[string]any{"enabled": !disable}

			var result map[string]any
			path := fmt.Sprintf("/api/v1/flows/%s/nodes/%s/tap", id, nodeID)
			if err := (*client).Post(path, body, &result); err != nil {
				return err
			}

			format := getFormat(cmd)
			w := cmd.OutOrStdout()
			if format == "table" || format == "text" {
				state := "활성화"
				if disable {
					state = "비활성화"
				}
				fmt.Fprintf(w, "플로우 '%s' 노드 '%s' tap %s 완료.\n", id, nodeID, state)
				return nil
			}
			return PrintResult(w, format, result, nil, nil)
		},
	}

	cmd.Flags().BoolVar(&disable, "disable", false, "tap 비활성화 (기본값: 활성화)")

	return cmd
}

// newFlowTapsCmd 는 flow taps <id|name> 서브커맨드를 생성한다.
// GET /api/v1/flows/:id/taps 로 현재 tap(관측) 중인 노드 ID 목록을 조회한다.
func newFlowTapsCmd(client **Client) *cobra.Command {
	return &cobra.Command{
		Use:   "taps <id|name>",
		Short: "활성 탭 목록 조회",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := resolveFlowID(*client, args[0])
			if err != nil {
				return err
			}

			var result map[string]any
			path := fmt.Sprintf("/api/v1/flows/%s/taps", id)
			if err := (*client).Get(path, &result); err != nil {
				return err
			}

			format := getFormat(cmd)
			w := cmd.OutOrStdout()

			if format == "table" || format == "text" {
				df := NewDetailFormatter(flowTapsFieldOrder, flowTapsLabelMap, flowTapsSectionKeys)
				return df.Format(result, w)
			}
			return PrintResult(w, format, result, nil, nil)
		},
	}
}

// newFlowNodeConfigureCmd 는 flow node-configure <id|name> <nodeID> [-f file | key=value ...]
// 서브커맨드를 생성한다.
// POST /api/v1/flows/:id/nodes/:nodeID/configure 로 실행 중인 노드에 부분 설정을 즉시 적용한다.
// 요청 본문은 {"config": {...}} 형식이다 (dto.NodeConfigureRequest).
//
// 설정값은 파일(-f) 또는 key=value 인자로 전달할 수 있다.
//
//	xflow flow node-configure <id|name> <nodeID> -f config.json
//	xflow flow node-configure <id|name> <nodeID> output_enabled=false
func newFlowNodeConfigureCmd(client **Client) *cobra.Command {
	var filePath string

	cmd := &cobra.Command{
		Use:   "node-configure <id|name> <nodeID> [key=value ...]",
		Short: "노드 구성 수정",
		Long: `실행 중인 플로우 내 특정 노드의 구성을 라이브로 수정합니다.

설정값은 파일(-f) 또는 key=value 인자로 전달할 수 있습니다.

예시:
  xflow flow node-configure my-flow node-123 -f config.json
  xflow flow node-configure my-flow node-123 output_enabled=false`,
		Args: cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := resolveFlowID(*client, args[0])
			if err != nil {
				return err
			}
			nodeID := args[1]

			config, err := buildConfigFromArgs(filePath, args[2:])
			if err != nil {
				return err
			}

			// 요청 본문: NodeConfigureRequest = {"config": {...}}
			body := map[string]any{"config": config}

			var result map[string]any
			path := fmt.Sprintf("/api/v1/flows/%s/nodes/%s/configure", id, nodeID)
			if err := (*client).Post(path, body, &result); err != nil {
				return err
			}

			format := getFormat(cmd)
			w := cmd.OutOrStdout()
			if format == "table" || format == "text" {
				fmt.Fprintf(w, "플로우 '%s' 노드 '%s' 구성이 수정되었습니다.\n", id, nodeID)
				return nil
			}
			return PrintResult(w, format, result, nil, nil)
		},
	}

	cmd.Flags().StringVarP(&filePath, "file", "f", "", "구성 정의 파일 경로 (JSON/YAML)")

	return cmd
}

// buildConfigFromArgs 는 config 맵을 구성한다.
// filePath 가 지정되면 파일(-f)을 우선하고, 아니면 key=value 인자를 파싱한다.
// 둘 다 없으면 에러를 반환한다.
// 파일/인자 처리 규칙은 agent config 커맨드(parseConfigFile, parseParamValue)와 동일하다.
func buildConfigFromArgs(filePath string, kvArgs []string) (map[string]any, error) {
	if filePath != "" {
		return parseConfigFile(filePath)
	}

	if len(kvArgs) == 0 {
		return nil, ErrInvalidInput("설정값을 지정해주세요 (-f 파일 또는 key=value 인자)")
	}

	config := make(map[string]any, len(kvArgs))
	for _, arg := range kvArgs {
		k, v, ok := strings.Cut(arg, "=")
		if !ok {
			return nil, ErrInvalidInput(fmt.Sprintf("잘못된 파라미터 형식: %q (key=value 형식 필요)", arg))
		}
		config[k] = parseParamValue(v)
	}
	return config, nil
}

// subflowStatsFieldOrder 는 서브플로우 통계 출력의 필드 순서이다.
var subflowStatsFieldOrder = []string{"flow_id", "nodes"}

// subflowStatsLabelMap 는 서브플로우 통계 출력의 필드 라벨 매핑이다.
var subflowStatsLabelMap = map[string]string{
	"flow_id": "Flow ID",
	"nodes":   "Node Stats",
}

// subflowStatsSectionKeys 는 별도 섹션으로 출력할 키 목록이다.
var subflowStatsSectionKeys = map[string]bool{
	"nodes": true,
}

// flowTapsFieldOrder 는 활성 탭 목록 출력의 필드 순서이다.
var flowTapsFieldOrder = []string{"flow_id", "node_ids"}

// flowTapsLabelMap 는 활성 탭 목록 출력의 필드 라벨 매핑이다.
var flowTapsLabelMap = map[string]string{
	"flow_id":  "Flow ID",
	"node_ids": "Tapped Nodes",
}

// flowTapsSectionKeys 는 별도 섹션으로 출력할 키 목록이다.
var flowTapsSectionKeys = map[string]bool{
	"node_ids": true,
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
	var (
		filePath     string
		skipExisting bool
	)

	cmd := &cobra.Command{
		Use:   "import",
		Short: "파일에서 플로우 가져오기",
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := loadFlowFile(filePath)
			if err != nil {
				return err
			}

			name, desc, definition := extractDefinition(body)

			// --skip-existing: 동일 이름의 플로우가 있으면 건너뛴다
			if skipExisting && name != "" {
				if exists, id := flowExistsByName(*client, name); exists {
					w := cmd.OutOrStdout()
					fmt.Fprintf(w, "플로우 '%s' 가 이미 존재합니다 (ID: %s). 건너뜁니다.\n", name, id)
					return nil
				}
			}

			// FlowCreateRequest 형식으로 래핑
			request := map[string]any{
				"name":        name,
				"description": desc,
				"definition":  definition,
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
	cmd.Flags().BoolVar(&skipExisting, "skip-existing", false, "동일 이름의 플로우가 있으면 건너뛰기")
	_ = cmd.MarkFlagRequired("file")

	return cmd
}

// flowDetailFieldOrder 는 플로우 상세 출력의 필드 순서이다.
var flowDetailFieldOrder = []string{"name", "id", "status", "node_count", "created_at", "updated_at"}

// flowDetailLabelMap 는 플로우 상세 출력의 필드 라벨 매핑이다.
var flowDetailLabelMap = map[string]string{
	"id":         "ID",
	"name":       "Name",
	"status":     "Status",
	"node_count": "Nodes",
	"created_at": "Created At",
	"updated_at": "Updated At",
	"nodes":      "Nodes",
	"edges":      "Edges",
}

// flowDetailSectionKeys 는 별도 섹션으로 출력할 플로우 상세 키 목록이다.
var flowDetailSectionKeys = map[string]bool{
	"nodes": true,
	"edges": true,
}

// buildNodeIDMap 는 노드 ID → 노드 라벨 매핑을 생성한다.
func buildNodeIDMap(nodes []any) map[string]string {
	result := make(map[string]string, len(nodes))
	for _, n := range nodes {
		nm, ok := n.(map[string]any)
		if !ok {
			continue
		}
		id, _ := nm["id"].(string)
		data, _ := nm["data"].(map[string]any)
		if data == nil {
			continue
		}
		label, _ := data["label"].(string)
		if id != "" && label != "" {
			result[id] = label
		}
	}
	return result
}

// extractNodeSummaries 는 플로우 config 에서 표시용 노드 데이터를 추출한다.
func extractNodeSummaries(nodes []any) []any {
	var result []any
	for _, n := range nodes {
		nm, ok := n.(map[string]any)
		if !ok {
			continue
		}
		data, _ := nm["data"].(map[string]any)
		if data == nil {
			continue
		}
		summary := map[string]any{
			"name":      strOrDash(data, "label"),
			"type":      strOrDash(data, "nodeType"),
			"direction": strOrDash(data, "direction"),
			"agent":     strOrDash(data, "agent_name"),
		}
		result = append(result, summary)
	}
	return result
}

// formatEdgeSummaries 는 원시 엣지 데이터를 사람이 읽기 쉬운 문자열로 변환한다.
func formatEdgeSummaries(edges []any, nodeIDMap map[string]string) []any {
	var result []any
	for _, e := range edges {
		em, ok := e.(map[string]any)
		if !ok {
			continue
		}
		src, _ := em["source"].(string)
		tgt, _ := em["target"].(string)
		srcHandle, _ := em["sourceHandle"].(string)
		tgtHandle, _ := em["targetHandle"].(string)

		srcLabel := nodeIDMap[src]
		if srcLabel == "" {
			srcLabel = src
		}
		tgtLabel := nodeIDMap[tgt]
		if tgtLabel == "" {
			tgtLabel = tgt
		}

		result = append(result, fmt.Sprintf("%s -> %s (%s -> %s)", srcLabel, tgtLabel, srcHandle, tgtHandle))
	}
	return result
}

// strOrDash 는 맵에서 문자열 값을 꺼내고, 비어있으면 "-" 를 반환한다.
func strOrDash(m map[string]any, key string) string {
	v, _ := m[key].(string)
	if v == "" {
		return "-"
	}
	return v
}

// prepareFlowDetail 은 원시 API 응답을 표시용 형식으로 변환한다.
func prepareFlowDetail(flow map[string]any) map[string]any {
	result := make(map[string]any)

	// 스칼라 필드 복사
	for _, key := range []string{"id", "name", "status", "created_at", "updated_at"} {
		if v, ok := flow[key]; ok {
			result[key] = v
		}
	}

	// config 데이터 추출
	config, _ := flow["config"].(map[string]any)
	if config == nil {
		// config 없으면 flow 에서 직접 node_count 복사
		if nc, ok := flow["node_count"]; ok {
			result["node_count"] = nc
		}
		return result
	}

	nodes, _ := config["nodes"].([]any)
	edges, _ := config["edges"].([]any)

	result["node_count"] = len(nodes)

	// 노드 요약 정보를 미니 테이블로 표시
	if len(nodes) > 0 {
		result["nodes"] = extractNodeSummaries(nodes)
	}

	// 엣지 요약 정보를 라벨 해석 포함하여 표시
	if len(edges) > 0 {
		nodeIDMap := buildNodeIDMap(nodes)
		result["edges"] = formatEdgeSummaries(edges, nodeIDMap)
	}

	return result
}

// flowStatusFieldOrder 는 플로우 상태 출력의 필드 순서이다.
var flowStatusFieldOrder = []string{"status", "id", "uptime", "message_count", "error_count", "node_stats"}

// flowStatusLabelMap 는 플로우 상태 출력의 필드 라벨 매핑이다.
var flowStatusLabelMap = map[string]string{
	"status":        "Status",
	"id":            "Flow ID",
	"uptime":        "Uptime",
	"message_count": "Messages",
	"error_count":   "Errors",
	"node_stats":    "Node Stats",
}

// flowStatusSectionKeys 는 별도 섹션으로 출력할 키 목록이다.
var flowStatusSectionKeys = map[string]bool{
	"node_stats": true,
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

			if format == "table" || format == "text" {
				df := NewDetailFormatter(flowStatusFieldOrder, flowStatusLabelMap, flowStatusSectionKeys)
				return df.Format(status, w)
			}
			return PrintResult(w, format, status, nil, nil)
		},
	}
}

// flowExistsByName 은 주어진 이름의 플로우가 이미 존재하는지 확인한다.
// 존재하면 true 와 해당 ID 를 반환한다.
func flowExistsByName(client *Client, name string) (bool, string) {
	var flows []map[string]any
	if err := client.Get("/api/v1/flows", &flows); err != nil {
		return false, ""
	}
	for _, f := range flows {
		if n, _ := f["name"].(string); n == name {
			id, _ := f["id"].(string)
			return true, id
		}
	}
	return false, ""
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

// extractDefinition 은 로드된 플로우 파일을 (name, description, definition) 으로 정규화한다.
// 두 가지 입력 형식을 지원한다:
//   - 내보내기 형식: {"name": "...", "description": "...", "definition": {nodes, edges, ...}}
//   - 평면 레거시 형식: {"name": "...", "nodes": [...], "edges": [...], ...}
//
// 두 형식이 동시에 존재하면 내보내기 형식(definition 키)이 우선한다.
// definition 값이 map 이 아니면 평면 형식으로 폴백한다.
func extractDefinition(body map[string]any) (name, description string, definition map[string]any) {
	name, _ = body["name"].(string)
	description, _ = body["description"].(string)

	if d, ok := body["definition"].(map[string]any); ok {
		return name, description, d
	}

	// 평면 형식: top-level name/description 만 제거하고 나머지를 definition 으로 취급한다
	flat := make(map[string]any, len(body))
	for k, v := range body {
		if k == "name" || k == "description" {
			continue
		}
		flat[k] = v
	}
	return name, description, flat
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
