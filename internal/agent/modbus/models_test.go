package modbus

import (
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/agent"
)

// ---------------------------------------------------------------------------
// SPEC-MODBUS-013 REQ-04 — 디바이스 모델 카탈로그
// AC-15 스캔 / AC-16 fail-open / AC-17 디렉터리 부재 / AC-18 id 중복 / AC-19 GIPAM 동봉
// ---------------------------------------------------------------------------

// quietLogger 는 로그를 버리는 로거이다(경고 로그 자체는 검증 대상이 아님).
func quietLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
}

// writeModel 은 테스트 디렉터리에 모델 JSON 파일을 쓴다.
func writeModel(t *testing.T, dir, filename string, body any) {
	t.Helper()
	var raw []byte
	switch v := body.(type) {
	case string:
		raw = []byte(v)
	default:
		b, err := json.Marshal(v)
		require.NoError(t, err)
		raw = b
	}
	require.NoError(t, os.WriteFile(filepath.Join(dir, filename), raw, 0o600))
}

// validModel 은 검증을 통과하는 최소 모델을 만든다.
func validModel(id, name string) DeviceModel {
	return DeviceModel{
		ID: id, Name: name,
		Registers: []ModelRegister{{FunctionCode: 3, Address: 0, Count: 2, DataType: "uint16", Name: "r0"}},
	}
}

// TestLoadDeviceModels_Scan 은 디렉터리 스캔으로 유효 모델이 로드됨을 검증한다(AC-15).
func TestLoadDeviceModels_Scan(t *testing.T) {
	dir := t.TempDir()
	writeModel(t, dir, "a.json", validModel("model-a", "Model A"))
	writeModel(t, dir, "b.json", validModel("model-b", "Model B"))

	models := LoadDeviceModels(dir, quietLogger())
	require.Len(t, models, 2)
	assert.Equal(t, "model-a", models[0].ID, "파일명 사전순으로 결정적이어야 한다")
	assert.Equal(t, "model-b", models[1].ID)
	assert.Equal(t, 1, len(models[0].Registers))
}

// TestLoadDeviceModels_FailOpen 은 무효 파일이 나머지 로드를 막지 않음을 검증한다(AC-16).
func TestLoadDeviceModels_FailOpen(t *testing.T) {
	dir := t.TempDir()
	writeModel(t, dir, "1-ok.json", validModel("ok-1", "OK 1"))
	writeModel(t, dir, "2-broken.json", "{ this is not json")
	bad := validModel("bad-fc", "Bad FC")
	bad.Registers[0].FunctionCode = 9 // fc 범위 밖
	writeModel(t, dir, "3-badfc.json", bad)
	writeModel(t, dir, "4-ok.json", validModel("ok-2", "OK 2"))

	models := LoadDeviceModels(dir, quietLogger())
	require.Len(t, models, 2, "유효 모델 2개만 남아야 한다")
	assert.Equal(t, "ok-1", models[0].ID)
	assert.Equal(t, "ok-2", models[1].ID)
}

// TestLoadDeviceModels_MissingDir 은 디렉터리 부재 시 빈 카탈로그를 반환함을 검증한다(AC-17).
func TestLoadDeviceModels_MissingDir(t *testing.T) {
	assert.Empty(t, LoadDeviceModels(filepath.Join(t.TempDir(), "nope"), quietLogger()))
	assert.Empty(t, LoadDeviceModels("", quietLogger()), "빈 경로도 안전해야 한다")
	assert.Empty(t, LoadDeviceModels(t.TempDir(), quietLogger()), "빈 디렉터리도 안전해야 한다")
}

// TestLoadDeviceModels_DuplicateID 는 id 중복 시 먼저 온 파일이 이김을 검증한다(AC-18).
func TestLoadDeviceModels_DuplicateID(t *testing.T) {
	dir := t.TempDir()
	writeModel(t, dir, "1-first.json", validModel("dup", "First"))
	writeModel(t, dir, "2-second.json", validModel("dup", "Second"))

	models := LoadDeviceModels(dir, quietLogger())
	require.Len(t, models, 1)
	assert.Equal(t, "First", models[0].Name, "사전순으로 먼저 온 파일이 이겨야 한다")
}

// TestValidateDeviceModel 은 검증 규칙을 항목별로 확인한다.
func TestValidateDeviceModel(t *testing.T) {
	mutate := func(f func(*DeviceModel)) DeviceModel {
		m := validModel("id", "Name")
		f(&m)
		return m
	}
	tests := []struct {
		name    string
		model   DeviceModel
		wantErr bool
	}{
		{name: "정상", model: validModel("id", "Name")},
		{name: "id 누락", model: mutate(func(m *DeviceModel) { m.ID = "" }), wantErr: true},
		{name: "name 누락", model: mutate(func(m *DeviceModel) { m.Name = "" }), wantErr: true},
		{name: "registers 비어있음", model: mutate(func(m *DeviceModel) { m.Registers = nil }), wantErr: true},
		{name: "fc 0", model: mutate(func(m *DeviceModel) { m.Registers[0].FunctionCode = 0 }), wantErr: true},
		{name: "fc 5", model: mutate(func(m *DeviceModel) { m.Registers[0].FunctionCode = 5 }), wantErr: true},
		{name: "count 0", model: mutate(func(m *DeviceModel) { m.Registers[0].Count = 0 }), wantErr: true},
		{name: "주소공간 초과", model: mutate(func(m *DeviceModel) { m.Registers[0].Address = 65535; m.Registers[0].Count = 10 }), wantErr: true},
		{name: "무효 data_type", model: mutate(func(m *DeviceModel) { m.Registers[0].DataType = "float64" }), wantErr: true},
		{name: "빈 data_type 허용", model: mutate(func(m *DeviceModel) { m.Registers[0].DataType = "" })},
		{name: "무효 poll_interval", model: mutate(func(m *DeviceModel) { m.Registers[0].PollInterval = "5분" }), wantErr: true},
		{name: "무효 defaults.poll_interval", model: mutate(func(m *DeviceModel) { m.Defaults.PollInterval = "nope" }), wantErr: true},
		{name: "무효 transport", model: mutate(func(m *DeviceModel) { m.Transport = "serial" }), wantErr: true},
		{name: "rtu transport 허용", model: mutate(func(m *DeviceModel) { m.Transport = TransportRTU })},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateDeviceModel(tt.model)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestModelsDir_EnvOverride 는 XFLOW_MODELS_DIR 오버라이드를 검증한다.
func TestModelsDir_EnvOverride(t *testing.T) {
	t.Setenv(ModelsDirEnv, "/custom/models")
	assert.Equal(t, "/custom/models", ModelsDir())
}

// TestModelsDir_HomeFallback 은 환경변수가 없을 때 ~/.xflow/models 로 폴백함을 검증한다.
func TestModelsDir_HomeFallback(t *testing.T) {
	t.Setenv(ModelsDirEnv, "")
	got := ModelsDir()
	home, err := os.UserHomeDir()
	if err != nil {
		assert.Empty(t, got, "홈 디렉터리를 얻을 수 없으면 빈 경로여야 한다")
		return
	}
	assert.Equal(t, filepath.Join(home, ".xflow", "models"), got)
}

// TestProcessListModels 는 list_models exec 응답 형상을 검증한다(AC-15).
func TestProcessListModels(t *testing.T) {
	dir := t.TempDir()
	m := validModel("m1", "Model One")
	m.Vendor = "ACME"
	m.Defaults = ModelDefaults{MaxBlockRegisters: 56, PollInterval: "2s"}
	m.Registers = append(m.Registers, ModelRegister{FunctionCode: 4, Address: 10, Count: 2, DataType: "float32", Name: "temp"})
	writeModel(t, dir, "m1.json", m)
	t.Setenv(ModelsDirEnv, dir)

	a, _ := newTestModbusAgent(t, enabledAgentConfig([3]any{nil, nil, nil}),
		&mockModbusTransport{connected: true})

	raw, err := a.Process(mustCommandJSON(t, "list_models"))
	require.NoError(t, err)

	var out struct {
		Data []struct {
			ID                string `json:"id"`
			Name              string `json:"name"`
			Vendor            string `json:"vendor"`
			MaxBlockRegisters int    `json:"max_block_registers"`
			RegisterCount     int    `json:"register_count"`
			Registers         []struct {
				FC           int    `json:"fc"`
				Address      int    `json:"address"`
				Count        int    `json:"count"`
				DataType     string `json:"data_type"`
				PollInterval string `json:"poll_interval"`
				Name         string `json:"name"`
				Enabled      *bool  `json:"enabled"`
			} `json:"registers"`
		} `json:"data"`
		ModelCount int `json:"model_count"`
	}
	require.NoError(t, json.Unmarshal(raw, &out))

	require.Equal(t, 1, out.ModelCount)
	require.Len(t, out.Data, 1)
	assert.Equal(t, "m1", out.Data[0].ID)
	assert.Equal(t, "ACME", out.Data[0].Vendor)
	assert.Equal(t, 56, out.Data[0].MaxBlockRegisters)
	assert.Equal(t, 2, out.Data[0].RegisterCount)
	require.Len(t, out.Data[0].Registers, 2)

	// 레지스터가 자체 주기를 지정하지 않으면 모델 기본 주기를 상속해 방출한다.
	assert.Equal(t, "2s", out.Data[0].Registers[0].PollInterval)
	// enabled 는 항상 유효값으로 방출된다.
	require.NotNil(t, out.Data[0].Registers[0].Enabled)
	assert.True(t, *out.Data[0].Registers[0].Enabled)
	assert.Equal(t, "float32", out.Data[0].Registers[1].DataType)
}

// TestProcessListModels_EmptyDir 은 모델이 없어도 오류 없이 빈 배열을 반환함을 검증한다(AC-17).
func TestProcessListModels_EmptyDir(t *testing.T) {
	t.Setenv(ModelsDirEnv, t.TempDir())
	a, _ := newTestModbusAgent(t, enabledAgentConfig([3]any{nil, nil, nil}),
		&mockModbusTransport{connected: true})

	raw, err := a.Process(mustCommandJSON(t, "list_models"))
	require.NoError(t, err)

	var out struct {
		Data       []any `json:"data"`
		ModelCount int   `json:"model_count"`
	}
	require.NoError(t, json.Unmarshal(raw, &out))
	assert.Equal(t, 0, out.ModelCount)
	assert.Empty(t, out.Data)
}

// mustCommandJSON 은 파라미터 없는 exec 명령 JSON 을 만든다.
func mustCommandJSON(t *testing.T, cmd string) []byte {
	t.Helper()
	b, err := json.Marshal(map[string]any{"command": cmd})
	require.NoError(t, err)
	return b
}

// ---------------------------------------------------------------------------
// AC-19 — 저장소 동봉 GIPAM-115FI 모델
// ---------------------------------------------------------------------------

// TestShippedGIPAMModel 은 저장소에 동봉된 GIPAM-115FI 모델이 검증을 통과하고
// PDF Address Map 과 일치하는 레지스터 구성을 갖는지 확인한다(AC-19).
func TestShippedGIPAMModel(t *testing.T) {
	models := LoadDeviceModels("../../../assets/models", quietLogger())
	var gipam *DeviceModel
	for i := range models {
		if models[i].ID == "gipam-115fi" {
			gipam = &models[i]
			break
		}
	}
	require.NotNil(t, gipam, "gipam-115fi 모델이 동봉되어 있어야 한다")

	assert.Equal(t, TransportRTU, gipam.Transport)
	assert.Equal(t, uint16(56), gipam.Defaults.MaxBlockRegisters,
		"GIPAM-115FI 자체 상한은 56 (PDF p.4 MAX register read count)")
	require.Len(t, gipam.Registers, 72, "레지스터 72건")

	var fc4Count, fc3Count int
	var fc4Words, fc3Words int
	for _, r := range gipam.Registers {
		switch r.FunctionCode {
		case 4:
			fc4Count++
			fc4Words += int(r.Count)
		case 3:
			fc3Count++
			fc3Words += int(r.Count)
		default:
			t.Fatalf("예상 밖 fc %d", r.FunctionCode)
		}
	}
	assert.Equal(t, 25, fc4Count, "입력 레지스터 25건")
	assert.Equal(t, 46, fc4Words, "입력 레지스터 46워드 (주소 0-45)")
	assert.Equal(t, 47, fc3Count, "보유 레지스터 47건")
	assert.Equal(t, 50, fc3Words, "보유 레지스터 50워드")
}

// TestShippedGIPAMModel_ReadPlanReducesTransactions 는 GIPAM 모델을 레지스터별 그룹으로
// 등록했을 때 블록 병합이 실제로 트랜잭션 수를 줄이는지 확인한다(REQ-02 종단 검증).
func TestShippedGIPAMModel_ReadPlanReducesTransactions(t *testing.T) {
	models := LoadDeviceModels("../../../assets/models", quietLogger())
	require.NotEmpty(t, models)
	var gipam DeviceModel
	for _, m := range models {
		if m.ID == "gipam-115fi" {
			gipam = m
		}
	}
	require.Equal(t, "gipam-115fi", gipam.ID)

	groups := make([]RegisterGroupConfig, 0, len(gipam.Registers))
	for _, r := range gipam.Registers {
		poll := r.PollInterval
		if poll == "" {
			poll = gipam.Defaults.PollInterval
		}
		d, err := time.ParseDuration(poll)
		require.NoError(t, err)
		groups = append(groups, RegisterGroupConfig{
			Name: r.Name, FunctionCode: r.FunctionCode, StartAddress: r.Address,
			Quantity: r.Count, DataType: r.DataType, PollInterval: d,
		})
	}

	plan := buildReadPlan(groups, gipam.Defaults.MaxBlockRegisters)

	// 레지스터별 그룹 72개가 훨씬 적은 수의 물리 읽기로 합쳐져야 한다.
	assert.Less(t, len(plan), 20, "72그룹이 20회 미만 트랜잭션으로 합쳐져야 한다")
	t.Logf("GIPAM-115FI: 그룹 %d개 → 물리 읽기 %d회", len(groups), len(plan))

	// 모든 블록은 기종 상한(56) 이내여야 하고, 멤버 합이 원래 그룹 수와 같아야 한다(누락 없음).
	total := 0
	for _, b := range plan {
		assert.LessOrEqual(t, b.Quantity, uint16(56), "블록이 기종 상한을 넘으면 안 된다")
		total += len(b.Members)
	}
	assert.Equal(t, len(groups), total, "모든 그룹이 정확히 한 블록에 속해야 한다")
}

// 컴파일 타임 의존 고정(테스트 헬퍼가 agent 패키지를 쓰는지 확인).
var _ = agent.AgentConfig{}
