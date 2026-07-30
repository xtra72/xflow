package xsfm

import (
	"fmt"
	"hash/fnv"
	"regexp"
	"strings"

	"golang.org/x/text/unicode/norm"
)

// ---------------------------------------------------------------------------
// 통일 코드 포맷 & slugify (SPEC-XSFM-LINE-001 RD-6, §4.6)
// ---------------------------------------------------------------------------
//
// 통일 코드 포맷은 station/line/custom 신규 코드에 공통으로 적용된다: 소문자 영숫자로
// 시작하고, 이후 소문자 영숫자·언더스코어·하이픈만 허용한다. slugify 는 로드 마이그레이션
// 에서 포맷 비적합 레거시 custom:<name> 을 code 로 변환할 때만 쓴다(신규 생성 경로는 검증만
// 하고 slugify 하지 않는다). 규칙은 결정적이어서 재기동 시 동일 결과를 보장한다(멱등).

// codePattern 은 통일 코드 포맷 정규식이다 (RD-6). 소문자 영숫자 시작 + 이후 [a-z0-9_-].
var codePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

// validCode 는 s 가 통일 코드 포맷을 만족하는지 판정한다 (RD-6). 빈 문자열은 위반이다.
func validCode(s string) bool {
	return codePattern.MatchString(s)
}

// slugify 는 임의 문자열을 통일 코드 포맷을 만족하는 결정적 slug 로 변환한다 (§4.6 규칙).
//
//  1. Unicode NFC 정규화 후 소문자화.
//  2. 한글 음절은 Revised Romanization(자모 분해 기반 고정 매핑, 음운 동화 미적용)으로 변환.
//  3. 그 외 비-ASCII·공백·콜론 등 비허용 문자는 각각 '-' 로 치환(허용: [a-z0-9_-]).
//  4. 연속 하이픈은 하나로 축약하고 양끝의 '-'/'_' 를 트림(선두는 [a-z0-9] 제약 충족).
//  5. 결과가 비면 원문의 fnv1a32 해시를 폴백으로 사용: "g<hex8>".
//
// 충돌 처리(접미 번호)는 호출부(마이그레이션)가 담당한다.
func slugify(input string) string {
	normalized := norm.NFC.String(input)
	lower := strings.ToLower(normalized)

	var b strings.Builder
	for _, r := range lower {
		switch {
		case r >= hangulBase && r <= hangulEnd:
			b.WriteString(romanizeHangul(r))
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}

	slug := collapseHyphens(b.String())
	slug = trimNonAlnumEdges(slug)
	if slug == "" {
		return hashFallback(input)
	}
	return slug
}

// collapseHyphens 는 연속 하이픈을 하나로 축약한다.
func collapseHyphens(s string) string {
	var b strings.Builder
	prevHyphen := false
	for _, r := range s {
		if r == '-' {
			if prevHyphen {
				continue
			}
			prevHyphen = true
		} else {
			prevHyphen = false
		}
		b.WriteRune(r)
	}
	return b.String()
}

// trimNonAlnumEdges 는 양끝의 '-'/'_' 를 제거해 선두 [a-z0-9] 제약을 충족시킨다.
func trimNonAlnumEdges(s string) string {
	return strings.Trim(s, "-_")
}

// hashFallback 은 로마자화 불가한 순수 비-ASCII 등 빈 slug 에 대해 원문의 안정 해시를
// 코드로 사용한다: "g" + fnv1a32(원문) 8자리 hex (§4.6 규칙 5).
func hashFallback(input string) string {
	h := fnv.New32a()
	_, _ = h.Write([]byte(input))
	return fmt.Sprintf("g%08x", h.Sum32())
}

// ---------------------------------------------------------------------------
// 한글 Revised Romanization (자모 분해 기반 고정 매핑, 음운 동화 미적용) — §4.6 규칙 2
// ---------------------------------------------------------------------------

const (
	hangulBase = 0xAC00 // '가'
	hangulEnd  = 0xD7A3 // '힣'
	jamoVCount = 21     // 중성 개수.
	jamoTCount = 28     // 종성 개수(0=받침 없음 포함).
)

// hangulLead 는 초성(19) 로마자 매핑이다(인덱스 11 = ㅇ 은 무음).
var hangulLead = [...]string{
	"g", "kk", "n", "d", "tt", "r", "m", "b", "pp", "s",
	"ss", "", "j", "jj", "ch", "k", "t", "p", "h",
}

// hangulVowel 은 중성(21) 로마자 매핑이다.
var hangulVowel = [...]string{
	"a", "ae", "ya", "yae", "eo", "e", "yeo", "ye", "o", "wa",
	"wae", "oe", "yo", "u", "wo", "we", "wi", "yu", "eu", "ui", "i",
}

// hangulTail 은 종성(28, 인덱스 0 = 받침 없음) 로마자 매핑이다.
var hangulTail = [...]string{
	"", "k", "kk", "ks", "n", "nj", "nh", "t", "l", "lk",
	"lm", "lp", "ls", "lt", "lp", "lh", "m", "p", "ps", "s",
	"ss", "ng", "j", "ch", "k", "t", "p", "h",
}

// romanizeHangul 은 한글 음절 하나를 초성+중성+종성 로마자 합성으로 변환한다.
func romanizeHangul(r rune) string {
	idx := int(r) - hangulBase
	lead := idx / (jamoVCount * jamoTCount)
	vowel := (idx % (jamoVCount * jamoTCount)) / jamoTCount
	tail := idx % jamoTCount
	return hangulLead[lead] + hangulVowel[vowel] + hangulTail[tail]
}
