package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sampleGroups 는 테스트용 그룹 목록을 반환한다.
// 빈 group_name 은 가상 "전체"(미지정) 버킷이다.
func sampleGroups() []map[string]any {
	return []map[string]any{
		{"group_name": "", "node_count": float64(3)},
		{"group_name": "1f", "node_count": float64(2)},
		{"group_name": "2f", "node_count": float64(1)},
	}
}

// --- remote group list ---

func TestRemoteGroupList(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/api/v1/remote/groups", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		w.Write(remoteEnvelope(sampleGroups()))
	})

	buf, cmd, cleanup := setupRemoteTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"remote", "group", "list"})
	require.NoError(t, cmd.Execute())

	out := buf.String()
	assert.Contains(t, out, "GROUP")
	assert.Contains(t, out, "NODE_COUNT")
	assert.Contains(t, out, "1f")
	assert.Contains(t, out, "2f")
	// 빈 group_name 은 가독성을 위해 (all) 로 렌더링한다.
	assert.Contains(t, out, "(all)")
	// node_count 는 정수 문자열로 표시되어야 한다(과학표기 금지).
	assert.Contains(t, out, "3")
}

func TestRemoteGroupList_JSONFormat(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/remote/groups", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		w.Write(remoteEnvelope(sampleGroups()))
	})

	buf, cmd, cleanup := setupRemoteTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"--format", "json", "remote", "group", "list"})
	require.NoError(t, cmd.Execute())

	var parsed []map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &parsed))
	require.Len(t, parsed, 3)
	assert.Equal(t, "1f", parsed[1]["group_name"])
}

// --- remote group set ---

func TestRemoteGroupSet(t *testing.T) {
	var gotPath, gotMethod string
	var gotBody map[string]any

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		body, _ := io.ReadAll(r.Body)
		require.NoError(t, json.Unmarshal(body, &gotBody))
		w.Header().Set("Content-Type", "application/json")
		w.Write(remoteEnvelope(map[string]any{
			"instance_id": testInstanceID,
			"group_name":  "1f",
		}))
	})

	buf, cmd, cleanup := setupRemoteTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"remote", "group", "set", testInstanceID, "1f"})
	require.NoError(t, cmd.Execute())

	assert.Equal(t, http.MethodPut, gotMethod)
	assert.Equal(t, "/api/v1/remote/nodes/"+testInstanceID+"/group", gotPath)
	assert.Equal(t, "1f", gotBody["group_name"])
	out := buf.String()
	assert.Contains(t, out, testInstanceID)
	assert.Contains(t, out, "1f")
}

func TestRemoteGroupSet_JSONFormat(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(remoteEnvelope(map[string]any{
			"instance_id": testInstanceID,
			"group_name":  "1f",
		}))
	})

	buf, cmd, cleanup := setupRemoteTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"--format", "json", "remote", "group", "set", testInstanceID, "1f"})
	require.NoError(t, cmd.Execute())

	var parsed map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &parsed))
	assert.Equal(t, "1f", parsed["group_name"])
}

func TestRemoteGroupSet_EmptyID(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("빈 instance_id 면 서버를 호출하면 안 됩니다")
	})

	_, cmd, cleanup := setupRemoteTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"remote", "group", "set", "   ", "1f"})
	require.Error(t, cmd.Execute())
}

func TestRemoteGroupSet_EmptyGroup(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("빈 group_name 이면 서버를 호출하면 안 됩니다")
	})

	_, cmd, cleanup := setupRemoteTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"remote", "group", "set", testInstanceID, "  "})
	require.Error(t, cmd.Execute())
}

// --- remote group clear ---

func TestRemoteGroupClear(t *testing.T) {
	var gotPath, gotMethod string

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		// 204 No Content (본문 없음).
		w.WriteHeader(http.StatusNoContent)
	})

	buf, cmd, cleanup := setupRemoteTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"remote", "group", "clear", testInstanceID})
	require.NoError(t, cmd.Execute())

	assert.Equal(t, http.MethodDelete, gotMethod)
	assert.Equal(t, "/api/v1/remote/nodes/"+testInstanceID+"/group", gotPath)
	assert.Contains(t, buf.String(), testInstanceID)
}

func TestRemoteGroupClear_EmptyID(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("빈 instance_id 면 서버를 호출하면 안 됩니다")
	})

	_, cmd, cleanup := setupRemoteTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"remote", "group", "clear", "  "})
	require.Error(t, cmd.Execute())
}

// --- remote group rename ---

func TestRemoteGroupRename(t *testing.T) {
	var gotPath, gotMethod string
	var gotBody map[string]any

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		body, _ := io.ReadAll(r.Body)
		require.NoError(t, json.Unmarshal(body, &gotBody))
		w.Header().Set("Content-Type", "application/json")
		w.Write(remoteEnvelope(map[string]any{
			"group_name": "3f",
			"moved":      float64(2),
		}))
	})

	buf, cmd, cleanup := setupRemoteTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"remote", "group", "rename", "1f", "3f"})
	require.NoError(t, cmd.Execute())

	assert.Equal(t, http.MethodPut, gotMethod)
	assert.Equal(t, "/api/v1/remote/groups/1f", gotPath)
	assert.Equal(t, "3f", gotBody["new_name"])
	out := buf.String()
	assert.Contains(t, out, "3f")
	// moved 카운트가 출력에 포함되어야 한다.
	assert.Contains(t, out, "2")
}

func TestRemoteGroupRename_JSONFormat(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(remoteEnvelope(map[string]any{
			"group_name": "3f",
			"moved":      float64(2),
		}))
	})

	buf, cmd, cleanup := setupRemoteTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"--format", "json", "remote", "group", "rename", "1f", "3f"})
	require.NoError(t, cmd.Execute())

	var parsed map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &parsed))
	assert.Equal(t, "3f", parsed["group_name"])
}

func TestRemoteGroupRename_EmptyName(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("빈 그룹명이면 서버를 호출하면 안 됩니다")
	})

	_, cmd, cleanup := setupRemoteTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"remote", "group", "rename", "1f", "  "})
	require.Error(t, cmd.Execute())
}

// --- remote group delete ---

func TestRemoteGroupDelete_Confirmed(t *testing.T) {
	var gotPath, gotMethod string

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		w.Header().Set("Content-Type", "application/json")
		w.Write(remoteEnvelope(map[string]any{"moved": float64(2)}))
	})

	buf, cmd, cleanup := setupRemoteTestWithConfirm(t, handler, func(string, io.Reader) bool { return true })
	defer cleanup()

	cmd.SetArgs([]string{"remote", "group", "delete", "1f"})
	require.NoError(t, cmd.Execute())

	assert.Equal(t, http.MethodDelete, gotMethod)
	assert.Equal(t, "/api/v1/remote/groups/1f", gotPath)
	out := buf.String()
	assert.Contains(t, out, "삭제")
	assert.Contains(t, out, "2")
}

func TestRemoteGroupDelete_Cancelled(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("확인이 거부되면 서버를 호출하면 안 됩니다")
	})

	buf, cmd, cleanup := setupRemoteTestWithConfirm(t, handler, func(string, io.Reader) bool { return false })
	defer cleanup()

	cmd.SetArgs([]string{"remote", "group", "delete", "1f"})
	require.NoError(t, cmd.Execute())
	assert.Contains(t, buf.String(), "취소되었습니다")
}

func TestRemoteGroupDelete_YesSkipsConfirm(t *testing.T) {
	var called bool

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.Header().Set("Content-Type", "application/json")
		w.Write(remoteEnvelope(map[string]any{"moved": float64(0)}))
	})

	_, cmd, cleanup := setupRemoteTestWithConfirm(t, handler, func(string, io.Reader) bool { return false })
	defer cleanup()

	cmd.SetArgs([]string{"remote", "group", "delete", "1f", "--yes"})
	require.NoError(t, cmd.Execute())
	assert.True(t, called, "--yes 플래그는 확인 없이 삭제를 진행해야 합니다")
}

func TestRemoteGroupDelete_EmptyName(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("빈 그룹명이면 서버를 호출하면 안 됩니다")
	})

	_, cmd, cleanup := setupRemoteTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"remote", "group", "delete", "  ", "--yes"})
	require.Error(t, cmd.Execute())
}

// --- remote group update ---

func TestRemoteGroupUpdate(t *testing.T) {
	var gotPath, gotMethod string
	var gotBody map[string]any

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		body, _ := io.ReadAll(r.Body)
		require.NoError(t, json.Unmarshal(body, &gotBody))
		w.Header().Set("Content-Type", "application/json")
		w.Write(remoteEnvelope(map[string]any{
			"group_name": "1f",
			"results":    []any{map[string]any{"instance_id": testInstanceID, "ok": true}},
		}))
	})

	buf, cmd, cleanup := setupRemoteTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{
		"remote", "group", "update", "1f",
		"--version", "v0.19.0", "--strategy", "pin", "--restart", "--yes",
	})
	require.NoError(t, cmd.Execute())

	assert.Equal(t, http.MethodPost, gotMethod)
	assert.Equal(t, "/api/v1/remote/groups/1f/update", gotPath)
	assert.Equal(t, "v0.19.0", gotBody["version"])
	assert.Equal(t, "pin", gotBody["strategy"])
	assert.Equal(t, true, gotBody["restart"])
	// 미지정 플래그는 본문에 포함되지 않아야 한다.
	_, hasChannel := gotBody["channel"]
	assert.False(t, hasChannel, "--channel 미지정 시 channel 필드는 생략되어야 합니다")
	assert.Contains(t, buf.String(), testInstanceID)
}

func TestRemoteGroupUpdate_VersionByArch(t *testing.T) {
	var gotBody map[string]any

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		require.NoError(t, json.Unmarshal(body, &gotBody))
		w.Header().Set("Content-Type", "application/json")
		w.Write(remoteEnvelope(map[string]any{"group_name": "1f", "results": []any{}}))
	})

	_, cmd, cleanup := setupRemoteTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{
		"remote", "group", "update", "1f",
		"--strategy", "per_arch",
		"--version-by-arch", "linux/amd64=v0.19.0",
		"--version-by-arch", "linux/arm64=v0.19.1",
		"--yes",
	})
	require.NoError(t, cmd.Execute())

	vba, ok := gotBody["version_by_arch"].(map[string]any)
	require.True(t, ok, "version_by_arch 는 객체여야 합니다")
	assert.Equal(t, "v0.19.0", vba["linux/amd64"])
	assert.Equal(t, "v0.19.1", vba["linux/arm64"])
}

func TestRemoteGroupUpdate_Cancelled(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("확인이 거부되면 서버를 호출하면 안 됩니다")
	})

	buf, cmd, cleanup := setupRemoteTestWithConfirm(t, handler, func(string, io.Reader) bool { return false })
	defer cleanup()

	cmd.SetArgs([]string{"remote", "group", "update", "1f", "--version", "v0.19.0"})
	require.NoError(t, cmd.Execute())
	assert.Contains(t, buf.String(), "취소되었습니다")
}

func TestRemoteGroupUpdate_NoFlags(t *testing.T) {
	var called bool
	var gotBody map[string]any

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		body, _ := io.ReadAll(r.Body)
		require.NoError(t, json.Unmarshal(body, &gotBody))
		w.Header().Set("Content-Type", "application/json")
		w.Write(remoteEnvelope(map[string]any{"group_name": "1f", "results": []any{}}))
	})

	_, cmd, cleanup := setupRemoteTest(t, handler)
	defer cleanup()

	// 플래그 없이도 채널 최신 업데이트를 트리거할 수 있어야 한다(빈 본문 허용).
	cmd.SetArgs([]string{"remote", "group", "update", "1f", "--yes"})
	require.NoError(t, cmd.Execute())
	assert.True(t, called)
	assert.Empty(t, gotBody, "플래그 미지정 시 본문은 비어 있어야 합니다")
}

func TestRemoteGroupUpdate_EmptyName(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("빈 그룹명이면 서버를 호출하면 안 됩니다")
	})

	_, cmd, cleanup := setupRemoteTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"remote", "group", "update", "  ", "--yes"})
	require.Error(t, cmd.Execute())
}

func TestRemoteGroupUpdate_BadVersionByArch(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("잘못된 version-by-arch 형식이면 서버를 호출하면 안 됩니다")
	})

	_, cmd, cleanup := setupRemoteTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{
		"remote", "group", "update", "1f",
		"--version-by-arch", "no-equals-sign", "--yes",
	})
	require.Error(t, cmd.Execute())
}

// --- remote group command ---

func TestRemoteGroupCommand(t *testing.T) {
	var gotPath, gotMethod string
	var gotBody map[string]any

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		body, _ := io.ReadAll(r.Body)
		require.NoError(t, json.Unmarshal(body, &gotBody))
		w.Header().Set("Content-Type", "application/json")
		w.Write(remoteEnvelope(map[string]any{
			"group_name": "1f",
			"results":    []any{map[string]any{"instance_id": testInstanceID, "ok": true}},
		}))
	})

	buf, cmd, cleanup := setupRemoteTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{
		"remote", "group", "command", "1f",
		"--domain", "flow", "--action", "restart",
		"--args", `{"flow_id":"f1"}`,
	})
	require.NoError(t, cmd.Execute())

	assert.Equal(t, http.MethodPost, gotMethod)
	assert.Equal(t, "/api/v1/remote/groups/1f/command", gotPath)
	assert.Equal(t, "flow", gotBody["domain"])
	assert.Equal(t, "restart", gotBody["action"])
	args, ok := gotBody["args"].(map[string]any)
	require.True(t, ok, "args 는 JSON 객체로 전달되어야 합니다")
	assert.Equal(t, "f1", args["flow_id"])
	assert.Contains(t, buf.String(), testInstanceID)
}

func TestRemoteGroupCommand_NoArgsOmitsField(t *testing.T) {
	var gotBody map[string]any

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		require.NoError(t, json.Unmarshal(body, &gotBody))
		w.Header().Set("Content-Type", "application/json")
		w.Write(remoteEnvelope(map[string]any{"group_name": "1f", "results": []any{}}))
	})

	_, cmd, cleanup := setupRemoteTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{
		"remote", "group", "command", "1f",
		"--domain", "system", "--action", "ping",
	})
	require.NoError(t, cmd.Execute())

	assert.Equal(t, "system", gotBody["domain"])
	_, hasArgs := gotBody["args"]
	assert.False(t, hasArgs, "--args 미지정 시 args 필드는 생략되어야 합니다")
}

func TestRemoteGroupCommand_MissingDomain(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("domain 누락 시 서버를 호출하면 안 됩니다")
	})

	_, cmd, cleanup := setupRemoteTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"remote", "group", "command", "1f", "--action", "ping"})
	require.Error(t, cmd.Execute())
}

func TestRemoteGroupCommand_MissingAction(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("action 누락 시 서버를 호출하면 안 됩니다")
	})

	_, cmd, cleanup := setupRemoteTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"remote", "group", "command", "1f", "--domain", "flow"})
	require.Error(t, cmd.Execute())
}

func TestRemoteGroupCommand_BadArgsJSON(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("잘못된 args JSON 이면 서버를 호출하면 안 됩니다")
	})

	_, cmd, cleanup := setupRemoteTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{
		"remote", "group", "command", "1f",
		"--domain", "flow", "--action", "restart", "--args", "{bad json",
	})
	require.Error(t, cmd.Execute())
}

// --- buildGroupUpdateBody 단위 테스트 ---

func TestBuildGroupUpdateBody(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		wantKeys  []string
		wantNoKey []string
	}{
		{
			name:      "only version",
			args:      []string{"--version", "v0.19.0"},
			wantKeys:  []string{"version"},
			wantNoKey: []string{"strategy", "channel", "restart", "version_by_arch"},
		},
		{
			name:      "strategy and channel",
			args:      []string{"--strategy", "latest", "--channel", "stable"},
			wantKeys:  []string{"strategy", "channel"},
			wantNoKey: []string{"version", "restart", "version_by_arch"},
		},
		{
			name:      "restart only",
			args:      []string{"--restart"},
			wantKeys:  []string{"restart"},
			wantNoKey: []string{"version", "strategy", "channel", "version_by_arch"},
		},
		{
			name:      "no flags yields empty body",
			args:      nil,
			wantKeys:  nil,
			wantNoKey: []string{"version", "strategy", "channel", "restart", "version_by_arch"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := newRemoteGroupUpdateCmdForTest()
			require.NoError(t, cmd.ParseFlags(tt.args))

			body, err := buildGroupUpdateBodyFromCmd(cmd)
			require.NoError(t, err)

			for _, k := range tt.wantKeys {
				_, ok := body[k]
				assert.True(t, ok, "키 %q 가 본문에 있어야 합니다", k)
			}
			for _, k := range tt.wantNoKey {
				_, ok := body[k]
				assert.False(t, ok, "키 %q 는 본문에 없어야 합니다", k)
			}
		})
	}
}
