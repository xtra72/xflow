package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// newNodeCmd 는 노드 타입 관리 커맨드 그룹을 생성한다.
// 읽기 전용 서브커맨드(list, info) 만 포함하므로 confirmFn 이 필요 없다.
func newNodeCmd(client **Client) *cobra.Command {
	nodeCmd := &cobra.Command{
		Use:   "node",
		Short: "노드 타입 관리 명령어",
		Long:  "등록된 노드 타입의 조회 및 상세 정보 확인을 수행합니다.",
	}

	nodeCmd.AddCommand(newNodeListCmd(client))
	nodeCmd.AddCommand(newNodeInfoCmd(client))

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

// newNodeListCmd 는 node list 서브커맨드를 생성한다.
// GET /api/v1/nodes 로 등록된 노드 타입 목록을 조회한다.
// --name 플래그로 타입명 부분 일치 필터링을 지원한다.
func newNodeListCmd(client **Client) *cobra.Command {
	var name string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "노드 타입 목록 조회",
		RunE: func(cmd *cobra.Command, args []string) error {
			var nodes []map[string]any
			if err := (*client).Get("/api/v1/nodes", &nodes); err != nil {
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
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "타입명 또는 설명으로 필터링 (부분 일치)")

	return cmd
}

// newNodeInfoCmd 는 node info <type> 서브커맨드를 생성한다.
// GET /api/v1/nodes/:type 으로 노드 타입의 상세 정보를 조회한다.
// 입출력 포트, 설정 스키마, 설명 등을 표시한다.
func newNodeInfoCmd(client **Client) *cobra.Command {
	return &cobra.Command{
		Use:   "info <type>",
		Short: "노드 타입 상세 조회",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			nodeType := args[0]
			var nodeInfo map[string]any
			if err := (*client).Get("/api/v1/nodes/"+nodeType, &nodeInfo); err != nil {
				return err
			}

			format := getFormat(cmd)
			w := cmd.OutOrStdout()

			// 단일 객체는 테이블 대신 text 로 표시
			if format == "table" {
				format = "text"
			}
			return PrintResult(w, format, nodeInfo, nil, nil)
		},
	}
}
