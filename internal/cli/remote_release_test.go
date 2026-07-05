package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testReleaseVersion = "v0.19.0"

// sampleReleasesObject 는 테스트용 릴리스 목록 응답 객체({"releases":[...]})를 반환한다.
// 목록 엔드포인트는 배열이 아니라 객체로 응답한다.
func sampleReleasesObject() map[string]any {
	return map[string]any{
		"releases": []any{
			map[string]any{
				"version":         testReleaseVersion,
				"channel":         "stable",
				"notes":           "버그 수정",
				"published_at_ms": float64(1700000000000),
				"assets": []any{
					map[string]any{
						"os":             "linux",
						"arch":           "amd64",
						"filename":       "xflow-linux-amd64",
						"size":           float64(1024),
						"sha256":         "abc123",
						"has_sig":        true,
						"uploaded_at_ms": float64(1700000001000),
					},
					map[string]any{
						"os":             "linux",
						"arch":           "arm64",
						"filename":       "xflow-linux-arm64",
						"size":           float64(2048),
						"sha256":         "def456",
						"has_sig":        false,
						"uploaded_at_ms": float64(1700000002000),
					},
				},
			},
			map[string]any{
				"version":         "v0.18.0",
				"channel":         "beta",
				"notes":           "",
				"published_at_ms": float64(1699000000000),
				"assets":          []any{},
			},
		},
	}
}

// --- remote release list ---

func TestRemoteReleaseList(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/api/v1/remote/releases", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		w.Write(remoteEnvelope(sampleReleasesObject()))
	})

	buf, cmd, cleanup := setupRemoteTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"remote", "release", "list"})
	require.NoError(t, cmd.Execute())

	out := buf.String()
	for _, h := range []string{"VERSION", "CHANNEL", "PUBLISHED_AT", "ASSETS", "NOTES"} {
		assert.Contains(t, out, h)
	}
	assert.Contains(t, out, testReleaseVersion)
	assert.Contains(t, out, "v0.18.0")
	assert.Contains(t, out, "stable")
	// published_at_ms 는 epoch ms 정수 문자열로 표시되어야 한다(과학표기 금지).
	assert.Contains(t, out, "1700000000000")
	// assets 개수(첫 릴리스 2개)가 표시되어야 한다.
	assert.Contains(t, out, "2")
}

func TestRemoteReleaseList_JSONFormat(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/remote/releases", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		w.Write(remoteEnvelope(sampleReleasesObject()))
	})

	buf, cmd, cleanup := setupRemoteTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"--format", "json", "remote", "release", "list"})
	require.NoError(t, cmd.Execute())

	// json 포맷은 전체 객체({releases:[...]})를 그대로 통과시킨다.
	var parsed map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &parsed))
	releases, ok := parsed["releases"].([]any)
	require.True(t, ok, "json passthrough 는 releases 배열을 포함해야 합니다")
	require.Len(t, releases, 2)
}

func TestRemoteReleaseList_EmptyOrMissing(t *testing.T) {
	// releases 키가 없거나 빈 경우에도 안전하게 처리되어야 한다.
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(remoteEnvelope(map[string]any{}))
	})

	buf, cmd, cleanup := setupRemoteTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"remote", "release", "list"})
	require.NoError(t, cmd.Execute())
	// 헤더는 여전히 표시되어야 한다.
	assert.Contains(t, buf.String(), "VERSION")
}

// --- remote release create ---

func TestRemoteReleaseCreate(t *testing.T) {
	var gotPath, gotMethod string
	var gotBody map[string]any

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		body, _ := io.ReadAll(r.Body)
		require.NoError(t, json.Unmarshal(body, &gotBody))
		w.Header().Set("Content-Type", "application/json")
		w.Write(remoteEnvelope(map[string]any{
			"version":         testReleaseVersion,
			"channel":         "stable",
			"notes":           "버그 수정",
			"published_at_ms": float64(1700000000000),
			"assets":          []any{},
		}))
	})

	buf, cmd, cleanup := setupRemoteTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{
		"remote", "release", "create", testReleaseVersion,
		"--channel", "stable", "--notes", "버그 수정",
	})
	require.NoError(t, cmd.Execute())

	assert.Equal(t, http.MethodPost, gotMethod)
	assert.Equal(t, "/api/v1/remote/releases", gotPath)
	assert.Equal(t, testReleaseVersion, gotBody["version"])
	assert.Equal(t, "stable", gotBody["channel"])
	assert.Equal(t, "버그 수정", gotBody["notes"])

	out := buf.String()
	assert.Contains(t, out, testReleaseVersion)
	assert.Contains(t, out, "생성")
}

func TestRemoteReleaseCreate_JSONFormat(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(remoteEnvelope(map[string]any{
			"version":         testReleaseVersion,
			"channel":         "stable",
			"published_at_ms": float64(1700000000000),
			"assets":          []any{},
		}))
	})

	buf, cmd, cleanup := setupRemoteTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"--format", "json", "remote", "release", "create", testReleaseVersion})
	require.NoError(t, cmd.Execute())

	var parsed map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &parsed))
	assert.Equal(t, testReleaseVersion, parsed["version"])
}

func TestRemoteReleaseCreate_NoFlagsOmitsFields(t *testing.T) {
	var gotBody map[string]any

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		require.NoError(t, json.Unmarshal(body, &gotBody))
		w.Header().Set("Content-Type", "application/json")
		w.Write(remoteEnvelope(map[string]any{"version": testReleaseVersion, "assets": []any{}}))
	})

	_, cmd, cleanup := setupRemoteTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"remote", "release", "create", testReleaseVersion})
	require.NoError(t, cmd.Execute())

	// version 은 인자에서 항상 포함되어야 한다.
	assert.Equal(t, testReleaseVersion, gotBody["version"])
	// 미지정 플래그는 본문에서 생략되어야 한다.
	for _, k := range []string{"channel", "notes"} {
		_, ok := gotBody[k]
		assert.False(t, ok, "플래그 미지정 시 %q 필드는 생략되어야 합니다", k)
	}
}

func TestRemoteReleaseCreate_EmptyVersion(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("빈 version 이면 서버를 호출하면 안 됩니다")
	})

	_, cmd, cleanup := setupRemoteTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"remote", "release", "create", "  "})
	require.Error(t, cmd.Execute())
}

// --- remote release delete ---

func TestRemoteReleaseDelete_Confirmed(t *testing.T) {
	var gotPath, gotMethod string

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		w.Header().Set("Content-Type", "application/json")
		w.Write(remoteEnvelope(map[string]any{"version": testReleaseVersion, "deleted": true}))
	})

	buf, cmd, cleanup := setupRemoteTestWithConfirm(t, handler, func(string, io.Reader) bool { return true })
	defer cleanup()

	cmd.SetArgs([]string{"remote", "release", "delete", testReleaseVersion})
	require.NoError(t, cmd.Execute())

	assert.Equal(t, http.MethodDelete, gotMethod)
	assert.Equal(t, "/api/v1/remote/releases/"+testReleaseVersion, gotPath)
	out := buf.String()
	assert.Contains(t, out, "삭제")
	assert.Contains(t, out, testReleaseVersion)
}

func TestRemoteReleaseDelete_Cancelled(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("확인이 거부되면 서버를 호출하면 안 됩니다")
	})

	buf, cmd, cleanup := setupRemoteTestWithConfirm(t, handler, func(string, io.Reader) bool { return false })
	defer cleanup()

	cmd.SetArgs([]string{"remote", "release", "delete", testReleaseVersion})
	require.NoError(t, cmd.Execute())
	assert.Contains(t, buf.String(), "취소되었습니다")
}

func TestRemoteReleaseDelete_YesSkipsConfirm(t *testing.T) {
	var called bool

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.Header().Set("Content-Type", "application/json")
		w.Write(remoteEnvelope(map[string]any{"version": testReleaseVersion, "deleted": true}))
	})

	_, cmd, cleanup := setupRemoteTestWithConfirm(t, handler, func(string, io.Reader) bool { return false })
	defer cleanup()

	cmd.SetArgs([]string{"remote", "release", "delete", testReleaseVersion, "--yes"})
	require.NoError(t, cmd.Execute())
	assert.True(t, called, "--yes 플래그는 확인 없이 삭제를 진행해야 합니다")
}

func TestRemoteReleaseDelete_JSONFormat(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(remoteEnvelope(map[string]any{"version": testReleaseVersion, "deleted": true}))
	})

	buf, cmd, cleanup := setupRemoteTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"--format", "json", "remote", "release", "delete", testReleaseVersion, "--yes"})
	require.NoError(t, cmd.Execute())

	var parsed map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &parsed))
	assert.Equal(t, true, parsed["deleted"])
}

func TestRemoteReleaseDelete_EmptyVersion(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("빈 version 이면 서버를 호출하면 안 됩니다")
	})

	_, cmd, cleanup := setupRemoteTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"remote", "release", "delete", "  ", "--yes"})
	require.Error(t, cmd.Execute())
}

// --- remote release delete-asset ---

func TestRemoteReleaseDeleteAsset_Confirmed(t *testing.T) {
	var gotPath, gotMethod string

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		w.Header().Set("Content-Type", "application/json")
		w.Write(remoteEnvelope(map[string]any{
			"version": testReleaseVersion,
			"os":      "linux",
			"arch":    "amd64",
			"deleted": true,
		}))
	})

	buf, cmd, cleanup := setupRemoteTestWithConfirm(t, handler, func(string, io.Reader) bool { return true })
	defer cleanup()

	cmd.SetArgs([]string{"remote", "release", "delete-asset", testReleaseVersion, "linux", "amd64"})
	require.NoError(t, cmd.Execute())

	assert.Equal(t, http.MethodDelete, gotMethod)
	assert.Equal(t, "/api/v1/remote/releases/"+testReleaseVersion+"/assets/linux/amd64", gotPath)
	out := buf.String()
	assert.Contains(t, out, "삭제")
	assert.Contains(t, out, "linux")
	assert.Contains(t, out, "amd64")
}

func TestRemoteReleaseDeleteAsset_Cancelled(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("확인이 거부되면 서버를 호출하면 안 됩니다")
	})

	buf, cmd, cleanup := setupRemoteTestWithConfirm(t, handler, func(string, io.Reader) bool { return false })
	defer cleanup()

	cmd.SetArgs([]string{"remote", "release", "delete-asset", testReleaseVersion, "linux", "amd64"})
	require.NoError(t, cmd.Execute())
	assert.Contains(t, buf.String(), "취소되었습니다")
}

func TestRemoteReleaseDeleteAsset_YesSkipsConfirm(t *testing.T) {
	var called bool

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.Header().Set("Content-Type", "application/json")
		w.Write(remoteEnvelope(map[string]any{"deleted": true}))
	})

	_, cmd, cleanup := setupRemoteTestWithConfirm(t, handler, func(string, io.Reader) bool { return false })
	defer cleanup()

	cmd.SetArgs([]string{"remote", "release", "delete-asset", testReleaseVersion, "linux", "amd64", "--yes"})
	require.NoError(t, cmd.Execute())
	assert.True(t, called, "--yes 플래그는 확인 없이 삭제를 진행해야 합니다")
}

// TestRemoteReleaseDeleteAsset_EscapesSegments 는 version/os/arch 가 각각 URL 경로로
// 이스케이프되는지 검증한다.
func TestRemoteReleaseDeleteAsset_EscapesSegments(t *testing.T) {
	var gotRawPath string

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotRawPath = r.URL.EscapedPath()
		w.Header().Set("Content-Type", "application/json")
		w.Write(remoteEnvelope(map[string]any{"deleted": true}))
	})

	_, cmd, cleanup := setupRemoteTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"remote", "release", "delete-asset", "v 1", "linux", "x 86", "--yes"})
	require.NoError(t, cmd.Execute())

	// 공백이 %20 으로 이스케이프되어야 한다.
	assert.Equal(t, "/api/v1/remote/releases/v%201/assets/linux/x%2086", gotRawPath)
}

func TestRemoteReleaseDeleteAsset_EmptyArgs(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("빈 인자면 서버를 호출하면 안 됩니다")
	})

	_, cmd, cleanup := setupRemoteTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"remote", "release", "delete-asset", testReleaseVersion, "  ", "amd64", "--yes"})
	require.Error(t, cmd.Execute())
}
