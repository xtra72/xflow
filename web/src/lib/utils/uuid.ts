// UUID 생성 헬퍼.
//
// `crypto.randomUUID()` 는 모던 브라우저 표준이지만 **secure context**
// (HTTPS 또는 localhost) 외부에서는 정의되지 않는다. RPi 등 호스트에서
// 서빙되는 Web UI 를 다른 PC 의 브라우저로 HTTP 로 접근하는 경우
// `crypto.randomUUID is not a function` 런타임 에러가 발생한다.
//
// 본 헬퍼는 (1) 표준 API 가 있으면 우선 사용하고, (2) 없으면 RFC 4122 v4
// 호환 fallback 으로 UUID 를 생성한다. fallback 은 보안에 민감하지 않은
// 클라이언트 측 식별자(노드 id, 패널 id 등) 용도로만 사용된다.

// generateUUID — 환경에 따라 안전하게 UUID v4 문자열을 반환한다.
export function generateUUID(): string {
  const c = globalThis.crypto;
  if (c?.randomUUID) {
    return c.randomUUID();
  }
  // crypto.getRandomValues 가 있으면 그것을 이용한 RFC 4122 v4 생성.
  if (c?.getRandomValues) {
    const bytes = new Uint8Array(16);
    c.getRandomValues(bytes);
    // version (0100) + variant (10xx) 비트 세팅. noUncheckedIndexedAccess 보호.
    bytes[6] = ((bytes[6] ?? 0) & 0x0f) | 0x40;
    bytes[8] = ((bytes[8] ?? 0) & 0x3f) | 0x80;
    const hex = Array.from(bytes, (b) => b.toString(16).padStart(2, '0')).join('');
    return (
      hex.slice(0, 8) +
      '-' +
      hex.slice(8, 12) +
      '-' +
      hex.slice(12, 16) +
      '-' +
      hex.slice(16, 20) +
      '-' +
      hex.slice(20, 32)
    );
  }
  // 최후 fallback: Math.random + Date.now 의 weak ID. UUID 형식은 유지하지 않으나
  // 본 헬퍼 사용처 (클라이언트 측 노드/패널 id) 는 형식보다 unique 성이 중요.
  return (
    Math.random().toString(36).slice(2, 10) +
    '-' +
    Math.random().toString(36).slice(2, 6) +
    '-' +
    '4' +
    Math.random().toString(36).slice(2, 5) +
    '-' +
    Math.random().toString(36).slice(2, 6) +
    '-' +
    Date.now().toString(36).padStart(12, '0').slice(-12)
  );
}
