package cli

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"
)

// newChartCmd 는 차트 채널 조회 커맨드 그룹을 생성한다.
// 차트 API(GET /api/v1/charts/channels)를 CLI 로 노출한다.
//
// 서브커맨드:
//
//	channels  활성 chart-emitter 채널 목록 조회
//
// (WebSocket 스트림 /ws/chart/{channel} 은 본 P4 범위 밖이다.)
func newChartCmd(client **Client) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "chart",
		Short: "차트 채널 조회",
		Long:  "활성화된 chart-emitter 채널의 요약 정보를 조회합니다.",
	}

	cmd.AddCommand(newChartChannelsCmd(client))

	return cmd
}

// chartChannelHeaders 는 차트 채널 목록 테이블의 헤더이다.
var chartChannelHeaders = []string{
	"NAME", "FLOW_ID", "NODE_ID", "BUFFER", "RETENTION_SEC", "SUBSCRIBERS", "LAST_MESSAGE",
}

// chartChannelRowFunc 는 채널 맵에서 테이블 행을 추출한다.
// last_message_ms 는 epoch ms 이므로 formatEpochValue 로 정수 문자열 변환한다.
func chartChannelRowFunc(item any) []string {
	m, ok := item.(map[string]any)
	if !ok {
		return []string{"", "", "", "", "", "", ""}
	}
	return []string{
		fmt.Sprintf("%v", m["name"]),
		fmt.Sprintf("%v", m["flow_id"]),
		fmt.Sprintf("%v", m["node_id"]),
		formatCountValue(m["buffer_size"]),
		formatCountValue(m["retention_sec"]),
		formatCountValue(m["subscriber_count"]),
		formatEpochValue(m["last_message_ms"]),
	}
}

// formatCountValue 는 정수 카운트 값을 정수 문자열로 포맷한다.
// JSON 디코딩 시 숫자는 float64 로 들어오므로, 큰 값의 과학표기 출력을 피하기 위해
// 정수로 변환한다(formatEpochValue 와 동일 전략, 카운트 의미만 다름).
func formatCountValue(v any) string {
	switch n := v.(type) {
	case float64:
		return strconv.FormatInt(int64(n), 10)
	case int64:
		return strconv.FormatInt(n, 10)
	case int:
		return strconv.Itoa(n)
	case nil:
		return "0"
	default:
		return fmt.Sprintf("%v", v)
	}
}

// newChartChannelsCmd 는 chart channels 서브커맨드를 생성한다.
// GET /api/v1/charts/channels 로 활성 채널 목록을 조회한다.
//
// 응답은 객체 {"channels":[...]} 형식이므로, table/text 출력 시 channels 배열을
// 추출하여 테이블로 렌더링하고, json/yaml 은 전체 객체를 그대로 통과시킨다.
func newChartChannelsCmd(client **Client) *cobra.Command {
	return &cobra.Command{
		Use:   "channels",
		Short: "활성 차트 채널 목록 조회",
		Long: `현재 활성화된 모든 chart-emitter 채널의 요약 정보를 조회합니다.

예시:
  xflow chart channels
  xflow chart channels --format json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			var resp map[string]any
			if err := (*client).Get("/api/v1/charts/channels", &resp); err != nil {
				return err
			}

			format := getFormat(cmd)
			w := cmd.OutOrStdout()

			if format == "table" || format == "text" {
				channels := extractChartChannels(resp)
				return PrintResult(w, "table", channels, chartChannelHeaders, chartChannelRowFunc)
			}
			return PrintResult(w, format, resp, nil, nil)
		},
	}
}

// extractChartChannels 는 응답 객체에서 channels 배열을 맵 슬라이스로 추출한다.
// device history 의 extractHistoryEntries 패턴을 따른다.
func extractChartChannels(resp map[string]any) []map[string]any {
	raw, ok := resp["channels"].([]any)
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
