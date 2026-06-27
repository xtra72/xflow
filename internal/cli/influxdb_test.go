package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- 테스트 헬퍼 ---

// influxEnvelope 은 InfluxDB API 의 성공 응답 엔벨로프를 생성한다.
func influxEnvelope(data any) []byte {
	resp := map[string]any{"success": true, "data": data}
	b, _ := json.Marshal(resp)
	return b
}

// setupInfluxdbTest 는 mock 서버와 influxdb 커맨드 루트를 세팅하는 헬퍼이다.
func setupInfluxdbTest(t *testing.T, handler http.HandlerFunc) (*bytes.Buffer, *cobra.Command, func()) {
	t.Helper()

	server := httptest.NewServer(handler)

	client := NewClient(server.URL, "test-token", 5*time.Second, false)
	clientPtr := &client

	rootCmd := &cobra.Command{Use: "xflow"}
	rootCmd.PersistentFlags().String("format", "table", "출력 형식")
	rootCmd.AddCommand(newInfluxdbCmd(clientPtr))

	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)

	return &buf, rootCmd, server.Close
}

// sampleInfluxResult 는 테스트용 chartQueryResponse 응답 데이터를 생성한다.
func sampleInfluxResult() map[string]any {
	return map[string]any{
		"entries": []map[string]any{
			{
				"timestamp": float64(1700000000000),
				"value":     float64(23.5),
				"labels":    map[string]any{"host": "server1", "region": "kr"},
			},
			{
				"timestamp": float64(1700000001000),
				"value":     float64(24.0),
				"labels":    map[string]any{"host": "server2"},
			},
		},
		"count":     float64(2),
		"truncated": false,
	}
}

// --- influxdb query ---

func TestInfluxdbQuery_Flux(t *testing.T) {
	var gotPath, gotMethod string
	var gotBody map[string]any

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		body, _ := io.ReadAll(r.Body)
		require.NoError(t, json.Unmarshal(body, &gotBody))
		w.Header().Set("Content-Type", "application/json")
		w.Write(influxEnvelope(sampleInfluxResult()))
	})

	buf, cmd, cleanup := setupInfluxdbTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{
		"influxdb", "query", "my-influx",
		"--query", `from(bucket:"b") |> range(start:-1h)`,
	})
	require.NoError(t, cmd.Execute())

	assert.Equal(t, http.MethodPost, gotMethod)
	assert.Equal(t, "/api/v1/influxdb/my-influx/query", gotPath)
	assert.Equal(t, "flux", gotBody["query_language"])
	assert.Equal(t, `from(bucket:"b") |> range(start:-1h)`, gotBody["query"])
	_, hasParams := gotBody["params"]
	assert.False(t, hasParams, "param 미지정 시 params 키가 없어야 합니다")

	out := buf.String()
	assert.Contains(t, out, "TIMESTAMP")
	assert.Contains(t, out, "VALUE")
	assert.Contains(t, out, "LABELS")
	assert.Contains(t, out, "1700000000000")
	assert.NotContains(t, out, "1.7e")
	// labels 는 정렬되어 "host=server1,region=kr" 로 압축 렌더링된다.
	assert.Contains(t, out, "host=server1,region=kr")
	// 요약 라인.
	assert.Contains(t, out, "Count:")
	assert.Contains(t, out, "Truncated:")
}

func TestInfluxdbQuery_InfluxQLWithParams(t *testing.T) {
	var gotBody map[string]any

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		require.NoError(t, json.Unmarshal(body, &gotBody))
		w.Header().Set("Content-Type", "application/json")
		w.Write(influxEnvelope(sampleInfluxResult()))
	})

	_, cmd, cleanup := setupInfluxdbTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{
		"influxdb", "query", "my-influx",
		"--lang", "influxql",
		"--query", "SELECT * FROM cpu",
		"--param", "host=server1",
		"--param", "limit=10",
	})
	require.NoError(t, cmd.Execute())

	assert.Equal(t, "influxql", gotBody["query_language"])
	assert.Equal(t, "SELECT * FROM cpu", gotBody["query"])

	params, ok := gotBody["params"].(map[string]any)
	require.True(t, ok, "params 가 객체여야 합니다")
	assert.Equal(t, "server1", params["host"])
	// limit=10 은 정수로 파싱됨.
	assert.Equal(t, float64(10), params["limit"])
}

func TestInfluxdbQuery_AgentNameEscaped(t *testing.T) {
	var gotEscapedPath string

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// r.URL.Path 는 디코딩된 경로이므로, 전송된 raw(인코딩) 경로는 EscapedPath() 로 확인한다.
		gotEscapedPath = r.URL.EscapedPath()
		w.Header().Set("Content-Type", "application/json")
		w.Write(influxEnvelope(sampleInfluxResult()))
	})

	_, cmd, cleanup := setupInfluxdbTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"influxdb", "query", "agent name", "--query", "q"})
	require.NoError(t, cmd.Execute())

	// 공백이 포함된 에이전트명은 URL 인코딩되어 전송되어야 한다.
	assert.Equal(t, "/api/v1/influxdb/agent%20name/query", gotEscapedPath)
}

func TestInfluxdbQuery_JSONFormat(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(influxEnvelope(sampleInfluxResult()))
	})

	buf, cmd, cleanup := setupInfluxdbTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"--format", "json", "influxdb", "query", "my-influx", "--query", "q"})
	require.NoError(t, cmd.Execute())

	var parsed map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &parsed))
	assert.Equal(t, float64(2), parsed["count"])
	entries, ok := parsed["entries"].([]any)
	require.True(t, ok)
	assert.Len(t, entries, 2)
}

func TestInfluxdbQuery_MissingQuery(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("query 미지정 시 서버를 호출하면 안 됩니다")
	})

	_, cmd, cleanup := setupInfluxdbTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"influxdb", "query", "my-influx"})
	require.Error(t, cmd.Execute(), "--query 는 필수 플래그입니다")
}

func TestInfluxdbQuery_InvalidLang(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("잘못된 lang 이면 서버를 호출하면 안 됩니다")
	})

	_, cmd, cleanup := setupInfluxdbTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"influxdb", "query", "my-influx", "--query", "q", "--lang", "sql"})
	require.Error(t, cmd.Execute())
}

func TestInfluxdbQuery_InvalidParamFormat(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("잘못된 파라미터 형식이면 서버를 호출하면 안 됩니다")
	})

	_, cmd, cleanup := setupInfluxdbTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"influxdb", "query", "my-influx", "--query", "q", "--param", "badparam"})
	require.Error(t, cmd.Execute())
}

func TestInfluxdbQuery_NoArgs(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("에이전트명 미지정 시 서버를 호출하면 안 됩니다")
	})

	_, cmd, cleanup := setupInfluxdbTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"influxdb", "query"})
	require.Error(t, cmd.Execute(), "에이전트명 인자가 필요합니다 (ExactArgs(1))")
}

// --- buildInfluxQueryBody 단위 테스트 ---

func TestBuildInfluxQueryBody(t *testing.T) {
	tests := []struct {
		name     string
		query    string
		lang     string
		params   []string
		wantErr  bool
		wantLang string
	}{
		{"flux default", "q", "flux", nil, false, "flux"},
		{"influxql", "q", "influxql", nil, false, "influxql"},
		{"empty lang defaults to flux", "q", "", nil, false, "flux"},
		{"empty query", "", "flux", nil, true, ""},
		{"whitespace query", "   ", "flux", nil, true, ""},
		{"invalid lang", "q", "sql", nil, true, ""},
		{"bad param", "q", "flux", []string{"noequals"}, true, ""},
		{"valid param", "q", "flux", []string{"k=v"}, false, "flux"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, err := buildInfluxQueryBody(tt.query, tt.lang, tt.params)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantLang, body["query_language"])
			assert.Equal(t, "q", body["query"])
		})
	}
}

// --- formatLabels 단위 테스트 ---

func TestFormatLabels(t *testing.T) {
	tests := []struct {
		name string
		in   any
		want string
	}{
		{"sorted multi", map[string]any{"region": "kr", "host": "s1"}, "host=s1,region=kr"},
		{"single", map[string]any{"host": "s1"}, "host=s1"},
		{"empty map", map[string]any{}, ""},
		{"nil", nil, ""},
		{"not a map", "oops", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, formatLabels(tt.in))
		})
	}
}

// --- extractInfluxEntries 단위 테스트 ---

func TestExtractInfluxEntries(t *testing.T) {
	// JSON 디코딩 결과와 동일하게 []any 로 정규화.
	resp := map[string]any{
		"entries": []any{
			any(map[string]any{"timestamp": float64(1), "value": float64(2)}),
		},
	}
	got := extractInfluxEntries(resp)
	assert.Len(t, got, 1)

	assert.Nil(t, extractInfluxEntries(map[string]any{}))
	assert.Nil(t, extractInfluxEntries(map[string]any{"entries": "oops"}))
}
