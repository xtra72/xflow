// 목록·스케줄 화면의 색은 테마가 정한다 — 박힌 팔레트가 되돌아오지 못하게 막는다.
//
// `text-red-700` 한 줄을 다시 놓아도 타입 검사도 린트도 아무 말을 하지 않는다. 화면은
// 멀쩡히 뜨고, 그 자리만 팔레트를 무시할 뿐이다 — 사용자가 오류색을 바꿔도 따라오지
// 않는 자리가 하나씩 늘어나는 것이 이 저장소가 이미 플로우 편집기에서 겪은 경로다.
//
// 여기서 지키는 것은 **의미색**(초록·빨강·호박·파랑)이다. 중립 회색은 이 세 화면에
// 이미 없고, 있더라도 다른 가드가 다룰 일이다.
//
// @spec SPEC-THEME-001 (목록·스케줄 토큰화)

import { describe, expect, it } from 'vitest';
import { readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const PAGES = dirname(fileURLToPath(import.meta.url));

/** 토큰화를 끝낸 화면들. 새 화면을 옮길 때마다 여기 더한다. */
const TOKENIZED = [
  'agents/AgentListPage.tsx',
  'devices/DeviceListPage.tsx',
  'schedules/ScheduleManagementTab.tsx',
] as const;

/**
 * 뜻을 가진 색의 Tailwind 팔레트 유틸리티.
 *
 * 이 셋은 전부 `status-*` 나 `interactive-*` 토큰으로 갈 자리가 있다. 자리가 없는
 * 색(브랜드 강조 등)이 생기면 그때 예외를 적되, 예외는 **이유와 함께** 적는다.
 */
const SEMANTIC =
  /\b(?:[a-z-]+:)*!?(?:text|bg|border|ring|divide|from|to)-(?:green|red|amber|yellow|blue|sky|emerald|violet|orange)-\d+(?:\/\d+)?/g;

/** 주석 줄은 뺀다 — 옛 클래스 이름을 설명하는 주석까지 잡으면 문서를 못 쓴다. */
function hits(text: string): readonly string[] {
  return text
    .split('\n')
    .filter((line) => !/^\s*(\/\/|\*|\/\*)/.test(line))
    .flatMap((line) => [...line.matchAll(SEMANTIC)].map((m) => m[0]));
}

describe('목록·스케줄 화면은 팔레트를 따른다', () => {
  it.each(TOKENIZED)('%s — 박힌 의미색이 없다', (rel) => {
    const text = readFileSync(join(PAGES, rel), 'utf8');
    // 교체 전 이 수는 AgentList 27 · DeviceList 25 · Schedule 29 였다.
    expect(hits(text)).toEqual([]);
  });

  it('상태 배지는 칠과 글자를 다른 토큰으로 쓴다', () => {
    // 한 토큰으로 둘을 다 하면 밝은 테마에서 글자가 AA 를 넘지 못한다
    // (`statusTextContrast.test.ts` 가 그 사실을 수로 잰다). 배지를 쓰는 화면이
    // 글자용 토큰을 실제로 부르는지 여기서 확인한다.
    for (const rel of TOKENIZED) {
      const text = readFileSync(join(PAGES, rel), 'utf8');
      if (!/--color-status-(?:running|error|warning|info)\b/.test(text)) continue;
      expect(text, rel).toMatch(/--color-status-(?:running|stopped|error|warning|info)-text/);
    }
  });
});
