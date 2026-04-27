package service

import "strings"

// parseSortParam 은 "field:direction" 형식의 정렬 파라미터를 파싱하여
// 필드명과 오름차순 여부를 반환한다.
// 빈 문자열이거나 필드명이 비어있으면 기본값 ("name", true) 을 반환한다.
func parseSortParam(sort string) (field string, ascending bool) {
	if sort == "" {
		return "name", true
	}
	parts := strings.SplitN(sort, ":", 2)
	field = parts[0]
	if field == "" {
		return "name", true
	}
	ascending = true
	if len(parts) == 2 && parts[1] == "desc" {
		ascending = false
	}
	return field, ascending
}
