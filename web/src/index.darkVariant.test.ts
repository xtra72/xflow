// `dark:` 가 무엇에 걸리는지 못으로 박는다.
//
// Tailwind v4 에서 `dark:` 의 기본값은 `prefers-color-scheme` 미디어쿼리다. 그 기본값을
// 그대로 두면 `useTheme` 이 `<html>` 에 붙이는 `.dark` 클래스가 **아무것도 선택하지
// 못하고**, 화면에 진실이 둘 생긴다 — `--color-*` 토큰은 앱이 고른 테마를 따르는데
// `dark:` 유틸리티는 OS 를 따른다. 둘이 어긋나는 조합(OS 밝음 + 앱 야간)에서 어두운
// 카드 위에 밝은 모드용 검은 글자가 남아 읽히지 않았고, 그것이 플로우 편집기 가시성
// 신고의 원인이었다.
//
// 고침은 `index.css` 의 `@custom-variant dark` 한 줄이다. 한 줄이라 지우기도 쉽고,
// 지워도 **타입 검사도 린트도 아무 말을 하지 않는다** — 화면이 조용히 OS 추종으로
// 되돌아갈 뿐이다. 그래서 선언의 존재와 형태를 여기서 지킨다.
//
// @spec 플로우 편집기 다크 모드 가시성 (사용자 신고 2026-09-14)

import { describe, expect, it } from 'vitest';
import { readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const SRC = dirname(fileURLToPath(import.meta.url));

function indexCss(): string {
  return readFileSync(join(SRC, 'index.css'), 'utf8');
}

describe('`dark:` 는 OS 가 아니라 앱이 고른 테마를 따른다', () => {
  it('`@custom-variant dark` 선언이 있다', () => {
    expect(indexCss()).toMatch(/@custom-variant\s+dark\s*\(/);
  });

  it('`.dark` 클래스를 조상으로 삼는다', () => {
    // `useTheme` 이 붙이는 것이 `.dark` 클래스이므로 셀렉터가 그것을 가리켜야 한다.
    const decl = indexCss().match(/@custom-variant\s+dark\s*\(([^)]*\)[^;]*)\)/)?.[1] ?? '';
    expect(decl).toContain('.dark');
  });

  it('`:where()` 로 감싸 명시도를 0 으로 둔다', () => {
    // 감싸지 않으면 `dark:` 유틸 하나가 클래스 둘 몫의 무게를 얻어, 같은 속성을 정하는
    // 다른 유틸리티를 선언 순서와 무관하게 이긴다.
    const decl = indexCss().match(/@custom-variant\s+dark\s*\(([^)]*\)[^;]*)\)/)?.[1] ?? '';
    expect(decl).toContain(':where(');
  });

  it('`useTheme` 이 그 클래스를 실제로 붙인다', () => {
    // 선언과 적용이 짝이다. 한쪽만 있으면 둘 다 없는 것과 같다.
    const hook = readFileSync(join(SRC, 'hooks', 'useTheme.ts'), 'utf8');
    expect(hook).toMatch(/classList\.toggle\(\s*'dark'/);
  });
});
