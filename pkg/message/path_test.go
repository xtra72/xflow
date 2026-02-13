package message

import (
	"reflect"
	"testing"
)

// testPathData 는 경로 테스트에 사용할 공통 데이터를 생성한다.
func testPathData() map[string]any {
	return map[string]any{
		"name": "John",
		"user": map[string]any{
			"address": map[string]any{
				"city":    "Seoul",
				"country": "Korea",
			},
			"age": float64(30),
		},
		"items": []any{
			map[string]any{"name": "apple", "price": float64(1000)},
			map[string]any{"name": "banana", "price": float64(500)},
			map[string]any{"name": "cherry", "price": float64(2000)},
		},
		"tags": []any{"go", "tdd", "message"},
	}
}

// TestEvaluatePath_SimpleKey 는 단순 키 접근을 검증한다.
func TestEvaluatePath_SimpleKey(t *testing.T) {
	data := testPathData()
	got, err := evaluatePath(data, "$.name")
	if err != nil {
		t.Fatalf("$.name 에러: %v", err)
	}
	if got != "John" {
		t.Errorf("$.name = %v, 기대값 \"John\"", got)
	}
}

// TestEvaluatePath_NestedAccess 는 중첩된 객체 접근을 검증한다.
func TestEvaluatePath_NestedAccess(t *testing.T) {
	data := testPathData()
	got, err := evaluatePath(data, "$.user.address.city")
	if err != nil {
		t.Fatalf("$.user.address.city 에러: %v", err)
	}
	if got != "Seoul" {
		t.Errorf("$.user.address.city = %v, 기대값 \"Seoul\"", got)
	}
}

// TestEvaluatePath_ArrayIndex 는 배열 인덱스 접근을 검증한다.
func TestEvaluatePath_ArrayIndex(t *testing.T) {
	data := testPathData()

	tests := []struct {
		name     string
		path     string
		expected any
	}{
		{name: "첫 번째 요소", path: "$.items[0]", expected: map[string]any{"name": "apple", "price": float64(1000)}},
		{name: "마지막 요소", path: "$.items[2]", expected: map[string]any{"name": "cherry", "price": float64(2000)}},
		{name: "단순 배열 요소", path: "$.tags[1]", expected: "tdd"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := evaluatePath(data, tt.path)
			if err != nil {
				t.Fatalf("%s 에러: %v", tt.path, err)
			}
			if !reflect.DeepEqual(got, tt.expected) {
				t.Errorf("%s = %v, 기대값 %v", tt.path, got, tt.expected)
			}
		})
	}
}

// TestEvaluatePath_ArrayWildcard 는 배열 와일드카드 접근을 검증한다.
func TestEvaluatePath_ArrayWildcard(t *testing.T) {
	data := testPathData()
	got, err := evaluatePath(data, "$.items[*].name")
	if err != nil {
		t.Fatalf("$.items[*].name 에러: %v", err)
	}

	expected := []any{"apple", "banana", "cherry"}
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("$.items[*].name = %v, 기대값 %v", got, expected)
	}
}

// TestEvaluatePath_InvalidPath 는 잘못된 경로 구문에 대해 ErrInvalidPath를 반환하는지 검증한다.
func TestEvaluatePath_InvalidPath(t *testing.T) {
	data := testPathData()

	invalidPaths := []struct {
		name string
		path string
	}{
		{name: "$. 접두사 없음", path: "name"},
		{name: "빈 문자열", path: ""},
		{name: "$ 만", path: "$"},
		{name: "점 없이 $key", path: "$name"},
	}

	for _, tt := range invalidPaths {
		t.Run(tt.name, func(t *testing.T) {
			_, err := evaluatePath(data, tt.path)
			if err != ErrInvalidPath {
				t.Errorf("경로 %q 에러 = %v, 기대값 ErrInvalidPath", tt.path, err)
			}
		})
	}
}

// TestEvaluatePath_PathNotFound 는 존재하지 않는 경로에 대해 ErrPathNotFound를 반환하는지 검증한다.
func TestEvaluatePath_PathNotFound(t *testing.T) {
	data := testPathData()

	tests := []struct {
		name string
		path string
	}{
		{name: "존재하지 않는 키", path: "$.nonexistent"},
		{name: "중첩 경로 없음", path: "$.user.phone"},
		{name: "깊은 중첩 없음", path: "$.user.address.zip"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := evaluatePath(data, tt.path)
			if err != ErrPathNotFound {
				t.Errorf("경로 %q 에러 = %v, 기대값 ErrPathNotFound", tt.path, err)
			}
		})
	}
}

// TestEvaluatePath_ArrayOutOfBounds 는 배열 인덱스 범위 초과에 대해 ErrPathNotFound를 반환하는지 검증한다.
func TestEvaluatePath_ArrayOutOfBounds(t *testing.T) {
	data := testPathData()
	_, err := evaluatePath(data, "$.items[99]")
	if err != ErrPathNotFound {
		t.Errorf("범위 초과 에러 = %v, 기대값 ErrPathNotFound", err)
	}
}

// TestEvaluatePath_EmptyData 는 빈 데이터에 대해 ErrPathNotFound를 반환하는지 검증한다.
func TestEvaluatePath_EmptyData(t *testing.T) {
	data := map[string]any{}
	_, err := evaluatePath(data, "$.anything")
	if err != ErrPathNotFound {
		t.Errorf("빈 데이터 에러 = %v, 기대값 ErrPathNotFound", err)
	}
}

// TestEvaluatePath_ArrayIndexWithNestedAccess 는 배열 인덱스 후 필드 접근을 검증한다.
func TestEvaluatePath_ArrayIndexWithNestedAccess(t *testing.T) {
	data := testPathData()
	got, err := evaluatePath(data, "$.items[0].name")
	if err != nil {
		t.Fatalf("$.items[0].name 에러: %v", err)
	}
	if got != "apple" {
		t.Errorf("$.items[0].name = %v, 기대값 \"apple\"", got)
	}
}
