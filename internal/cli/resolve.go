package cli

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

// uuidPattern 은 UUID v4 형식을 검사하는 정규표현식이다.
var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// isUUID 는 문자열이 UUID 형식인지 검사한다.
func isUUID(s string) bool {
	return uuidPattern.MatchString(s)
}

// resolveEntityArg 는 positional 인자와 --name 플래그에서 엔티티 식별자를 추출한다.
// 둘 다 존재하면 에러, 둘 다 없으면 에러를 반환한다.
func resolveEntityArg(args []string, name string) (string, error) {
	var idOrName string
	if len(args) > 0 {
		idOrName = args[0]
	}
	if name != "" {
		if idOrName != "" {
			return "", ErrInvalidInput("ID와 --name 플래그를 동시에 사용할 수 없습니다")
		}
		idOrName = name
	}
	if idOrName == "" {
		return "", ErrInvalidInput("대상을 지정해주세요 (ID 또는 --name)")
	}
	return idOrName, nil
}

// filterByName 은 맵 슬라이스를 이름 필드의 부분 문자열로 필터링한다.
// 대소문자를 구분하지 않으며, nameFilter 가 빈 문자열이면 원본을 그대로 반환한다.
func filterByName(items []map[string]any, nameFilter, fieldName string) []map[string]any {
	if nameFilter == "" {
		return items
	}
	lower := strings.ToLower(nameFilter)
	var filtered []map[string]any
	for _, item := range items {
		val, _ := item[fieldName].(string)
		if strings.Contains(strings.ToLower(val), lower) {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

// resolveAgentID 는 인자를 에이전트 ID로 해석한다.
// 에이전트 목록에서 이름이 일치하는 항목을 찾아 ID를 반환한다.
// 이름이 중복되면 에러를 반환하고, 일치하는 이름이 없으면 원본을 그대로 반환한다.
func resolveAgentID(client *Client, idOrName string) (string, error) {
	// UUID 형식이면 바로 반환 (API 호출 불필요)
	if isUUID(idOrName) {
		return idOrName, nil
	}
	// 이름으로 검색
	var agents []map[string]any
	if err := client.Get("/api/v1/agents", &agents); err != nil {
		if client.verbose {
			fmt.Fprintf(os.Stderr, "[resolve] 에이전트 목록 조회 실패: %v\n", err)
		}
		// 목록 조회 실패 시 원본 그대로 반환 (서버가 ID로 처리)
		return idOrName, nil
	}

	var matches []string
	for _, a := range agents {
		name, _ := a["name"].(string)
		if name == idOrName {
			id, _ := a["id"].(string)
			matches = append(matches, id)
		}
	}

	switch len(matches) {
	case 0:
		// 이름 매칭 없음 → 원본 그대로 반환 (서버가 ID로 처리 시도)
		return idOrName, nil
	case 1:
		return matches[0], nil
	default:
		return "", fmt.Errorf("동일한 이름의 에이전트가 %d개 있습니다: %q (ID를 사용하세요)", len(matches), idOrName)
	}
}
