package service

import "testing"

func TestParseSortParam(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantField string
		wantAsc   bool
	}{
		{
			name:      "빈 문자열은 기본값 반환",
			input:     "",
			wantField: "name",
			wantAsc:   true,
		},
		{
			name:      "name:asc 파싱",
			input:     "name:asc",
			wantField: "name",
			wantAsc:   true,
		},
		{
			name:      "name:desc 파싱",
			input:     "name:desc",
			wantField: "name",
			wantAsc:   false,
		},
		{
			name:      "created_at:asc 파싱",
			input:     "created_at:asc",
			wantField: "created_at",
			wantAsc:   true,
		},
		{
			name:      "created_at:desc 파싱",
			input:     "created_at:desc",
			wantField: "created_at",
			wantAsc:   false,
		},
		{
			name:      "콜론 없는 문자열은 필드명만 추출하고 오름차순",
			input:     "invalid",
			wantField: "invalid",
			wantAsc:   true,
		},
		{
			name:      "빈 필드명은 기본값 반환",
			input:     ":desc",
			wantField: "name",
			wantAsc:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			field, asc := parseSortParam(tt.input)
			if field != tt.wantField {
				t.Errorf("field 불일치: got=%q, want=%q", field, tt.wantField)
			}
			if asc != tt.wantAsc {
				t.Errorf("ascending 불일치: got=%v, want=%v", asc, tt.wantAsc)
			}
		})
	}
}
