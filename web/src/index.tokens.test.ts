// 시맨틱 컬러 토큰 참조 검사.
//
// Tailwind 의 `bg-(--color-x)` 나 `var(--color-x)` 는 그 토큰이 없어도 **조용히**
// 아무것도 그리지 않는다. 오타 하나로 테두리가 사라지거나 막대 배경이 비는데,
// 타입 검사도 린트도 잡지 못하고 테스트는 DOM 만 보므로 통과한다. 실제로 sysmetrics
// 패널이 `--color-border`(정의: `--color-border-default`)를 써서 카드가 테두리 없이
// 나온 적이 있다.
//
// 그래서 "소스가 참조하는 토큰은 index.css 에 정의되어 있어야 한다"를 못으로 박는다.

import { describe, expect, it } from 'vitest';
import { readFileSync, readdirSync, statSync } from 'node:fs';
import { dirname, join, relative } from 'node:path';
import { fileURLToPath } from 'node:url';

const SRC = dirname(fileURLToPath(import.meta.url));

/**
 * 이미 존재하던 위반 목록.
 *
 * 이 검사를 넣기 전부터 쓰이던 토큰들이다. 함께 고치면 이번 변경의 범위를 넘고
 * 화면 여러 곳의 색이 한꺼번에 바뀌므로, 새 위반만 막고 기존 것은 여기 남겨 둔다.
 * 하나를 고칠 때마다 이 목록에서 지우면 된다 — 목록이 비면 이 상수도 지운다.
 */
const KNOWN_UNDEFINED = new Set([
  '--color-bg-base',
  '--color-bg-hover',
  '--color-border',
  '--color-border-warning',
  '--color-primary',
  '--color-text-default',
]);

/** index.css 의 토큰 정의를 모은다. */
function definedTokens(): Set<string> {
  const css = readFileSync(join(SRC, 'index.css'), 'utf8');
  return new Set(css.match(/--color-[a-z0-9-]+(?=\s*:)/g) ?? []);
}

/** 소스에서 참조하는 토큰을 파일별로 모은다 (테스트 파일 제외). */
function referencedTokens(): Map<string, Set<string>> {
  const found = new Map<string, Set<string>>();

  const walk = (dir: string) => {
    for (const entry of readdirSync(dir)) {
      const full = join(dir, entry);
      if (statSync(full).isDirectory()) {
        if (entry !== 'node_modules' && entry !== '__mocks__') walk(full);
        continue;
      }
      if (!/\.tsx?$/.test(entry) || entry.includes('.test.')) continue;

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
      if (defined.has(token) || KNOWN_UNDEFINED.has(token)) continue;
      offenders.push(`${token}  ←  ${[...files].sort().join(', ')}`);
    }

    // 실패 메시지에 어느 파일인지 담는다 — 토큰 이름만으로는 찾아 헤매게 된다.
    expect(offenders.sort()).toEqual([]);
  });

  it('기존 위반 목록에 실제로 쓰이지 않는 토큰이 남아 있지 않다', () => {
    // 고친 뒤 목록에서 지우지 않으면, 그 자리에 새 오타가 들어와도 통과한다.
    const referenced = referencedTokens();
    const stale = [...KNOWN_UNDEFINED].filter((t) => !referenced.has(t));

    expect(stale.sort()).toEqual([]);
  });

  it('sysmetrics 패널은 정의된 토큰만 쓴다', () => {
    // 이 검사를 만들게 한 당사자다. 기존 위반 목록의 면제를 받지 않는다.
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
