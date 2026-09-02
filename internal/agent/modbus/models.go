package modbus

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"time"

	modbus "github.com/xtra/xflow/internal/modbus"
)

// ---------------------------------------------------------------------------
// SPEC-MODBUS-013 REQ-04 — 디바이스 모델 카탈로그
// ---------------------------------------------------------------------------
//
// 동일 기종을 여러 대 등록할 때 레지스터 정의를 매번 다시 입력하는 비용을 없애기 위해,
// 기종별 레지스터 맵을 JSON 파일로 두고 카탈로그로 노출한다.
//
// 저장 위치는 런타임 디렉터리 `~/.xflow/models/*.json` 이다(환경변수 XFLOW_MODELS_DIR 로 override).
// 바이너리 내장이 아니라 파일 스캔인 이유: 기종 추가에 재빌드·재배포가 필요 없어야 하기 때문이다.
//
// 로딩은 **파일 단위 fail-open** 이다. 깨진 파일 하나가 카탈로그 전체를 막지 않는다 —
// 무효 파일은 경고 로그를 남기고 건너뛰며, 나머지 파일은 정상 로드된다.

// ModelsDirEnv 는 모델 디렉터리를 덮어쓰는 환경변수 이름이다(테스트·배포 경로 분리용).
const ModelsDirEnv = "XFLOW_MODELS_DIR"

// ModelDefaults 는 모델 전체에 적용되는 기본값이다.
type ModelDefaults struct {
	// MaxBlockRegisters 는 이 기종의 블록 상한 권장값이다(0 이면 미지정).
	// 예: GIPAM-115FI 는 자체 상한이 56 이라 규격 상한 125 를 쓸 수 없다.
	MaxBlockRegisters uint16 `json:"max_block_registers,omitempty"`
	// PollInterval 은 레지스터가 자체 주기를 지정하지 않을 때 상속되는 기본 주기이다.
	PollInterval string `json:"poll_interval,omitempty"`
}

// ModelRegister 는 모델이 정의하는 레지스터 그룹 1건이다.
// 필드 이름은 일괄등록 열 순서(fc, 주소, 개수, 데이터타입, 폴링간격, 설명, 사용)와 정렬한다.
type ModelRegister struct {
	FunctionCode byte   `json:"fc"`
	Address      uint16 `json:"address"`
	Count        uint16 `json:"count"`
	DataType     string `json:"data_type,omitempty"`
	PollInterval string `json:"poll_interval,omitempty"`
	Name         string `json:"name,omitempty"`
	// Enabled 가 nil 이면 사용(true) — RegisterGroupConfig.Enabled 와 동일 규약.
	Enabled *bool `json:"enabled,omitempty"`
}

// DeviceModel 은 카탈로그 항목 하나(기종 하나)이다.
type DeviceModel struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Vendor      string          `json:"vendor,omitempty"`
	Description string          `json:"description,omitempty"`
	Transport   string          `json:"transport,omitempty"`
	Defaults    ModelDefaults   `json:"defaults,omitempty"`
	Registers   []ModelRegister `json:"registers"`

	// SourcePath 는 이 모델이 로드된 파일 경로이다(응답에는 포함하지 않는 진단용 필드).
	SourcePath string `json:"-"`
}

// ModelsDir 은 모델 카탈로그 디렉터리 경로를 반환한다.
// XFLOW_MODELS_DIR 이 설정되어 있으면 그 값을, 아니면 ~/.xflow/models 를 쓴다.
// 홈 디렉터리를 얻을 수 없으면 빈 문자열을 반환하며, 호출자는 빈 카탈로그로 처리한다.
func ModelsDir() string {
	if v := os.Getenv(ModelsDirEnv); v != "" {
		return v
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".xflow", "models")
}

// validateDeviceModel 은 모델의 필수 필드와 레지스터 정의를 검증한다.
// 하나라도 어긋나면 오류를 반환하며, 호출자는 해당 파일을 건너뛴다(fail-open).
func validateDeviceModel(m DeviceModel) error {
	if m.ID == "" {
		return fmt.Errorf("id is required")
	}
	if m.Name == "" {
		return fmt.Errorf("name is required")
	}
	switch m.Transport {
	case "", TransportTCP, TransportRTU:
	default:
		return fmt.Errorf("transport %q must be empty, %q, or %q", m.Transport, TransportTCP, TransportRTU)
	}
	if m.Defaults.PollInterval != "" {
		if _, err := time.ParseDuration(m.Defaults.PollInterval); err != nil {
			return fmt.Errorf("defaults.poll_interval invalid: %w", err)
		}
	}
	if len(m.Registers) == 0 {
		return fmt.Errorf("registers must not be empty")
	}
	for i, r := range m.Registers {
		if r.FunctionCode < 1 || r.FunctionCode > 4 {
			return fmt.Errorf("registers[%d].fc must be 1-4 (got %d)", i, r.FunctionCode)
		}
		if r.Count == 0 {
			return fmt.Errorf("registers[%d].count must be > 0", i)
		}
		if int(r.Address)+int(r.Count) > 65536 {
			return fmt.Errorf("registers[%d] range %d+%d exceeds address space", i, r.Address, r.Count)
		}
		if r.DataType != "" && !modbus.IsValidDataType(r.DataType) {
			return fmt.Errorf("registers[%d].data_type %q is not supported", i, r.DataType)
		}
		if r.PollInterval != "" {
			if _, err := time.ParseDuration(r.PollInterval); err != nil {
				return fmt.Errorf("registers[%d].poll_interval invalid: %w", i, err)
			}
		}
	}
	return nil
}

// LoadDeviceModels 는 디렉터리의 *.json 을 스캔해 유효한 모델만 반환한다(REQ-04).
//
// fail-open 규약:
//   - 디렉터리가 없거나 읽을 수 없으면 빈 슬라이스를 오류 없이 반환한다(AC-17).
//   - JSON 파싱 실패·검증 실패 파일은 경고 로그 후 건너뛴다(AC-16).
//   - id 가 중복되면 파일명 사전순으로 먼저 온 파일이 이기고 나머지는 건너뛴다(AC-18).
//
// 반환 순서는 파일명 사전순으로 결정적이다.
func LoadDeviceModels(dir string, logger *slog.Logger) []DeviceModel {
	if dir == "" {
		return nil
	}
	paths, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil || len(paths) == 0 {
		return nil
	}
	sort.Strings(paths) // Glob 은 정렬을 보장하지만 명시해 결정성을 고정한다.

	seen := make(map[string]string, len(paths))
	models := make([]DeviceModel, 0, len(paths))

	for _, path := range paths {
		raw, err := os.ReadFile(path) //nolint:gosec // 관리자가 배치한 로컬 카탈로그 디렉터리.
		if err != nil {
			logModelSkip(logger, path, "읽기 실패", err)
			continue
		}
		var m DeviceModel
		if err := json.Unmarshal(raw, &m); err != nil {
			logModelSkip(logger, path, "JSON 파싱 실패", err)
			continue
		}
		if err := validateDeviceModel(m); err != nil {
			logModelSkip(logger, path, "검증 실패", err)
			continue
		}
		if prev, dup := seen[m.ID]; dup {
			logModelSkip(logger, path, "id 중복 — 먼저 로드된 파일 유지: "+prev, nil)
			continue
		}
		seen[m.ID] = path
		m.SourcePath = path
		models = append(models, m)
	}
	return models
}

// logModelSkip 은 건너뛴 모델 파일을 경고 로그로 남긴다(logger 가 nil 이면 무시).
func logModelSkip(logger *slog.Logger, path, reason string, err error) {
	if logger == nil {
		return
	}
	args := []any{"file", path, "reason", reason}
	if err != nil {
		args = append(args, "error", err)
	}
	logger.Warn("modbus: 디바이스 모델 건너뜀", args...)
}
