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

// setupModbusTest 는 modbus 커맨드 테스트를 위한 공통 설정을 수행한다.
func setupModbusTest(t *testing.T, handler http.Handler) (*httptest.Server, *cobra.Command, *bytes.Buffer) {
	t.Helper()

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	client := NewClient(srv.URL, "", 5*time.Second, false)

	rootCmd := &cobra.Command{Use: "xflow"}
	rootCmd.PersistentFlags().String("format", "table", "출력 형식")

	rootCmd.AddCommand(newModbusCmd(&client))

	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)

	return srv, rootCmd, &buf
}

// modbusAgentHandler 는 MODBUS 에이전트 관련 API 를 모킹한다.
// agentType 은 "modbus-client" 또는 "modbus-server" 이다.
func modbusAgentHandler(agentType string, execResult map[string]any) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// resolveAgentID 를 위한 에이전트 목록 (빈 목록 → 원본 ID 사용)
		if r.URL.Path == "/api/v1/agents" && r.Method == http.MethodGet {
			json.NewEncoder(w).Encode(map[string]any{"success": true, "data": []map[string]any{}})
			return
		}

		// 에이전트 단건 조회 (resolveModbusAgentType 에서 사용)
		if r.Method == http.MethodGet && r.URL.Path == "/api/v1/agents/test-agent" {
			json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"data": map[string]any{
					"id":   "test-agent",
					"name": "test-agent",
					"type": agentType,
				},
			})
			return
		}

		// exec 커맨드 실행
		if r.Method == http.MethodPost && r.URL.Path == "/api/v1/agents/test-agent/exec" {
			json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"data":    execResult,
			})
			return
		}

		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]any{"success": false, "error": map[string]any{"message": "not found"}})
	}
}

// execCapturingHandler 는 exec 요청의 본문을 캡처하고 결과를 반환한다.
func execCapturingHandler(agentType string, capturedBody *map[string]any, execResult map[string]any) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if r.URL.Path == "/api/v1/agents" && r.Method == http.MethodGet {
			json.NewEncoder(w).Encode(map[string]any{"success": true, "data": []map[string]any{}})
			return
		}

		if r.Method == http.MethodGet && r.URL.Path == "/api/v1/agents/test-agent" {
			json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"data": map[string]any{
					"id":   "test-agent",
					"name": "test-agent",
					"type": agentType,
				},
			})
			return
		}

		if r.Method == http.MethodPost && r.URL.Path == "/api/v1/agents/test-agent/exec" {
			if capturedBody != nil {
				json.NewDecoder(r.Body).Decode(capturedBody)
			}
			json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"data":    execResult,
			})
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}
}

// --- resolveModbusAgentType 테스트 ---

func TestResolveModbusAgentType(t *testing.T) {
	tests := []struct {
		name       string
		agentType  string
		wantRole   string
		wantErr    bool
		errContain string
	}{
		{
			name:      "Client 에이전트",
			agentType: "modbus-client",
			wantRole:  "client",
		},
		{
			name:      "Server 에이전트",
			agentType: "modbus-server",
			wantRole:  "server",
		},
		{
			name:       "비 MODBUS 에이전트",
			agentType:  "mqtt-subscriber",
			wantErr:    true,
			errContain: "MODBUS 타입이 아닙니다",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]any{
					"success": true,
					"data": map[string]any{
						"id":   "agent-1",
						"type": tc.agentType,
					},
				})
			}))
			defer srv.Close()

			client := NewClient(srv.URL, "", 5*time.Second, false)
			role, err := resolveModbusAgentType(client, "agent-1")

			if tc.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.errContain)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.wantRole, role)
			}
		})
	}
}

func TestResolveModbusAgentType_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"success": false, "error": map[string]any{"message": "internal error"}})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "", 5*time.Second, false)
	_, err := resolveModbusAgentType(client, "agent-1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "에이전트 조회 실패")
}

// --- normalizeRegisterArea 테스트 ---

func TestNormalizeRegisterArea(t *testing.T) {
	tests := []struct {
		alias   string
		want    string
		wantErr bool
	}{
		// holding_registers 별칭
		{"holding", "holding_registers", false},
		{"hr", "holding_registers", false},
		{"holding_registers", "holding_registers", false},
		// input_registers 별칭
		{"input", "input_registers", false},
		{"ir", "input_registers", false},
		{"input_registers", "input_registers", false},
		// coils 별칭
		{"coils", "coils", false},
		{"c", "coils", false},
		// discrete_inputs 별칭
		{"discrete", "discrete_inputs", false},
		{"di", "discrete_inputs", false},
		{"discrete_inputs", "discrete_inputs", false},
		// 유효하지 않은 영역
		{"invalid", "", true},
		{"", "", true},
	}

	for _, tc := range tests {
		t.Run(tc.alias, func(t *testing.T) {
			got, err := normalizeRegisterArea(tc.alias)
			if tc.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "알 수 없는 레지스터 영역")
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.want, got)
			}
		})
	}
}

// --- modbus read 커맨드 테스트 ---

func TestModbusRead_ServerAgent(t *testing.T) {
	tests := []struct {
		name        string
		register    string
		wantCommand string
	}{
		{"holding", "holding", "get_holding_registers"},
		{"hr alias", "hr", "get_holding_registers"},
		{"input", "input", "get_input_registers"},
		{"ir alias", "ir", "get_input_registers"},
		{"coils", "coils", "get_coils"},
		{"c alias", "c", "get_coils"},
		{"discrete", "discrete", "get_discrete_inputs"},
		{"di alias", "di", "get_discrete_inputs"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var captured map[string]any
			handler := execCapturingHandler(modbusServerType, &captured, map[string]any{
				"ok":      true,
				"command": tc.wantCommand,
				"values":  []any{0, 0, 0},
			})

			_, rootCmd, buf := setupModbusTest(t, handler)
			rootCmd.SetArgs([]string{"modbus", "read", "test-agent", "-r", tc.register, "-a", "10", "-q", "3", "--format", "json"})

			err := rootCmd.Execute()
			require.NoError(t, err)

			// 전송된 커맨드 확인
			assert.Equal(t, tc.wantCommand, captured["command"])
			params, _ := captured["params"].(map[string]any)
			assert.Equal(t, float64(10), params["address"])
			assert.Equal(t, float64(3), params["quantity"])

			// 출력에 결과 포함 확인
			assert.Contains(t, buf.String(), tc.wantCommand)
		})
	}
}

func TestModbusRead_ServerAgent_Typed(t *testing.T) {
	var captured map[string]any
	handler := execCapturingHandler(modbusServerType, &captured, map[string]any{
		"ok":      true,
		"command": "get_register_typed",
		"value":   3.14,
	})

	_, rootCmd, _ := setupModbusTest(t, handler)
	rootCmd.SetArgs([]string{"modbus", "read", "test-agent", "-r", "holding", "-a", "0", "-t", "float32", "--format", "json"})

	err := rootCmd.Execute()
	require.NoError(t, err)

	assert.Equal(t, "get_register_typed", captured["command"])
	params, _ := captured["params"].(map[string]any)
	assert.Equal(t, "holding_registers", params["area"])
	assert.Equal(t, "float32", params["data_type"])
	assert.Equal(t, "big_endian", params["byte_order"])
}

func TestModbusRead_ServerAgent_WithUnitID(t *testing.T) {
	var captured map[string]any
	handler := execCapturingHandler(modbusServerType, &captured, map[string]any{
		"ok":      true,
		"command": "get_holding_registers",
		"values":  []any{0, 0},
	})

	_, rootCmd, _ := setupModbusTest(t, handler)
	rootCmd.SetArgs([]string{"modbus", "read", "test-agent", "-r", "holding", "-a", "0", "-q", "2", "-u", "3", "--format", "json"})

	err := rootCmd.Execute()
	require.NoError(t, err)

	assert.Equal(t, "get_holding_registers", captured["command"])
	params, _ := captured["params"].(map[string]any)
	assert.Equal(t, float64(3), params["unit_id"])
	assert.Equal(t, float64(0), params["address"])
	assert.Equal(t, float64(2), params["quantity"])
}

func TestModbusRead_ServerAgent_TypedWithUnitID(t *testing.T) {
	var captured map[string]any
	handler := execCapturingHandler(modbusServerType, &captured, map[string]any{
		"ok":      true,
		"command": "get_register_typed",
		"value":   3.14,
	})

	_, rootCmd, _ := setupModbusTest(t, handler)
	rootCmd.SetArgs([]string{"modbus", "read", "test-agent", "-r", "holding", "-a", "0", "-t", "float32", "-u", "5", "--format", "json"})

	err := rootCmd.Execute()
	require.NoError(t, err)

	assert.Equal(t, "get_register_typed", captured["command"])
	params, _ := captured["params"].(map[string]any)
	assert.Equal(t, float64(5), params["unit_id"])
	assert.Equal(t, "holding_registers", params["area"])
	assert.Equal(t, "float32", params["data_type"])
}

func TestModbusRead_ServerAgent_UnitIDZeroOmitted(t *testing.T) {
	var captured map[string]any
	handler := execCapturingHandler(modbusServerType, &captured, map[string]any{
		"ok":      true,
		"command": "get_holding_registers",
		"values":  []any{0},
	})

	_, rootCmd, _ := setupModbusTest(t, handler)
	rootCmd.SetArgs([]string{"modbus", "read", "test-agent", "-r", "holding", "-a", "0", "-q", "1", "--format", "json"})

	err := rootCmd.Execute()
	require.NoError(t, err)

	params, _ := captured["params"].(map[string]any)
	_, hasUnitID := params["unit_id"]
	assert.False(t, hasUnitID, "unit_id should not be present when not specified")
}

func TestModbusRead_ClientAgent(t *testing.T) {
	var captured map[string]any
	handler := execCapturingHandler(modbusClientType, &captured, map[string]any{
		"ok":      true,
		"command": "read_registers",
		"values":  []any{100, 200},
	})

	_, rootCmd, _ := setupModbusTest(t, handler)
	rootCmd.SetArgs([]string{"modbus", "read", "test-agent", "-r", "hr", "-a", "0", "-q", "2", "-d", "1", "--format", "json"})

	err := rootCmd.Execute()
	require.NoError(t, err)

	assert.Equal(t, "read_registers", captured["command"])
	params, _ := captured["params"].(map[string]any)
	assert.Equal(t, "1", params["device_id"])
	assert.Equal(t, "holding_registers", params["register"])
	assert.Equal(t, float64(0), params["address"])
	assert.Equal(t, float64(2), params["quantity"])
}

func TestModbusRead_ClientAgent_MissingDevice(t *testing.T) {
	handler := modbusAgentHandler(modbusClientType, nil)
	_, rootCmd, _ := setupModbusTest(t, handler)
	rootCmd.SetArgs([]string{"modbus", "read", "test-agent", "-r", "holding"})

	err := rootCmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--device (-d) 플래그가 필수입니다")
}

func TestModbusRead_ClientAgent_Typed(t *testing.T) {
	var captured map[string]any
	handler := execCapturingHandler(modbusClientType, &captured, map[string]any{
		"ok":    true,
		"value": 42.5,
	})

	_, rootCmd, _ := setupModbusTest(t, handler)
	rootCmd.SetArgs([]string{"modbus", "read", "test-agent", "-r", "ir", "-a", "0", "-q", "2", "-d", "1", "-t", "float32", "--format", "json"})

	err := rootCmd.Execute()
	require.NoError(t, err)

	assert.Equal(t, "read_registers", captured["command"])
	params, _ := captured["params"].(map[string]any)
	assert.Equal(t, "float32", params["data_type"])
	assert.Equal(t, "big_endian", params["byte_order"])
}

func TestModbusRead_ClientAgent_Force(t *testing.T) {
	var captured map[string]any
	handler := execCapturingHandler(modbusClientType, &captured, map[string]any{
		"ok":     true,
		"values": []any{1},
	})

	_, rootCmd, _ := setupModbusTest(t, handler)
	rootCmd.SetArgs([]string{"modbus", "read", "test-agent", "-r", "holding", "-d", "1", "--force", "--format", "json"})

	err := rootCmd.Execute()
	require.NoError(t, err)

	params, _ := captured["params"].(map[string]any)
	assert.Equal(t, true, params["force"])
}

func TestModbusRead_InvalidRegister(t *testing.T) {
	handler := modbusAgentHandler(modbusServerType, nil)
	_, rootCmd, _ := setupModbusTest(t, handler)
	rootCmd.SetArgs([]string{"modbus", "read", "test-agent", "-r", "invalid"})

	err := rootCmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "알 수 없는 레지스터 영역")
}

// --- modbus write 커맨드 테스트 ---

func TestModbusWrite_ServerAgent_SingleHolding(t *testing.T) {
	var captured map[string]any
	handler := execCapturingHandler(modbusServerType, &captured, map[string]any{
		"ok":      true,
		"command": "set_register",
	})

	_, rootCmd, _ := setupModbusTest(t, handler)
	rootCmd.SetArgs([]string{"modbus", "write", "test-agent", "-r", "holding", "-a", "0", "-v", "100", "--format", "json"})

	err := rootCmd.Execute()
	require.NoError(t, err)

	assert.Equal(t, "set_register", captured["command"])
	params, _ := captured["params"].(map[string]any)
	assert.Equal(t, float64(0), params["address"])
	assert.Equal(t, float64(100), params["value"])
}

func TestModbusWrite_ServerAgent_MultiHolding(t *testing.T) {
	var captured map[string]any
	handler := execCapturingHandler(modbusServerType, &captured, map[string]any{
		"ok":      true,
		"command": "set_registers",
	})

	_, rootCmd, _ := setupModbusTest(t, handler)
	rootCmd.SetArgs([]string{"modbus", "write", "test-agent", "-r", "holding", "-a", "0", "-v", "100,200,300", "--format", "json"})

	err := rootCmd.Execute()
	require.NoError(t, err)

	assert.Equal(t, "set_registers", captured["command"])
	params, _ := captured["params"].(map[string]any)
	values, ok := params["values"].([]any)
	require.True(t, ok)
	assert.Len(t, values, 3)
}

func TestModbusWrite_ServerAgent_SingleCoil(t *testing.T) {
	var captured map[string]any
	handler := execCapturingHandler(modbusServerType, &captured, map[string]any{
		"ok":      true,
		"command": "set_coil",
	})

	_, rootCmd, _ := setupModbusTest(t, handler)
	rootCmd.SetArgs([]string{"modbus", "write", "test-agent", "-r", "coils", "-a", "0", "-v", "true", "--format", "json"})

	err := rootCmd.Execute()
	require.NoError(t, err)

	assert.Equal(t, "set_coil", captured["command"])
	params, _ := captured["params"].(map[string]any)
	assert.Equal(t, true, params["value"])
}

func TestModbusWrite_ServerAgent_MultiCoils(t *testing.T) {
	var captured map[string]any
	handler := execCapturingHandler(modbusServerType, &captured, map[string]any{
		"ok":      true,
		"command": "set_coils",
	})

	_, rootCmd, _ := setupModbusTest(t, handler)
	rootCmd.SetArgs([]string{"modbus", "write", "test-agent", "-r", "c", "-a", "0", "-v", "true,false,true", "--format", "json"})

	err := rootCmd.Execute()
	require.NoError(t, err)

	assert.Equal(t, "set_coils", captured["command"])
	params, _ := captured["params"].(map[string]any)
	values, ok := params["values"].([]any)
	require.True(t, ok)
	assert.Len(t, values, 3)
}

func TestModbusWrite_ClientAgent_SingleHolding(t *testing.T) {
	var captured map[string]any
	handler := execCapturingHandler(modbusClientType, &captured, map[string]any{
		"ok":      true,
		"command": "write_register",
	})

	_, rootCmd, _ := setupModbusTest(t, handler)
	rootCmd.SetArgs([]string{"modbus", "write", "test-agent", "-r", "hr", "-a", "10", "-v", "500", "-d", "1", "--format", "json"})

	err := rootCmd.Execute()
	require.NoError(t, err)

	assert.Equal(t, "write_register", captured["command"])
	params, _ := captured["params"].(map[string]any)
	assert.Equal(t, "1", params["device_id"])
	assert.Equal(t, float64(10), params["address"])
	assert.Equal(t, float64(500), params["value"])
}

func TestModbusWrite_ClientAgent_MultiHolding(t *testing.T) {
	var captured map[string]any
	handler := execCapturingHandler(modbusClientType, &captured, map[string]any{
		"ok":      true,
		"command": "write_registers",
	})

	_, rootCmd, _ := setupModbusTest(t, handler)
	rootCmd.SetArgs([]string{"modbus", "write", "test-agent", "-r", "holding", "-a", "0", "-v", "10,20", "-d", "2", "--format", "json"})

	err := rootCmd.Execute()
	require.NoError(t, err)

	assert.Equal(t, "write_registers", captured["command"])
	params, _ := captured["params"].(map[string]any)
	assert.Equal(t, "2", params["device_id"])
}

func TestModbusWrite_ClientAgent_Coil(t *testing.T) {
	var captured map[string]any
	handler := execCapturingHandler(modbusClientType, &captured, map[string]any{
		"ok":      true,
		"command": "write_coil",
	})

	_, rootCmd, _ := setupModbusTest(t, handler)
	rootCmd.SetArgs([]string{"modbus", "write", "test-agent", "-r", "coils", "-a", "5", "-v", "true", "-d", "1", "--format", "json"})

	err := rootCmd.Execute()
	require.NoError(t, err)

	assert.Equal(t, "write_coil", captured["command"])
	params, _ := captured["params"].(map[string]any)
	assert.Equal(t, true, params["value"])
}

func TestModbusWrite_ServerAgent_SingleHoldingWithUnitID(t *testing.T) {
	var captured map[string]any
	handler := execCapturingHandler(modbusServerType, &captured, map[string]any{
		"ok":      true,
		"command": "set_register",
	})

	_, rootCmd, _ := setupModbusTest(t, handler)
	rootCmd.SetArgs([]string{"modbus", "write", "test-agent", "-r", "holding", "-a", "0", "-v", "100", "-u", "2", "--format", "json"})

	err := rootCmd.Execute()
	require.NoError(t, err)

	assert.Equal(t, "set_register", captured["command"])
	params, _ := captured["params"].(map[string]any)
	assert.Equal(t, float64(2), params["unit_id"])
	assert.Equal(t, float64(0), params["address"])
	assert.Equal(t, float64(100), params["value"])
}

func TestModbusWrite_ServerAgent_MultiHoldingWithUnitID(t *testing.T) {
	var captured map[string]any
	handler := execCapturingHandler(modbusServerType, &captured, map[string]any{
		"ok":      true,
		"command": "set_registers",
	})

	_, rootCmd, _ := setupModbusTest(t, handler)
	rootCmd.SetArgs([]string{"modbus", "write", "test-agent", "-r", "holding", "-a", "0", "-v", "100,200", "-u", "7", "--format", "json"})

	err := rootCmd.Execute()
	require.NoError(t, err)

	assert.Equal(t, "set_registers", captured["command"])
	params, _ := captured["params"].(map[string]any)
	assert.Equal(t, float64(7), params["unit_id"])
}

func TestModbusWrite_ServerAgent_CoilWithUnitID(t *testing.T) {
	var captured map[string]any
	handler := execCapturingHandler(modbusServerType, &captured, map[string]any{
		"ok":      true,
		"command": "set_coil",
	})

	_, rootCmd, _ := setupModbusTest(t, handler)
	rootCmd.SetArgs([]string{"modbus", "write", "test-agent", "-r", "coils", "-a", "0", "-v", "true", "-u", "4", "--format", "json"})

	err := rootCmd.Execute()
	require.NoError(t, err)

	assert.Equal(t, "set_coil", captured["command"])
	params, _ := captured["params"].(map[string]any)
	assert.Equal(t, float64(4), params["unit_id"])
	assert.Equal(t, true, params["value"])
}

func TestModbusWrite_ServerAgent_UnitIDZeroOmitted(t *testing.T) {
	var captured map[string]any
	handler := execCapturingHandler(modbusServerType, &captured, map[string]any{
		"ok":      true,
		"command": "set_register",
	})

	_, rootCmd, _ := setupModbusTest(t, handler)
	rootCmd.SetArgs([]string{"modbus", "write", "test-agent", "-r", "holding", "-a", "0", "-v", "100", "--format", "json"})

	err := rootCmd.Execute()
	require.NoError(t, err)

	params, _ := captured["params"].(map[string]any)
	_, hasUnitID := params["unit_id"]
	assert.False(t, hasUnitID, "unit_id should not be present when not specified")
}

func TestModbusWrite_ReadOnlyArea(t *testing.T) {
	handler := modbusAgentHandler(modbusServerType, nil)
	_, rootCmd, _ := setupModbusTest(t, handler)
	rootCmd.SetArgs([]string{"modbus", "write", "test-agent", "-r", "input", "-a", "0", "-v", "100"})

	err := rootCmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "쓰기를 지원하지 않습니다")
}

func TestModbusWrite_DiscreteReadOnly(t *testing.T) {
	handler := modbusAgentHandler(modbusServerType, nil)
	_, rootCmd, _ := setupModbusTest(t, handler)
	rootCmd.SetArgs([]string{"modbus", "write", "test-agent", "-r", "discrete", "-a", "0", "-v", "true"})

	err := rootCmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "쓰기를 지원하지 않습니다")
}

func TestModbusWrite_MissingValues(t *testing.T) {
	handler := modbusAgentHandler(modbusServerType, nil)
	_, rootCmd, _ := setupModbusTest(t, handler)
	rootCmd.SetArgs([]string{"modbus", "write", "test-agent", "-r", "holding", "-a", "0"})

	err := rootCmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--values (-v) 플래그는 필수입니다")
}

func TestModbusWrite_ClientMissingDevice(t *testing.T) {
	handler := modbusAgentHandler(modbusClientType, nil)
	_, rootCmd, _ := setupModbusTest(t, handler)
	rootCmd.SetArgs([]string{"modbus", "write", "test-agent", "-r", "holding", "-a", "0", "-v", "100"})

	err := rootCmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--device (-d) 플래그가 필수입니다")
}

// --- modbus status 커맨드 테스트 ---

func TestModbusStatus_ServerAgent(t *testing.T) {
	handler := modbusAgentHandler(modbusServerType, map[string]any{
		"ok":        true,
		"command":   "get_status",
		"listening": true,
		"address":   ":5020",
	})

	_, rootCmd, buf := setupModbusTest(t, handler)
	rootCmd.SetArgs([]string{"modbus", "status", "test-agent", "--format", "json"})

	err := rootCmd.Execute()
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "get_status")
}

func TestModbusStatus_ClientAgent(t *testing.T) {
	handler := modbusAgentHandler(modbusClientType, map[string]any{
		"ok":        true,
		"command":   "get_status",
		"connected": true,
	})

	_, rootCmd, buf := setupModbusTest(t, handler)
	rootCmd.SetArgs([]string{"modbus", "status", "test-agent", "--format", "json"})

	err := rootCmd.Execute()
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "get_status")
}

func TestModbusStatus_NonModbusAgent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/agents" && r.Method == http.MethodGet {
			json.NewEncoder(w).Encode(map[string]any{"success": true, "data": []map[string]any{}})
			return
		}
		if r.Method == http.MethodGet && r.URL.Path == "/api/v1/agents/test-agent" {
			json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"data": map[string]any{
					"id":   "test-agent",
					"type": "mqtt-subscriber",
				},
			})
			return
		}
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "", 5*time.Second, false)
	rootCmd := &cobra.Command{Use: "xflow"}
	rootCmd.PersistentFlags().String("format", "table", "출력 형식")
	rootCmd.AddCommand(newModbusCmd(&client))

	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)
	rootCmd.SetArgs([]string{"modbus", "status", "test-agent"})

	err := rootCmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "MODBUS 타입이 아닙니다")
}

// --- modbus map 커맨드 테스트 ---

func TestModbusMap_ServerAgent(t *testing.T) {
	handler := modbusAgentHandler(modbusServerType, map[string]any{
		"ok":      true,
		"command": "get_map",
		"registers": []any{
			map[string]any{"address": 0, "name": "temperature", "data_type": "float32"},
		},
	})

	_, rootCmd, buf := setupModbusTest(t, handler)
	rootCmd.SetArgs([]string{"modbus", "map", "test-agent", "--format", "json"})

	err := rootCmd.Execute()
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "get_map")
}

func TestModbusMap_ServerAgent_WithUnitID(t *testing.T) {
	var captured map[string]any
	handler := execCapturingHandler(modbusServerType, &captured, map[string]any{
		"ok":      true,
		"command": "get_map",
		"registers": []any{
			map[string]any{"address": 0, "name": "temperature", "data_type": "float32"},
		},
	})

	_, rootCmd, _ := setupModbusTest(t, handler)
	rootCmd.SetArgs([]string{"modbus", "map", "test-agent", "-u", "3", "--format", "json"})

	err := rootCmd.Execute()
	require.NoError(t, err)

	assert.Equal(t, "get_map", captured["command"])
	params, _ := captured["params"].(map[string]any)
	assert.Equal(t, float64(3), params["unit_id"])
}

func TestModbusMap_ServerAgent_UnitIDZeroOmitted(t *testing.T) {
	var captured map[string]any
	handler := execCapturingHandler(modbusServerType, &captured, map[string]any{
		"ok":      true,
		"command": "get_map",
	})

	_, rootCmd, _ := setupModbusTest(t, handler)
	rootCmd.SetArgs([]string{"modbus", "map", "test-agent", "--format", "json"})

	err := rootCmd.Execute()
	require.NoError(t, err)

	_, hasParams := captured["params"]
	assert.False(t, hasParams, "params should not be present when unit_id is not specified")
}

func TestModbusMap_ClientAgent_Error(t *testing.T) {
	handler := modbusAgentHandler(modbusClientType, nil)
	_, rootCmd, _ := setupModbusTest(t, handler)
	rootCmd.SetArgs([]string{"modbus", "map", "test-agent"})

	err := rootCmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Server 에이전트에서만 사용할 수 있습니다")
}

// --- 에러 케이스 테스트 ---

func TestModbusRead_NoAgent(t *testing.T) {
	handler := modbusAgentHandler(modbusServerType, nil)
	_, rootCmd, _ := setupModbusTest(t, handler)
	rootCmd.SetArgs([]string{"modbus", "read"})

	err := rootCmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "대상을 지정해주세요")
}

func TestModbusWrite_NonBoolForCoils(t *testing.T) {
	handler := modbusAgentHandler(modbusServerType, nil)
	_, rootCmd, _ := setupModbusTest(t, handler)
	rootCmd.SetArgs([]string{"modbus", "write", "test-agent", "-r", "coils", "-a", "0", "-v", "123"})

	err := rootCmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bool 값이 필요합니다")
}

// --- parseWriteValues 테스트 ---

func TestParseWriteValues(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want int // 결과 슬라이스 길이
	}{
		{"단일 정수", "100", 1},
		{"다중 정수", "100,200,300", 3},
		{"불린 값", "true,false,true", 3},
		{"공백 포함", " 100 , 200 ", 2},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := parseWriteValues(tc.raw)
			assert.Len(t, result, tc.want)
		})
	}
}

// --- toBoolSlice 테스트 ---

func TestToBoolSlice(t *testing.T) {
	t.Run("정상 변환", func(t *testing.T) {
		result, err := toBoolSlice([]any{true, false, true})
		require.NoError(t, err)
		assert.Equal(t, []bool{true, false, true}, result)
	})

	t.Run("비 bool 값 에러", func(t *testing.T) {
		_, err := toBoolSlice([]any{true, int64(1), false})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "bool 값이 필요합니다")
	})
}

// --- 출력 형식 테스트 ---

func TestModbusRead_JSONFormat(t *testing.T) {
	handler := modbusAgentHandler(modbusServerType, map[string]any{
		"ok":      true,
		"command": "get_holding_registers",
		"values":  []any{100, 200},
	})

	_, rootCmd, buf := setupModbusTest(t, handler)
	rootCmd.SetArgs([]string{"modbus", "read", "test-agent", "-r", "holding", "--format", "json"})

	err := rootCmd.Execute()
	require.NoError(t, err)

	// JSON 출력 검증
	var parsed map[string]any
	err = json.Unmarshal(buf.Bytes(), &parsed)
	require.NoError(t, err)
	assert.Equal(t, true, parsed["ok"])
}

func TestModbusRead_TableFormat(t *testing.T) {
	handler := modbusAgentHandler(modbusServerType, map[string]any{
		"ok":      true,
		"command": "get_holding_registers",
		"address": 0,
		"values":  []any{100, 200},
	})

	_, rootCmd, buf := setupModbusTest(t, handler)
	rootCmd.SetArgs([]string{"modbus", "read", "test-agent", "-r", "holding", "--format", "table"})

	err := rootCmd.Execute()
	require.NoError(t, err)

	// table 형식 (DetailFormatter) 출력에 주요 필드가 포함되었는지 확인
	output := buf.String()
	assert.Contains(t, output, "ok")
	assert.Contains(t, output, "command")
}

// --- 이름 기반 에이전트 조회 테스트 ---

func TestModbusRead_ByName(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// 이름 검색용 에이전트 목록
		if r.URL.Path == "/api/v1/agents" && r.Method == http.MethodGet {
			json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"data": []map[string]any{
					{"id": "uuid-123", "name": "my-modbus-server"},
				},
			})
			return
		}

		// 에이전트 단건 조회
		if r.Method == http.MethodGet && r.URL.Path == "/api/v1/agents/uuid-123" {
			json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"data": map[string]any{
					"id":   "uuid-123",
					"name": "my-modbus-server",
					"type": modbusServerType,
				},
			})
			return
		}

		// exec
		if r.Method == http.MethodPost && r.URL.Path == "/api/v1/agents/uuid-123/exec" {
			json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"data":    map[string]any{"ok": true, "command": "get_holding_registers"},
			})
			return
		}
	})

	_, rootCmd, buf := setupModbusTest(t, handler)
	rootCmd.SetArgs([]string{"modbus", "read", "--name", "my-modbus-server", "-r", "holding", "--format", "json"})

	err := rootCmd.Execute()
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "get_holding_registers")
}
