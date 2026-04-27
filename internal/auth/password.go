package auth

import (
	"errors"
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

// 비밀번호 관련 에러
var (
	ErrPasswordTooShort = errors.New("auth: 비밀번호는 최소 4자 이상이어야 합니다")
)

// bcrypt 해싱 비용
const bcryptCost = 10

// HashPassword 는 비밀번호를 bcrypt로 해싱한다.
// 최소 4자 이상의 비밀번호만 허용한다.
func HashPassword(password string) (string, error) {
	if len(password) < 4 {
		return "", ErrPasswordTooShort
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return "", fmt.Errorf("auth: 비밀번호 해싱 실패: %w", err)
	}
	return string(hash), nil
}

// CheckPassword 는 해시와 비밀번호를 비교한다.
// 일치하면 nil, 불일치하면 에러를 반환한다.
func CheckPassword(hash, password string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
}
