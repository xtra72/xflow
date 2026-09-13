// `PanelSettingsDialog.tsx` 가 이고 있는 **캔버스 표면**을 재는 도우미 — 시험 전용이다.
//
// 006 불변식 I7 이 세운 것은 "캔버스 작업(006·007·008)이 이 파일로 새지 않는다" 이고,
// 007 K13 · 008 J12 가 같은 것을 각자의 목소리로 묻는다. 그 둘이 오래 써 온 자는
// **행 수 등식**이었다. 등식은 불변식의 대리지 불변식이 아니다 — 캔버스와 무관한 편집
// (그리고 **줄이는** 편집)에도 울리므로, 다음 사람은 왜 울렸는지 보지 않고 수만 고쳐
// 놓는다. 실제로 그렇게 한 번 재기준되었고, 그 순간 SPEC 이 적은 수와 시험이 든 수가
// 갈라졌다. **보호처럼 보이는 것이 보호가 아니게 되는 자리**다.
//
// 그래서 여기서는 대리를 버리고 **불변식을 직접** 잰다. 둘이다.
//
//   (가) `panels/canvas/` 에서 들이는 **모듈과 이름의 집합**이 아래 허용목록과 같다.
//        캔버스 기능은 캔버스 모듈을 필요로 하므로, 새 캔버스 수입이 곧 신호다.
//   (나) 파일 안의 `Canvas[A-Z]…` **식별자**가 그 허용목록에서 **파생되는 것**뿐이다.
//        (가)가 못 보는 한 가지 — 수입 없이 **펴 넣은** 캔버스 코드 — 를 여기서 문다.
//        열거가 아니라 파생이므로, 새로 이름 붙인 캔버스 표면도 함께 걸린다.
//
// 이 파일이 `src/test/` 에 사는 이유: `node:` 수입은 이 저장소에서 시험 파일에만 있고
// (실측), `panels/canvas/**` 는 커버리지 허용목록에 들어 있어 그 아래에 두면 시험 도구가
// 제품 모듈로 세어진다.

import fs from 'node:fs';
import path from 'node:path';

/** 재는 대상. 두 SPEC 시험이 같은 파일을 가리키도록 자리를 하나로 둔다. */
// `__dirname` 을 쓴다 — 이 저장소의 시험들이 쓰는 자이며, vitest 가 이 파일에 주는
// `import.meta.url` 은 `file:` 스킴이 아니라 그 자로는 경로를 뽑을 수 없다(실측).
export const DIALOG_PATH = path.resolve(__dirname, '../pages/dashboard/PanelSettingsDialog.tsx');

/**
 * 다이얼로그가 `panels/canvas/` 에서 들여도 되는 것 전부 — **모듈마다 이름까지** 적는다.
 *
 * 008 이 심은 것은 도크 자리(`CanvasEditDockRegion`) 하나지만, 이 파일의 캔버스 표면은
 * 그 하나가 아니다. 001 이 미리보기(`CanvasPanel`)를, 004~006 이 편집기와 컨텍스트 셋을
 * 이미 놓아 두었다. **오늘의 실측이 다섯 모듈**이며, 그것이 곧 허용되는 크기다.
 *
 * 이 표가 늘어야 한다면 그것은 캔버스 작업이 다이얼로그로 들어왔다는 뜻이다. 표를 고치기
 * 전에 **그 코드가 `panels/canvas/` 안에 살 수 없는지** 부터 묻는다.
 */
export const ALLOWED_CANVAS_IMPORTS: Readonly<Record<string, readonly string[]>> = {
  './panels/canvas/CanvasEditDock': ['CanvasEditDockRegion'],
  './panels/canvas/CanvasPanel': ['CanvasPanel'],
  './panels/canvas/CanvasElementsEditor': ['CanvasElementsEditor'],
  './panels/canvas/canvasEditContext': [
    'CanvasEditSelectionContext',
    'CanvasLiveSeriesContext',
    'useCanvasEditSelectionState',
    'useCanvasLiveSeriesState',
  ],
  './panels/canvas/canvasStageAspect': ['CanvasStageAspectContext', 'useCanvasStageAspectState'],
};

/**
 * 주석을 걷는다. 금지 이름을 **산문에 적어 둔 머리말**이 가드를 빨갛게 만들면 다음 사람은
 * 가드가 아니라 머리말을 지운다(`svgimport/svgDocument.test.ts` 가 같은 이유로 같은 일을
 * 한다). `://` 를 line comment 로 오인하지 않도록 앞 글자를 하나 남긴다.
 */
function stripComments(text: string): string {
  return text.replace(/\/\*[\s\S]*?\*\//g, ' ').replace(/(^|[^:])\/\/.*$/gm, '$1');
}

/** `import … ;` 문장 전량. 줄머리에 걸어 산문 속의 "import" 를 집지 않는다. */
function importStatements(source: string): string[] {
  return source.match(/^import\b[^;]*;/gm) ?? [];
}

/**
 * 한 `import` 문이 묶는 이름들. **모르는 꼴은 조용히 버리지 않고 `?…` 로 돌려준다** —
 * 파서가 못 읽은 수입이 가드를 통과하면 그 가드는 그 순간부터 무동작이다.
 */
function boundNames(clause: string): string[] {
  const body = clause.replace(/^import\s+/, '').replace(/\btype\s+/g, '').trim();
  if (body === '') return [];

  const namespace = /^\*\s+as\s+([\w$]+)$/.exec(body);
  // 이름공간 수입은 모듈 전체를 들이는 것이라 허용목록으로 셀 수 없다 — 그대로 신호다.
  if (namespace !== null) return [`* as ${namespace[1] ?? ''}`];

  const braced = /\{([^}]*)\}/.exec(body);
  const names: string[] = [];

  const beforeBrace = (braced === null ? body : body.slice(0, braced.index)).replace(/,\s*$/, '').trim();
  if (beforeBrace !== '') {
    // 기본 수입. 식별자 하나가 아니면 못 읽은 것이다.
    names.push(/^[\w$]+$/.test(beforeBrace) ? beforeBrace : `?${beforeBrace}`);
  }
  for (const raw of (braced?.[1] ?? '').split(',')) {
    const piece = raw.trim();
    if (piece === '') continue;
    // `A as B` 는 **들여온 쪽**(A)을 센다. 개명으로 캔버스 이름을 감출 수 없게 한다.
    const imported = piece.split(/\s+as\s+/)[0]?.trim() ?? '';
    names.push(/^[\w$]+$/.test(imported) ? imported : `?${piece}`);
  }
  return names;
}

/** 다이얼로그가 실제로 들이는 `panels/canvas/` 수입 — 모듈 → 이름(정렬). */
export function actualCanvasImports(source = fs.readFileSync(DIALOG_PATH, 'utf8')): Record<string, string[]> {
  const found: Record<string, string[]> = {};
  for (const statement of importStatements(source)) {
    const withFrom = /^([\s\S]*?)\bfrom\s+'([^']+)'/.exec(statement);
    // 부작용 수입(`import 'x';`)도 수입이다. 빠뜨리면 그 통로가 무검사로 열린다.
    const sideEffect = /^import\s+'([^']+)'\s*;$/.exec(statement);
    const specifier = withFrom?.[2] ?? sideEffect?.[1];
    if (specifier === undefined || !specifier.includes('panels/canvas/')) continue;
    found[specifier] = [...(found[specifier] ?? []), ...boundNames(withFrom?.[1] ?? 'import')].sort();
  }
  return found;
}

/** 허용목록을 같은 꼴(정렬)로 편다. 비교가 순서에 걸려 헛되이 빨개지지 않게 한다. */
export function allowedCanvasImports(): Record<string, string[]> {
  return Object.fromEntries(
    Object.entries(ALLOWED_CANVAS_IMPORTS).map(([mod, names]) => [mod, [...names].sort()]),
  );
}

/**
 * 허용목록에서 **파생되는** `Canvas[A-Z]…` 식별자 전부. 모듈 경로와 이름 양쪽에서 뽑으므로
 * `useCanvasStageAspectState` 는 `CanvasStageAspectState` 를, 경로는 `CanvasEditDock` 을 낸다.
 * 두 번째 열거표를 만들지 않는 것이 요점이다 — 허용목록 한 곳만 고치면 여기도 따라온다.
 */
export function allowedCanvasIdentifiers(): string[] {
  const pool = Object.entries(ALLOWED_CANVAS_IMPORTS).flatMap(([mod, names]) => [mod, ...names]);
  return [...new Set(pool.flatMap((t) => [...t.matchAll(/Canvas[A-Z]\w*/g)].map((m) => m[0])))].sort();
}

/** 주석을 걷은 본문에 실제로 서 있는 `Canvas[A-Z]…` 식별자 전부. */
export function actualCanvasIdentifiers(source = fs.readFileSync(DIALOG_PATH, 'utf8')): string[] {
  const code = stripComments(source);
  return [...new Set([...code.matchAll(/Canvas[A-Z]\w*/g)].map((m) => m[0]))].sort();
}
