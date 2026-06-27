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
//	node   원격 노드 조회/승인/거부/폐기/사전등록
//	group  원격 노드 그룹 관리(배정/해제/이름변경/삭제/일괄 업데이트·명령)
//	token  등록 토큰 발급/목록/폐기
//
// confirmFn 은 revoke(폐기)/group delete/group update 같은 파괴적 작업의 확인
// 프롬프트에 사용된다(device metadata delete 패턴 참고).
func newRemoteCmd(client **Client, confirmFn func(string, io.Reader) bool) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remote",
		Short: "원격 노드 관리",
		Long:  "원격(엣지) 노드의 조회, 승인, 거부, 폐기, 사전 등록, 그룹 관리, 등록 토큰 관리를 수행합니다.",
	}

	cmd.AddCommand(newRemoteNodeCmd(client, confirmFn))
	cmd.AddCommand(newRemoteGroupCmd(client, confirmFn))
	cmd.AddCommand(newRemoteTokenCmd(client, confirmFn))

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

// remoteNodeDetailFieldOrder 는 노드 상세(NodeDetailDTO) 출력의 필드 순서이다.
// pre-register 응답(ManagedNodeDTO)도 같은 포맷터를 재사용하며, 누락 필드는 건너뛴다.
var remoteNodeDetailFieldOrder = []string{
	"instance_id", "hostname", "version", "status",
	"online", "group_name", "outdated",
	"os", "arch", "started_at", "uptime",
	"display_width", "display_height",
	"display_override_width", "display_override_height",
	"display_reported_width", "display_reported_height",
	"last_seen", "summary",
}

// remoteNodeDetailLabelMap 는 노드 상세 출력의 필드 라벨 매핑이다.
var remoteNodeDetailLabelMap = map[string]string{
	"instance_id":             "Instance ID",
	"hostname":                "Hostname",
	"version":                 "Version",
	"status":                  "Status",
	"online":                  "Online",
	"group_name":              "Group",
	"outdated":                "Outdated",
	"os":                      "OS",
	"arch":                    "Arch",
	"started_at":              "Started At",
	"uptime":                  "Uptime (ms)",
	"display_width":           "Display W",
	"display_height":          "Display H",
	"display_override_width":  "Override W",
	"display_override_height": "Override H",
	"display_reported_width":  "Reported W",
	"display_reported_height": "Reported H",
	"last_seen":               "Last Seen",
	"summary":                 "Summary",
}

// remoteNodeDetailSectionKeys 는 별도 섹션으로 출력할 노드 상세 키 목록이다.
// summary(중첩 객체)는 device.go 의 metadata/state/commands 섹션 처리와 동일하게
// 별도 섹션으로 렌더링한다(deviceDetailSectionKeys 패턴 참고).
var remoteNodeDetailSectionKeys = map[string]bool{
	"summary": true,
}

// printRemoteNodeDetail 은 단일 노드 상세를 포맷에 맞게 출력한다.
//
// table/text 는 epoch-ms 필드(started_at/last_seen)와 uptime 을 정수 문자열로
// 변환한 표시용 사본을 사용한다(JSON 디코딩 float64 의 과학표기 방지). 원본 node 는
// json/yaml passthrough 를 위해 보존한다.
func printRemoteNodeDetail(cmd *cobra.Command, node map[string]any) error {
	format := getFormat(cmd)
	w := cmd.OutOrStdout()

	if format == "table" || format == "text" {
		df := NewDetailFormatter(remoteNodeDetailFieldOrder, remoteNodeDetailLabelMap, remoteNodeDetailSectionKeys)
		return df.Format(remoteNodeDetailDisplay(node), w)
	}
	return PrintResult(w, format, node, nil, nil)
}

// remoteNodeDetailDisplay 는 table/text 출력을 위한 표시용 사본을 만든다.
// epoch-ms 필드(started_at/last_seen)와 uptime 을 formatEpochValue 로 정수 문자열화하고,
// 존재할 때만 변환한다(pre-register 응답처럼 일부 필드가 없을 수 있음). 원본은 변경하지 않는다.
func remoteNodeDetailDisplay(node map[string]any) map[string]any {
	out := make(map[string]any, len(node))
	for k, v := range node {
		out[k] = v
	}
	// epoch-ms 필드: float64 과학표기 방지를 위해 정수 문자열로 변환(nil-safe).
	for _, k := range []string{"started_at", "last_seen", "uptime"} {
		if v, ok := out[k]; ok {
			// uptime 은 started_at==0 시 JSON null → formatEpochValue 가 ""(미표시)로 처리.
			out[k] = formatEpochValue(v)
		}
	}
	return out
}

// newRemoteNodeGetCmd 는 remote node get 서브커맨드를 생성한다.
//
// GET /api/v1/remote/nodes/{instance_id} (NodeDetail 핸들러)로 단일 노드 상세를
// 조회한다. 노드가 없으면 서버가 404(not-found)를 반환하며, 클라이언트의
// MapAPIError 가 이를 에러 메시지로 surface 한다(클라이언트 측 필터링 없음).
func newRemoteNodeGetCmd(client **Client) *cobra.Command {
	return &cobra.Command{
		Use:   "get <instance_id>",
		Short: "원격 노드 상세 조회",
		Long: `원격 노드의 상세 정보(시스템 정보, uptime, 운영 요약 포함)를 조회합니다.

예시:
  xflow remote node get node-01`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id := strings.TrimSpace(args[0])
			if id == "" {
				return ErrInvalidInput("instance_id 를 지정해야 합니다")
			}

			var node map[string]any
			path := fmt.Sprintf("/api/v1/remote/nodes/%s", url.PathEscape(id))
			if err := (*client).Get(path, &node); err != nil {
				return err
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
