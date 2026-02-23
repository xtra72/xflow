package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// newNodeCmd 는 노드 관리 커맨드 그룹을 생성한다.
// 읽기 전용 서브커맨드(type, list) 만 포함하므로 confirmFn 이 필요 없다.
func newNodeCmd(client **Client) *cobra.Command {
	nodeCmd := &cobra.Command{
		Use:   "node",
		Short: "노드 관리 명령어",
		Long:  "노드 타입 조회 및 런타임 노드 인스턴스 목록을 확인합니다.",
	}

	nodeCmd.AddCommand(newNodeTypeCmd(client))
	nodeCmd.AddCommand(newNodeListCmd(client))
	nodeCmd.AddCommand(newNodeInfoCmd(client))

	return nodeCmd
}

// nodeInstanceTableHeaders 는 런타임 노드 인스턴스 목록 테이블의 헤더이다.
var nodeInstanceTableHeaders = []string{"FLOW", "NODE_ID", "NAME", "TYPE", "STATE"}

// nodeInstanceRowFunc 는 노드 인스턴스 맵에서 테이블 행을 추출한다.
func nodeInstanceRowFunc(item any) []string {
	m, ok := item.(map[string]any)
	if !ok {
		return []string{"", "", "", "", ""}
	}
	return []string{
		fmt.Sprintf("%v", m["flow"]),
		fmt.Sprintf("%v", m["node_id"]),
		fmt.Sprintf("%v", m["name"]),
		fmt.Sprintf("%v", m["type"]),
		fmt.Sprintf("%v", m["state"]),
	}
}

// newNodeListCmd 는 node list [flow_id|flow_name] 서브커맨드를 생성한다.
// 인자 없이 호출하면 모든 배포된 플로우의 노드 인스턴스 목록을 조회한다.
// 인자가 있으면 특정 플로우의 노드 인스턴스 목록을 조회한다.
// --name 플래그로 노드 이름 부분 일치 필터링을 지원한다.
func newNodeListCmd(client **Client) *cobra.Command {
	var name string

	cmd := &cobra.Command{
		Use:   "list [flow_id|flow_name]",
		Short: "런타임 노드 인스턴스 목록 조회",
		Long: `배포된 플로우의 런타임 노드 인스턴스 목록을 표시합니다.
인자 없이 호출하면 모든 플로우의 노드를 표시합니다.
플로우 ID 또는 이름을 지정하면 해당 플로우의 노드만 표시합니다.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 {
				return nodeListForFlow(*client, cmd, args[0], name)
			}
			return nodeListAll(*client, cmd, name)
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "노드 이름으로 필터링 (부분 일치)")

	return cmd
}

// nodeListForFlow 는 특정 플로우의 노드 인스턴스 목록을 조회한다.
func nodeListForFlow(client *Client, cmd *cobra.Command, flowArg, name string) error {
	flowID, err := resolveFlowID(client, flowArg)
	if err != nil {
		return err
	}

	// 플로우 이름 조회
	flowName := resolveFlowName(client, flowID, flowArg)

	var nodes []map[string]any
	if err := client.Get("/api/v1/flows/"+flowID+"/nodes", &nodes); err != nil {
		return err
	}

	// flow 컬럼 추가
	for _, n := range nodes {
		n["flow"] = flowName
	}

	nodes = filterByName(nodes, name, "name")

	format := getFormat(cmd)
	w := cmd.OutOrStdout()
	return PrintResult(w, format, nodes, nodeInstanceTableHeaders, nodeInstanceRowFunc)
}

// nodeListAll 는 모든 배포된 플로우의 노드 인스턴스 목록을 조회한다.
func nodeListAll(client *Client, cmd *cobra.Command, name string) error {
	// 1. 모든 플로우 목록 조회
	var flows []map[string]any
	if err := client.Get("/api/v1/flows", &flows); err != nil {
		return err
	}

	// 2. 각 플로우의 노드를 수집
	var allNodes []map[string]any
	for _, f := range flows {
		flowID, _ := f["id"].(string)
		flowName, _ := f["name"].(string)
		if flowID == "" {
			continue
		}

		var nodes []map[string]any
		if err := client.Get("/api/v1/flows/"+flowID+"/nodes", &nodes); err != nil {
			// 미배포 플로우는 노드가 없으므로 스킵
			continue
		}

		for _, n := range nodes {
			n["flow"] = flowName
		}
		allNodes = append(allNodes, nodes...)
	}

	allNodes = filterByName(allNodes, name, "name")

	format := getFormat(cmd)
	w := cmd.OutOrStdout()
	return PrintResult(w, format, allNodes, nodeInstanceTableHeaders, nodeInstanceRowFunc)
}

// resolveFlowName 는 플로우 ID에서 이름을 조회한다.
// 실패 시 fallback 을 반환한다.
func resolveFlowName(client *Client, flowID, fallback string) string {
	var flow map[string]any
	if err := client.Get("/api/v1/flows/"+flowID, &flow); err != nil {
		return fallback
	}
	if name, ok := flow["name"].(string); ok && name != "" {
		return name
	}
	return fallback
}

// nodeDetailFieldOrder 는 노드 인스턴스 상세 출력의 필드 순서이다.
var nodeDetailFieldOrder = []string{"node_id", "name", "type", "state", "config", "ports"}

// nodeDetailLabelMap 는 노드 인스턴스 상세 출력의 필드 라벨 매핑이다.
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

// newNodeInfoCmd 는 node info <flow_id|flow_name> <node_id|node_name> 서브커맨드를 생성한다.
// GET /api/v1/flows/:id/nodes/:nodeID 로 특정 노드 인스턴스의 상세 정보를 조회한다.
func newNodeInfoCmd(client **Client) *cobra.Command {
	return &cobra.Command{
		Use:   "info <flow_id|flow_name> <node_id|node_name>",
		Short: "노드 인스턴스 상세 조회",
		Long:  "배포된 플로우 내 특정 노드 인스턴스의 설정, 상태, 포트 정보를 표시합니다.",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			flowID, err := resolveFlowID(*client, args[0])
			if err != nil {
				return err
			}

			var node map[string]any
			path := fmt.Sprintf("/api/v1/flows/%s/nodes/%s", flowID, args[1])
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

// nodeTableHeaders 는 노드 목록 테이블의 헤더이다.
var nodeTableHeaders = []string{"TYPE", "CATEGORY", "DESCRIPTION", "SOURCE"}

// nodeRowFunc 는 노드 맵에서 테이블 행을 추출한다.
func nodeRowFunc(item any) []string {
	m, ok := item.(map[string]any)
	if !ok {
		return []string{"", "", "", ""}
	}
	return []string{
		fmt.Sprintf("%v", m["type"]),
		fmt.Sprintf("%v", m["category"]),
		fmt.Sprintf("%v", m["description"]),
		fmt.Sprintf("%v", m["source"]),
	}
}

// newNodeTypeCmd 는 node type [type] 서브커맨드를 생성한다.
// 인자 없이 호출하면 GET /api/v1/nodes 로 등록된 노드 타입 목록을 조회한다.
// 인자가 있으면 GET /api/v1/nodes/:type 으로 노드 타입 상세 정보를 조회한다.
// --name 플래그로 타입명 부분 일치 필터링을 지원한다 (목록 조회 시).
func newNodeTypeCmd(client **Client) *cobra.Command {
	var name string

	cmd := &cobra.Command{
		Use:   "type [node_type]",
		Short: "노드 타입 목록 조회 또는 상세 조회",
		Long: `인자 없이 호출하면 등록된 노드 타입 목록을 표시합니다.
노드 타입을 인자로 전달하면 해당 타입의 상세 정보를 표시합니다.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return nodeTypeList(*client, cmd, name)
			}
			return nodeTypeInfo(*client, cmd, args[0])
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "타입명 또는 설명으로 필터링 (부분 일치, 목록 조회 시)")

	return cmd
}

// nodeTypeList 는 등록된 노드 타입 목록을 조회한다.
func nodeTypeList(client *Client, cmd *cobra.Command, name string) error {
	var nodes []map[string]any
	if err := client.Get("/api/v1/nodes", &nodes); err != nil {
		return err
	}

	if name != "" {
		lower := strings.ToLower(name)
		var filtered []map[string]any
		for _, n := range nodes {
			nodeType, _ := n["type"].(string)
			nodeDesc, _ := n["description"].(string)
			if strings.Contains(strings.ToLower(nodeType), lower) ||
				strings.Contains(strings.ToLower(nodeDesc), lower) {
				filtered = append(filtered, n)
			}
		}
		nodes = filtered
	}

	format := getFormat(cmd)
	w := cmd.OutOrStdout()
	return PrintResult(w, format, nodes, nodeTableHeaders, nodeRowFunc)
}

// nodeTypeInfo 는 특정 노드 타입의 상세 정보를 조회한다.
func nodeTypeInfo(client *Client, cmd *cobra.Command, nodeType string) error {
	var nodeInfo map[string]any
	if err := client.Get("/api/v1/nodes/"+nodeType, &nodeInfo); err != nil {
		return err
	}

	format := getFormat(cmd)
	w := cmd.OutOrStdout()

	// 단일 객체는 테이블 대신 text 로 표시
	if format == "table" {
		format = "text"
	}
	return PrintResult(w, format, nodeInfo, nil, nil)
}
