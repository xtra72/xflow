package cli

import (
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

// newRemoteCmd 는 원격 노드 관리 커맨드 그룹을 생성한다.
// 원격 관리 API(/api/v1/remote/*)를 CLI 로 노출한다(SPEC-CLI-004 P3a).
//
// 서브그룹:
//
//	node  원격 노드 조회/승인/거부/폐기/사전등록
//
// confirmFn 은 revoke(폐기) 같은 파괴적 작업의 확인 프롬프트에 사용된다
// (device metadata delete 패턴 참고).
func newRemoteCmd(client **Client, confirmFn func(string, io.Reader) bool) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remote",
		Short: "원격 노드 관리",
		Long:  "원격(엣지) 노드의 조회, 승인, 거부, 폐기, 사전 등록을 수행합니다.",
	}

	cmd.AddCommand(newRemoteNodeCmd(client, confirmFn))

	return cmd
}

// newRemoteNodeCmd 는 remote node 서브커맨드 그룹을 생성한다.
// list / get / approve / reject / revoke / pre-register 하위 커맨드를 포함한다.
func newRemoteNodeCmd(client **Client, confirmFn func(string, io.Reader) bool) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "node",
		Short: "원격 노드 조회 및 등록 관리",
		Long:  "원격 관리 노드의 목록 조회, 상세 조회, 승인/거부/폐기, 사전 등록을 수행합니다.",
	}

	cmd.AddCommand(newRemoteNodeListCmd(client))
	cmd.AddCommand(newRemoteNodeGetCmd(client))
	cmd.AddCommand(newRemoteNodeApproveCmd(client))
	cmd.AddCommand(newRemoteNodeRejectCmd(client))
	cmd.AddCommand(newRemoteNodeRevokeCmd(client, confirmFn))
	cmd.AddCommand(newRemoteNodePreRegisterCmd(client))

	return cmd
}

// remoteNodeTableHeaders 는 원격 노드 목록 테이블의 헤더이다.
var remoteNodeTableHeaders = []string{
	"INSTANCE_ID", "HOSTNAME", "VERSION", "STATUS", "ONLINE", "GROUP", "LAST_SEEN", "OUTDATED",
}

// remoteNodeRowFunc 는 노드 맵에서 테이블 행을 추출한다.
// online/outdated bool 값은 yes/no 로, last_seen(epoch ms)은 정수 문자열로 렌더링한다.
func remoteNodeRowFunc(item any) []string {
	m, ok := item.(map[string]any)
	if !ok {
		return []string{"", "", "", "", "", "", "", ""}
	}
	return []string{
		fmt.Sprintf("%v", m["instance_id"]),
		fmt.Sprintf("%v", m["hostname"]),
		fmt.Sprintf("%v", m["version"]),
		fmt.Sprintf("%v", m["status"]),
		boolToYesNo(m["online"]),
		fmt.Sprintf("%v", m["group_name"]),
		formatEpochValue(m["last_seen"]),
		boolToYesNo(m["outdated"]),
	}
}

// boolToYesNo 는 bool 값을 yes/no 문자열로 변환한다(device.go online 렌더링과 동일).
func boolToYesNo(v any) string {
	if b, ok := v.(bool); ok && b {
		return "yes"
	}
	return "no"
}

// newRemoteNodeListCmd 는 remote node list 서브커맨드를 생성한다.
// 기본: GET /api/v1/remote/nodes, --pending: GET /api/v1/remote/nodes/pending.
//
// 응답은 노드 객체 배열이며 []map[string]any 로 디코딩한다.
func newRemoteNodeListCmd(client **Client) *cobra.Command {
	var pending bool

	cmd := &cobra.Command{
		Use:   "list",
		Short: "원격 노드 목록 조회",
		Long: `원격 관리 노드 목록을 조회합니다.

예시:
  xflow remote node list
  xflow remote node list --pending`,
		RunE: func(cmd *cobra.Command, args []string) error {
			path := "/api/v1/remote/nodes"
			if pending {
				path = "/api/v1/remote/nodes/pending"
			}

			var nodes []map[string]any
			if err := (*client).Get(path, &nodes); err != nil {
				return err
			}

			format := getFormat(cmd)
			w := cmd.OutOrStdout()
			return PrintResult(w, format, nodes, remoteNodeTableHeaders, remoteNodeRowFunc)
		},
	}

	cmd.Flags().BoolVar(&pending, "pending", false, "승인 대기(pending) 노드만 표시")

	return cmd
}

// remoteNodeDetailFieldOrder 는 노드 상세 출력의 필드 순서이다.
var remoteNodeDetailFieldOrder = []string{
	"instance_id", "hostname", "version", "status",
	"online", "group_name", "last_seen", "outdated",
}

// remoteNodeDetailLabelMap 는 노드 상세 출력의 필드 라벨 매핑이다.
var remoteNodeDetailLabelMap = map[string]string{
	"instance_id": "Instance ID",
	"hostname":    "Hostname",
	"version":     "Version",
	"status":      "Status",
	"online":      "Online",
	"group_name":  "Group",
	"last_seen":   "Last Seen",
	"outdated":    "Outdated",
}

// printRemoteNodeDetail 은 단일 노드 상세를 포맷에 맞게 출력한다.
func printRemoteNodeDetail(cmd *cobra.Command, node map[string]any) error {
	format := getFormat(cmd)
	w := cmd.OutOrStdout()

	if format == "table" || format == "text" {
		df := NewDetailFormatter(remoteNodeDetailFieldOrder, remoteNodeDetailLabelMap, nil)
		return df.Format(node, w)
	}
	return PrintResult(w, format, node, nil, nil)
}

// findNodeByID 는 노드 목록에서 instance_id 가 일치하는 노드를 찾는다.
// 일치 항목이 없으면 (nil, false)를 반환한다.
func findNodeByID(nodes []map[string]any, id string) (map[string]any, bool) {
	for _, n := range nodes {
		if v, ok := n["instance_id"].(string); ok && v == id {
			return n, true
		}
	}
	return nil, false
}

// newRemoteNodeGetCmd 는 remote node get 서브커맨드를 생성한다.
//
// 백엔드에는 단일 노드 GET 엔드포인트가 없으므로(라우트는 list/pending/approve/
// reject/revoke/command 만 존재), GET /api/v1/remote/nodes 로 전체 목록을 받아
// instance_id 가 일치하는 노드를 클라이언트 측에서 필터링한다.
func newRemoteNodeGetCmd(client **Client) *cobra.Command {
	return &cobra.Command{
		Use:   "get <instance_id>",
		Short: "원격 노드 상세 조회",
		Long: `원격 노드의 상세 정보를 조회합니다.

백엔드에 단일 노드 조회 엔드포인트가 없어, 전체 목록을 받아
instance_id 로 클라이언트 측에서 필터링합니다.

예시:
  xflow remote node get node-01`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id := strings.TrimSpace(args[0])
			if id == "" {
				return ErrInvalidInput("instance_id 를 지정해야 합니다")
			}

			var nodes []map[string]any
			if err := (*client).Get("/api/v1/remote/nodes", &nodes); err != nil {
				return err
			}

			node, ok := findNodeByID(nodes, id)
			if !ok {
				return ErrInvalidInput(fmt.Sprintf("원격 노드를 찾을 수 없습니다: %s", id))
			}

			return printRemoteNodeDetail(cmd, node)
		},
	}
}

// newRemoteNodeApproveCmd 는 remote node approve 서브커맨드를 생성한다.
// POST /api/v1/remote/nodes/{id}/approve (본문 없음)로 노드를 승인한다.
//
// 응답은 {instance_id, status} 객체이다.
func newRemoteNodeApproveCmd(client **Client) *cobra.Command {
	return &cobra.Command{
		Use:   "approve <instance_id>",
		Short: "원격 노드 승인",
		Long: `승인 대기 중인 원격 노드를 승인합니다.

예시:
  xflow remote node approve node-01`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id := strings.TrimSpace(args[0])
			if id == "" {
				return ErrInvalidInput("instance_id 를 지정해야 합니다")
			}

			var result map[string]any
			path := fmt.Sprintf("/api/v1/remote/nodes/%s/approve", url.PathEscape(id))
			if err := (*client).Post(path, nil, &result); err != nil {
				return err
			}

			format := getFormat(cmd)
			w := cmd.OutOrStdout()
			if format == "table" || format == "text" {
				fmt.Fprintf(w, "노드 '%s' 승인됨\n", id)
				return nil
			}
			return PrintResult(w, format, result, nil, nil)
		},
	}
}

// newRemoteNodeRejectCmd 는 remote node reject 서브커맨드를 생성한다.
// POST /api/v1/remote/nodes/{id}/reject 로 노드를 거부한다.
//
// --reason 이 지정되면 본문 {"reason":"..."} 을 보낸다(reason 은 omitempty 이므로
// 미지정 시 필드를 생략한다).
func newRemoteNodeRejectCmd(client **Client) *cobra.Command {
	var reason string

	cmd := &cobra.Command{
		Use:   "reject <instance_id>",
		Short: "원격 노드 거부",
		Long: `승인 대기 중인 원격 노드를 거부합니다.

예시:
  xflow remote node reject node-01
  xflow remote node reject node-01 --reason "버전 미달"`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id := strings.TrimSpace(args[0])
			if id == "" {
				return ErrInvalidInput("instance_id 를 지정해야 합니다")
			}

			// reason 미지정 시 본문에서 필드를 생략한다(서버 측 reason,omitempty 와 일치).
			var body map[string]any
			if reason != "" {
				body = map[string]any{"reason": reason}
			}

			var result map[string]any
			path := fmt.Sprintf("/api/v1/remote/nodes/%s/reject", url.PathEscape(id))
			if err := (*client).Post(path, body, &result); err != nil {
				return err
			}

			format := getFormat(cmd)
			w := cmd.OutOrStdout()
			if format == "table" || format == "text" {
				fmt.Fprintf(w, "노드 '%s' 거부됨\n", id)
				return nil
			}
			return PrintResult(w, format, result, nil, nil)
		},
	}

	cmd.Flags().StringVar(&reason, "reason", "", "거부 사유 (선택)")

	return cmd
}

// newRemoteNodeRevokeCmd 는 remote node revoke 서브커맨드를 생성한다.
// POST /api/v1/remote/nodes/{id}/revoke (본문 없음)로 노드를 폐기한다.
//
// 폐기는 파괴적 작업이므로 --yes 가 없으면 confirmFn 으로 확인을 요청한다
// (device metadata delete 패턴 참고).
func newRemoteNodeRevokeCmd(client **Client, confirmFn func(string, io.Reader) bool) *cobra.Command {
	var yes bool

	cmd := &cobra.Command{
		Use:   "revoke <instance_id>",
		Short: "원격 노드 폐기",
		Long: `승인된 원격 노드를 폐기합니다(파괴적 작업).

예시:
  xflow remote node revoke node-01
  xflow remote node revoke node-01 --yes`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id := strings.TrimSpace(args[0])
			if id == "" {
				return ErrInvalidInput("instance_id 를 지정해야 합니다")
			}

			w := cmd.OutOrStdout()

			if !yes {
				prompt := fmt.Sprintf("원격 노드 '%s' 를 폐기하시겠습니까?", id)
				if !confirmFn(prompt, os.Stdin) {
					fmt.Fprintln(w, "취소되었습니다.")
					return nil
				}
			}

			var result map[string]any
			path := fmt.Sprintf("/api/v1/remote/nodes/%s/revoke", url.PathEscape(id))
			if err := (*client).Post(path, nil, &result); err != nil {
				return err
			}

			format := getFormat(cmd)
			if format == "table" || format == "text" {
				fmt.Fprintf(w, "노드 '%s' 폐기됨\n", id)
				return nil
			}
			return PrintResult(w, format, result, nil, nil)
		},
	}

	cmd.Flags().BoolVar(&yes, "yes", false, "확인 프롬프트 건너뛰기")

	return cmd
}

// newRemoteNodePreRegisterCmd 는 remote node pre-register 서브커맨드를 생성한다.
// POST /api/v1/remote/nodes 로 사전 승인 노드를 생성한다.
//
// 요청 본문은 {"instance_id":"...","name":"..."} 이며 name 은 omitempty 이므로
// --name 미지정 시 필드를 생략한다. 응답은 노드 객체(ManagedNodeDTO)이다.
func newRemoteNodePreRegisterCmd(client **Client) *cobra.Command {
	var name string

	cmd := &cobra.Command{
		Use:   "pre-register <instance_id>",
		Short: "원격 노드 사전 등록",
		Long: `원격 노드를 사전 승인 상태로 등록합니다.

예시:
  xflow remote node pre-register node-01
  xflow remote node pre-register node-01 --name edge-a`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id := strings.TrimSpace(args[0])
			if id == "" {
				return ErrInvalidInput("instance_id 를 지정해야 합니다")
			}

			body := map[string]any{"instance_id": id}
			// name 미지정 시 본문에서 필드를 생략한다(서버 측 name,omitempty 와 일치).
			if name != "" {
				body["name"] = name
			}

			var node map[string]any
			if err := (*client).Post("/api/v1/remote/nodes", body, &node); err != nil {
				return err
			}

			format := getFormat(cmd)
			w := cmd.OutOrStdout()
			if format == "table" || format == "text" {
				fmt.Fprintf(w, "노드 '%s' 사전 등록됨\n\n", id)
				return printRemoteNodeDetail(cmd, node)
			}
			return PrintResult(w, format, node, nil, nil)
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "노드 hostname (선택)")

	return cmd
}
