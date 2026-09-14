// 시맨틱 컬러 토큰 참조 검사.
//
// Tailwind 의 `bg-(--color-<없는이름>)` 나 `var(--color-<없는이름>)` 는 그 토큰이
// 없어도 **조용히** 아무것도 그리지 않는다. 오타 하나로 테두리가 사라지거나 막대
// 배경이 비는데, 타입 검사도 린트도 잡지 못하고 테스트는 DOM 만 보므로 통과한다.
// 실제로 sysmetrics 패널이 `--color-border`(정의: `--color-border-default`)를 써서
// 카드가 테두리 없이 나온 적이 있다.
//
// 그래서 "소스가 참조하는 토큰은 index.css 에 정의되어 있어야 한다"를 못으로 박는다.
//
// 이 검사를 처음 넣을 때는 이미 쓰이던 위반 토큰 여섯 개(`--color-bg-base`,
// `--color-bg-hover`, `--color-border`, `--color-border-warning`, `--color-primary`,
// `--color-text-default`)를 KNOWN_UNDEFINED 로 면제했었다. 여섯 개를 모두 정의된
// 토큰으로 옮긴 뒤 그 면제 목록을 없앴다 — 이제 예외는 하나도 없다.
//
// 위 예시 토큰명을 꺾쇠로 감싼 이유: Tailwind 는 테스트 파일의 주석까지 스캔하므로
// 진짜 유틸리티처럼 생긴 예시를 적으면 그 예시 자체가 미정의 토큰을 참조하는 죽은
// 규칙이 되어 번들에 실려 나간다. 실제로 그렇게 만들어진 규칙이 하나 섞여 있었다.

import { describe, expect, it } from 'vitest';
import { readFileSync, readdirSync, statSync } from 'node:fs';
import { dirname, join, relative } from 'node:path';
import { fileURLToPath } from 'node:url';

const SRC = dirname(fileURLToPath(import.meta.url));

/** index.css 의 토큰 정의를 모은다. */
function definedTokens(): Set<string> {
  const css = readFileSync(join(SRC, 'index.css'), 'utf8');
  return new Set(css.match(/--color-[a-z0-9-]+(?=\s*:)/g) ?? []);
}

/**
 * 소스에서 참조하는 토큰을 파일별로 모은다 (테스트 파일 제외).
 *
 * `.css` 도 함께 훑는다. 컴포넌트가 아니라 스타일시트에서 오타가 나도
 * 똑같이 조용히 사라지기 때문이다.
 */
function referencedTokens(): Map<string, Set<string>> {
  const found = new Map<string, Set<string>>();

  const walk = (dir: string) => {
    for (const entry of readdirSync(dir)) {
      const full = join(dir, entry);
      if (statSync(full).isDirectory()) {
        if (entry !== 'node_modules' && entry !== '__mocks__') walk(full);
        continue;
      }
      if (!/\.(tsx?|css)$/.test(entry) || entry.includes('.test.')) continue;

      for (const token of readFileSync(full, 'utf8').match(/--color-[a-z0-9-]+/g) ?? []) {
        if (!found.has(token)) found.set(token, new Set());
        found.get(token)!.add(relative(SRC, full));
      }
    }
  };

  walk(SRC);
  return found;
}

describe('시맨틱 컬러 토큰', () => {
  it('소스가 참조하는 토큰은 index.css 에 정의되어 있다', () => {
    const defined = definedTokens();
    const referenced = referencedTokens();

    const offenders: string[] = [];
    for (const [token, files] of referenced) {
      if (defined.has(token)) continue;
      offenders.push(`${token}  ←  ${[...files].sort().join(', ')}`);
    }

    // 실패 메시지에 어느 토큰이 어느 파일에 있는지 담는다 — 토큰 이름만으로는
    // 찾아 헤매게 된다. 정의된 토큰 목록도 함께 보여 무엇으로 바꿔야 할지 알린다.
    expect(
      offenders.sort(),
      `index.css 에 정의되지 않은 컬러 토큰을 참조한다.\n` +
        `아래 각 줄은 "토큰  ←  참조한 파일" 이다:\n` +
        `${offenders.sort().map((o) => `  ${o}`).join('\n')}\n\n` +
        `정의된 토큰: ${[...defined].sort().join(', ')}`,
    ).toEqual([]);
  });

  it('sysmetrics 패널은 정의된 토큰만 쓴다', () => {
    // 이 검사를 만들게 한 당사자다.
    const defined = definedTokens();
    const referenced = referencedTokens();

    const offenders: string[] = [];
    for (const [token, files] of referenced) {
      if (defined.has(token)) continue;
      const inSysMetrics = [...files].filter((f) => f.includes('panels/sysmetrics/'));
      if (inSysMetrics.length > 0) offenders.push(`${token}  ←  ${inSysMetrics.join(', ')}`);
    }

    expect(offenders.sort()).toEqual([]);
  });
});
