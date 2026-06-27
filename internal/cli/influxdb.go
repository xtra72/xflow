package cli

import (
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

// newInfluxdbCmd 는 InfluxDB 에이전트 쿼리 커맨드 그룹을 생성한다.
// InfluxDB API(POST /api/v1/influxdb/{agent_name}/query)를 CLI 로 노출한다.
//
// 서브커맨드:
//
//	query  지정한 InfluxDB 에이전트에 Flux/InfluxQL 쿼리 실행
func newInfluxdbCmd(client **Client) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "influxdb",
		Short: "InfluxDB 에이전트 쿼리",
		Long:  "InfluxDB 에이전트에 Flux 또는 InfluxQL 쿼리를 실행하고 결과를 조회합니다.",
	}

	cmd.AddCommand(newInfluxdbQueryCmd(client))

	return cmd
}

// influxQueryEntryHeaders 는 InfluxDB 쿼리 결과 테이블의 헤더이다.
var influxQueryEntryHeaders = []string{"TIMESTAMP", "VALUE", "LABELS"}

// influxQueryRowFunc 는 결과 항목 맵에서 테이블 행을 추출한다.
// timestamp 는 epoch ms 이므로 formatEpochValue 로 정수 문자열 변환하고,
// labels 는 "k=v,k=v" 형식으로 압축 렌더링한다.
func influxQueryRowFunc(item any) []string {
	m, ok := item.(map[string]any)
	if !ok {
		return []string{"", "", ""}
	}
	return []string{
		formatEpochValue(m["timestamp"]),
		fmt.Sprintf("%v", m["value"]),
		formatLabels(m["labels"]),
	}
}

// formatLabels 는 labels 맵을 정렬된 "k=v,k=v" 문자열로 압축 렌더링한다.
// 키를 정렬하여 출력 순서를 결정적으로 만든다.
func formatLabels(v any) string {
	m, ok := v.(map[string]any)
	if !ok || len(m) == 0 {
		return ""
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

// newInfluxdbQueryCmd 는 influxdb query 서브커맨드를 생성한다.
// POST /api/v1/influxdb/{agent_name}/query 로 쿼리를 실행한다.
//
// 요청 본문은 influxDBQueryRequest 형식
// {"query_language":"flux|influxql","query":"...","params":{...}} 이다.
// 응답은 chartQueryResponse 형식
// {"entries":[{"timestamp":<epoch ms>,"value":any,"labels":{...}}],"count":N,"truncated":bool} 이다.
func newInfluxdbQueryCmd(client **Client) *cobra.Command {
	var (
		query  string
		lang   string
		params []string
	)

	cmd := &cobra.Command{
		Use:   "query <agent_name>",
		Short: "InfluxDB 에이전트에 쿼리 실행",
		Long: `지정한 InfluxDB 에이전트에 Flux 또는 InfluxQL 쿼리를 실행합니다.

예시:
  xflow influxdb query my-influx --query 'from(bucket:"b") |> range(start:-1h)'
  xflow influxdb query my-influx --lang influxql --query 'SELECT * FROM cpu'
  xflow influxdb query my-influx --query '...' --param host=server1 --param region=kr`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			agentName := strings.TrimSpace(args[0])
			if agentName == "" {
				return ErrInvalidInput("에이전트명을 지정해야 합니다")
			}

			body, err := buildInfluxQueryBody(query, lang, params)
			if err != nil {
				return err
			}

			var resp map[string]any
			path := fmt.Sprintf("/api/v1/influxdb/%s/query", url.PathEscape(agentName))
			if err := (*client).Post(path, body, &resp); err != nil {
				return err
			}

			format := getFormat(cmd)
			w := cmd.OutOrStdout()

			if format == "table" || format == "text" {
				entries := extractInfluxEntries(resp)
				if err := PrintResult(w, "table", entries, influxQueryEntryHeaders, influxQueryRowFunc); err != nil {
					return err
				}
				fmt.Fprintf(w, "\nCount:     %v\n", formatCountValue(resp["count"]))
				fmt.Fprintf(w, "Truncated: %v\n", resp["truncated"])
				return nil
			}
			return PrintResult(w, format, resp, nil, nil)
		},
	}

	cmd.Flags().StringVar(&query, "query", "", "실행할 쿼리 (필수)")
	cmd.Flags().StringVar(&lang, "lang", "flux", "쿼리 언어 (flux|influxql)")
	cmd.Flags().StringArrayVar(&params, "param", nil, "쿼리 파라미터 key=value (반복 지정 가능)")
	_ = cmd.MarkFlagRequired("query")

	return cmd
}

// buildInfluxQueryBody 는 query/lang/params 플래그를 요청 본문으로 변환한다.
//
// query 는 비어 있으면 안 되고, lang 은 "flux" 또는 "influxql" 이어야 한다.
// params 는 하나 이상 주어진 경우에만 본문에 포함한다(key=value 형식).
func buildInfluxQueryBody(query, lang string, params []string) (map[string]any, error) {
	q := strings.TrimSpace(query)
	if q == "" {
		return nil, ErrInvalidInput("쿼리를 지정해야 합니다 (--query)")
	}

	if lang == "" {
		lang = "flux"
	}
	if lang != "flux" && lang != "influxql" {
		return nil, ErrInvalidInput(fmt.Sprintf("쿼리 언어는 'flux' 또는 'influxql' 이어야 합니다: %q", lang))
	}

	body := map[string]any{
		"query_language": lang,
		"query":          q,
	}

	if len(params) > 0 {
		paramMap := make(map[string]any, len(params))
		for _, p := range params {
			k, v, ok := strings.Cut(p, "=")
			if !ok {
				return nil, ErrInvalidInput(fmt.Sprintf("잘못된 파라미터 형식: %q (key=value 형식 필요)", p))
			}
			paramMap[k] = parseParamValue(v)
		}
		body["params"] = paramMap
	}

	return body, nil
}

// extractInfluxEntries 는 chartQueryResponse 의 entries 배열을 맵 슬라이스로 추출한다.
func extractInfluxEntries(resp map[string]any) []map[string]any {
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
