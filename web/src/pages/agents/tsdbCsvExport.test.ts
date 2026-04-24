// tsdbCsvExport 유틸 테스트 — 로컬 ISO 포맷, CSV 직렬화, 파일명 sanitize, 다운로드 트리거.
//
// @spec SPEC-WEB-005

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import type { SeriesMatrix } from '@/services/api/seriesDataSource';

import {
  downloadSeriesMatrixCsv,
  formatLocalIsoWithOffset,
  sanitizeFilenamePart,
  seriesMatrixToCsv,
} from './tsdbCsvExport';

describe('formatLocalIsoWithOffset', () => {
  it('로컬 타임존 오프셋을 부호와 HH:MM 으로 부착한다', () => {
    // 테스트 환경 타임존에 종속되지 않도록 정규식으로 검증한다.
    const ms = new Date(2026, 3, 23, 12, 34, 56).getTime();
    const out = formatLocalIsoWithOffset(ms);
    expect(out).toMatch(
      /^2026-04-23T12:34:56[+-]\d{2}:\d{2}$/,
    );
  });

  it('양수 오프셋은 +, 음수 오프셋은 - 부호를 사용', () => {
    // getTimezoneOffset 을 강제로 음수(= 로컬이 UTC 보다 앞선 타임존)로 만들어
    // `+HH:MM` 이 출력되는지 확인.
    const orig = Date.prototype.getTimezoneOffset;
    Date.prototype.getTimezoneOffset = function () {
      return -540; // UTC+9 (KST)
    };
    try {
      const ms = new Date(2026, 3, 23, 0, 0, 0).getTime();
      expect(formatLocalIsoWithOffset(ms)).toMatch(/\+09:00$/);
    } finally {
      Date.prototype.getTimezoneOffset = orig;
    }

    Date.prototype.getTimezoneOffset = function () {
      return 300; // UTC-5 (EST)
    };
    try {
      const ms = new Date(2026, 3, 23, 0, 0, 0).getTime();
      expect(formatLocalIsoWithOffset(ms)).toMatch(/-05:00$/);
    } finally {
      Date.prototype.getTimezoneOffset = orig;
    }
  });
});

describe('sanitizeFilenamePart', () => {
  it('콜론/공백/슬래시를 하이픈으로 치환', () => {
    expect(sanitizeFilenamePart('2026-04-23T12:34:56+09:00')).toBe(
      '2026-04-23T12-34-56+09-00',
    );
    expect(sanitizeFilenamePart('agent name/path')).toBe('agent-name-path');
  });

  it('연속 하이픈은 하나로 축약하고 양끝 하이픈 제거', () => {
    expect(sanitizeFilenamePart('  spaces   ')).toBe('spaces');
    expect(sanitizeFilenamePart('a::b')).toBe('a-b');
  });

  it('빈 문자열은 빈 문자열 반환', () => {
    expect(sanitizeFilenamePart('')).toBe('');
    expect(sanitizeFilenamePart(':::')).toBe('');
  });
});

describe('seriesMatrixToCsv', () => {
  it('헤더 + 행 — 키는 원본 그대로, 타임스탬프는 ISO + 오프셋', () => {
    const matrix: SeriesMatrix = {
      columns: ['a', 'b'],
      rows: [
        {
          bucketStartMs: new Date(2026, 3, 23, 0, 0, 0).getTime(),
          values: [1, 2],
        },
        {
          bucketStartMs: new Date(2026, 3, 23, 1, 0, 0).getTime(),
          values: [3, null],
        },
      ],
    };
    const csv = seriesMatrixToCsv(matrix);
    const lines = csv.trimEnd().split('\n');
    expect(lines[0]).toBe('timestamp,a,b');
    expect(lines[1]).toMatch(/^2026-04-23T00:00:00[+-]\d{2}:\d{2},1,2$/);
    // null 은 빈 문자열.
    expect(lines[2]).toMatch(/^2026-04-23T01:00:00[+-]\d{2}:\d{2},3,$/);
  });

  it('콤마/따옴표가 포함된 키는 따옴표로 감싸고 내부 따옴표 이중화', () => {
    const matrix: SeriesMatrix = {
      columns: ['a,b', 'q"x'],
      rows: [{ bucketStartMs: 0, values: [1, 2] }],
    };
    const csv = seriesMatrixToCsv(matrix);
    const lines = csv.split('\n');
    expect(lines[0]).toBe('timestamp,"a,b","q""x"');
  });

  it('NaN/Infinity 값은 빈 문자열', () => {
    const matrix: SeriesMatrix = {
      columns: ['v'],
      rows: [
        {
          bucketStartMs: new Date(2026, 3, 23, 0, 0, 0).getTime(),
          values: [Number.NaN],
        },
        {
          bucketStartMs: new Date(2026, 3, 23, 1, 0, 0).getTime(),
          values: [Number.POSITIVE_INFINITY],
        },
      ],
    };
    const csv = seriesMatrixToCsv(matrix);
    const lines = csv.trimEnd().split('\n');
    // 타임스탬프, (빈 문자열) 두 컬럼 → "timestamp,".
    expect(lines[1]).toMatch(/,$/);
    expect(lines[2]).toMatch(/,$/);
  });

  it('행이 없으면 헤더만 반환한다 (trailing newline 포함)', () => {
    const csv = seriesMatrixToCsv({ columns: ['v'], rows: [] });
    expect(csv).toBe('timestamp,v\n');
  });
});

describe('downloadSeriesMatrixCsv', () => {
  // Blob → URL 변환 모의. 실제 브라우저에서는 Blob 을 받지만 테스트에서는 호출 인자만
  // 확인하면 되므로 `unknown` 으로 느슨하게 타입한다.
  // Blob → URL 변환 모의. 테스트에서는 호출 인자만 확인하면 되므로 제네릭 rest 타입 사용.
  const createObjectURLMock = vi.fn<(...args: unknown[]) => string>(() => 'blob:fake-url');
  const revokeObjectURLMock = vi.fn<(...args: unknown[]) => void>();
  let clickMock: ReturnType<typeof vi.fn>;
  // appendChild/removeChild 를 감시해 실제 DOM 에서 anchor 를 추적한다.
  // spyOn 의 반환 타입이 제네릭 오버로드라 vi.SpyInstance 로 명시하지 못하므로 `any` 로 담는다.
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  let appendChildSpy: any;
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  let removeChildSpy: any;

  beforeEach(() => {
    clickMock = vi.fn();
    // jsdom 에는 URL.createObjectURL 이 기본 제공되지 않으므로 주입한다.
    (URL as unknown as { createObjectURL: () => string }).createObjectURL =
      createObjectURLMock;
    (URL as unknown as { revokeObjectURL: (u: string) => void }).revokeObjectURL =
      revokeObjectURLMock;

    // createElement('a') 가 반환할 더미 앵커에 click 을 주입한다.
    const origCreate = document.createElement.bind(document);
    vi.spyOn(document, 'createElement').mockImplementation((tag: string) => {
      const el = origCreate(tag);
      if (tag === 'a') {
        Object.defineProperty(el, 'click', {
          configurable: true,
          value: clickMock,
        });
      }
      return el;
    });
    appendChildSpy = vi.spyOn(document.body, 'appendChild');
    removeChildSpy = vi.spyOn(document.body, 'removeChild');

    createObjectURLMock.mockClear();
    revokeObjectURLMock.mockClear();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('matrix.rows 가 비어있으면 no-op (다운로드 미트리거)', () => {
    downloadSeriesMatrixCsv(
      { columns: ['a'], rows: [] },
      { agentName: 'agent', startMs: 0, endMs: 1 },
    );
    expect(createObjectURLMock).not.toHaveBeenCalled();
    expect(clickMock).not.toHaveBeenCalled();
  });

  it('유효한 매트릭스면 Blob 생성 + <a> 클릭 + URL 해제', () => {
    const matrix: SeriesMatrix = {
      columns: ['a'],
      rows: [
        {
          bucketStartMs: new Date(2026, 3, 23, 0, 0, 0).getTime(),
          values: [1],
        },
      ],
    };
    const start = new Date(2026, 3, 23, 0, 0, 0).getTime();
    const end = new Date(2026, 3, 23, 1, 0, 0).getTime();

    downloadSeriesMatrixCsv(matrix, { agentName: 'tsdb', startMs: start, endMs: end });

    expect(createObjectURLMock).toHaveBeenCalledTimes(1);
    // Blob 첫 인자: BOM + CSV 문자열이어야 한다.
    const blobArg = createObjectURLMock.mock.calls[0]![0] as unknown as Blob;
    expect(blobArg).toBeInstanceOf(Blob);
    expect(blobArg.type).toBe('text/csv;charset=utf-8');

    expect(clickMock).toHaveBeenCalledTimes(1);
    expect(appendChildSpy).toHaveBeenCalled();
    expect(removeChildSpy).toHaveBeenCalled();
    expect(revokeObjectURLMock).toHaveBeenCalledWith('blob:fake-url');

    // 첨부된 anchor 의 download 속성이 series-<agent>-...-to-....csv 규칙을 따르는지 확인.
    const anchor = appendChildSpy!.mock.calls[0]![0] as HTMLAnchorElement;
    expect(anchor.tagName).toBe('A');
    expect(anchor.download).toMatch(/^series-tsdb-.+-to-.+\.csv$/);
  });

  it('agentName 특수문자는 파일명에서 하이픈으로 치환', () => {
    const matrix: SeriesMatrix = {
      columns: ['a'],
      rows: [{ bucketStartMs: 0, values: [1] }],
    };
    downloadSeriesMatrixCsv(matrix, {
      agentName: 'my agent/with:colon',
      startMs: 0,
      endMs: 1,
    });
    const anchor = appendChildSpy!.mock.calls[0]![0] as HTMLAnchorElement;
    expect(anchor.download).toMatch(/^series-my-agent-with-colon-/);
  });

  it('빈 agentName 은 "series" 로 대체', () => {
    const matrix: SeriesMatrix = {
      columns: ['a'],
      rows: [{ bucketStartMs: 0, values: [1] }],
    };
    downloadSeriesMatrixCsv(matrix, { agentName: '', startMs: 0, endMs: 1 });
    const anchor = appendChildSpy!.mock.calls[0]![0] as HTMLAnchorElement;
    // sanitizeFilenamePart('') = '' 이므로 최종 agentPart 가 'series' 로 fallback.
    expect(anchor.download).toMatch(/^series-series-/);
  });
});
