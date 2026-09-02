// Modbus 명령셋 에디터의 값(value/values) 입력 파싱 헬퍼.
//
// 컴포넌트 파일(ModbusCommandSetEditor.tsx)에서 분리한 순수 함수 모듈이다.
// 컴포넌트 파일이 컴포넌트만 내보내야 Fast Refresh 가 동작하기 때문이다
// (react-refresh/only-export-components).

/** 콤마/공백 구분 문자열 → 토큰 배열. 숫자면 number, 아니면 string 으로 변환. */
function tokenizeValues(text: string): Array<number | string> {
  return text
    .split(/[\s,]+/)
    .map((t) => t.trim())
    .filter((t) => t !== '')
    .map((t) => {
      const n = Number(t);
      return Number.isFinite(n) && t !== '' ? n : t;
    });
}

/**
 * 값 입력 문자열 → WriteOp 의 value/values 조각(순수 함수, 단위 테스트 대상).
 * 토큰 0개 → {} (생략), 1개 → { value }, 2개 이상 → { values: [...] }.
 */
export function parseValuesInput(
  text: string,
): { value: number | string } | { values: Array<number | string> } | Record<string, never> {
  const tokens = tokenizeValues(text);
  if (tokens.length === 0) return {};
  if (tokens.length === 1) return { value: tokens[0]! };
  return { values: tokens };
}
