package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

// newDeviceCmd 는 디바이스 관리 커맨드 그룹을 생성한다.
// 디바이스 API(GET/POST/PUT/DELETE /api/v1/devices/*)를 CLI 로 노출한다.
//
// 서브커맨드:
//
//	list      디바이스 목록 조회 (필터 지원)
//	get       단일 디바이스 상세 조회 (UUID / agent/name)
//	resolve   에이전트명으로 디바이스 검색
//	execute   디바이스 커맨드 실행
//	metadata  메타데이터 set/delete
//	history   수신 데이터 이력 조회
//
// confirmFn 은 metadata delete 의 삭제 확인 프롬프트에 사용된다(flow delete 패턴 참고).
func newDeviceCmd(client **Client, confirmFn func(string, io.Reader) bool) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "device",
		Short: "디바이스 관리",
		Long:  "디바이스의 조회, 명령 실행, 메타데이터 관리, 수신 이력 조회를 수행합니다.",
	}

	cmd.AddCommand(newDeviceListCmd(client))
	cmd.AddCommand(newDeviceGetCmd(client))
	cmd.AddCommand(newDeviceResolveCmd(client))
	cmd.AddCommand(newDeviceExecuteCmd(client))
	cmd.AddCommand(newDeviceMetadataCmd(client, confirmFn))
	cmd.AddCommand(newDeviceHistoryCmd(client))

	return cmd
}

// deviceTableHeaders 는 디바이스 목록 테이블의 헤더이다.
var deviceTableHeaders = []string{"ID", "NAME", "TYPE", "PROTOCOL", "AGENT", "ONLINE"}

// deviceRowFunc 는 디바이스 맵에서 테이블 행을 추출한다.
func deviceRowFunc(item any) []string {
	m, ok := item.(map[string]any)
	if !ok {
		return []string{"", "", "", "", "", ""}
	}
	online := "no"
	if v, ok := m["online"].(bool); ok && v {
		online = "yes"
	}
	return []string{
		fmt.Sprintf("%v", m["id"]),
		fmt.Sprintf("%v", m["name"]),
		fmt.Sprintf("%v", m["type"]),
		fmt.Sprintf("%v", m["protocol"]),
		fmt.Sprintf("%v", m["agent_name"]),
		online,
	}
}

// newDeviceListCmd 는 device list 서브커맨드를 생성한다.
// GET /api/v1/devices?protocol=&agent=&type=&online=&group=&tags= 로 목록을 조회한다.
func newDeviceListCmd(client **Client) *cobra.Command {
	var (
		protocol string
		agent    string
		typ      string
		group    string
		tags     []string
		online   bool
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "디바이스 목록 조회",
		Long: `디바이스 목록을 필터 조건과 함께 조회합니다.

예시:
  xflow device list
  xflow device list --protocol nasa --agent agent1
  xflow device list --type indoor --online
  xflow device list --tag floor1 --tag hvac --group 1f`,
		RunE: func(cmd *cobra.Command, args []string) error {
			query := url.Values{}
			if protocol != "" {
				query.Set("protocol", protocol)
			}
			if agent != "" {
				query.Set("agent", agent)
			}
			if typ != "" {
				query.Set("type", typ)
			}
			if group != "" {
				query.Set("group", group)
			}
			if len(tags) > 0 {
				query.Set("tags", strings.Join(tags, ","))
			}
			// --online 플래그가 명시적으로 설정된 경우에만 online 필터를 적용한다.
			if cmd.Flags().Changed("online") {
				query.Set("online", strconv.FormatBool(online))
			}

			path := "/api/v1/devices"
			if encoded := query.Encode(); encoded != "" {
				path += "?" + encoded
			}

			var devices []map[string]any
			if err := (*client).Get(path, &devices); err != nil {
				return err
			}

			format := getFormat(cmd)
			w := cmd.OutOrStdout()
			return PrintResult(w, format, devices, deviceTableHeaders, deviceRowFunc)
		},
	}

	cmd.Flags().StringVar(&protocol, "protocol", "", "프로토콜로 필터링 (예: nasa, modbus)")
	cmd.Flags().StringVar(&agent, "agent", "", "에이전트 이름으로 필터링")
	cmd.Flags().StringVar(&typ, "type", "", "디바이스 타입으로 필터링")
	cmd.Flags().StringVar(&group, "group", "", "그룹으로 필터링")
	cmd.Flags().StringArrayVar(&tags, "tag", nil, "태그로 필터링 (반복 지정 가능)")
	cmd.Flags().BoolVar(&online, "online", false, "온라인 디바이스만 표시")

	return cmd
}

// deviceDetailFieldOrder 는 디바이스 상세 출력의 필드 순서이다.
var deviceDetailFieldOrder = []string{
	"id", "name", "type", "protocol", "agent_name",
	"online", "source", "last_seen", "capabilities",
	"metadata", "state", "commands",
}

// deviceDetailLabelMap 는 디바이스 상세 출력의 필드 라벨 매핑이다.
var deviceDetailLabelMap = map[string]string{
	"id":           "ID",
	"name":         "Name",
	"type":         "Type",
	"protocol":     "Protocol",
	"agent_name":   "Agent",
	"online":       "Online",
	"source":       "Source",
	"last_seen":    "Last Seen",
	"capabilities": "Capabilities",
	"metadata":     "Metadata",
	"state":        "State",
	"commands":     "Commands",
}

// deviceDetailSectionKeys 는 별도 섹션으로 출력할 디바이스 상세 키 목록이다.
var deviceDetailSectionKeys = map[string]bool{
	"metadata": true,
	"state":    true,
	"commands": true,
}

// printDeviceDetail 은 단일 디바이스 상세 응답을 포맷에 맞게 출력한다.
func printDeviceDetail(cmd *cobra.Command, device map[string]any) error {
	format := getFormat(cmd)
	w := cmd.OutOrStdout()

	if format == "table" || format == "text" {
		df := NewDetailFormatter(deviceDetailFieldOrder, deviceDetailLabelMap, deviceDetailSectionKeys)
		return df.Format(device, w)
	}
	return PrintResult(w, format, device, nil, nil)
}

// newDeviceGetCmd 는 device get 서브커맨드를 생성한다.
// 단일 인자: GET /api/v1/devices/{ref} (UUID 또는 "agent/name").
// 두 인자:   GET /api/v1/devices/{agent}/{name} (2-세그먼트 경로).
//
// "agent/name" 형식의 단일 인자는 2-세그먼트 경로로 분해하여 호출한다.
func newDeviceGetCmd(client **Client) *cobra.Command {
	return &cobra.Command{
		Use:   "get <ref> | <agent> <name>",
		Short: "디바이스 상세 조회",
		Long: `UUID, "agent/name", 또는 (agent, name) 두 인자로 디바이스를 조회합니다.

예시:
  xflow device get a58ba668-5741-4b3c-9d2e-7f3c8a1b2c3d
  xflow device get lg_icp01/indoor1
  xflow device get lg_icp01 indoor1`,
		Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			path, err := buildDeviceGetPath(args)
			if err != nil {
				return err
			}

			var device map[string]any
			if err := (*client).Get(path, &device); err != nil {
				return err
			}

			return printDeviceDetail(cmd, device)
		},
	}
}

// buildDeviceGetPath 는 get 인자를 디바이스 조회 경로로 변환한다.
//
//   - 두 인자 (agent, name)            -> /api/v1/devices/{agent}/{name}
//   - 단일 인자 "agent/name"           -> /api/v1/devices/{agent}/{name}
//   - 단일 인자 (UUID 등)              -> /api/v1/devices/{ref}
//
// 경로 세그먼트는 URL 인코딩하여 특수문자를 안전하게 처리한다.
func buildDeviceGetPath(args []string) (string, error) {
	switch len(args) {
	case 2:
		agent := strings.TrimSpace(args[0])
		name := strings.TrimSpace(args[1])
		if agent == "" || name == "" {
			return "", ErrInvalidInput("agent 와 name 을 모두 지정해야 합니다")
		}
		return fmt.Sprintf("/api/v1/devices/%s/%s",
			url.PathEscape(agent), url.PathEscape(name)), nil
	case 1:
		ref := strings.TrimSpace(args[0])
		if ref == "" {
			return "", ErrInvalidInput("디바이스 reference 를 지정해야 합니다")
		}
		// "agent/name" 단일 인자는 2-세그먼트 경로로 분해한다.
		if agent, name, ok := strings.Cut(ref, "/"); ok {
			if agent == "" || name == "" {
				return "", ErrInvalidInput("잘못된 reference 형식입니다: agent/name 형식이어야 합니다")
			}
			return fmt.Sprintf("/api/v1/devices/%s/%s",
				url.PathEscape(agent), url.PathEscape(name)), nil
		}
		return "/api/v1/devices/" + url.PathEscape(ref), nil
	default:
		return "", ErrInvalidInput("디바이스 reference 를 지정해야 합니다")
	}
}

// newDeviceResolveCmd 는 device resolve 서브커맨드를 생성한다.
// GET /api/v1/devices:resolve?agent=X&name=Y 로 (agent, name) 디바이스를 검색한다.
//
// name 인자가 생략되면 GET /api/v1/devices?agent=X 로 해당 에이전트의
// 디바이스를 일괄(bulk) 조회한다.
func newDeviceResolveCmd(client **Client) *cobra.Command {
	return &cobra.Command{
		Use:   "resolve <agent> [name]",
		Short: "에이전트명으로 디바이스 검색",
		Long: `에이전트명으로 디바이스를 검색합니다.

name 을 함께 지정하면 단일 디바이스를 정확히 조회하고,
생략하면 해당 에이전트의 모든 디바이스를 일괄 조회합니다.

예시:
  xflow device resolve lg_icp01            # 에이전트의 모든 디바이스
  xflow device resolve lg_icp01 indoor1    # 단일 디바이스`,
		Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			agent := strings.TrimSpace(args[0])
			if agent == "" {
				return ErrInvalidInput("에이전트명을 지정해야 합니다")
			}

			// name 미지정: 에이전트 필터로 bulk 조회.
			if len(args) == 1 {
				query := url.Values{}
				query.Set("agent", agent)
				path := "/api/v1/devices?" + query.Encode()

				var devices []map[string]any
				if err := (*client).Get(path, &devices); err != nil {
					return err
				}

				format := getFormat(cmd)
				w := cmd.OutOrStdout()
				return PrintResult(w, format, devices, deviceTableHeaders, deviceRowFunc)
			}

			// name 지정: 단일 디바이스 resolve.
			name := strings.TrimSpace(args[1])
			if name == "" {
				return ErrInvalidInput("디바이스 이름을 지정해야 합니다")
			}
			query := url.Values{}
			query.Set("agent", agent)
			query.Set("name", name)
			path := "/api/v1/devices:resolve?" + query.Encode()

			var device map[string]any
			if err := (*client).Get(path, &device); err != nil {
				return err
			}

			return printDeviceDetail(cmd, device)
		},
	}
}

// newDeviceExecuteCmd 는 device execute 서브커맨드를 생성한다.
// POST /api/v1/devices/{id}/execute 로 디바이스 커맨드를 실행한다.
//
// 사용법:
//
//	xflow device execute <id> <command> [key=value ...]
//	xflow device execute <id> --json '{"command":"...","params":{...}}'
//
// 요청 본문은 ExecuteRequest 형식 {"command": "...", "params": {...}} 이다.
func newDeviceExecuteCmd(client **Client) *cobra.Command {
	var rawJSON string

	cmd := &cobra.Command{
		Use:   "execute <id> [command] [key=value ...]",
		Short: "디바이스 커맨드 실행",
		Long: `디바이스에 커맨드를 전송하고 결과를 반환합니다.

예시:
  xflow device execute <id> set_temperature value=24
  xflow device execute <id> power_on
  xflow device execute <id> --json '{"command":"set_mode","params":{"mode":"cool"}}'`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id := strings.TrimSpace(args[0])
			if id == "" {
				return ErrInvalidInput("디바이스 ID 를 지정해야 합니다")
			}

			body, err := buildExecuteBody(args[1:], rawJSON)
			if err != nil {
				return err
			}

			var result map[string]any
			path := fmt.Sprintf("/api/v1/devices/%s/execute", url.PathEscape(id))
			if err := (*client).Post(path, body, &result); err != nil {
				return err
			}

			format := getFormat(cmd)
			w := cmd.OutOrStdout()
			return PrintResult(w, format, result, nil, nil)
		},
	}

	cmd.Flags().StringVar(&rawJSON, "json", "", "JSON 형식의 커맨드 (전체 요청 본문)")

	return cmd
}

// buildExecuteBody 는 execute 인자를 ExecuteRequest 본문으로 변환한다.
// rawJSON 이 주어지면 그대로 파싱하고, 아니면 command + key=value params 로 구성한다.
func buildExecuteBody(args []string, rawJSON string) (map[string]any, error) {
	if rawJSON != "" {
		var body map[string]any
		if err := json.Unmarshal([]byte(rawJSON), &body); err != nil {
			return nil, ErrInvalidInput(fmt.Sprintf("JSON 파싱 실패: %v", err))
		}
		if cmdVal, _ := body["command"].(string); cmdVal == "" {
			return nil, ErrInvalidInput("JSON 본문에 command 가 필요합니다")
		}
		return body, nil
	}

	if len(args) < 1 {
		return nil, ErrInvalidInput("커맨드를 지정해주세요 (예: set_temperature)")
	}

	body := map[string]any{
		"command": args[0],
	}
	if len(args) > 1 {
		params := make(map[string]any, len(args)-1)
		for _, arg := range args[1:] {
			k, v, ok := strings.Cut(arg, "=")
			if !ok {
				return nil, ErrInvalidInput(fmt.Sprintf("잘못된 파라미터 형식: %q (key=value 형식 필요)", arg))
			}
			params[k] = parseParamValue(v)
		}
		body["params"] = params
	}
	return body, nil
}

// newDeviceMetadataCmd 는 device metadata 서브커맨드 그룹을 생성한다.
// set / delete 두 하위 커맨드를 포함한다.
func newDeviceMetadataCmd(client **Client, confirmFn func(string, io.Reader) bool) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "metadata",
		Short: "디바이스 메타데이터 관리",
		Long:  "디바이스의 사용자 정의 메타데이터(이름, 위치, 태그, 그룹)를 설정하거나 삭제합니다.",
	}

	cmd.AddCommand(newDeviceMetadataSetCmd(client))
	cmd.AddCommand(newDeviceMetadataDeleteCmd(client, confirmFn))

	return cmd
}

// newDeviceMetadataSetCmd 는 device metadata set 서브커맨드를 생성한다.
// PUT /api/v1/devices/{id}/metadata 로 메타데이터를 업데이트한다.
//
// 요청 본문은 DeviceMetadata 형식
// {"name":"","location":"","tags":[],"group":"","labels":{}} 이다.
func newDeviceMetadataSetCmd(client **Client) *cobra.Command {
	var (
		name     string
		location string
		group    string
		tags     []string
		labels   []string
	)

	cmd := &cobra.Command{
		Use:   "set <id>",
		Short: "디바이스 메타데이터 설정",
		Long: `디바이스의 메타데이터를 설정합니다.

예시:
  xflow device metadata set <id> --name "거실 에어컨" --location "1층 거실"
  xflow device metadata set <id> --tag hvac --tag floor1 --group living
  xflow device metadata set <id> --label vendor=lg --label model=icp01`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id := strings.TrimSpace(args[0])
			if id == "" {
				return ErrInvalidInput("디바이스 ID 를 지정해야 합니다")
			}

			meta, err := buildMetadataBody(cmd, name, location, group, tags, labels)
			if err != nil {
				return err
			}

			var result map[string]any
			path := fmt.Sprintf("/api/v1/devices/%s/metadata", url.PathEscape(id))
			if err := (*client).Put(path, meta, &result); err != nil {
				return err
			}

			format := getFormat(cmd)
			w := cmd.OutOrStdout()
			if format == "table" || format == "text" {
				fmt.Fprintf(w, "디바이스 '%s' 메타데이터가 설정되었습니다.\n", id)
				return nil
			}
			return PrintResult(w, format, result, nil, nil)
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "사용자 정의 디바이스 이름")
	cmd.Flags().StringVar(&location, "location", "", "디바이스 위치")
	cmd.Flags().StringVar(&group, "group", "", "디바이스 그룹")
	cmd.Flags().StringArrayVar(&tags, "tag", nil, "태그 (반복 지정 가능)")
	cmd.Flags().StringArrayVar(&labels, "label", nil, "라벨 key=value (반복 지정 가능)")

	return cmd
}

// buildMetadataBody 는 set 플래그를 DeviceMetadata 요청 본문으로 변환한다.
// 명시적으로 설정된 플래그만 본문에 포함하여 부분 업데이트 의도를 보존한다.
func buildMetadataBody(cmd *cobra.Command, name, location, group string, tags, labels []string) (map[string]any, error) {
	meta := map[string]any{}

	if cmd.Flags().Changed("name") {
		meta["name"] = name
	}
	if cmd.Flags().Changed("location") {
		meta["location"] = location
	}
	if cmd.Flags().Changed("group") {
		meta["group"] = group
	}
	if cmd.Flags().Changed("tag") {
		meta["tags"] = tags
	}
	if cmd.Flags().Changed("label") {
		labelMap := make(map[string]string, len(labels))
		for _, l := range labels {
			k, v, ok := strings.Cut(l, "=")
			if !ok {
				return nil, ErrInvalidInput(fmt.Sprintf("잘못된 라벨 형식: %q (key=value 형식 필요)", l))
			}
			labelMap[k] = v
		}
		meta["labels"] = labelMap
	}

	if len(meta) == 0 {
		return nil, ErrInvalidInput("설정할 메타데이터를 하나 이상 지정해야 합니다 (--name, --location, --tag, --group, --label)")
	}

	return meta, nil
}

// newDeviceMetadataDeleteCmd 는 device metadata delete 서브커맨드를 생성한다.
// DELETE /api/v1/devices/{id}/metadata 로 메타데이터를 삭제한다.
// --yes 플래그가 없으면 confirmFn 으로 확인을 요청한다(flow delete 패턴 참고).
func newDeviceMetadataDeleteCmd(client **Client, confirmFn func(string, io.Reader) bool) *cobra.Command {
	var yes bool

	cmd := &cobra.Command{
		Use:   "delete <id>",
		Short: "디바이스 메타데이터 삭제",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id := strings.TrimSpace(args[0])
			if id == "" {
				return ErrInvalidInput("디바이스 ID 를 지정해야 합니다")
			}

			w := cmd.OutOrStdout()

			if !yes {
				prompt := fmt.Sprintf("디바이스 '%s' 메타데이터를 삭제하시겠습니까?", id)
				if !confirmFn(prompt, os.Stdin) {
					fmt.Fprintln(w, "삭제가 취소되었습니다.")
					return nil
				}
			}

			var result map[string]any
			path := fmt.Sprintf("/api/v1/devices/%s/metadata", url.PathEscape(id))
			if err := (*client).Delete(path, &result); err != nil {
				return err
			}

			format := getFormat(cmd)
			if format == "table" || format == "text" {
				fmt.Fprintf(w, "디바이스 '%s' 메타데이터가 삭제되었습니다.\n", id)
				return nil
			}
			return PrintResult(w, format, result, nil, nil)
		},
	}

	cmd.Flags().BoolVar(&yes, "yes", false, "확인 프롬프트 건너뛰기")

	return cmd
}

// deviceHistoryEntryHeaders 는 디바이스 이력 항목 테이블의 헤더이다.
var deviceHistoryEntryHeaders = []string{"TIMESTAMP", "ONLINE", "LAST_SEEN"}

// deviceHistoryRowFunc 는 이력 스냅샷 맵에서 테이블 행을 추출한다.
func deviceHistoryRowFunc(item any) []string {
	m, ok := item.(map[string]any)
	if !ok {
		return []string{"", "", ""}
	}
	online := "no"
	if v, ok := m["online"].(bool); ok && v {
		online = "yes"
	}
	return []string{
		formatEpochValue(m["timestamp"]),
		online,
		formatEpochValue(m["last_seen"]),
	}
}

// formatEpochValue 는 epoch 타임스탬프 값을 정수 문자열로 포맷한다.
// JSON 디코딩 시 숫자는 float64 로 들어와 %v 가 과학표기(1.7e+12)로
// 출력되므로, int64 정수 문자열로 변환한다(epoch ms 는 2^53 미만이라 안전).
func formatEpochValue(v any) string {
	switch n := v.(type) {
	case float64:
		return strconv.FormatInt(int64(n), 10)
	case int64:
		return strconv.FormatInt(n, 10)
	case json.Number:
		return n.String()
	case nil:
		return ""
	default:
		return fmt.Sprintf("%v", v)
	}
}

// newDeviceHistoryCmd 는 device history 서브커맨드를 생성한다.
// GET /api/v1/devices/{id}/history?limit=N 로 수신 데이터 이력을 조회한다.
//
// 응답은 HistoryResponse 형식 {"device_id":"","count":N,"entries":[...]} 이다.
func newDeviceHistoryCmd(client **Client) *cobra.Command {
	var limit int

	cmd := &cobra.Command{
		Use:   "history <id>",
		Short: "디바이스 수신 데이터 이력 조회",
		Long: `디바이스의 수신 데이터 이력(주기 스냅샷)을 최신순으로 조회합니다.

예시:
  xflow device history <id>
  xflow device history <id> --limit 50`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id := strings.TrimSpace(args[0])
			if id == "" {
				return ErrInvalidInput("디바이스 ID 를 지정해야 합니다")
			}

			path := fmt.Sprintf("/api/v1/devices/%s/history", url.PathEscape(id))
			if cmd.Flags().Changed("limit") && limit > 0 {
				query := url.Values{}
				query.Set("limit", strconv.Itoa(limit))
				path += "?" + query.Encode()
			}

			var resp map[string]any
			if err := (*client).Get(path, &resp); err != nil {
				return err
			}

			format := getFormat(cmd)
			w := cmd.OutOrStdout()

			if format == "table" || format == "text" {
				entries := extractHistoryEntries(resp)
				fmt.Fprintf(w, "Device: %v\n", resp["device_id"])
				fmt.Fprintf(w, "Count:  %v\n\n", resp["count"])
				return PrintResult(w, "table", entries, deviceHistoryEntryHeaders, deviceHistoryRowFunc)
			}
			return PrintResult(w, format, resp, nil, nil)
		},
	}

	cmd.Flags().IntVar(&limit, "limit", 0, "조회할 최대 이력 개수")

	return cmd
}

// extractHistoryEntries 는 HistoryResponse 의 entries 배열을 맵 슬라이스로 추출한다.
func extractHistoryEntries(resp map[string]any) []map[string]any {
	raw, ok := resp["entries"].([]any)
	if !ok {
		return nil
	}
	result := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		if m, ok := item.(map[string]any); ok {
			result = append(result, m)
		}
	}
	return result
}
