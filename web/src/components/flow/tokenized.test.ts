// 플로우 편집기의 색은 테마가 정한다 — 박힌 팔레트가 되돌아오지 못하게 막는다.
//
// `text-zinc-400` 한 줄을 다시 놓아도 타입 검사도 린트도 아무 말을 하지 않는다.
// 화면은 멀쩡히 뜨고, 그 자리만 팔레트를 무시할 뿐이다 — 사용자가 색을 바꿔도
// 따라오지 않는 자리가 하나씩 늘어나는 것이 이 저장소가 이미 한 번 겪은 경로다.
//
// `DebugPanel` 은 예외다. `bg-gray-950 font-mono` 로 선 **터미널**이며, 터미널을
// 테마에 따라 밝게 만드는 것은 고침이 아니라 다른 제품이다(spec.md §결정 2 가).
//
// @spec SPEC-THEME-001 AC-01 · 불변식 I1 · I6 (M3)

import { describe, expect, it } from 'vitest';
import { readFileSync, readdirSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const FLOW_DIR = dirname(fileURLToPath(import.meta.url));
const EDITOR_PAGE = join(FLOW_DIR, '..', '..', 'pages', 'editor', 'EditorPage.tsx');

/** 터미널 — 토큰화 대상이 아니다. */
const TERMINAL = 'DebugPanel.tsx';

/** 중립 팔레트 유틸리티. 변형 접두(`dark:` · `hover:` · `!`)가 앞에 붙는다. */
const NEUTRAL =
  /\b(?:[a-z-]+:)*!?(?:text|bg|border|ring|stroke|divide|placeholder|from|to|via|outline)-(?:zinc|gray|slate|neutral|stone)-\d+(?:\/\d+)?/g;

/** 주석 줄은 뺀다 — 옛 클래스 이름을 설명하는 주석까지 잡으면 문서를 못 쓴다. */
function hits(text: string): readonly string[] {
  return text
    .split('\n')
    .filter((line) => !/^\s*(\/\/|\*|\/\*)/.test(line))
    .flatMap((line) => [...line.matchAll(NEUTRAL)].map((m) => m[0]));
}

function flowFiles(): readonly { file: string; text: string }[] {
  return readdirSync(FLOW_DIR)
    .filter((f) => f.endsWith('.tsx') && !f.includes('.test.'))
    .map((f) => ({ file: f, text: readFileSync(join(FLOW_DIR, f), 'utf8') }));
}

describe('플로우 편집기는 팔레트를 따른다 (AC-01 · I1)', () => {
  it('박힌 중립 팔레트가 하나도 없다 — 터미널 제외', () => {
    const offenders = flowFiles()
      .filter(({ file }) => file !== TERMINAL)
      .flatMap(({ file, text }) => hits(text).map((c) => `${file}: ${c}`));
    // 교체 전 이 수는 111 이었다.
    expect(offenders).toEqual([]);
  });

  it('편집기 페이지에 박힌 색 리터럴이 없다', () => {
    // 이 파일은 날 NUL 바이트 때문에 grep 이 통째로 건너뛰던 자리다. 점격자의
    // `#d1d5db` 와 미니맵의 두 색이 그 침묵 뒤에 숨어 있었다.
    const text = readFileSync(EDITOR_PAGE, 'utf8');
    const literals = text
      .split('\n')
      .filter((line) => !/^\s*(\/\/|\*|\/\*)/.test(line))
      .flatMap((line) => [...line.matchAll(/#[0-9a-fA-F]{3,8}\b|rgba?\(/g)].map((m) => m[0]));
    expect(literals).toEqual([]);
  });
});

describe('터미널은 건드리지 않는다 (I6)', () => {
  it('DebugPanel 의 고정 다크 팔레트가 그대로다', () => {
    const dbg = flowFiles().find(({ file }) => file === TERMINAL);
    expect(dbg, 'DebugPanel.tsx 가 없다').toBeDefined();
    // 수를 못 박는 것은 "실수로 함께 옮기지 않았다" 를 보이기 위해서다. 터미널을
    // 의도적으로 테마화하기로 하면 이 수를 함께 고치면 된다.
    expect(hits(dbg!.text)).toHaveLength(32);
  });
});
