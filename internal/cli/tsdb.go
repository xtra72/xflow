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

// newTsdbCmd 는 시계열 DB(TSDB) 관리 커맨드 그룹을 생성한다.
// 서브커맨드: series, latest, query, stats, delete, write.
// 삭제 커맨드는 confirmFn 으로 사용자 확인을 거친다.
func newTsdbCmd(client **Client, confirmFn func(string, io.Reader) bool) *cobra.Command {
	tsdbCmd := &cobra.Command{
		Use:   "tsdb",
		Short: "시계열 DB(TSDB) 관리 명령어",
		Long:  "시계열 데이터의 시리즈 조회, 최신 포인트 조회, 쿼리, 통계, 삭제, 쓰기를 수행합니다.",
	}

	tsdbCmd.AddCommand(newTsdbSeriesCmd(client))
	tsdbCmd.AddCommand(newTsdbLatestCmd(client))
	tsdbCmd.AddCommand(newTsdbQueryCmd(client))
	tsdbCmd.AddCommand(newTsdbStatsCmd(client))
	tsdbCmd.AddCommand(newTsdbDeleteCmd(client, confirmFn))
	tsdbCmd.AddCommand(newTsdbWriteCmd(client))

	return tsdbCmd
}

// tsdbSeriesListResponse 는 GET /api/v1/tsdb/series 응답의 data 필드를 매핑한다.
type tsdbSeriesListResponse struct {
	Series []string `json:"series"`
	Count  int      `json:"count"`
}

// tsdbSeriesTableHeaders 는 시리즈 목록 테이블의 헤더이다.
var tsdbSeriesTableHeaders = []string{"SERIES KEY"}

// tsdbSeriesRowFunc 는 시리즈 키 문자열에서 테이블 행을 추출한다.
func tsdbSeriesRowFunc(item any) []string {
	return []string{fmt.Sprintf("%v", item)}
}

// newTsdbSeriesCmd 는 tsdb series 서브커맨드를 생성한다.
// GET /api/v1/tsdb/series 로 시계열 메타(시리즈 키) 목록을 조회한다.
// --measurement 로 측정값 필터, --tag k=v (반복) 로 태그 필터를 지원한다.
func newTsdbSeriesCmd(client **Client) *cobra.Command {
	var (
		measurement string
		tags        []string
	)

	cmd := &cobra.Command{
		Use:   "series",
		Short: "시계열 메타(시리즈 키) 목록 조회",
		RunE: func(cmd *cobra.Command, args []string) error {
			tagMap, err := parseTagFlags(tags)
			if err != nil {
				return err
			}

			path, err := buildTsdbSeriesPath(measurement, tagMap)
			if err != nil {
				return err
			}

			var resp tsdbSeriesListResponse
			if err := (*client).Get(path, &resp); err != nil {
				return err
			}

			format := getFormat(cmd)
			w := cmd.OutOrStdout()
			return PrintResult(w, format, resp.Series, tsdbSeriesTableHeaders, tsdbSeriesRowFunc)
		},
	}

	cmd.Flags().StringVar(&measurement, "measurement", "", "측정값(measurement)으로 필터링")
	cmd.Flags().StringArrayVar(&tags, "tag", nil, "태그 필터 (key=value 형식, 반복 가능)")

	return cmd
}

// buildTsdbSeriesPath 는 measurement/tag 필터를 쿼리 스트링으로 인코딩하여
// /api/v1/tsdb/series 경로를 생성한다.
func buildTsdbSeriesPath(measurement string, tags map[string]string) (string, error) {
	const base = "/api/v1/tsdb/series"

	q := url.Values{}
	if measurement != "" {
		q.Set("measurement", measurement)
	}
	// 결정적 출력을 위해 태그 키를 정렬한다.
	keys := make([]string, 0, len(tags))
	for k := range tags {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		q.Set(k, tags[k])
	}

	if len(q) == 0 {
		return base, nil
	}
	return base + "?" + q.Encode(), nil
}

// parseTagFlags 는 "key=value" 형식의 문자열 슬라이스를 맵으로 파싱한다.
// 등호가 없거나 키가 비어있으면 에러를 반환한다.
func parseTagFlags(tags []string) (map[string]string, error) {
	result := make(map[string]string, len(tags))
	for _, raw := range tags {
		idx := strings.Index(raw, "=")
		if idx < 0 {
			return nil, ErrInvalidInput(fmt.Sprintf("태그는 key=value 형식이어야 합니다: %q", raw))
		}
		key := raw[:idx]
		val := raw[idx+1:]
		if key == "" {
			return nil, ErrInvalidInput(fmt.Sprintf("태그 키가 비어 있습니다: %q", raw))
		}
		result[key] = val
	}
	return result, nil
}

// newTsdbLatestCmd 는 tsdb latest <key> [--n N] 서브커맨드를 생성한다.
// GET /api/v1/tsdb/series/{key}/latest?n=N 로 최신 데이터 포인트를 조회한다.
// 시리즈 키는 콜론/쉼표 등 특수문자를 포함할 수 있으므로 URL 경로 이스케이프한다.
func newTsdbLatestCmd(client **Client) *cobra.Command {
	var n int

	cmd := &cobra.Command{
		Use:   "latest <key>",
		Short: "시리즈의 최신 데이터 포인트 조회",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			key := args[0]
			if key == "" {
				return ErrInvalidInput("시리즈 키를 지정해주세요")
			}
			if n < 1 {
				return ErrInvalidInput("--n 값은 1 이상이어야 합니다")
			}

			path := fmt.Sprintf("/api/v1/tsdb/series/%s/latest", url.PathEscape(key))
			if n > 1 {
				path += fmt.Sprintf("?n=%d", n)
			}

			var points []map[string]any
			if err := (*client).Get(path, &points); err != nil {
				return err
			}

			format := getFormat(cmd)
			w := cmd.OutOrStdout()
			return PrintResult(w, format, points, tsdbPointTableHeaders, tsdbPointRowFunc)
		},
	}

	cmd.Flags().IntVar(&n, "n", 1, "조회할 최신 포인트 개수")

	return cmd
}

// tsdbPointTableHeaders 는 데이터 포인트 테이블의 헤더이다.
var tsdbPointTableHeaders = []string{"TIMESTAMP", "FIELDS"}

// tsdbPointRowFunc 는 데이터 포인트 맵에서 테이블 행을 추출한다.
func tsdbPointRowFunc(item any) []string {
	m, ok := item.(map[string]any)
	if !ok {
		return []string{"", ""}
	}
	return []string{
		fmt.Sprintf("%v", m["timestamp"]),
		formatFields(m["fields"]),
	}
}

// formatFields 는 fields 맵을 "k=v, k=v" 형식의 정렬된 문자열로 변환한다.
func formatFields(v any) string {
	m, ok := v.(map[string]any)
	if !ok {
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
	return strings.Join(parts, ", ")
}

// newTsdbQueryCmd 는 tsdb query [옵션] 서브커맨드를 생성한다.
// POST /api/v1/tsdb/query 로 시계열을 조회한다.
//
// 플래그를 TSDBQueryRequest 바디로 매핑한다:
//
//	--series/--key  -> series_key
//	--measurement   -> measurement
//	--tag k=v       -> tags
//	--field         -> field
//	--start         -> start (RFC3339Nano)
//	--end           -> end (RFC3339Nano)
//	--last/--limit  -> limit (마지막 N개)
//	--aggregation   -> aggregation
//	--bucket        -> bucket
//	--fill          -> fill
//
// --json 으로 원시 JSON 바디 파일을 직접 전달할 수도 있다.
func newTsdbQueryCmd(client **Client) *cobra.Command {
	var (
		seriesKey   string
		measurement string
		tags        []string
		field       string
		start       string
		end         string
		last        int
		aggregation string
		bucket      string
		fill        string
		jsonFile    string
	)

	cmd := &cobra.Command{
		Use:   "query",
		Short: "시계열 조회 (시간 범위 / 마지막 N개)",
		RunE: func(cmd *cobra.Command, args []string) error {
			var body map[string]any

			if jsonFile != "" {
				// --json: 원시 JSON 바디 파일을 그대로 전달한다.
				raw, err := readFile(jsonFile)
				if err != nil {
					return err
				}
				if err := json.Unmarshal(raw, &body); err != nil {
					return fmt.Errorf("JSON 파싱 실패: %w", err)
				}
			} else {
				tagMap, err := parseTagFlags(tags)
				if err != nil {
					return err
				}
				body = buildTsdbQueryBody(tsdbQueryParams{
					seriesKey:   seriesKey,
					measurement: measurement,
					tags:        tagMap,
					field:       field,
					start:       start,
					end:         end,
					limit:       last,
					aggregation: aggregation,
					bucket:      bucket,
					fill:        fill,
				})
			}

			var result map[string]any
			if err := (*client).Post("/api/v1/tsdb/query", body, &result); err != nil {
				return err
			}

			format := getFormat(cmd)
			w := cmd.OutOrStdout()
			return PrintResult(w, format, result, nil, nil)
		},
	}

	cmd.Flags().StringVar(&seriesKey, "series", "", "시리즈 키 (series_key)")
	cmd.Flags().StringVar(&seriesKey, "key", "", "시리즈 키 별칭 (--series 와 동일)")
	cmd.Flags().StringVar(&measurement, "measurement", "", "측정값(measurement) 필터")
	cmd.Flags().StringArrayVar(&tags, "tag", nil, "태그 필터 (key=value 형식, 반복 가능)")
	cmd.Flags().StringVar(&field, "field", "", "조회할 필드 이름")
	cmd.Flags().StringVar(&start, "start", "", "시작 시간 (RFC3339Nano)")
	cmd.Flags().StringVar(&end, "end", "", "종료 시간 (RFC3339Nano)")
	cmd.Flags().IntVar(&last, "last", 0, "마지막 N개 포인트 (limit)")
	cmd.Flags().IntVar(&last, "limit", 0, "limit 별칭 (--last 와 동일)")
	cmd.Flags().StringVar(&aggregation, "aggregation", "", "집계 함수 (min,max,avg,sum,count,first,last)")
	cmd.Flags().StringVar(&bucket, "bucket", "", "버킷 간격 (duration: 5m, 1h)")
	cmd.Flags().StringVar(&fill, "fill", "", "빈 버킷 채우기 (null,zero,previous,avg)")
	cmd.Flags().StringVar(&jsonFile, "json", "", "원시 JSON 쿼리 바디 파일 경로")

	return cmd
}

// tsdbQueryParams 는 query 바디 빌더의 입력이다.
type tsdbQueryParams struct {
	seriesKey   string
	measurement string
	tags        map[string]string
	field       string
	start       string
	end         string
	limit       int
	aggregation string
	bucket      string
	fill        string
}

// buildTsdbQueryBody 는 플래그 값을 TSDBQueryRequest JSON 바디(맵)로 매핑한다.
// 비어 있는 값은 omitempty 시맨틱에 맞춰 바디에서 제외한다.
func buildTsdbQueryBody(p tsdbQueryParams) map[string]any {
	body := make(map[string]any)

	if p.seriesKey != "" {
		body["series_key"] = p.seriesKey
	}
	if p.measurement != "" {
		body["measurement"] = p.measurement
	}
	if len(p.tags) > 0 {
		body["tags"] = p.tags
	}
	if p.field != "" {
		body["field"] = p.field
	}
	if p.start != "" {
		body["start"] = p.start
	}
	if p.end != "" {
		body["end"] = p.end
	}
	if p.limit > 0 {
		body["limit"] = p.limit
	}
	if p.aggregation != "" {
		body["aggregation"] = p.aggregation
	}
	if p.bucket != "" {
		body["bucket"] = p.bucket
	}
	if p.fill != "" {
		body["fill"] = p.fill
	}

	return body
}

// tsdbStatsFieldOrder 는 통계 상세 출력의 필드 순서이다.
var tsdbStatsFieldOrder = []string{
	"series_count", "total_points", "memory_human", "memory_bytes",
	"oldest_point", "newest_point",
}

// tsdbStatsLabelMap 은 통계 상세 출력의 필드 라벨 매핑이다.
var tsdbStatsLabelMap = map[string]string{
	"series_count": "Series Count",
	"total_points": "Total Points",
	"memory_human": "Memory",
	"memory_bytes": "Memory (bytes)",
	"oldest_point": "Oldest Point",
	"newest_point": "Newest Point",
}

// newTsdbStatsCmd 는 tsdb stats 서브커맨드를 생성한다.
// GET /api/v1/tsdb/stats 로 TSDB 통계(시리즈 수/용량 등)를 조회한다.
func newTsdbStatsCmd(client **Client) *cobra.Command {
	return &cobra.Command{
		Use:   "stats",
		Short: "TSDB 통계 조회 (시리즈 수 / 용량)",
		RunE: func(cmd *cobra.Command, args []string) error {
			var stats map[string]any
			if err := (*client).Get("/api/v1/tsdb/stats", &stats); err != nil {
				return err
			}

			format := getFormat(cmd)
			w := cmd.OutOrStdout()

			if format == "table" || format == "text" {
				df := NewDetailFormatter(tsdbStatsFieldOrder, tsdbStatsLabelMap, nil)
				return df.Format(stats, w)
			}
			return PrintResult(w, format, stats, nil, nil)
		},
	}
}

// newTsdbDeleteCmd 는 tsdb delete <key> [--yes] 서브커맨드를 생성한다.
// 확인 프롬프트 후 DELETE /api/v1/tsdb/series/{key} 로 시계열을 삭제한다.
// 시리즈 키는 특수문자를 포함할 수 있으므로 URL 경로 이스케이프한다.
func newTsdbDeleteCmd(client **Client, confirmFn func(string, io.Reader) bool) *cobra.Command {
	var yes bool

	cmd := &cobra.Command{
		Use:   "delete <key>",
		Short: "시계열(시리즈) 삭제",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			key := args[0]
			if key == "" {
				return ErrInvalidInput("시리즈 키를 지정해주세요")
			}
			w := cmd.OutOrStdout()

			// --yes 플래그가 없으면 확인 요청
			if !yes {
				prompt := fmt.Sprintf("시리즈 '%s' 를 삭제하시겠습니까?", key)
				if !confirmFn(prompt, os.Stdin) {
					fmt.Fprintln(w, "삭제가 취소되었습니다.")
					return nil
				}
			}

			path := fmt.Sprintf("/api/v1/tsdb/series/%s", url.PathEscape(key))

			var result map[string]any
			if err := (*client).Delete(path, &result); err != nil {
				return err
			}

			format := getFormat(cmd)
			if format == "table" || format == "text" {
				fmt.Fprintf(w, "시리즈 '%s' 가 삭제되었습니다.\n", key)
				return nil
			}
			return PrintResult(w, format, result, nil, nil)
		},
	}

	cmd.Flags().BoolVar(&yes, "yes", false, "확인 프롬프트 생략")

	return cmd
}

// newTsdbWriteCmd 는 tsdb write [옵션] 서브커맨드를 생성한다.
// POST /api/v1/tsdb/write 로 데이터 포인트를 기록한다.
//
// 두 가지 입력 방식을 지원한다:
//   - --json <file>: 원시 TSDBWriteRequest JSON 바디 파일을 그대로 전달
//   - 플래그: --measurement, --field k=v(반복), --tag k=v(반복), --timestamp
//     단일 포인트를 points 배열로 래핑하여 전송
func newTsdbWriteCmd(client **Client) *cobra.Command {
	var (
		measurement string
		fields      []string
		tags        []string
		timestamp   string
		jsonFile    string
	)

	cmd := &cobra.Command{
		Use:   "write",
		Short: "데이터 포인트 쓰기",
		RunE: func(cmd *cobra.Command, args []string) error {
			var body map[string]any

			if jsonFile != "" {
				raw, err := readFile(jsonFile)
				if err != nil {
					return err
				}
				if err := json.Unmarshal(raw, &body); err != nil {
					return fmt.Errorf("JSON 파싱 실패: %w", err)
				}
			} else {
				if measurement == "" {
					return ErrInvalidInput("--measurement 를 지정하거나 --json 파일을 사용하세요")
				}
				if len(fields) == 0 {
					return ErrInvalidInput("최소 하나의 --field key=value 를 지정해야 합니다")
				}

				fieldMap, err := parseFieldFlags(fields)
				if err != nil {
					return err
				}
				tagMap, err := parseTagFlags(tags)
				if err != nil {
					return err
				}

				point := map[string]any{
					"measurement": measurement,
					"fields":      fieldMap,
				}
				if len(tagMap) > 0 {
					point["tags"] = tagMap
				}
				if timestamp != "" {
					point["timestamp"] = timestamp
				}

				body = map[string]any{
					"points": []any{point},
				}
			}

			var result map[string]any
			if err := (*client).Post("/api/v1/tsdb/write", body, &result); err != nil {
				return err
			}

			format := getFormat(cmd)
			w := cmd.OutOrStdout()
			if format == "table" || format == "text" {
				fmt.Fprintf(w, "데이터 포인트 %v개를 기록했습니다.\n", result["written"])
				return nil
			}
			return PrintResult(w, format, result, nil, nil)
		},
	}

	cmd.Flags().StringVar(&measurement, "measurement", "", "측정값(measurement) 이름")
	cmd.Flags().StringArrayVar(&fields, "field", nil, "필드 (key=value 형식, 반복 가능)")
	cmd.Flags().StringArrayVar(&tags, "tag", nil, "태그 (key=value 형식, 반복 가능)")
	cmd.Flags().StringVar(&timestamp, "timestamp", "", "타임스탬프 (RFC3339Nano 또는 unix nano)")
	cmd.Flags().StringVar(&jsonFile, "json", "", "원시 JSON 쓰기 바디 파일 경로")

	return cmd
}

// parseFieldFlags 는 "key=value" 형식의 필드 슬라이스를 맵으로 파싱한다.
// 값이 숫자나 불리언으로 해석되면 해당 타입으로 변환하고, 그 외에는 문자열로 둔다.
func parseFieldFlags(fields []string) (map[string]any, error) {
	result := make(map[string]any, len(fields))
	for _, raw := range fields {
		idx := strings.Index(raw, "=")
		if idx < 0 {
			return nil, ErrInvalidInput(fmt.Sprintf("필드는 key=value 형식이어야 합니다: %q", raw))
		}
		key := raw[:idx]
		val := raw[idx+1:]
		if key == "" {
			return nil, ErrInvalidInput(fmt.Sprintf("필드 키가 비어 있습니다: %q", raw))
		}
		result[key] = coerceFieldValue(val)
	}
	return result, nil
}

// coerceFieldValue 는 문자열 값을 가능한 경우 숫자/불리언으로 변환한다.
// JSON 숫자/불리언으로 파싱되면 그 값을, 아니면 원본 문자열을 반환한다.
func coerceFieldValue(s string) any {
	switch s {
	case "true":
		return true
	case "false":
		return false
	}
	var num json.Number
	dec := json.NewDecoder(strings.NewReader(s))
	dec.UseNumber()
	if err := dec.Decode(&num); err == nil {
		// 디코더가 추가 토큰 없이 전체를 소비했는지 확인
		if dec.More() {
			return s
		}
		if f, err := num.Float64(); err == nil {
			return f
		}
	}
	return s
}
