package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- 테스트 헬퍼 ---

// setupTsdbTest 는 mock 서버와 tsdb 커맨드를 세팅하는 헬퍼이다.
// confirmFn 은 항상 true (삭제 승인) 로 동작한다.
func setupTsdbTest(t *testing.T, handler http.HandlerFunc) (*bytes.Buffer, *cobra.Command, func()) {
	t.Helper()
	return setupTsdbTestWithConfirm(t, handler, func(string, io.Reader) bool { return true })
}

// setupTsdbTestWithConfirm 은 confirmFn 을 커스텀할 수 있는 세팅 헬퍼이다.
func setupTsdbTestWithConfirm(t *testing.T, handler http.HandlerFunc, confirmFn func(string, io.Reader) bool) (*bytes.Buffer, *cobra.Command, func()) {
	t.Helper()

	server := httptest.NewServer(handler)

	client := NewClient(server.URL, "test-token", 5*time.Second, false)
	clientPtr := &client

	rootCmd := &cobra.Command{Use: "xflow"}
	rootCmd.PersistentFlags().String("format", "table", "출력 형식")
	rootCmd.AddCommand(newTsdbCmd(clientPtr, confirmFn))

	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)

	return &buf, rootCmd, server.Close
}

// --- tsdb series 테스트 ---

// TestTsdbSeries - 시리즈 목록 조회: 경로/메서드/응답 매핑 검증
func TestTsdbSeries(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/tsdb/series", r.URL.Path)
		assert.Equal(t, http.MethodGet, r.Method)
		w.Header().Set("Content-Type", "application/json")
		w.Write(apiEnvelope(map[string]any{
			"series": []string{"cpu,host=a", "mem,host=b"},
			"count":  2,
		}))
	})

	buf, cmd, cleanup := setupTsdbTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"tsdb", "series"})
	require.NoError(t, cmd.Execute())

	out := buf.String()
	assert.Contains(t, out, "SERIES KEY")
	assert.Contains(t, out, "cpu,host=a")
	assert.Contains(t, out, "mem,host=b")
}

// TestTsdbSeriesWithFilters - measurement/tag 필터가 쿼리 스트링으로 인코딩되는지 검증
func TestTsdbSeriesWithFilters(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/tsdb/series", r.URL.Path)
		assert.Equal(t, "cpu", r.URL.Query().Get("measurement"))
		assert.Equal(t, "seoul", r.URL.Query().Get("region"))
		assert.Equal(t, "a", r.URL.Query().Get("host"))
		w.Header().Set("Content-Type", "application/json")
		w.Write(apiEnvelope(map[string]any{"series": []string{"cpu,host=a"}, "count": 1}))
	})

	buf, cmd, cleanup := setupTsdbTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"tsdb", "series", "--measurement", "cpu", "--tag", "region=seoul", "--tag", "host=a"})
	require.NoError(t, cmd.Execute())
	assert.Contains(t, buf.String(), "cpu,host=a")
}

// TestTsdbSeriesInvalidTag - 잘못된 태그 형식은 에러를 반환한다
func TestTsdbSeriesInvalidTag(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("서버는 호출되지 않아야 합니다")
	})

	_, cmd, cleanup := setupTsdbTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"tsdb", "series", "--tag", "invalidtag"})
	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "key=value")
}

// --- tsdb latest 테스트 ---

// TestTsdbLatest - 최신 포인트 조회: 경로/메서드/n 파라미터 검증
func TestTsdbLatest(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/tsdb/series/cpu/latest", r.URL.Path)
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "5", r.URL.Query().Get("n"))
		w.Header().Set("Content-Type", "application/json")
		w.Write(apiEnvelope([]map[string]any{
			{"timestamp": "2026-01-01T00:00:00Z", "fields": map[string]any{"value": 42.0}},
		}))
	})

	buf, cmd, cleanup := setupTsdbTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"tsdb", "latest", "cpu", "--n", "5"})
	require.NoError(t, cmd.Execute())

	out := buf.String()
	assert.Contains(t, out, "TIMESTAMP")
	assert.Contains(t, out, "2026-01-01T00:00:00Z")
	assert.Contains(t, out, "value=42")
}

// TestTsdbLatestDefaultN - n 기본값 1 이면 쿼리 스트링이 없어야 한다
func TestTsdbLatestDefaultN(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/tsdb/series/cpu/latest", r.URL.Path)
		assert.Equal(t, "", r.URL.RawQuery, "n=1 이면 쿼리 스트링이 없어야 합니다")
		w.Header().Set("Content-Type", "application/json")
		w.Write(apiEnvelope([]map[string]any{}))
	})

	_, cmd, cleanup := setupTsdbTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"tsdb", "latest", "cpu"})
	require.NoError(t, cmd.Execute())
}

// TestTsdbLatestKeyEscaping - 특수문자(콜론/쉼표/등호)를 포함한 키가 URL 이스케이프되는지 검증
func TestTsdbLatestKeyEscaping(t *testing.T) {
	const rawKey = "cpu,host=a:b/c"

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 서버가 PathValue 로 디코딩한 결과(r.URL.Path)는 원본 키와 일치해야 한다.
		assert.Equal(t, "/api/v1/tsdb/series/"+rawKey+"/latest", r.URL.Path)
		// 전송된 raw 경로는 이스케이프된 형태여야 한다.
		// url.PathEscape 는 경로 세그먼트에서 허용되는 '='/':' 는 인코딩하지 않지만,
		// 경로 구분자로 오인될 수 있는 '/' 와 시리즈 구분자 ',' 는 반드시 인코딩한다.
		escaped := r.URL.EscapedPath()
		assert.Contains(t, escaped, "%2C") // 쉼표
		assert.Contains(t, escaped, "%2F") // 슬래시 (경로 구분자로 오인 방지)
		w.Header().Set("Content-Type", "application/json")
		w.Write(apiEnvelope([]map[string]any{}))
	})

	_, cmd, cleanup := setupTsdbTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"tsdb", "latest", rawKey})
	require.NoError(t, cmd.Execute())
}

// --- tsdb query 테스트 ---

// TestTsdbQuery - 쿼리 조회: 경로/메서드/바디 매핑 검증
func TestTsdbQuery(t *testing.T) {
	var received map[string]any

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/tsdb/query", r.URL.Path)
		assert.Equal(t, http.MethodPost, r.Method)
		body, _ := io.ReadAll(r.Body)
		require.NoError(t, json.Unmarshal(body, &received))
		w.Header().Set("Content-Type", "application/json")
		w.Write(apiEnvelope(map[string]any{
			"results": []map[string]any{
				{"series_key": "cpu,host=a", "points": []any{}},
			},
		}))
	})

	buf, cmd, cleanup := setupTsdbTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{
		"tsdb", "query",
		"--series", "cpu,host=a",
		"--measurement", "cpu",
		"--field", "value",
		"--tag", "host=a",
		"--start", "2026-01-01T00:00:00Z",
		"--end", "2026-01-02T00:00:00Z",
		"--last", "10",
		"--aggregation", "avg",
		"--bucket", "5m",
		"--fill", "null",
		"--format", "json",
	})
	require.NoError(t, cmd.Execute())

	// 바디 매핑 검증
	assert.Equal(t, "cpu,host=a", received["series_key"])
	assert.Equal(t, "cpu", received["measurement"])
	assert.Equal(t, "value", received["field"])
	assert.Equal(t, "2026-01-01T00:00:00Z", received["start"])
	assert.Equal(t, "2026-01-02T00:00:00Z", received["end"])
	assert.Equal(t, float64(10), received["limit"])
	assert.Equal(t, "avg", received["aggregation"])
	assert.Equal(t, "5m", received["bucket"])
	assert.Equal(t, "null", received["fill"])
	tags, ok := received["tags"].(map[string]any)
	require.True(t, ok, "tags 가 객체여야 합니다")
	assert.Equal(t, "a", tags["host"])

	assert.Contains(t, buf.String(), "cpu,host=a")
}

// TestTsdbQueryOmitsEmpty - 지정하지 않은 플래그는 바디에서 제외되어야 한다
func TestTsdbQueryOmitsEmpty(t *testing.T) {
	var received map[string]any

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		require.NoError(t, json.Unmarshal(body, &received))
		w.Header().Set("Content-Type", "application/json")
		w.Write(apiEnvelope(map[string]any{"results": []any{}}))
	})

	_, cmd, cleanup := setupTsdbTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"tsdb", "query", "--series", "cpu", "--format", "json"})
	require.NoError(t, cmd.Execute())

	assert.Equal(t, "cpu", received["series_key"])
	_, hasMeasurement := received["measurement"]
	assert.False(t, hasMeasurement, "measurement 미지정 시 바디에 없어야 합니다")
	_, hasLimit := received["limit"]
	assert.False(t, hasLimit, "limit 미지정 시 바디에 없어야 합니다")
	_, hasFill := received["fill"]
	assert.False(t, hasFill, "fill 미지정 시 바디에 없어야 합니다")
}

// TestTsdbQueryWithKeyAlias - --key 별칭이 series_key 로 매핑되는지 검증
func TestTsdbQueryWithKeyAlias(t *testing.T) {
	var received map[string]any

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		require.NoError(t, json.Unmarshal(body, &received))
		w.Header().Set("Content-Type", "application/json")
		w.Write(apiEnvelope(map[string]any{"results": []any{}}))
	})

	_, cmd, cleanup := setupTsdbTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"tsdb", "query", "--key", "disk,host=x", "--format", "json"})
	require.NoError(t, cmd.Execute())
	assert.Equal(t, "disk,host=x", received["series_key"])
}

// TestTsdbQueryRawJSON - --json 파일이 바디로 그대로 전달되는지 검증
func TestTsdbQueryRawJSON(t *testing.T) {
	var received map[string]any

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/tsdb/query", r.URL.Path)
		body, _ := io.ReadAll(r.Body)
		require.NoError(t, json.Unmarshal(body, &received))
		w.Header().Set("Content-Type", "application/json")
		w.Write(apiEnvelope(map[string]any{"results": []any{}}))
	})

	_, cmd, cleanup := setupTsdbTest(t, handler)
	defer cleanup()

	dir := t.TempDir()
	jsonPath := filepath.Join(dir, "query.json")
	require.NoError(t, os.WriteFile(jsonPath, []byte(`{"series_key":"raw,host=z","limit":3}`), 0644))

	cmd.SetArgs([]string{"tsdb", "query", "--json", jsonPath, "--format", "json"})
	require.NoError(t, cmd.Execute())

	assert.Equal(t, "raw,host=z", received["series_key"])
	assert.Equal(t, float64(3), received["limit"])
}

// --- tsdb stats 테스트 ---

// TestTsdbStats - 통계 조회: 경로/메서드/출력 검증
func TestTsdbStats(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/tsdb/stats", r.URL.Path)
		assert.Equal(t, http.MethodGet, r.Method)
		w.Header().Set("Content-Type", "application/json")
		w.Write(apiEnvelope(map[string]any{
			"series_count": 3,
			"total_points": 1500,
			"memory_bytes": 1048576,
			"memory_human": "1.0 MB",
		}))
	})

	buf, cmd, cleanup := setupTsdbTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"tsdb", "stats"})
	require.NoError(t, cmd.Execute())

	out := buf.String()
	assert.Contains(t, out, "Series Count")
	assert.Contains(t, out, "Memory")
	assert.Contains(t, out, "1.0 MB")
}

// --- tsdb delete 테스트 ---

// TestTsdbDelete - 삭제: 경로/메서드/확인 후 호출 검증
func TestTsdbDelete(t *testing.T) {
	called := false
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		assert.Equal(t, "/api/v1/tsdb/series/cpu,host=a", r.URL.Path)
		assert.Equal(t, http.MethodDelete, r.Method)
		w.WriteHeader(http.StatusNoContent)
	})

	buf, cmd, cleanup := setupTsdbTest(t, handler) // confirmFn -> true
	defer cleanup()

	cmd.SetArgs([]string{"tsdb", "delete", "cpu,host=a"})
	require.NoError(t, cmd.Execute())

	assert.True(t, called, "삭제 API 가 호출되어야 합니다")
	assert.Contains(t, buf.String(), "삭제되었습니다")
}

// TestTsdbDeleteCancelled - 확인 거부 시 API 가 호출되지 않아야 한다
func TestTsdbDeleteCancelled(t *testing.T) {
	called := false
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	})

	buf, cmd, cleanup := setupTsdbTestWithConfirm(t, handler, func(string, io.Reader) bool { return false })
	defer cleanup()

	cmd.SetArgs([]string{"tsdb", "delete", "cpu,host=a"})
	require.NoError(t, cmd.Execute())

	assert.False(t, called, "확인 거부 시 API 가 호출되지 않아야 합니다")
	assert.Contains(t, buf.String(), "취소")
}

// TestTsdbDeleteYesFlag - --yes 플래그는 확인을 건너뛰고 바로 삭제한다
func TestTsdbDeleteYesFlag(t *testing.T) {
	called := false
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	})

	// confirmFn 이 false 여도 --yes 면 삭제되어야 한다
	_, cmd, cleanup := setupTsdbTestWithConfirm(t, handler, func(string, io.Reader) bool { return false })
	defer cleanup()

	cmd.SetArgs([]string{"tsdb", "delete", "cpu,host=a", "--yes"})
	require.NoError(t, cmd.Execute())
	assert.True(t, called, "--yes 면 확인 없이 삭제되어야 합니다")
}

// TestTsdbDeleteKeyEscaping - 특수문자 키가 DELETE 경로에서 이스케이프되는지 검증
func TestTsdbDeleteKeyEscaping(t *testing.T) {
	const rawKey = "cpu,host=a:b/c"
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/tsdb/series/"+rawKey, r.URL.Path)
		escaped := r.URL.EscapedPath()
		assert.Contains(t, escaped, "%2C")
		assert.Contains(t, escaped, "%2F")
		w.WriteHeader(http.StatusNoContent)
	})

	_, cmd, cleanup := setupTsdbTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"tsdb", "delete", rawKey})
	require.NoError(t, cmd.Execute())
}

// --- tsdb write 테스트 ---

// TestTsdbWrite - 쓰기: 경로/메서드/단일 포인트 바디 래핑 검증
func TestTsdbWrite(t *testing.T) {
	var received map[string]any

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/tsdb/write", r.URL.Path)
		assert.Equal(t, http.MethodPost, r.Method)
		body, _ := io.ReadAll(r.Body)
		require.NoError(t, json.Unmarshal(body, &received))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		w.Write(apiEnvelope(map[string]any{"written": 1}))
	})

	buf, cmd, cleanup := setupTsdbTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{
		"tsdb", "write",
		"--measurement", "cpu",
		"--field", "value=42.5",
		"--field", "online=true",
		"--tag", "host=a",
		"--timestamp", "2026-01-01T00:00:00Z",
	})
	require.NoError(t, cmd.Execute())

	points, ok := received["points"].([]any)
	require.True(t, ok, "points 가 배열이어야 합니다")
	require.Len(t, points, 1)
	point := points[0].(map[string]any)
	assert.Equal(t, "cpu", point["measurement"])
	assert.Equal(t, "2026-01-01T00:00:00Z", point["timestamp"])

	fields := point["fields"].(map[string]any)
	assert.Equal(t, float64(42.5), fields["value"], "숫자 필드는 number 로 변환되어야 합니다")
	assert.Equal(t, true, fields["online"], "불리언 필드는 bool 로 변환되어야 합니다")

	tags := point["tags"].(map[string]any)
	assert.Equal(t, "a", tags["host"])

	assert.Contains(t, buf.String(), "기록했습니다")
}

// TestTsdbWriteRawJSON - --json 파일이 바디로 그대로 전달되는지 검증
func TestTsdbWriteRawJSON(t *testing.T) {
	var received map[string]any

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/tsdb/write", r.URL.Path)
		body, _ := io.ReadAll(r.Body)
		require.NoError(t, json.Unmarshal(body, &received))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		w.Write(apiEnvelope(map[string]any{"written": 2}))
	})

	_, cmd, cleanup := setupTsdbTest(t, handler)
	defer cleanup()

	dir := t.TempDir()
	jsonPath := filepath.Join(dir, "write.json")
	raw := `{"points":[{"measurement":"cpu","fields":{"v":1}},{"measurement":"mem","fields":{"v":2}}]}`
	require.NoError(t, os.WriteFile(jsonPath, []byte(raw), 0644))

	cmd.SetArgs([]string{"tsdb", "write", "--json", jsonPath})
	require.NoError(t, cmd.Execute())

	points, ok := received["points"].([]any)
	require.True(t, ok)
	assert.Len(t, points, 2)
}

// TestTsdbWriteMissingMeasurement - measurement 누락 시 에러 (서버 미호출)
func TestTsdbWriteMissingMeasurement(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("서버는 호출되지 않아야 합니다")
	})

	_, cmd, cleanup := setupTsdbTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"tsdb", "write", "--field", "value=1"})
	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "measurement")
}

// --- 순수 함수 단위 테스트 ---

// TestParseTagFlags - 태그 파싱 검증
func TestParseTagFlags(t *testing.T) {
	tags, err := parseTagFlags([]string{"host=a", "region=seoul=east"})
	require.NoError(t, err)
	assert.Equal(t, "a", tags["host"])
	assert.Equal(t, "seoul=east", tags["region"], "첫 등호만 분리해야 합니다")

	_, err = parseTagFlags([]string{"noequal"})
	assert.Error(t, err)

	_, err = parseTagFlags([]string{"=value"})
	assert.Error(t, err)
}

// TestCoerceFieldValue - 필드 값 타입 변환 검증
func TestCoerceFieldValue(t *testing.T) {
	assert.Equal(t, float64(42), coerceFieldValue("42"))
	assert.Equal(t, 3.14, coerceFieldValue("3.14"))
	assert.Equal(t, true, coerceFieldValue("true"))
	assert.Equal(t, false, coerceFieldValue("false"))
	assert.Equal(t, "hello", coerceFieldValue("hello"))
	assert.Equal(t, "42abc", coerceFieldValue("42abc"), "부분 숫자는 문자열로 둬야 합니다")
}

// TestBuildTsdbSeriesPath - 시리즈 경로 빌더 검증
func TestBuildTsdbSeriesPath(t *testing.T) {
	path, err := buildTsdbSeriesPath("", nil)
	require.NoError(t, err)
	assert.Equal(t, "/api/v1/tsdb/series", path, "필터 없으면 쿼리 스트링이 없어야 합니다")

	path, err = buildTsdbSeriesPath("cpu", map[string]string{"host": "a"})
	require.NoError(t, err)
	assert.Contains(t, path, "measurement=cpu")
	assert.Contains(t, path, "host=a")
}
