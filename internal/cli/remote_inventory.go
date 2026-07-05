package cli

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

// 이 파일은 SPEC-CLI-004 P3d 의 원격 명령 디스패치 / 감사 로그 / 인벤토리 조회
// 커맨드를 구현한다(P3a/P3b/P3c 패턴 답습). 세 그룹 생성자를 한 파일에 모은다:
//
//	command    POST /api/v1/remote/nodes/{id}/command   원격 노드 명령 디스패치
//	audit      GET  /api/v1/remote/audit                관리 감사 로그 조회
//	inventory  GET  /api/v1/remote/{flows|agents|...}   인벤토리(미러/라이브) 조회
//
// 셋 다 파괴적 mutation 이 아니므로 confirmFn 을 받지 않는다.

// =============================================================================
// remote command <instance_id>
// =============================================================================

// newRemoteCommandCmd 는 remote command 커맨드를 생성한다(SPEC-CLI-004 P3d).
// POST /api/v1/remote/nodes/{instance_id}/command 로 원격 노드에 임의 명령을
// 디스패치한다.
//
// 요청 본문은 {"domain":..,"action":..,"args":<rawjson>} 이며 domain/action 은 필수이다
// (remote group command 패턴과 동일). --args 는 JSON 문자열로 받아 검증 후 그대로
// 전달한다(미지정 시 args 필드 생략 — 서버 측 args,omitempty 와 일치).
// 응답은 {instance_id, domain, action, result} 이며 result 는 노드가 반환한 임의 JSON 이다.
func newRemoteCommandCmd(client **Client) *cobra.Command {
	var (
		domain  string
		action  string
		rawArgs string
	)

	cmd := &cobra.Command{
		Use:   "command <instance_id>",
		Short: "원격 노드 명령 디스패치",
		Long: `승인·온라인 원격 노드에 임의 명령을 디스패치합니다.

예시:
  xflow remote command node-01 --domain system --action ping
  xflow remote command node-01 --domain flow --action restart --args '{"flow_id":"f1"}'`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id := strings.TrimSpace(args[0])
			if id == "" {
				return ErrInvalidInput("instance_id 를 지정해야 합니다")
			}
			if strings.TrimSpace(domain) == "" {
				return ErrInvalidInput("--domain 은 필수입니다")
			}
			if strings.TrimSpace(action) == "" {
				return ErrInvalidInput("--action 은 필수입니다")
			}

			body := map[string]any{
				"domain": domain,
				"action": action,
			}
			// --args 는 JSON 으로 검증 후 그대로 전달한다(미지정 시 필드 생략).
			if rawArgs != "" {
				var parsed json.RawMessage
				if err := json.Unmarshal([]byte(rawArgs), &parsed); err != nil {
					return ErrInvalidInput(fmt.Sprintf("--args JSON 파싱 실패: %v", err))
				}
				body["args"] = parsed
			}

			var result map[string]any
			path := fmt.Sprintf("/api/v1/remote/nodes/%s/command", url.PathEscape(id))
			if err := (*client).Post(path, body, &result); err != nil {
				return err
			}

			return printRemoteCommandResult(cmd, id, result)
		},
	}

	cmd.Flags().StringVar(&domain, "domain", "", "명령 도메인 (필수, 예: system, flow)")
	cmd.Flags().StringVar(&action, "action", "", "명령 액션 (필수, 예: ping, restart)")
	cmd.Flags().StringVar(&rawArgs, "args", "", "명령 인자 (JSON 문자열, 선택)")

	return cmd
}

// printRemoteCommandResult 는 단일 노드 명령 디스패치 결과를 출력한다.
// table/text 포맷에서는 노드/도메인/액션 헤더 + result 본문을, json/yaml 에서는 원본
// 응답({instance_id,domain,action,result})을 그대로 통과시킨다(printGroupDispatchResult
// 패턴 참고).
func printRemoteCommandResult(cmd *cobra.Command, id string, result map[string]any) error {
	format := getFormat(cmd)
	w := cmd.OutOrStdout()

	if format == "table" || format == "text" {
		fmt.Fprintf(w, "Node:   %s\n", id)
		fmt.Fprintf(w, "Domain: %v\n", result["domain"])
		fmt.Fprintf(w, "Action: %v\n\n", result["action"])
		return PrintResult(w, "text", result["result"], nil, nil)
	}
	return PrintResult(w, format, result, nil, nil)
}

// =============================================================================
// remote audit
// =============================================================================

// remoteAuditTableHeaders 는 감사 로그 목록 테이블의 헤더이다.
var remoteAuditTableHeaders = []string{"ID", "NODE", "ACTOR", "ACTION", "RESULT", "TIMESTAMP"}

// remoteAuditRowFunc 는 감사 항목 맵에서 테이블 행을 추출한다.
// timestamp(epoch ms)는 formatEpochValue 로 정수 문자열화한다(과학표기 방지).
func remoteAuditRowFunc(item any) []string {
	m, ok := item.(map[string]any)
	if !ok {
		return []string{"", "", "", "", "", ""}
	}
	return []string{
		formatEpochValue(m["id"]),
		fmt.Sprintf("%v", m["instance_id"]),
		fmt.Sprintf("%v", m["actor"]),
		fmt.Sprintf("%v", m["action"]),
		fmt.Sprintf("%v", m["result"]),
		formatEpochValue(m["timestamp"]),
	}
}

// newRemoteAuditCmd 는 remote audit 커맨드를 생성한다(SPEC-CLI-004 P3d).
// GET /api/v1/remote/audit 로 원격 관리 감사 로그를 조회한다.
//
// 선택적 쿼리 파라미터: instance_id(--node), limit(--limit, Changed & >0),
// offset(--offset, Changed & >=0). 명시적으로 설정된 플래그만 쿼리에 포함한다
// (device.go list 의 url.Values 패턴 참고). 응답은 감사 항목 배열이며
// []map[string]any 로 디코딩한다. json/yaml 포맷은 그대로 통과시킨다.
func newRemoteAuditCmd(client **Client) *cobra.Command {
	var (
		node   string
		limit  int
		offset int
	)

	cmd := &cobra.Command{
		Use:   "audit",
		Short: "원격 관리 감사 로그 조회",
		Long: `원격 관리 감사 로그(승인/거부/폐기/명령 등)를 조회합니다.

예시:
  xflow remote audit
  xflow remote audit --node node-01 --limit 50 --offset 0`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			query := url.Values{}
			if node != "" {
				query.Set("instance_id", node)
			}
			// limit 은 명시적으로 설정되고 양수일 때만 포함한다(device history 패턴).
			if cmd.Flags().Changed("limit") && limit > 0 {
				query.Set("limit", strconv.Itoa(limit))
			}
			// offset 은 명시적으로 설정되고 음이 아닐 때만 포함한다(0 허용).
			if cmd.Flags().Changed("offset") && offset >= 0 {
				query.Set("offset", strconv.Itoa(offset))
			}

			path := "/api/v1/remote/audit"
			if encoded := query.Encode(); encoded != "" {
				path += "?" + encoded
			}

			var entries []map[string]any
			if err := (*client).Get(path, &entries); err != nil {
				return err
			}

			format := getFormat(cmd)
			w := cmd.OutOrStdout()
			return PrintResult(w, format, entries, remoteAuditTableHeaders, remoteAuditRowFunc)
		},
	}

	cmd.Flags().StringVar(&node, "node", "", "instance_id 로 필터링 (선택)")
	cmd.Flags().IntVar(&limit, "limit", 0, "조회할 최대 항목 개수 (선택)")
	cmd.Flags().IntVar(&offset, "offset", 0, "건너뛸 항목 개수 (선택)")

	return cmd
}

// =============================================================================
// remote inventory {flows|agents|devices}
// =============================================================================

// remoteInventoryTableHeaders 는 미러 인벤토리 목록 테이블의 헤더이다.
var remoteInventoryTableHeaders = []string{"ID", "SOURCE_NODE", "NAME", "KIND", "STATUS", "ONLINE", "UPDATED_AT"}

// remoteInventoryRowFunc 는 미러 자원(MirroredResourceDTO) 맵에서 테이블 행을 추출한다.
// online bool 은 yes/no 로, updated_at(epoch ms)은 formatEpochValue 로 렌더링한다.
func remoteInventoryRowFunc(item any) []string {
	m, ok := item.(map[string]any)
	if !ok {
		return []string{"", "", "", "", "", "", ""}
	}
	return []string{
		fmt.Sprintf("%v", m["id"]),
		fmt.Sprintf("%v", m["source_instance_id"]),
		fmt.Sprintf("%v", m["name"]),
		fmt.Sprintf("%v", m["kind"]),
		fmt.Sprintf("%v", m["status"]),
		boolToYesNo(m["online"]),
		formatEpochValue(m["updated_at"]),
	}
}

// newRemoteInventoryCmd 는 remote inventory 서브커맨드 그룹을 생성한다(SPEC-CLI-004 P3d).
// 원격 노드의 인벤토리(flows/agents/devices)를 미러(요약) 또는 라이브(노드 직접)로
// 조회한다.
//
// 하위 커맨드(자원별):
//
//	flows    [instance_id] [--live]
//	agents   [instance_id] [--live]
//	devices  [instance_id] [--live]
//
// 라우팅 매트릭스(자원=flows|agents|devices):
//
//	args=0, --live=off  → GET /api/v1/remote/{resource}                       (통합 미러)
//	args=1, --live=off  → GET /api/v1/remote/nodes/{instance_id}/{resource}   (노드 미러)
//	args=1, --live=on   → GET /api/v1/remote/nodes/{instance_id}/{resource}/live (노드 라이브)
//	args=0, --live=on   → ErrInvalidInput (인스턴스 ID 필요)
func newRemoteInventoryCmd(client **Client) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "inventory",
		Short: "원격 노드 인벤토리(flows/agents/devices) 조회",
		Long:  "원격 노드의 flows/agents/devices 인벤토리를 통합 미러, 노드 미러, 노드 라이브 형태로 조회합니다.",
	}

	cmd.AddCommand(newRemoteInventoryResourceCmd(client, "flows"))
	cmd.AddCommand(newRemoteInventoryResourceCmd(client, "agents"))
	cmd.AddCommand(newRemoteInventoryResourceCmd(client, "devices"))

	return cmd
}

// newRemoteInventoryResourceCmd 는 자원 이름(flows/agents/devices)으로 매개변수화된
// 인벤토리 서브커맨드를 생성한다(중복 제거를 위한 공유 헬퍼).
func newRemoteInventoryResourceCmd(client **Client, resource string) *cobra.Command {
	var live bool

	cmd := &cobra.Command{
		Use:   resource + " [instance_id]",
		Short: fmt.Sprintf("원격 %s 인벤토리 조회", resource),
		Long: fmt.Sprintf(`원격 %s 인벤토리를 조회합니다.

인스턴스 ID 없이 호출하면 전 노드 통합 미러를, 인스턴스 ID 와 함께 호출하면
해당 노드의 미러를 조회합니다. --live 는 인스턴스 ID 와 함께 사용하여 노드의
라이브 목록(runtime 필드 포함)을 직접 프록시합니다(미러 요약 아님).

예시:
  xflow remote inventory %s
  xflow remote inventory %s node-01
  xflow remote inventory %s node-01 --live`, resource, resource, resource, resource),
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id := ""
			if len(args) == 1 {
				id = strings.TrimSpace(args[0])
			}

			// --live 는 반드시 인스턴스 ID 를 동반해야 한다(통합 라이브 엔드포인트 없음).
			if live && id == "" {
				return ErrInvalidInput("--live 는 인스턴스 ID 가 필요합니다")
			}

			if live {
				// 노드 라이브: 자원별 형태가 상이하므로 generic 디코딩 + passthrough.
				path := fmt.Sprintf("/api/v1/remote/nodes/%s/%s/live", url.PathEscape(id), resource)
				return fetchAndPrintLiveInventory(cmd, *client, path)
			}

			// 미러(통합 또는 노드): MirroredResourceDTO 배열.
			path := "/api/v1/remote/" + resource
			if id != "" {
				path = fmt.Sprintf("/api/v1/remote/nodes/%s/%s", url.PathEscape(id), resource)
			}
			return fetchAndPrintMirrorInventory(cmd, *client, path)
		},
	}

	cmd.Flags().BoolVar(&live, "live", false, "노드의 라이브 목록을 직접 조회 (인스턴스 ID 필요)")

	return cmd
}

// fetchAndPrintMirrorInventory 는 미러 인벤토리(MirroredResourceDTO 배열)를 조회하고
// 출력한다. table/text 는 고정 헤더 테이블로, json/yaml 은 그대로 통과시킨다.
func fetchAndPrintMirrorInventory(cmd *cobra.Command, client *Client, path string) error {
	var items []map[string]any
	if err := client.Get(path, &items); err != nil {
		return err
	}

	format := getFormat(cmd)
	w := cmd.OutOrStdout()
	return PrintResult(w, format, items, remoteInventoryTableHeaders, remoteInventoryRowFunc)
}

// fetchAndPrintLiveInventory 는 노드 라이브 인벤토리를 조회하고 출력한다.
//
// 라이브 데이터는 노드가 반환한 원본 배열로 자원(flow/agent/device)마다 형태가 다르다.
// 따라서 generic 한 []any 로 디코딩해 출력 형식별로 처리한다:
//   - json/yaml: 원본 배열을 그대로 통과시킨다(전체 필드 보존, 가장 충실한 표현).
//   - table/text: 동적 형태이므로 고정 테이블을 만들 수 없다. text 포매터로 자유 형식
//     렌더링하며, 예기치 못한 형태(빈 배열/스칼라 등)에도 크래시 없이 동작한다.
func fetchAndPrintLiveInventory(cmd *cobra.Command, client *Client, path string) error {
	var data []any
	if err := client.Get(path, &data); err != nil {
		return err
	}

	format := getFormat(cmd)
	w := cmd.OutOrStdout()

	if format == "table" {
		// 라이브 데이터는 동적 형태라 고정 테이블 헤더를 만들 수 없다. text 로 폴백하며
		// --format json 이 가장 충실한 표현임을 안내한다.
		fmt.Fprintln(w, "라이브 데이터는 동적 형태입니다. 전체 필드는 --format json 으로 확인하세요.")
		fmt.Fprintln(w)
		return PrintResult(w, "text", data, nil, nil)
	}
	return PrintResult(w, format, data, nil, nil)
}
