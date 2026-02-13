package message

import (
	"strconv"
	"strings"
)

// evaluatePath 는 map[string]any 데이터에서 JSONPath 표현식으로 값을 조회한다.
// 지원 구문: $.key, $.a.b.c, $.items[0], $.items[*].name
func evaluatePath(data map[string]any, path string) (any, error) {
	// 경로 유효성 검사: "$." 접두사 필수
	if !strings.HasPrefix(path, "$.") {
		return nil, ErrInvalidPath
	}

	// "$." 이후의 경로 추출
	rest := path[2:]
	if rest == "" {
		return nil, ErrInvalidPath
	}

	// 경로를 토큰으로 분할
	tokens, err := parsePath(rest)
	if err != nil {
		return nil, err
	}

	return resolveTokens(data, tokens)
}

// pathToken 은 경로의 개별 요소를 나타낸다.
type pathToken struct {
	key      string // 필드 이름
	isIndex  bool   // 배열 인덱스 여부
	index    int    // 배열 인덱스 값
	wildcard bool   // 와일드카드(*) 여부
}

// parsePath 는 경로 문자열을 토큰 슬라이스로 분할한다.
// 예: "user.address.city" → [{key:"user"}, {key:"address"}, {key:"city"}]
// 예: "items[0].name" → [{key:"items"}, {isIndex:true, index:0}, {key:"name"}]
// 예: "items[*].name" → [{key:"items"}, {wildcard:true}, {key:"name"}]
func parsePath(path string) ([]pathToken, error) {
	var tokens []pathToken
	parts := splitPathParts(path)

	for _, part := range parts {
		// 배열 접근 확인: key[index] 또는 key[*]
		if bracketIdx := strings.Index(part, "["); bracketIdx != -1 {
			if !strings.HasSuffix(part, "]") {
				return nil, ErrInvalidPath
			}

			// 키 부분
			key := part[:bracketIdx]
			if key != "" {
				tokens = append(tokens, pathToken{key: key})
			}

			// 대괄호 내용
			inner := part[bracketIdx+1 : len(part)-1]
			if inner == "*" {
				tokens = append(tokens, pathToken{wildcard: true})
			} else {
				idx, err := strconv.Atoi(inner)
				if err != nil {
					return nil, ErrInvalidPath
				}
				tokens = append(tokens, pathToken{isIndex: true, index: idx})
			}
		} else {
			if part == "" {
				return nil, ErrInvalidPath
			}
			tokens = append(tokens, pathToken{key: part})
		}
	}

	return tokens, nil
}

// splitPathParts 는 경로를 점(.) 기준으로 분할하되 대괄호 안의 점은 무시한다.
func splitPathParts(path string) []string {
	var parts []string
	var current strings.Builder
	inBracket := false

	for _, ch := range path {
		switch {
		case ch == '[':
			inBracket = true
			current.WriteRune(ch)
		case ch == ']':
			inBracket = false
			current.WriteRune(ch)
		case ch == '.' && !inBracket:
			parts = append(parts, current.String())
			current.Reset()
		default:
			current.WriteRune(ch)
		}
	}

	if current.Len() > 0 {
		parts = append(parts, current.String())
	}

	return parts
}

// resolveTokens 는 토큰 슬라이스를 순회하며 데이터에서 값을 조회한다.
func resolveTokens(current any, tokens []pathToken) (any, error) {
	for i, token := range tokens {
		switch {
		case token.wildcard:
			// 와일드카드: 현재 값이 배열이어야 한다
			arr, ok := current.([]any)
			if !ok {
				return nil, ErrPathNotFound
			}
			// 나머지 토큰이 있으면 각 요소에 적용
			remaining := tokens[i+1:]
			if len(remaining) == 0 {
				return arr, nil
			}
			var results []any
			for _, item := range arr {
				val, err := resolveTokens(item, remaining)
				if err != nil {
					return nil, err
				}
				results = append(results, val)
			}
			return results, nil

		case token.isIndex:
			// 배열 인덱스 접근
			arr, ok := current.([]any)
			if !ok {
				return nil, ErrPathNotFound
			}
			if token.index < 0 || token.index >= len(arr) {
				return nil, ErrPathNotFound
			}
			current = arr[token.index]

		default:
			// 맵 키 접근
			m, ok := current.(map[string]any)
			if !ok {
				return nil, ErrPathNotFound
			}
			val, exists := m[token.key]
			if !exists {
				return nil, ErrPathNotFound
			}
			current = val
		}
	}

	return current, nil
}
