package message

import "errors"

// 패키지 수준 에러 정의
var (
	// ErrKeyExists 는 이미 존재하는 키에 Add 연산을 시도할 때 반환된다.
	ErrKeyExists = errors.New("key already exists")

	// ErrInvalidPath 는 잘못된 JSONPath 구문이 제공되었을 때 반환된다.
	ErrInvalidPath = errors.New("invalid path syntax")

	// ErrPathNotFound 는 지정된 경로에 값이 존재하지 않을 때 반환된다.
	ErrPathNotFound = errors.New("path not found")
)
