package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

// newStoreCmd 는 에이전트 KV 저장소 관리 커맨드 그룹을 생성한다.
// 5개의 서브커맨드(keys, tags, query, meta, reset)를 등록한다.
//
// 모든 경로는 /api/v1/store/{agent_name}/... 형식이며, 서버는 agent_name 을
// 이름 그대로 매칭하므로(findAgentByName) 별도의 ID 변환이 필요 없다.
//
// reset(삭제) 서브커맨드는 confirmFn 으로 사용자 확인을 받는다.
func newStoreCmd(client **Client, confirmFn func(string, io.Reader) bool) *cobra.Command {
	storeCmd := &cobra.Command{
		Use:   "store",
		Short: "에이전트 저장소 관리 명령어",
		Long:  "에이전트 KV 저장소의 키 조회, 태그 조회, 이력 쿼리, 메타데이터 설정 및 리셋을 수행합니다.",
	}

	storeCmd.AddCommand(newStoreKeysCmd(client))
	storeCmd.AddCommand(newStoreTagsCmd(client))
	storeCmd.AddCommand(newStoreQueryCmd(client))
	storeCmd.AddCommand(newStoreMetaCmd(client))
	storeCmd.AddCommand(newStoreResetCmd(client, confirmFn))

	return storeCmd
}

// storePath 는 주어진 agent_name 에 대한 store 베이스 경로를 만든다.
// agent_name 은 콜론/슬래시 등 특수문자를 안전히 다루기 위해 PathEscape 한다.
func storePath(agentName, suffix string) string {
	return "/api/v1/store/" + url.PathEscape(agentName) + suffix
}

// --- store keys ---

// storeKeysTableHeaders 는 키 목록 테이블의 헤더이다.
var storeKeysTableHeaders = []string{"KEY", "REGISTRATION", "DATA_TYPE", "METRIC_TYPE", "TAGS"}

// storeKeyRowFunc 는 키 응답 맵에서 테이블 행을 추출한다.
func storeKeyRowFunc(item any) []string {
	m, ok := item.(map[string]any)
	if !ok {
		return []string{"", "", "", "", ""}
	}
	return []string{
		fmt.Sprintf("%v", m["key"]),
		fmt.Sprintf("%v", m["registration"]),
		fmt.Sprintf("%v", m["data_type"]),
		fmt.Sprintf("%v", m["metric_type"]),
		formatTagsMap(m["tags"]),
	}
}

// formatTagsMap 은 tags 맵(JSON 디코딩 결과 map[string]any)을 "k=v,k=v" 형태의
// 결정적(키 정렬) 문자열로 변환한다. 빈 맵이거나 맵이 아니면 "-" 를 반환한다.
func formatTagsMap(v any) string {
	m, ok := v.(map[string]any)
	if !ok || len(m) == 0 {
		return "-"
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%v", k, m[k]))
	}
	return strings.Join(parts, ",")
}

// newStoreKeysCmd 는 store keys <agent_name> 서브커맨드를 생성한다.
// GET /api/v1/store/{agent_name}/keys 로 키 목록을 조회한다.
//
// 서버는 페이지네이션을 지원하지 않고 객체 배열({count, keys: [...]})을 반환하므로
// --filter(키 부분 일치), --page/--size(클라이언트 측 슬라이싱)는 CLI 에서 적용한다.
// 이는 flow/agent list 의 --name 클라이언트 필터링과 동일한 패턴이다.
func newStoreKeysCmd(client **Client) *cobra.Command {
	var (
		filter string
		page   int
		size   int
	)

	cmd := &cobra.Command{
		Use:   "keys <agent_name>",
		Short: "저장소 키 목록 조회",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			agentName := args[0]

			// 서버 응답 형식: {count, keys: [{key, registration, data_type, metric_type, tags}, ...]}
			var resp struct {
				Count int              `json:"count"`
				Keys  []map[string]any `json:"keys"`
			}
			if err := (*client).Get(storePath(agentName, "/keys"), &resp); err != nil {
				return err
			}

			keys := resp.Keys

			// --filter: 키 부분 일치 (클라이언트 측).
			if filter != "" {
				filtered := make([]map[string]any, 0, len(keys))
				for _, k := range keys {
					name, _ := k["key"].(string)
					if strings.Contains(name, filter) {
						filtered = append(filtered, k)
					}
				}
				keys = filtered
			}

			// --page/--size: 클라이언트 측 슬라이싱 (1-기반 페이지).
			keys = paginateKeys(keys, page, size)

			format := getFormat(cmd)
			w := cmd.OutOrStdout()
			return PrintResult(w, format, keys, storeKeysTableHeaders, storeKeyRowFunc)
		},
	}

	cmd.Flags().StringVar(&filter, "filter", "", "키 이름으로 필터링 (부분 일치)")
	cmd.Flags().IntVar(&page, "page", 0, "페이지 번호 (1-기반, 0=전체)")
	cmd.Flags().IntVar(&size, "size", 0, "페이지 크기 (0=전체)")

	return cmd
}

// paginateKeys 는 1-기반 page 와 size 로 슬라이스를 잘라낸다.
// page<=0 또는 size<=0 이면 슬라이싱 없이 원본을 반환한다 (전체 표시).
// 범위를 벗어나면 빈 슬라이스를 반환한다.
func paginateKeys(keys []map[string]any, page, size int) []map[string]any {
	if page <= 0 || size <= 0 {
		return keys
	}
	start := (page - 1) * size
	if start >= len(keys) {
		return []map[string]any{}
	}
	end := start + size
	if end > len(keys) {
		end = len(keys)
	}
	return keys[start:end]
}

// --- store tags ---

// newStoreTagsCmd 는 store tags <agent_name> 서브커맨드를 생성한다.
// GET /api/v1/store/{agent_name}/tags 로 고유 태그 목록을 조회한다.
// 응답: {"pairs": [{"key": "room", "values": ["1","2"]}, ...]}
func newStoreTagsCmd(client **Client) *cobra.Command {
	return &cobra.Command{
		Use:   "tags <agent_name>",
		Short: "저장소 고유 태그 목록 조회",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			agentName := args[0]

			var resp struct {
				Pairs []map[string]any `json:"pairs"`
			}
			if err := (*client).Get(storePath(agentName, "/tags"), &resp); err != nil {
				return err
			}

			format := getFormat(cmd)
			w := cmd.OutOrStdout()

			// table/text 는 가독성 위해 "key: v1, v2" 형식으로 표시한다.
			if format == "table" || format == "text" {
				if len(resp.Pairs) == 0 {
					fmt.Fprintln(w, "태그가 없습니다.")
					return nil
				}
				for _, p := range resp.Pairs {
					key, _ := p["key"].(string)
					fmt.Fprintf(w, "%s: %s\n", key, joinTagValues(p["values"]))
				}
				return nil
			}
			return PrintResult(w, format, resp.Pairs, nil, nil)
		},
	}
}

// joinTagValues 는 values 배열(JSON 디코딩 결과 []any)을 "v1, v2" 로 합친다.
func joinTagValues(v any) string {
	vals, ok := v.([]any)
	if !ok || len(vals) == 0 {
		return "-"
	}
	parts := make([]string, 0, len(vals))
	for _, x := range vals {
		parts = append(parts, fmt.Sprintf("%v", x))
	}
	return strings.Join(parts, ", ")
}

// --- store query ---

// newStoreQueryCmd 는 store query <agent_name> 서브커맨드를 생성한다.
// POST /api/v1/store/{agent_name}/query 로 키 이력을 조회한다.
//
// 요청 바디는 두 가지 방식으로 구성할 수 있다:
//   - 플래그 매핑: --key/--mode/--namespace/--count/--duration-sec/--start-ms/
//     --end-ms/--metric-type/--tag 를 storeQueryRequest 필드로 매핑.
//   - --json: 원시 JSON 바디 문자열을 그대로 전송(고급 사용자용). 지정 시 플래그는 무시된다.
//
// mode 별 필수 필드(서버 검증):
//   - latest      : (없음)
//   - last_n      : --count > 0
//   - duration    : --duration-sec > 0
//   - time_range  : --start-ms > 0, --end-ms > 0 (end >= start)
//   - since_n     : --start-ms > 0, --count > 0
func newStoreQueryCmd(client **Client) *cobra.Command {
	var (
		key         string
		mode        string
		namespace   string
		count       int
		durationSec int
		startMs     int64
		endMs       int64
		metricType  string
		tagPairs    []string
		rawJSON     string
	)

	cmd := &cobra.Command{
		Use:   "query <agent_name>",
		Short: "저장소 키 이력 쿼리",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			agentName := args[0]

			body, err := buildStoreQueryBody(rawJSON, key, mode, namespace, count, durationSec, startMs, endMs, metricType, tagPairs)
			if err != nil {
				return err
			}

			var result map[string]any
			if err := (*client).Post(storePath(agentName, "/query"), body, &result); err != nil {
				return err
			}

			format := getFormat(cmd)
			w := cmd.OutOrStdout()
			return PrintResult(w, format, result, nil, nil)
		},
	}

	cmd.Flags().StringVar(&key, "key", "", "조회할 키 (필수, --json 사용 시 제외)")
	cmd.Flags().StringVar(&mode, "mode", "", "쿼리 모드 (latest, last_n, duration, time_range, since_n)")
	cmd.Flags().StringVar(&namespace, "namespace", "", "네임스페이스 (생략 시 default)")
	cmd.Flags().IntVar(&count, "count", 0, "last_n/since_n 모드의 항목 개수")
	cmd.Flags().IntVar(&durationSec, "duration-sec", 0, "duration 모드의 기간(초)")
	cmd.Flags().Int64Var(&startMs, "start-ms", 0, "time_range/since_n 모드의 시작 시각(epoch ms)")
	cmd.Flags().Int64Var(&endMs, "end-ms", 0, "time_range 모드의 종료 시각(epoch ms)")
	cmd.Flags().StringVar(&metricType, "metric-type", "", "시리즈 필터: metric_type")
	cmd.Flags().StringArrayVar(&tagPairs, "tag", nil, "시리즈 필터: 태그 k=v (반복 가능)")
	cmd.Flags().StringVar(&rawJSON, "json", "", "원시 JSON 요청 바디 (지정 시 다른 플래그 무시)")

	return cmd
}

// buildStoreQueryBody 는 query 서브커맨드의 요청 바디를 구성한다.
//
// rawJSON 이 비어있지 않으면 그것을 파싱하여 그대로 바디로 사용한다(고급 모드).
// 그렇지 않으면 플래그를 storeQueryRequest JSON 필드로 매핑한다.
//
// 플래그 모드에서 key 는 필수이다(서버도 동일하게 검증하지만, 빈 POST 전송을 막아
// 명확한 에러 메시지를 조기에 제공한다).
func buildStoreQueryBody(
	rawJSON, key, mode, namespace string,
	count, durationSec int,
	startMs, endMs int64,
	metricType string,
	tagPairs []string,
) (map[string]any, error) {
	// --json: 원시 바디 우선.
	if rawJSON != "" {
		var body map[string]any
		if err := json.Unmarshal([]byte(rawJSON), &body); err != nil {
			return nil, fmt.Errorf("--json 파싱 실패: %w", err)
		}
		return body, nil
	}

	if key == "" {
		return nil, fmt.Errorf("--key 를 지정해야 합니다 (또는 --json 으로 원시 바디 전송)")
	}

	body := map[string]any{"key": key}
	if mode != "" {
		body["mode"] = mode
	}
	if namespace != "" {
		body["namespace"] = namespace
	}
	if count > 0 {
		body["count"] = count
	}
	if durationSec > 0 {
		body["duration_sec"] = durationSec
	}
	if startMs > 0 {
		body["start_ms"] = startMs
	}
	if endMs > 0 {
		body["end_ms"] = endMs
	}
	if metricType != "" {
		body["metric_type"] = metricType
	}

	tags, err := parseTagKV(tagPairs)
	if err != nil {
		return nil, err
	}
	if len(tags) > 0 {
		body["tags"] = tags
	}

	return body, nil
}

// --- store meta ---

// newStoreMetaCmd 는 store meta <agent_name> <key> 서브커맨드를 생성한다.
// PUT /api/v1/store/{agent_name}/keys/{key}/meta 로 키 메타데이터를 설정한다.
//
// 요청 바디: {"metric_type": "...", "tags": {"k": "v", ...}}
// tags 는 전체 교체(replace) 시맨틱이다(부분 갱신이 아님).
func newStoreMetaCmd(client **Client) *cobra.Command {
	var (
		metricType string
		tagPairs   []string
	)

	cmd := &cobra.Command{
		Use:   "meta <agent_name> <key>",
		Short: "저장소 키 메타데이터(metric_type/tags) 설정",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			agentName := args[0]
			key := args[1]

			tags, err := parseTagKV(tagPairs)
			if err != nil {
				return err
			}

			// 서버 바디 형식: {"metric_type": "...", "tags": {...}}
			// tags 는 항상 non-nil 맵으로 전송하여 replace 시맨틱을 명확히 한다.
			if tags == nil {
				tags = map[string]string{}
			}
			body := map[string]any{
				"metric_type": metricType,
				"tags":        tags,
			}

			// 키는 콜론 등 특수문자를 포함할 수 있어 path 세그먼트로 escape 한다.
			path := storePath(agentName, "/keys/"+url.PathEscape(key)+"/meta")

			var result map[string]any
			if err := (*client).Put(path, body, &result); err != nil {
				return err
			}

			format := getFormat(cmd)
			w := cmd.OutOrStdout()
			return PrintResult(w, format, result, nil, nil)
		},
	}

	cmd.Flags().StringVar(&metricType, "metric-type", "", "설정할 metric_type (생략 시 unknown)")
	cmd.Flags().StringArrayVar(&tagPairs, "tag", nil, "설정할 태그 k=v (반복 가능, 전체 교체)")

	return cmd
}

// --- store reset ---

// newStoreResetCmd 는 store reset <agent_name> [<key>] [--all] 서브커맨드를 생성한다.
//
// 분기:
//   - <key> 지정 (단일 키)  → DELETE /api/v1/store/{agent_name}/keys/{key}
//   - --all (벌크)          → DELETE /api/v1/store/{agent_name}/keys
//
// <key> 와 --all 은 상호 배타적이다. 둘 다 없거나 둘 다 있으면 에러이다.
// 삭제 전 confirmFn 으로 확인을 받으며, --yes 플래그로 생략할 수 있다.
func newStoreResetCmd(client **Client, confirmFn func(string, io.Reader) bool) *cobra.Command {
	var (
		all       bool
		yes       bool
		namespace string
	)

	cmd := &cobra.Command{
		Use:   "reset <agent_name> [<key>]",
		Short: "저장소 키 리셋(삭제)",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			agentName := args[0]
			w := cmd.OutOrStdout()

			var key string
			if len(args) == 2 {
				key = args[1]
			}

			// 상호 배타 검증: 단일 키 vs --all.
			if key != "" && all {
				return fmt.Errorf("<key> 와 --all 을 동시에 사용할 수 없습니다")
			}
			if key == "" && !all {
				return fmt.Errorf("<key> 를 지정하거나 --all 플래그를 사용해야 합니다")
			}

			// 확인 프롬프트 (--yes 면 생략).
			if !yes {
				var prompt string
				if all {
					prompt = fmt.Sprintf("에이전트 '%s' 의 모든 키를 리셋하시겠습니까?", agentName)
				} else {
					prompt = fmt.Sprintf("에이전트 '%s' 의 키 '%s' 를 리셋하시겠습니까?", agentName, key)
				}
				if !confirmFn(prompt, os.Stdin) {
					fmt.Fprintln(w, "리셋이 취소되었습니다.")
					return nil
				}
			}

			// 경로 구성: 단일 키 vs 벌크. namespace 는 쿼리 파라미터로 전달한다.
			var path string
			if all {
				path = storePath(agentName, "/keys")
			} else {
				path = storePath(agentName, "/keys/"+url.PathEscape(key))
			}
			if namespace != "" {
				path += "?namespace=" + url.QueryEscape(namespace)
			}

			var result map[string]any
			if err := (*client).Delete(path, &result); err != nil {
				return err
			}

			format := getFormat(cmd)
			if format == "table" || format == "text" {
				if all {
					fmt.Fprintf(w, "에이전트 '%s' 의 모든 키가 리셋되었습니다.\n", agentName)
				} else {
					fmt.Fprintf(w, "에이전트 '%s' 의 키 '%s' 가 리셋되었습니다.\n", agentName, key)
				}
				return nil
			}
			return PrintResult(w, format, result, nil, nil)
		},
	}

	cmd.Flags().BoolVar(&all, "all", false, "모든 키를 리셋 (벌크)")
	cmd.Flags().BoolVar(&yes, "yes", false, "확인 프롬프트 생략")
	cmd.Flags().StringVar(&namespace, "namespace", "", "네임스페이스 (생략 시 default)")

	return cmd
}

// --- 공통 헬퍼 ---

// parseTagKV 는 "key=value" 형식의 반복 플래그를 map[string]string 으로 변환한다.
// value 내 '=' 를 허용하기 위해 첫 번째 '=' 로만 분할한다.
// '=' 가 없는 입력은 에러를 반환한다. 빈 슬라이스는 nil 을 반환한다.
func parseTagKV(pairs []string) (map[string]string, error) {
	if len(pairs) == 0 {
		return nil, nil
	}
	out := make(map[string]string, len(pairs))
	for _, p := range pairs {
		idx := strings.IndexByte(p, '=')
		if idx < 0 {
			return nil, fmt.Errorf("잘못된 태그 형식입니다: %q (key=value 형식이어야 합니다)", p)
		}
		out[p[:idx]] = p[idx+1:]
	}
	return out, nil
}
