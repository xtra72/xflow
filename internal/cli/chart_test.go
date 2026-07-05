package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- 테스트 헬퍼 ---

// chartEnvelope 은 차트 API 의 성공 응답 엔벨로프를 생성한다.
func chartEnvelope(data any) []byte {
	resp := map[string]any{"success": true, "data": data}
	b, _ := json.Marshal(resp)
	return b
}

// setupChartTest 는 mock 서버와 chart 커맨드 루트를 세팅하는 헬퍼이다.
func setupChartTest(t *testing.T, handler http.HandlerFunc) (*bytes.Buffer, *cobra.Command, func()) {
	t.Helper()

	server := httptest.NewServer(handler)

	client := NewClient(server.URL, "test-token", 5*time.Second, false)
	clientPtr := &client

	rootCmd := &cobra.Command{Use: "xflow"}
	rootCmd.PersistentFlags().String("format", "table", "출력 형식")
	rootCmd.AddCommand(newChartCmd(clientPtr))

	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)

	return &buf, rootCmd, server.Close
}

// sampleChannels 는 테스트용 채널 목록 응답(객체 래핑)을 생성한다.
func sampleChannels() map[string]any {
	return map[string]any{
		"channels": []map[string]any{
			{
				"name":             "flow1.node1",
				"flow_id":          "flow1",
				"node_id":          "node1",
				"buffer_size":      float64(100),
				"retention_sec":    float64(3600),
				"subscriber_count": float64(2),
				"last_message_ms":  float64(1700000000000),
			},
			{
				"name":             "flow2.node2",
				"flow_id":          "flow2",
				"node_id":          "node2",
				"buffer_size":      float64(50),
				"retention_sec":    float64(1800),
				"subscriber_count": float64(0),
				"last_message_ms":  float64(0),
			},
		},
	}
}

// --- chart channels ---

func TestChartChannels(t *testing.T) {
	var gotPath, gotMethod string

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		w.Header().Set("Content-Type", "application/json")
		w.Write(chartEnvelope(sampleChannels()))
	})

	buf, cmd, cleanup := setupChartTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"chart", "channels"})
	require.NoError(t, cmd.Execute())

	assert.Equal(t, http.MethodGet, gotMethod)
	assert.Equal(t, "/api/v1/charts/channels", gotPath)

	out := buf.String()
	assert.Contains(t, out, "NAME")
	assert.Contains(t, out, "FLOW_ID")
	assert.Contains(t, out, "RETENTION_SEC")
	assert.Contains(t, out, "SUBSCRIBERS")
	assert.Contains(t, out, "LAST_MESSAGE")
	assert.Contains(t, out, "flow1.node1")
	assert.Contains(t, out, "flow2.node2")
	// last_message_ms 는 과학표기 없이 정수 문자열로 출력되어야 한다.
	assert.Contains(t, out, "1700000000000")
	assert.NotContains(t, out, "1.7e")
}

func TestChartChannels_Empty(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(chartEnvelope(map[string]any{"channels": []map[string]any{}}))
	})

	buf, cmd, cleanup := setupChartTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"chart", "channels"})
	require.NoError(t, cmd.Execute())

	// 빈 목록이어도 헤더는 출력된다.
	assert.Contains(t, buf.String(), "NAME")
}

func TestChartChannels_JSONFormat(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(chartEnvelope(sampleChannels()))
	})

	buf, cmd, cleanup := setupChartTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"--format", "json", "chart", "channels"})
	require.NoError(t, cmd.Execute())

	// json 포맷은 전체 객체(channels 래핑)를 그대로 통과시킨다.
	var parsed map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &parsed))
	channels, ok := parsed["channels"].([]any)
	require.True(t, ok, "json 출력은 channels 배열을 가진 객체여야 합니다")
	assert.Len(t, channels, 2)
}

// --- extractChartChannels 단위 테스트 ---

func TestExtractChartChannels(t *testing.T) {
	tests := []struct {
		name string
		resp map[string]any
		want int
	}{
		{"two channels", sampleChannels(), 2},
		{"missing channels key", map[string]any{}, 0},
		{"channels not array", map[string]any{"channels": "oops"}, 0},
		{"empty array", map[string]any{"channels": []any{}}, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// sampleChannels 는 []map[string]any 를 쓰므로 []any 로 정규화한다.
			resp := normalizeChannelsResp(tt.resp)
			got := extractChartChannels(resp)
			assert.Len(t, got, tt.want)
		})
	}
}

// normalizeChannelsResp 는 []map[string]any 형태의 channels 를 []any 로 변환하여
// 실제 JSON 디코딩 결과와 동일한 형태로 만든다.
func normalizeChannelsResp(resp map[string]any) map[string]any {
	out := make(map[string]any, len(resp))
	for k, v := range resp {
		if k == "channels" {
			if ms, ok := v.([]map[string]any); ok {
				anys := make([]any, 0, len(ms))
				for _, m := range ms {
					anys = append(anys, any(m))
				}
				out[k] = anys
				continue
			}
		}
		out[k] = v
	}
	return out
}

// --- formatCountValue 단위 테스트 ---

func TestFormatCountValue(t *testing.T) {
	tests := []struct {
		name string
		in   any
		want string
	}{
		{"float64", float64(42), "42"},
		{"large float64", float64(1700000000000), "1700000000000"},
		{"int64", int64(7), "7"},
		{"int", 5, "5"},
		{"nil", nil, "0"},
		{"string fallback", "n/a", "n/a"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, formatCountValue(tt.in))
		})
	}
}
