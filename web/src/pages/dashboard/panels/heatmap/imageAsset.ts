// 도면 이미지 인코딩/크기 검증 유틸 (SPEC-HEATMAP-PANEL-002 T2).
//
// 이미지 저장은 1차적으로 config JSON 에 data-URL 로 임베드한다(백엔드 무변경, plan.md 결정).
// 대용량 이미지가 대시보드 config 페이로드를 무제한 팽창시키지 않도록 크기 상한을 강제한다(REQ-04, AC-E3).
//
// DOM/브라우저 FileReader 에 직접 의존하지 않도록 FileReader 를 주입 가능하게 만들어(기본값은
// new FileReader()) DOM 없이 단위 테스트한다.
//
// @spec SPEC-HEATMAP-PANEL-002

/** data-URL 임베드 기본 크기 상한(2MB, 오케스트레이터 확정). */
export const DEFAULT_MAX_IMAGE_BYTES = 2 * 1024 * 1024;

/**
 * 크기 상한 초과 오류. 설정 UI(stage 2)가 안정적으로 분기·안내할 수 있도록 `code` 를 노출한다.
 * `actualBytes`/`maxBytes` 로 축소 안내에 필요한 수치를 전달한다.
 */
export class ImageSizeLimitError extends Error {
  /** 안정적 식별 코드(메시지 문자열에 의존하지 않도록). */
  readonly code = 'IMAGE_TOO_LARGE' as const;
  constructor(
    readonly actualBytes: number,
    readonly maxBytes: number,
  ) {
    super(`image ${actualBytes} bytes exceeds limit ${maxBytes} bytes`);
    this.name = 'ImageSizeLimitError';
  }
}

/**
 * data-URL 의 디코드된 바이트 크기를 추정한다. FileReader.readAsDataURL 이 생성하는
 * `data:<mime>;base64,<payload>` 형태는 base64 payload 로부터 정확히 계산한다(패딩 보정).
 * base64 가 아닌 형태는 콤마 이후 문자열 길이로 근사한다(방어적 폴백).
 */
export function dataUrlByteSize(dataUrl: string): number {
  const comma = dataUrl.indexOf(',');
  if (comma < 0) return 0;
  const meta = dataUrl.slice(0, comma);
  const payload = dataUrl.slice(comma + 1);
  if (/;base64$/i.test(meta)) {
    // base64: 4 문자 → 3 바이트, 말미 '=' 패딩만큼 차감.
    const len = payload.length;
    const padding = payload.endsWith('==') ? 2 : payload.endsWith('=') ? 1 : 0;
    return Math.max(0, Math.floor((len * 3) / 4) - padding);
  }
  return payload.length;
}

/**
 * data-URL 이 크기 상한 이내인지 검증한다. 초과 시 `ImageSizeLimitError` 를 throw 한다(REQ-04, AC-E3).
 * 상한 이내면 아무 것도 하지 않는다(반환값 없음).
 */
export function assertImageSizeUnderLimit(
  dataUrl: string,
  maxBytes: number = DEFAULT_MAX_IMAGE_BYTES,
): void {
  const size = dataUrlByteSize(dataUrl);
  if (size > maxBytes) {
    throw new ImageSizeLimitError(size, maxBytes);
  }
}

/**
 * File 을 data-URL 문자열로 인코딩한다. FileReader 를 주입 가능하게 하여(기본 new FileReader())
 * 실제 브라우저 FileReader 없이 단위 테스트할 수 있게 한다.
 */
export function readImageAsDataUrl(
  file: File,
  reader: FileReader = new FileReader(),
): Promise<string> {
  return new Promise<string>((resolve, reject) => {
    reader.onload = () => {
      const result = reader.result;
      if (typeof result === 'string') {
        resolve(result);
      } else {
        reject(new Error('FileReader 가 data-URL 문자열을 반환하지 않았습니다'));
      }
    };
    reader.onerror = () => {
      reject(reader.error ?? new Error('이미지 파일을 읽지 못했습니다'));
    };
    reader.readAsDataURL(file);
  });
}
