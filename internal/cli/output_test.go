package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Formatter interface tests ---

// TestNewFormatter_SupportedFormats - 지원되는 모든 출력 형식의 생성 검증
func TestNewFormatter_SupportedFormats(t *testing.T) {
	tests := []struct {
		name   string
		format string
	}{
		{"JSON 포맷터", "json"},
		{"YAML 포맷터", "yaml"},
		{"Table 포맷터", "table"},
		{"Text 포맷터", "text"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, err := NewFormatter(tt.format)
			require.NoError(t, err, "%s 생성 시 에러가 없어야 합니다", tt.format)
			assert.NotNil(t, f, "%s 포맷터가 nil 이면 안됩니다", tt.format)
		})
	}
}

// TestNewFormatter_UnsupportedFormat - 지원하지 않는 출력 형식의 에러 메시지 검증
func TestNewFormatter_UnsupportedFormat(t *testing.T) {
	f, err := NewFormatter("xml")
	assert.Nil(t, f, "지원하지 않는 형식은 nil 포맷터를 반환해야 합니다")
	require.Error(t, err, "지원하지 않는 형식은 에러를 반환해야 합니다")
	assert.Contains(t, err.Error(), "지원하지 않는 출력 형식입니다: xml",
		"에러 메시지에 형식 이름이 포함되어야 합니다")
	assert.Contains(t, err.Error(), "사용 가능: json, yaml, table, text",
		"에러 메시지에 사용 가능한 형식 목록이 포함되어야 합니다")
}

// --- JSONFormatter tests ---

// TestJSONFormatter_Format_Map - JSON 포맷터의 맵 출력 검증
func TestJSONFormatter_Format_Map(t *testing.T) {
	f, err := NewFormatter("json")
	require.NoError(t, err)

	data := map[string]string{"name": "테스트", "status": "active"}
	var buf bytes.Buffer
	err = f.Format(data, &buf)
	require.NoError(t, err, "JSON 포맷팅 에러가 없어야 합니다")

	// Verify it's valid JSON with 2-space indent
	var result map[string]string
	err = json.Unmarshal(buf.Bytes(), &result)
	require.NoError(t, err, "출력이 유효한 JSON 이어야 합니다")
	assert.Equal(t, "테스트", result["name"])
	assert.Contains(t, buf.String(), "  ", "2 칸 들여쓰기가 적용되어야 합니다")
}

// TestJSONFormatter_Format_Slice - JSON 포맷터의 슬라이스 출력 검증
func TestJSONFormatter_Format_Slice(t *testing.T) {
	f, err := NewFormatter("json")
	require.NoError(t, err)

	data := []string{"a", "b", "c"}
	var buf bytes.Buffer
	err = f.Format(data, &buf)
	require.NoError(t, err)

	var result []string
	err = json.Unmarshal(buf.Bytes(), &result)
	require.NoError(t, err, "출력이 유효한 JSON 배열이어야 합니다")
	assert.Equal(t, []string{"a", "b", "c"}, result)
}

// TestJSONFormatter_Format_TrailingNewline - JSON 출력 끝에 개행이 있는지 검증
func TestJSONFormatter_Format_TrailingNewline(t *testing.T) {
	f, err := NewFormatter("json")
	require.NoError(t, err)

	var buf bytes.Buffer
	err = f.Format("hello", &buf)
	require.NoError(t, err)
	assert.True(t, strings.HasSuffix(buf.String(), "\n"),
		"JSON 출력은 개행으로 끝나야 합니다")
}

// --- YAMLFormatter tests ---

// TestYAMLFormatter_Format_Map - YAML 포맷터의 맵 출력 검증
func TestYAMLFormatter_Format_Map(t *testing.T) {
	f, err := NewFormatter("yaml")
	require.NoError(t, err)

	data := map[string]string{"name": "워크플로우", "status": "running"}
	var buf bytes.Buffer
	err = f.Format(data, &buf)
	require.NoError(t, err, "YAML 포맷팅 에러가 없어야 합니다")

	output := buf.String()
	assert.Contains(t, output, "name:", "YAML 에 name 키가 포함되어야 합니다")
	assert.Contains(t, output, "워크플로우", "YAML 에 값이 포함되어야 합니다")
}

// TestYAMLFormatter_Format_Slice - YAML 포맷터의 슬라이스 출력 검증
func TestYAMLFormatter_Format_Slice(t *testing.T) {
	f, err := NewFormatter("yaml")
	require.NoError(t, err)

	data := []string{"alpha", "beta"}
	var buf bytes.Buffer
	err = f.Format(data, &buf)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "- alpha", "YAML 에 슬라이스 항목이 포함되어야 합니다")
	assert.Contains(t, output, "- beta", "YAML 에 슬라이스 항목이 포함되어야 합니다")
}

// --- TableFormatter tests ---

// TestTableFormatter_Format - 테이블 포맷터의 출력 검증
func TestTableFormatter_Format(t *testing.T) {
	headers := []string{"NAME", "STATUS", "AGE"}
	rowFunc := func(item any) []string {
		m := item.(map[string]string)
		return []string{m["name"], m["status"], m["age"]}
	}

	f := NewTableFormatter(headers, rowFunc)
	require.NotNil(t, f, "TableFormatter 는 nil 이면 안됩니다")

	data := []any{
		map[string]string{"name": "wf-1", "status": "running", "age": "5m"},
		map[string]string{"name": "wf-2", "status": "stopped", "age": "1h"},
	}

	var buf bytes.Buffer
	err := f.Format(data, &buf)
	require.NoError(t, err, "테이블 포맷팅 에러가 없어야 합니다")

	output := buf.String()
	assert.Contains(t, output, "NAME", "헤더에 NAME 이 포함되어야 합니다")
	assert.Contains(t, output, "STATUS", "헤더에 STATUS 가 포함되어야 합니다")
	assert.Contains(t, output, "wf-1", "데이터에 wf-1 이 포함되어야 합니다")
	assert.Contains(t, output, "wf-2", "데이터에 wf-2 이 포함되어야 합니다")
}

// TestTableFormatter_Format_EmptyData - 빈 데이터에 대한 테이블 출력 검증
func TestTableFormatter_Format_EmptyData(t *testing.T) {
	headers := []string{"NAME"}
	rowFunc := func(item any) []string {
		return []string{item.(string)}
	}

	f := NewTableFormatter(headers, rowFunc)
	var buf bytes.Buffer
	err := f.Format([]any{}, &buf)
	require.NoError(t, err, "빈 데이터 테이블 포맷팅 에러가 없어야 합니다")

	output := buf.String()
	assert.Contains(t, output, "NAME", "빈 데이터에도 헤더가 출력되어야 합니다")
}

// --- TextFormatter tests ---

// TestTextFormatter_Format_Map - 텍스트 포맷터의 맵 출력 검증
func TestTextFormatter_Format_Map(t *testing.T) {
	f, err := NewFormatter("text")
	require.NoError(t, err)

	// Use an ordered struct to test predictable output
	type item struct {
		Name   string
		Status string
	}
	data := map[string]string{"name": "wf-1", "status": "running"}
	var buf bytes.Buffer
	err = f.Format(data, &buf)
	require.NoError(t, err, "텍스트 포맷팅 에러가 없어야 합니다")

	output := buf.String()
	assert.Contains(t, output, "name: wf-1", "텍스트에 key: value 형식이 포함되어야 합니다")
	assert.Contains(t, output, "status: running", "텍스트에 key: value 형식이 포함되어야 합니다")
}

// TestTextFormatter_Format_Slice - 텍스트 포맷터의 슬라이스 출력 검증
func TestTextFormatter_Format_Slice(t *testing.T) {
	f, err := NewFormatter("text")
	require.NoError(t, err)

	data := []string{"alpha", "beta", "gamma"}
	var buf bytes.Buffer
	err = f.Format(data, &buf)
	require.NoError(t, err)

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	assert.Len(t, lines, 3, "슬라이스의 각 항목이 한 줄에 출력되어야 합니다")
	assert.Equal(t, "alpha", lines[0])
	assert.Equal(t, "beta", lines[1])
	assert.Equal(t, "gamma", lines[2])
}

// TestTextFormatter_Format_String - 텍스트 포맷터의 단일 문자열 출력 검증
func TestTextFormatter_Format_String(t *testing.T) {
	f, err := NewFormatter("text")
	require.NoError(t, err)

	var buf bytes.Buffer
	err = f.Format("단순 텍스트", &buf)
	require.NoError(t, err)
	assert.Equal(t, "단순 텍스트\n", buf.String(),
		"단일 문자열은 그대로 출력되어야 합니다")
}

// --- PrintResult tests ---

// TestPrintResult_JSON - PrintResult JSON 출력 검증
func TestPrintResult_JSON(t *testing.T) {
	data := map[string]string{"id": "123"}
	var buf bytes.Buffer

	err := PrintResult(&buf, "json", data, nil, nil)
	require.NoError(t, err, "PrintResult JSON 에러가 없어야 합니다")

	var result map[string]string
	err = json.Unmarshal(buf.Bytes(), &result)
	require.NoError(t, err)
	assert.Equal(t, "123", result["id"])
}

// TestPrintResult_Table - PrintResult 테이블 출력 검증
func TestPrintResult_Table(t *testing.T) {
	data := []any{
		map[string]string{"name": "item1"},
	}
	headers := []string{"NAME"}
	rowFunc := func(item any) []string {
		return []string{item.(map[string]string)["name"]}
	}

	var buf bytes.Buffer
	err := PrintResult(&buf, "table", data, headers, rowFunc)
	require.NoError(t, err, "PrintResult table 에러가 없어야 합니다")
	assert.Contains(t, buf.String(), "NAME")
	assert.Contains(t, buf.String(), "item1")
}

// TestPrintResult_UnsupportedFormat - PrintResult 미지원 형식 에러 검증
func TestPrintResult_UnsupportedFormat(t *testing.T) {
	var buf bytes.Buffer
	err := PrintResult(&buf, "csv", nil, nil, nil)
	require.Error(t, err, "미지원 형식은 에러를 반환해야 합니다")
}

// --- IsColorEnabled tests ---

// TestIsColorEnabled - 색상 지원 여부 판별 검증
func TestIsColorEnabled(t *testing.T) {
	tests := []struct {
		name     string
		noColor  bool
		expected bool
	}{
		{
			name:     "noColor 플래그가 true 이면 색상 비활성화",
			noColor:  true,
			expected: false,
		},
		{
			name:     "버퍼(비터미널)에서는 색상 비활성화",
			noColor:  false,
			expected: false, // bytes.Buffer is not a terminal
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			result := IsColorEnabled(tt.noColor, &buf)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// --- StartSpinner tests ---

// TestStartSpinner - 스피너 시작/정지 동작 검증
func TestStartSpinner(t *testing.T) {
	var buf bytes.Buffer
	stop := StartSpinner(&buf, "로딩 중...")
	require.NotNil(t, stop, "stop 함수가 nil 이면 안됩니다")

	// Let the spinner run briefly
	time.Sleep(200 * time.Millisecond)
	stop()

	output := buf.String()
	assert.NotEmpty(t, output, "스피너가 출력을 생성해야 합니다")
	assert.Contains(t, output, "로딩 중...", "스피너 메시지가 출력에 포함되어야 합니다")
}

// --- DetailFormatter tests ---

// TestDetailFormatter_NestedMapConfig - 중첩 맵 구조의 섹션 렌더링 검증
func TestDetailFormatter_NestedMapConfig(t *testing.T) {
	df := NewDetailFormatter(
		[]string{"name", "type"},
		map[string]string{
			"name":   "Name",
			"type":   "Type",
			"config": "Config",
		},
		map[string]bool{"config": true},
	)

	data := map[string]any{
		"name": "modbus-gateway",
		"type": "modbus-gateway",
		"config": map[string]any{
			"listen_port": 5020,
			"register_map": map[string]any{
				"coils": map[string]any{
					"count":         8,
					"start_address": 0,
				},
				"input_registers": map[string]any{
					"count":     64,
					"data_type": "uint16",
				},
			},
		},
	}

	var buf bytes.Buffer
	err := df.Format(data, &buf)
	require.NoError(t, err)

	output := buf.String()
	// 스칼라 필드 검증
	assert.Contains(t, output, "Name:")
	assert.Contains(t, output, "modbus-gateway")

	// 중첩 맵이 %v (map[key:value]) 형식이 아닌 재귀 렌더링 되어야 함
	assert.NotContains(t, output, "map[coils:", "중첩 맵이 Go 기본 형식으로 출력되면 안 됨")
	assert.NotContains(t, output, "map[count:", "중첩 맵이 Go 기본 형식으로 출력되면 안 됨")

	// 재귀적으로 키-값 쌍이 들여쓰기되어 출력되어야 함
	assert.Contains(t, output, "register_map:")
	assert.Contains(t, output, "coils:")
	assert.Contains(t, output, "count: 8")
	assert.Contains(t, output, "start_address: 0")
	assert.Contains(t, output, "input_registers:")
	assert.Contains(t, output, "data_type: uint16")
}

// TestDetailFormatter_SliceWithMaps - 슬라이스 내 맵 항목 렌더링 검증
func TestDetailFormatter_SliceWithMaps(t *testing.T) {
	df := NewDetailFormatter(
		[]string{"name"},
		map[string]string{"name": "Name", "items": "Items"},
		map[string]bool{"items": true},
	)

	t.Run("균일 맵 슬라이스는 미니 테이블 렌더링", func(t *testing.T) {
		data := map[string]any{
			"name": "test",
			"items": []any{
				map[string]any{"address": 0, "data_type": "float32"},
				map[string]any{"address": 2, "data_type": "uint16"},
			},
		}

		var buf bytes.Buffer
		err := df.Format(data, &buf)
		require.NoError(t, err)

		output := buf.String()
		assert.Contains(t, output, "ADDRESS")
		assert.Contains(t, output, "DATA_TYPE")
		assert.Contains(t, output, "float32")
		assert.Contains(t, output, "uint16")
	})

	t.Run("비균일 맵 슬라이스는 개별 렌더링", func(t *testing.T) {
		data := map[string]any{
			"name": "test",
			"items": []any{
				map[string]any{"address": 0, "data_type": "float32"},
				map[string]any{"address": 2, "data_type": "uint16", "extra": "field"},
			},
		}

		var buf bytes.Buffer
		err := df.Format(data, &buf)
		require.NoError(t, err)

		output := buf.String()
		// Go 기본 map 형식이 아닌 재귀 렌더링
		assert.NotContains(t, output, "map[address:", "비균일 맵이 Go 기본 형식으로 출력되면 안 됨")
		assert.Contains(t, output, "address: 0")
		assert.Contains(t, output, "data_type: float32")
		assert.Contains(t, output, "extra: field")
	})
}

// TestStartSpinner_StopIdempotent - 스피너 정지 함수 중복 호출 안전성 검증
func TestStartSpinner_StopIdempotent(t *testing.T) {
	var buf bytes.Buffer
	stop := StartSpinner(&buf, "테스트")

	time.Sleep(50 * time.Millisecond)
	stop()
	// Calling stop again should not panic
	assert.NotPanics(t, func() {
		stop()
	}, "stop 함수 중복 호출 시 패닉이 발생하면 안됩니다")
}
