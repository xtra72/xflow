// 화면 층의 순수 조각 시험 — 문구 표 · 채우기 · 미리보기 크기 · 파일 읽기
// (SPEC-CANVAS-007 M9 · M10 · REQ-04 · 위험 R14).
//
// **여기서 재는 것은 렌더가 아니라 산술과 표다.** DOM 을 세우지 않으므로 jsdom 의 변덕에
// 얽히지 않고, 사유가 하나 늘었을 때 어느 표가 비었는지가 이 파일에서 곧바로 드러난다.
//
// @spec SPEC-CANVAS-007 REQ-03 · REQ-04

import { describe, expect, it, vi } from 'vitest';

import ko from '@/lib/i18n/ko.json';
import en from '@/lib/i18n/en.json';

import { DEFAULT_CANVAS_SIZE } from '../canvasConfig';
import {
  NOTE_KEYS,
  PREVIEW_MAX_PX,
  REFUSAL_KEYS,
  fitPreviewSize,
  kb,
  readSvgFileText,
  refusalText,
} from './svgImportPresent';

/** 점 표기 키를 트리에서 조회한다. `t()` 와 **같은 규칙**이다(문자열이 아니면 부재). */
function lookup(tree: unknown, key: string): string | undefined {
  const value = key
    .split('.')
    .reduce<unknown>(
      (node, seg) =>
        node !== null && typeof node === 'object'
          ? (node as Record<string, unknown>)[seg]
          : undefined,
      tree,
    );
  return typeof value === 'string' ? value : undefined;
}

// --- 문구 표 --------------------------------------------------------------

describe('사유 → 문구 키 표가 빈 자리를 남기지 않는다 (REQ-04)', () => {
  it('보고 사유 30종 · 거절 사유 7종이 **양쪽 로케일**에서 잡힌다', () => {
    // 켜져 있음을 먼저 못박는다 — 표가 비면 아래 순회가 0회 돌고 초록이 된다.
    expect(Object.keys(NOTE_KEYS)).toHaveLength(30);
    expect(Object.keys(REFUSAL_KEYS)).toHaveLength(7);

    const missing: string[] = [];
    for (const key of [...Object.values(NOTE_KEYS), ...Object.values(REFUSAL_KEYS)]) {
      for (const [name, tree] of [
        ['ko', ko],
        ['en', en],
      ] as const) {
        const text = lookup(tree, key);
        if (text === undefined || text === '') missing.push(`${name}: ${key}`);
      }
    }
    expect(missing).toEqual([]);
  });

  it('키 이름 안에 점이 없다 — 어떤 경로로도 닿지 않는 키를 만들지 않는다', () => {
    for (const key of [...Object.values(NOTE_KEYS), ...Object.values(REFUSAL_KEYS)]) {
      expect(key.split('.').filter((seg) => seg === '')).toEqual([]);
      // 마지막 마디가 한 낱말이다 — 점이 들어가면 여기서 마디 수가 늘어난다.
      expect(key.split('.')).toHaveLength(4);
    }
  });

  it('두 표가 서로 다른 키를 쓴다 — 한 문구가 두 뜻을 나르지 않는다', () => {
    const all = [...Object.values(NOTE_KEYS), ...Object.values(REFUSAL_KEYS)];
    expect(new Set(all).size).toBe(all.length);
  });
});

// --- 채우기 ---------------------------------------------------------------

describe('거절 문구 채우기 (위험 R14 · 시험 규율 E14)', () => {
  it('상한을 **두 번** 말하는 문구에서 두 자리 모두 바뀐다 — `replace` 면 뒤가 남는다', () => {
    for (const tree of [ko, en]) {
      const template = lookup(tree, REFUSAL_KEYS.tooManyElements) ?? '';
      // 고정 입력이 그 성질을 실제로 갖는지 먼저 잰다(치환자가 하나뿐이면 이 시험은 무력하다).
      expect([...template.matchAll(/\{limit\}/g)]).toHaveLength(2);
      const filled = refusalText(template, { reason: 'tooManyElements', actual: 100, limit: 64 });
      expect(filled).not.toContain('{');
      expect([...filled.matchAll(/64/g)].length).toBeGreaterThanOrEqual(2);
      expect(filled).toContain('100');
    }
  });

  it('바이트 사유는 **KB 로** 말한다 — 개수 이름과 바이트 이름을 함께 갈아 끼운다', () => {
    const template = lookup(ko, REFUSAL_KEYS.fileTooLarge) ?? '';
    expect([...template.matchAll(/\{limitKb\}/g)]).toHaveLength(2);
    const filled = refusalText(template, {
      reason: 'fileTooLarge',
      actual: 3 * 1024 * 1024,
      limit: 2 * 1024 * 1024,
    });
    expect(filled).not.toContain('{');
    expect(filled).toContain('3072');
    expect(filled).toContain('2048');
  });

  it('치환자가 없는 사유는 그대로 남는다', () => {
    const template = lookup(ko, REFUSAL_KEYS.notSvg) ?? '';
    expect(template).not.toContain('{');
    expect(refusalText(template, { reason: 'notSvg', actual: 0, limit: 0 })).toBe(template);
  });

  it('0 바이트도 1KB 로 말한다 — "0KB 가 상한 2048KB 를 넘습니다" 를 내지 않는다', () => {
    expect(kb(0)).toBe(1);
    expect(kb(1)).toBe(1);
    expect(kb(1536)).toBe(2);
    expect(kb(2 * 1024 * 1024)).toBe(2048);
  });
});

// --- 미리보기 크기 ---------------------------------------------------------

describe('미리보기 크기는 종횡비를 지킨다 (§도크 UI)', () => {
  it('긴 축이 상한에 닿고 짧은 축이 비를 지킨다', () => {
    const size = fitPreviewSize(DEFAULT_CANVAS_SIZE);
    expect(Math.max(size.width, size.height)).toBe(PREVIEW_MAX_PX);
    expect(size.width / size.height).toBeCloseTo(
      DEFAULT_CANVAS_SIZE.width / DEFAULT_CANVAS_SIZE.height,
      1,
    );
  });

  it('아주 납작한 캔버스도 도크를 밀어내지 않는다', () => {
    const size = fitPreviewSize({ width: 4000, height: 3 });
    expect(size.width).toBeLessThanOrEqual(PREVIEW_MAX_PX);
    expect(size.height).toBeLessThanOrEqual(PREVIEW_MAX_PX);
    // 0 px 짜리 뒷면은 `getContext` 가 있어도 아무것도 그리지 못한다.
    expect(size.height).toBeGreaterThanOrEqual(1);
  });

  it('아주 긴 캔버스도 마찬가지다 — 두 축을 **같은 수로** 죈다', () => {
    const size = fitPreviewSize({ width: 3, height: 4000 });
    expect(size.height).toBe(PREVIEW_MAX_PX);
    expect(size.width).toBeGreaterThanOrEqual(1);
  });

  it('0 · 음수 크기에서도 유한한 값을 낸다 — 예외도 NaN 도 없다', () => {
    for (const canvas of [
      { width: 0, height: 0 },
      { width: -5, height: 10 },
      { width: 10, height: -5 },
    ]) {
      const size = fitPreviewSize(canvas);
      expect(Number.isFinite(size.width)).toBe(true);
      expect(Number.isFinite(size.height)).toBe(true);
      expect(size.width).toBeGreaterThanOrEqual(1);
      expect(size.height).toBeGreaterThanOrEqual(1);
    }
  });
});

// --- 파일 읽기 -------------------------------------------------------------

describe('파일 읽기 (`heatmap/imageAsset.ts` 의 형상)', () => {
  it('`File.text()` 에 기대지 않는다 — jsdom 에는 그 멤버가 **없다**', () => {
    // 이 단언이 이 함수가 `FileReader` 를 쓰는 이유 전부다. 사라지면(폴리필이 들어오면)
    // 다음 사람이 `File.text()` 로 갈아 끼울 근거가 생기고, 그때 이 줄이 먼저 빨개진다.
    const f = new File(['<svg/>'], 'a.svg', { type: 'image/svg+xml' });
    expect(typeof (f as unknown as { text?: unknown }).text).toBe('undefined');
  });

  it('진짜 `FileReader` 로 문자열을 돌려준다', async () => {
    const text = '<svg xmlns="http://www.w3.org/2000/svg" viewBox="-13 7 317 181"/>';
    await expect(readSvgFileText(new File([text], 'a.svg', { type: 'image/svg+xml' }))).resolves.toBe(
      text,
    );
  });

  it('한글이 든 문서도 글자 그대로 돌아온다 — 바이트가 아니라 문자열이다', async () => {
    const text = '<svg xmlns="http://www.w3.org/2000/svg"><title>도면</title></svg>';
    await expect(readSvgFileText(new File([text], 'a.svg'))).resolves.toBe(text);
  });

  it('읽기 실패는 **거부된 약속**으로 돌아온다 — 예외가 위로 새지 않는다', async () => {
    const reader = {
      onload: null as (() => void) | null,
      onerror: null as (() => void) | null,
      error: new Error('boom'),
      result: null,
      readAsText() {
        this.onerror?.();
      },
    };
    await expect(
      readSvgFileText(new File(['x'], 'a.svg'), reader as unknown as FileReader),
    ).rejects.toThrow('boom');
  });

  it('문자열이 아닌 결과도 거부다 — 조용히 `undefined` 를 흘리지 않는다', async () => {
    const reader = {
      onload: null as (() => void) | null,
      onerror: null as (() => void) | null,
      error: null,
      result: new ArrayBuffer(4),
      readAsText() {
        this.onload?.();
      },
    };
    await expect(
      readSvgFileText(new File(['x'], 'a.svg'), reader as unknown as FileReader),
    ).rejects.toThrow();
  });

  it('`error` 가 비어 있어도 거부 사유가 있다 — 벌거벗은 `undefined` 를 던지지 않는다', async () => {
    const reader = {
      onload: null as (() => void) | null,
      onerror: null as (() => void) | null,
      error: null,
      result: null,
      readAsText() {
        this.onerror?.();
      },
    };
    const spy = vi.fn();
    await readSvgFileText(new File(['x'], 'a.svg'), reader as unknown as FileReader).catch(spy);
    expect(spy).toHaveBeenCalledOnce();
    expect(spy.mock.calls[0]?.[0]).toBeInstanceOf(Error);
  });
});
