package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// newNodeCmd 는 노드 타입 관리 커맨드 그룹을 생성한다.
// 읽기 전용 서브커맨드(type) 만 포함하므로 confirmFn 이 필요 없다.
func newNodeCmd(client **Client) *cobra.Command {
	nodeCmd := &cobra.Command{
		Use:   "node",
		Short: "노드 타입 관리 명령어",
		Long:  "등록된 노드 타입의 조회 및 상세 정보 확인을 수행합니다.",
	}

	nodeCmd.AddCommand(newNodeTypeCmd(client))

	return nodeCmd
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
