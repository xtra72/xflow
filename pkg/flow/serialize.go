package flow

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"gopkg.in/yaml.v3"
)

// ---------------------------------------------------------------------------
// flowJSON – JSON 직렬화용 중간 표현 구조체
// ---------------------------------------------------------------------------

// flowJSON 은 Flow를 JSON으로 직렬화/역직렬화하기 위한 비공개 중간 구조체이다.
type flowJSON struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	State       string            `json:"state"`
	Config      FlowConfig        `json:"config"`
	Nodes       []NodeDef         `json:"nodes"`
	Wires       []Wire            `json:"wires"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
}

// ---------------------------------------------------------------------------
// MarshalJSON – *defaultFlow → JSON
// ---------------------------------------------------------------------------

// MarshalJSON 은 defaultFlow를 JSON 바이트로 직렬화한다.
// flowJSON 중간 구조체를 통해 직렬화를 수행한다.
func (f *defaultFlow) MarshalJSON() ([]byte, error) {
	fj := flowJSON{
		ID:          f.id,
		Name:        f.name,
		Description: f.description,
		State:       string(f.state),
		Config:      f.config,
		Nodes:       f.nodes,
		Wires:       f.wires,
		Metadata:    f.metadata,
		CreatedAt:   f.createdAt,
		UpdatedAt:   f.updatedAt,
	}

	if fj.Nodes == nil {
		fj.Nodes = []NodeDef{}
	}
	if fj.Wires == nil {
		fj.Wires = []Wire{}
	}

	return json.Marshal(fj)
}

// ---------------------------------------------------------------------------
// FlowFromJSON – JSON → Flow
// ---------------------------------------------------------------------------

// FlowFromJSON 은 JSON 바이트를 Flow 인터페이스로 역직렬화한다.
// Name이 비어있으면 ErrFlowNameRequired를 반환한다.
// ID가 비어있으면 새로운 UUID를 자동 생성한다.
// State가 비어있으면 FlowStored를 기본값으로 사용한다.
func FlowFromJSON(data []byte) (Flow, error) {
	var fj flowJSON
	if err := json.Unmarshal(data, &fj); err != nil {
		return nil, fmt.Errorf("json unmarshal: %w", err)
	}

	return buildFlowFromIntermediate(fj)
}

// ---------------------------------------------------------------------------
// FlowToYAML – Flow → YAML
// ---------------------------------------------------------------------------

// FlowToYAML 은 Flow를 YAML 바이트로 직렬화한다.
// 내부적으로 JSON 중간 표현을 거쳐 YAML로 변환한다.
func FlowToYAML(f Flow) ([]byte, error) {
	df, ok := f.(*defaultFlow)
	if !ok {
		return nil, fmt.Errorf("FlowToYAML: unsupported Flow implementation")
	}

	// JSON 바이트로 직렬화
	jsonData, err := df.MarshalJSON()
	if err != nil {
		return nil, fmt.Errorf("marshal json for yaml: %w", err)
	}

	// JSON → 범용 map
	var m map[string]any
	if err := json.Unmarshal(jsonData, &m); err != nil {
		return nil, fmt.Errorf("json to map: %w", err)
	}

	// map → YAML
	yamlData, err := yaml.Marshal(m)
	if err != nil {
		return nil, fmt.Errorf("yaml marshal: %w", err)
	}

	return yamlData, nil
}

// ---------------------------------------------------------------------------
// FlowFromYAML – YAML → Flow
// ---------------------------------------------------------------------------

// FlowFromYAML 은 YAML 바이트를 Flow 인터페이스로 역직렬화한다.
// 내부적으로 YAML을 범용 map으로 변환한 후 JSON을 거쳐 FlowFromJSON을 사용한다.
func FlowFromYAML(data []byte) (Flow, error) {
	// YAML → 범용 map
	var m map[string]any
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("yaml unmarshal: %w", err)
	}

	// map → JSON
	jsonData, err := json.Marshal(m)
	if err != nil {
		return nil, fmt.Errorf("map to json: %w", err)
	}

	return FlowFromJSON(jsonData)
}

// ---------------------------------------------------------------------------
// LoadFlowFromFile – 파일에서 Flow 로드
// ---------------------------------------------------------------------------

// LoadFlowFromFile 은 파일 확장자에 따라 JSON 또는 YAML 파일에서 Flow를 로드한다.
// 지원되지 않는 확장자이면 ErrUnsupportedFormat을 반환한다.
// 파일이 존재하지 않으면 os.ErrNotExist를 반환한다.
func LoadFlowFromFile(path string) (Flow, error) {
	ext := filepath.Ext(path)

	switch ext {
	case ".json":
		// JSON 파일 읽기
	case ".yaml", ".yml":
		// YAML 파일 읽기
	default:
		return nil, ErrUnsupportedFormat
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	switch ext {
	case ".json":
		return FlowFromJSON(data)
	case ".yaml", ".yml":
		return FlowFromYAML(data)
	default:
		return nil, ErrUnsupportedFormat
	}
}

// ---------------------------------------------------------------------------
// SaveFlowToFile – Flow를 파일에 저장
// ---------------------------------------------------------------------------

// SaveFlowToFile 은 파일 확장자에 따라 Flow를 JSON 또는 YAML 파일로 저장한다.
// JSON 파일은 들여쓰기가 적용된 pretty-printed 형식으로 저장된다.
// 지원되지 않는 확장자이면 ErrUnsupportedFormat을 반환한다.
func SaveFlowToFile(f Flow, path string) error {
	ext := filepath.Ext(path)

	switch ext {
	case ".json":
		df, ok := f.(*defaultFlow)
		if !ok {
			return fmt.Errorf("SaveFlowToFile: unsupported Flow implementation")
		}

		// flowJSON 중간 구조체 생성
		fj := flowJSON{
			ID:          df.id,
			Name:        df.name,
			Description: df.description,
			State:       string(df.state),
			Config:      df.config,
			Nodes:       df.nodes,
			Wires:       df.wires,
			Metadata:    df.metadata,
			CreatedAt:   df.createdAt,
			UpdatedAt:   df.updatedAt,
		}
		if fj.Nodes == nil {
			fj.Nodes = []NodeDef{}
		}
		if fj.Wires == nil {
			fj.Wires = []Wire{}
		}

		data, err := json.MarshalIndent(fj, "", "  ")
		if err != nil {
			return fmt.Errorf("json marshal indent: %w", err)
		}

		return os.WriteFile(path, data, 0644)

	case ".yaml", ".yml":
		data, err := FlowToYAML(f)
		if err != nil {
			return err
		}

		return os.WriteFile(path, data, 0644)

	default:
		return ErrUnsupportedFormat
	}
}

// ---------------------------------------------------------------------------
// 비공개 헬퍼
// ---------------------------------------------------------------------------

// buildFlowFromIntermediate 는 flowJSON 중간 구조체에서 Flow를 구성한다.
func buildFlowFromIntermediate(fj flowJSON) (Flow, error) {
	// 이름 필수 검증
	if fj.Name == "" {
		return nil, ErrFlowNameRequired
	}

	// ID가 비어있으면 자동 생성
	if fj.ID == "" {
		fj.ID = uuid.New().String()
	}

	// State 파싱 (비어있으면 FlowStored 기본값)
	var state FlowState
	if fj.State == "" {
		state = FlowStored
	} else {
		parsed, err := ParseFlowState(fj.State)
		if err != nil {
			return nil, fmt.Errorf("parse flow state: %w", err)
		}
		state = parsed
	}

	if fj.Nodes == nil {
		fj.Nodes = []NodeDef{}
	}
	if fj.Wires == nil {
		fj.Wires = []Wire{}
	}
	if fj.Metadata == nil {
		fj.Metadata = make(map[string]string)
	}

	return &defaultFlow{
		id:          fj.ID,
		name:        fj.Name,
		description: fj.Description,
		state:       state,
		nodes:       fj.Nodes,
		wires:       fj.Wires,
		config:      fj.Config,
		metadata:    fj.Metadata,
		createdAt:   fj.CreatedAt,
		updatedAt:   fj.UpdatedAt,
	}, nil
}
