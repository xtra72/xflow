package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- remote version target get ---

func TestRemoteVersionTargetGet(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/api/v1/remote/target-version", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		w.Write(remoteEnvelope(map[string]any{"version": "v0.19.0"}))
	})

	buf, cmd, cleanup := setupRemoteTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"remote", "version", "target", "get"})
	require.NoError(t, cmd.Execute())
	assert.Contains(t, buf.String(), "v0.19.0")
}

func TestRemoteVersionTargetGet_Unset(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(remoteEnvelope(map[string]any{"version": ""}))
	})

	buf, cmd, cleanup := setupRemoteTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"remote", "version", "target", "get"})
	require.NoError(t, cmd.Execute())
	// 빈 version 은 "(미설정)"으로 표시되어야 한다.
	assert.Contains(t, buf.String(), "(미설정)")
}

func TestRemoteVersionTargetGet_JSONFormat(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(remoteEnvelope(map[string]any{"version": "v0.19.0"}))
	})

	buf, cmd, cleanup := setupRemoteTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"--format", "json", "remote", "version", "target", "get"})
	require.NoError(t, cmd.Execute())

	var parsed map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &parsed))
	assert.Equal(t, "v0.19.0", parsed["version"])
}

// --- remote version target set ---

func TestRemoteVersionTargetSet(t *testing.T) {
	var gotPath, gotMethod string
	var gotBody map[string]any

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		body, _ := io.ReadAll(r.Body)
		require.NoError(t, json.Unmarshal(body, &gotBody))
		w.Header().Set("Content-Type", "application/json")
		w.Write(remoteEnvelope(map[string]any{"version": "v0.19.0"}))
	})

	buf, cmd, cleanup := setupRemoteTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"remote", "version", "target", "set", "v0.19.0"})
	require.NoError(t, cmd.Execute())

	assert.Equal(t, http.MethodPut, gotMethod)
	assert.Equal(t, "/api/v1/remote/target-version", gotPath)
	assert.Equal(t, "v0.19.0", gotBody["version"])
	assert.Contains(t, buf.String(), "v0.19.0")
}

func TestRemoteVersionTargetSet_EmptyUnsets(t *testing.T) {
	var gotBody map[string]any

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		require.NoError(t, json.Unmarshal(body, &gotBody))
		w.Header().Set("Content-Type", "application/json")
		w.Write(remoteEnvelope(map[string]any{"version": ""}))
	})

	buf, cmd, cleanup := setupRemoteTest(t, handler)
	defer cleanup()

	// 빈 문자열은 의도적 해제 의미로 그대로 전달되어야 한다.
	cmd.SetArgs([]string{"remote", "version", "target", "set", ""})
	require.NoError(t, cmd.Execute())

	v, ok := gotBody["version"]
	require.True(t, ok, "version 필드는 빈 값이라도 본문에 포함되어야 합니다")
	assert.Equal(t, "", v)
	assert.Contains(t, buf.String(), "해제")
}

func TestRemoteVersionTargetSet_JSONFormat(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(remoteEnvelope(map[string]any{"version": "v0.19.0"}))
	})

	buf, cmd, cleanup := setupRemoteTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"--format", "json", "remote", "version", "target", "set", "v0.19.0"})
	require.NoError(t, cmd.Execute())

	var parsed map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &parsed))
	assert.Equal(t, "v0.19.0", parsed["version"])
}

// --- remote version source get ---

func TestRemoteVersionSourceGet(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/api/v1/remote/update-source", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		w.Write(remoteEnvelope(map[string]any{
			"update_url": "https://updates.example.com",
			"channel":    "stable",
		}))
	})

	buf, cmd, cleanup := setupRemoteTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"remote", "version", "source", "get"})
	require.NoError(t, cmd.Execute())

	out := buf.String()
	assert.Contains(t, out, "https://updates.example.com")
	assert.Contains(t, out, "stable")
}

func TestRemoteVersionSourceGet_Unset(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(remoteEnvelope(map[string]any{"update_url": ""}))
	})

	buf, cmd, cleanup := setupRemoteTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"remote", "version", "source", "get"})
	require.NoError(t, cmd.Execute())
	// 빈 update_url 은 "(미설정)"으로 표시되어야 한다.
	assert.Contains(t, buf.String(), "(미설정)")
}

func TestRemoteVersionSourceGet_JSONFormat(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(remoteEnvelope(map[string]any{
			"update_url": "https://updates.example.com",
			"channel":    "stable",
		}))
	})

	buf, cmd, cleanup := setupRemoteTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"--format", "json", "remote", "version", "source", "get"})
	require.NoError(t, cmd.Execute())

	var parsed map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &parsed))
	assert.Equal(t, "https://updates.example.com", parsed["update_url"])
}

// --- remote version source set ---

func TestRemoteVersionSourceSet(t *testing.T) {
	var gotPath, gotMethod string
	var gotBody map[string]any

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		body, _ := io.ReadAll(r.Body)
		require.NoError(t, json.Unmarshal(body, &gotBody))
		w.Header().Set("Content-Type", "application/json")
		w.Write(remoteEnvelope(map[string]any{
			"update_url": "https://updates.example.com",
			"channel":    "stable",
		}))
	})

	buf, cmd, cleanup := setupRemoteTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{
		"remote", "version", "source", "set", "https://updates.example.com",
		"--channel", "stable",
	})
	require.NoError(t, cmd.Execute())

	assert.Equal(t, http.MethodPut, gotMethod)
	assert.Equal(t, "/api/v1/remote/update-source", gotPath)
	assert.Equal(t, "https://updates.example.com", gotBody["update_url"])
	assert.Equal(t, "stable", gotBody["channel"])
	assert.Contains(t, buf.String(), "https://updates.example.com")
}

func TestRemoteVersionSourceSet_NoChannelOmitsField(t *testing.T) {
	var gotBody map[string]any

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		require.NoError(t, json.Unmarshal(body, &gotBody))
		w.Header().Set("Content-Type", "application/json")
		w.Write(remoteEnvelope(map[string]any{"update_url": "https://updates.example.com"}))
	})

	_, cmd, cleanup := setupRemoteTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"remote", "version", "source", "set", "https://updates.example.com"})
	require.NoError(t, cmd.Execute())

	assert.Equal(t, "https://updates.example.com", gotBody["update_url"])
	_, hasChannel := gotBody["channel"]
	assert.False(t, hasChannel, "--channel 미지정 시 channel 필드는 생략되어야 합니다")
}

func TestRemoteVersionSourceSet_ServerValidationError(t *testing.T) {
	// 서버가 https:// 가 아닌 URL 을 거부하면 그 에러가 surface 되어야 한다.
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"success":false,"error":{"code":"invalid_input","message":"update_url 은 https:// 로 시작해야 합니다"}}`))
	})

	_, cmd, cleanup := setupRemoteTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"remote", "version", "source", "set", "http://insecure.example.com"})
	err := cmd.Execute()
	require.Error(t, err)
	// MapAPIError 가 서버의 검증 실패 메시지를 surface 한다.
	assert.Contains(t, err.Error(), "https://")
}

// --- remote version history ---

func sampleVersionHistory() []map[string]any {
	return []map[string]any{
		{"version": "v0.19.0", "changed_at": float64(1700000002000)},
		{"version": "v0.18.6", "changed_at": float64(1700000001000)},
		{"version": "v0.18.0", "changed_at": float64(1700000000000)},
	}
}

func TestRemoteVersionHistory(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/api/v1/remote/nodes/"+testInstanceID+"/version-history", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		w.Write(remoteEnvelope(sampleVersionHistory()))
	})

	buf, cmd, cleanup := setupRemoteTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"remote", "version", "history", testInstanceID})
	require.NoError(t, cmd.Execute())

	out := buf.String()
	assert.Contains(t, out, "VERSION")
	assert.Contains(t, out, "CHANGED_AT")
	assert.Contains(t, out, "v0.19.0")
	assert.Contains(t, out, "v0.18.0")
	// changed_at 은 epoch ms 정수 문자열로 표시(과학표기 금지).
	assert.Contains(t, out, "1700000002000")
}

func TestRemoteVersionHistory_WithLimit(t *testing.T) {
	var gotQuery string

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		w.Write(remoteEnvelope(sampleVersionHistory()))
	})

	_, cmd, cleanup := setupRemoteTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"remote", "version", "history", testInstanceID, "--limit", "50"})
	require.NoError(t, cmd.Execute())
	// limit query 파라미터가 추가되어야 한다.
	assert.Contains(t, gotQuery, "limit=50")
}

func TestRemoteVersionHistory_ZeroLimitOmitsQuery(t *testing.T) {
	var gotQuery string

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		w.Write(remoteEnvelope(sampleVersionHistory()))
	})

	_, cmd, cleanup := setupRemoteTest(t, handler)
	defer cleanup()

	// --limit 0 은 query 에 포함하지 않아야 한다(device history 패턴).
	cmd.SetArgs([]string{"remote", "version", "history", testInstanceID, "--limit", "0"})
	require.NoError(t, cmd.Execute())
	assert.Empty(t, gotQuery, "--limit 0 은 query 파라미터를 생성하지 않아야 합니다")
}

func TestRemoteVersionHistory_JSONFormat(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(remoteEnvelope(sampleVersionHistory()))
	})

	buf, cmd, cleanup := setupRemoteTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"--format", "json", "remote", "version", "history", testInstanceID})
	require.NoError(t, cmd.Execute())

	var parsed []map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &parsed))
	require.Len(t, parsed, 3)
	assert.Equal(t, "v0.19.0", parsed[0]["version"])
}

func TestRemoteVersionHistory_EmptyID(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("빈 instance_id 면 서버를 호출하면 안 됩니다")
	})

	_, cmd, cleanup := setupRemoteTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"remote", "version", "history", "  "})
	require.Error(t, cmd.Execute())
}

// --- remote version update ---

func TestRemoteVersionUpdate_Confirmed(t *testing.T) {
	var gotPath, gotMethod string
	var gotBody map[string]any

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		body, _ := io.ReadAll(r.Body)
		require.NoError(t, json.Unmarshal(body, &gotBody))
		w.Header().Set("Content-Type", "application/json")
		w.Write(remoteEnvelope(map[string]any{
			"instance_id":    testInstanceID,
			"target_version": "v0.19.0",
			"result":         "dispatched",
		}))
	})

	buf, cmd, cleanup := setupRemoteTestWithConfirm(t, handler, func(string, io.Reader) bool { return true })
	defer cleanup()

	cmd.SetArgs([]string{
		"remote", "version", "update", testInstanceID,
		"--version", "v0.19.0", "--restart",
	})
	require.NoError(t, cmd.Execute())

	assert.Equal(t, http.MethodPost, gotMethod)
	assert.Equal(t, "/api/v1/remote/nodes/"+testInstanceID+"/update", gotPath)
	assert.Equal(t, "v0.19.0", gotBody["version"])
	assert.Equal(t, true, gotBody["restart"])
	// 미지정 플래그는 본문에 포함되지 않아야 한다.
	_, hasChannel := gotBody["channel"]
	assert.False(t, hasChannel, "--channel 미지정 시 channel 필드는 생략되어야 합니다")
	out := buf.String()
	assert.Contains(t, out, testInstanceID)
	assert.Contains(t, out, "dispatched")
}

func TestRemoteVersionUpdate_Cancelled(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("확인이 거부되면 서버를 호출하면 안 됩니다")
	})

	buf, cmd, cleanup := setupRemoteTestWithConfirm(t, handler, func(string, io.Reader) bool { return false })
	defer cleanup()

	cmd.SetArgs([]string{"remote", "version", "update", testInstanceID, "--version", "v0.19.0"})
	require.NoError(t, cmd.Execute())
	assert.Contains(t, buf.String(), "취소되었습니다")
}

func TestRemoteVersionUpdate_YesSkipsConfirm(t *testing.T) {
	var called bool

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.Header().Set("Content-Type", "application/json")
		w.Write(remoteEnvelope(map[string]any{
			"instance_id":    testInstanceID,
			"target_version": "v0.19.0",
			"result":         "dispatched",
		}))
	})

	_, cmd, cleanup := setupRemoteTestWithConfirm(t, handler, func(string, io.Reader) bool { return false })
	defer cleanup()

	cmd.SetArgs([]string{"remote", "version", "update", testInstanceID, "--version", "v0.19.0", "--yes"})
	require.NoError(t, cmd.Execute())
	assert.True(t, called, "--yes 플래그는 확인 없이 업데이트를 진행해야 합니다")
}

func TestRemoteVersionUpdate_NoFlags(t *testing.T) {
	var called bool
	var gotBody map[string]any

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		body, _ := io.ReadAll(r.Body)
		require.NoError(t, json.Unmarshal(body, &gotBody))
		w.Header().Set("Content-Type", "application/json")
		w.Write(remoteEnvelope(map[string]any{
			"instance_id":    testInstanceID,
			"target_version": "",
			"result":         "dispatched",
		}))
	})

	_, cmd, cleanup := setupRemoteTest(t, handler)
	defer cleanup()

	// 플래그 없이도 채널 최신 업데이트를 트리거할 수 있어야 한다(빈 본문 허용).
	cmd.SetArgs([]string{"remote", "version", "update", testInstanceID, "--yes"})
	require.NoError(t, cmd.Execute())
	assert.True(t, called)
	assert.Empty(t, gotBody, "플래그 미지정 시 본문은 비어 있어야 합니다")
}

func TestRemoteVersionUpdate_JSONFormat(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(remoteEnvelope(map[string]any{
			"instance_id":    testInstanceID,
			"target_version": "v0.19.0",
			"result":         "dispatched",
		}))
	})

	buf, cmd, cleanup := setupRemoteTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"--format", "json", "remote", "version", "update", testInstanceID, "--version", "v0.19.0", "--yes"})
	require.NoError(t, cmd.Execute())

	var parsed map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &parsed))
	assert.Equal(t, "dispatched", parsed["result"])
}

func TestRemoteVersionUpdate_EmptyID(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("빈 instance_id 면 서버를 호출하면 안 됩니다")
	})

	_, cmd, cleanup := setupRemoteTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"remote", "version", "update", "  ", "--yes"})
	require.Error(t, cmd.Execute())
}
