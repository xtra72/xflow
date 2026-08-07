// imageAsset 단위 테스트 (SPEC-HEATMAP-PANEL-002 T2).
// FileReader 를 주입/모킹해 실제 브라우저 FileReader 없이 data-URL 인코딩과 크기 상한을 커버한다.
// AC-E3(대용량 이미지 상한 경고/차단).

import { describe, it, expect } from 'vitest';

import {
  readImageAsDataUrl,
  assertImageSizeUnderLimit,
  dataUrlByteSize,
  ImageSizeLimitError,
  DEFAULT_MAX_IMAGE_BYTES,
} from './imageAsset';

/**
 * FileReader 최소 모의체. readAsDataURL 호출 시 지정한 결과/오류로 비동기 콜백을 트리거한다.
 * (DOM/브라우저 FileReader 미의존 — 주입 테스트 전용.)
 */
class MockFileReader {
  onload: (() => void) | null = null;
  onerror: (() => void) | null = null;
  result: string | ArrayBuffer | null = null;
  error: unknown = null;

  constructor(
    private readonly outcome:
      | { kind: 'ok'; result: string | ArrayBuffer }
      | { kind: 'error'; error: unknown },
  ) {}

  readAsDataURL(_file: File): void {
    // 실제 FileReader 처럼 비동기로 콜백을 호출한다.
    queueMicrotask(() => {
      if (this.outcome.kind === 'ok') {
        this.result = this.outcome.result;
        this.onload?.();
      } else {
        this.error = this.outcome.error;
        this.onerror?.();
      }
    });
  }
}

function fakeFile(): File {
  // jsdom 환경에는 File 이 존재한다. 내용은 무의미(모의 reader 가 무시).
  return new File(['x'], 'floor.png', { type: 'image/png' });
}

describe('readImageAsDataUrl', () => {
  it('FileReader 결과가 data-URL 문자열이면 그대로 resolve 한다', async () => {
    const dataUrl = 'data:image/png;base64,AAAA';
    const reader = new MockFileReader({ kind: 'ok', result: dataUrl });
    const out = await readImageAsDataUrl(fakeFile(), reader as unknown as FileReader);
    expect(out).toBe(dataUrl);
  });

  it('FileReader 오류 시 reject 한다', async () => {
    const reader = new MockFileReader({ kind: 'error', error: new Error('boom') });
    await expect(
      readImageAsDataUrl(fakeFile(), reader as unknown as FileReader),
    ).rejects.toThrow('boom');
  });

  it('결과가 문자열이 아니면(ArrayBuffer) reject 한다', async () => {
    const reader = new MockFileReader({ kind: 'ok', result: new ArrayBuffer(4) });
    await expect(
      readImageAsDataUrl(fakeFile(), reader as unknown as FileReader),
    ).rejects.toThrow();
  });
});

describe('dataUrlByteSize', () => {
  it('base64 payload 의 디코드 바이트 크기를 패딩 보정해 계산한다', () => {
    // "AAAA"(4 문자, 패딩 없음) → 3 바이트.
    expect(dataUrlByteSize('data:image/png;base64,AAAA')).toBe(3);
    // "AAA="(1 패딩) → 2 바이트.
    expect(dataUrlByteSize('data:image/png;base64,AAA=')).toBe(2);
    // "AA=="(2 패딩) → 1 바이트.
    expect(dataUrlByteSize('data:image/png;base64,AA==')).toBe(1);
  });

  it('콤마가 없으면 0 을 반환한다(방어)', () => {
    expect(dataUrlByteSize('not-a-data-url')).toBe(0);
  });
});

describe('assertImageSizeUnderLimit (AC-E3)', () => {
  it('상한 이내면 통과한다(throw 없음)', () => {
    const small = 'data:image/png;base64,AAAA'; // 3 바이트
    expect(() => assertImageSizeUnderLimit(small, 1024)).not.toThrow();
  });

  it('상한 초과면 ImageSizeLimitError(code=IMAGE_TOO_LARGE)를 throw 한다', () => {
    const small = 'data:image/png;base64,AAAA'; // 3 바이트
    try {
      assertImageSizeUnderLimit(small, 2); // 상한 2 바이트 → 초과
      throw new Error('should have thrown');
    } catch (err) {
      expect(err).toBeInstanceOf(ImageSizeLimitError);
      const e = err as ImageSizeLimitError;
      expect(e.code).toBe('IMAGE_TOO_LARGE');
      expect(e.actualBytes).toBe(3);
      expect(e.maxBytes).toBe(2);
    }
  });

  it('기본 상한(2MB) 이내의 작은 이미지는 통과한다', () => {
    expect(DEFAULT_MAX_IMAGE_BYTES).toBe(2 * 1024 * 1024);
    expect(() => assertImageSizeUnderLimit('data:image/png;base64,AAAA')).not.toThrow();
  });
});
