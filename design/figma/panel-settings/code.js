// xflow 그래프 패널 설정 화면 — Figma 아트보드 생성 플러그인.
//
// 값(색·라벨·간격)은 구현에서 그대로 옮겼다. index.css 의 테마 토큰과 ko.json 의 라벨을
// 손으로 다시 적으면 디자인과 화면이 조용히 갈라지므로, 바뀐 값은 여기서도 함께 고친다.
//
// 실행: Figma → Plugins → Development → Import plugin from manifest… → manifest.json

// ---------------------------------------------------------------- 테마 토큰
// web/src/index.css 의 :root / [data-theme='dark'] 값.
const PALETTES = {
  dark: {
    name: '다크',
    bgPrimary: '#111827',
    bgSurface: '#1f2937',
    bgElevated: '#374151',
    bgSunken: '#030712',
    textPrimary: '#f3f4f6',
    textSecondary: '#d1d5db',
    textMuted: '#9ca3af',
    borderDefault: '#374151',
    borderStrong: '#4b5563',
    accent: '#3b82f6',
  },
  light: {
    name: '라이트',
    bgPrimary: '#f1f5f9',
    bgSurface: '#ffffff',
    bgElevated: '#ffffff',
    bgSunken: '#e2e8f0',
    textPrimary: '#111827',
    textSecondary: '#374151',
    textMuted: '#6b7280',
    borderDefault: '#e5e7eb',
    borderStrong: '#d1d5db',
    accent: '#3b82f6',
  },
};

/** 시리즈 자동 팔레트 — chartChannelTypes.SERIES_PALETTE 앞 세 개. */
const SERIES_COLORS = ['#3b82f6', '#10b981', '#f59e0b'];

// ---------------------------------------------------------------- 유틸
function hex(h) {
  const v = h.replace('#', '');
  return {
    r: parseInt(v.slice(0, 2), 16) / 255,
    g: parseInt(v.slice(2, 4), 16) / 255,
    b: parseInt(v.slice(4, 6), 16) / 255,
  };
}
function solid(h, opacity) {
  const p = { type: 'SOLID', color: hex(h) };
  if (opacity !== undefined) p.opacity = opacity;
  return p;
}

/**
 * 쓸 수 있는 글꼴을 고른다.
 *
 * 한글이 든 화면이라 Inter 로는 글자가 깨진다(Inter 에 한글 글리프가 없다). 후보를 차례로
 * 시도해 **먼저 로드되는 것**을 쓴다 — 어떤 글꼴이 깔려 있는지는 실행하는 사람의 환경마다
 * 다르므로 하나로 못박을 수 없다.
 */
async function pickFont() {
  const candidates = ['Pretendard', 'Noto Sans KR', 'Apple SD Gothic Neo', 'Inter', 'Roboto'];
  const styleSets = [
    ['Regular', 'Medium', 'Bold'],
    ['Regular', 'Regular', 'Bold'],
    ['Regular', 'Regular', 'Regular'],
  ];
  for (const family of candidates) {
    for (const styles of styleSets) {
      try {
        const uniq = Array.from(new Set(styles));
        await Promise.all(uniq.map((style) => figma.loadFontAsync({ family, style })));
        return {
          regular: { family, style: styles[0] },
          medium: { family, style: styles[1] },
          bold: { family, style: styles[2] },
        };
      } catch (e) {
        /* 다음 후보 */
      }
    }
  }
  throw new Error('쓸 수 있는 글꼴을 찾지 못했습니다.');
}

let FONT = null;

function text(chars, opts) {
  const o = opts || {};
  const t = figma.createText();
  t.fontName = o.weight === 'bold' ? FONT.bold : o.weight === 'medium' ? FONT.medium : FONT.regular;
  t.fontSize = o.size || 12;
  t.characters = chars;
  t.fills = [solid(o.color || '#000000', o.opacity)];
  if (o.width) {
    t.textAutoResize = 'HEIGHT';
    t.resize(o.width, t.height);
  }
  if (o.lineHeight) t.lineHeight = { unit: 'PIXELS', value: o.lineHeight };
  return t;
}

/** 오토 레이아웃 프레임. 기본은 세로 hug. */
function frame(name, opts) {
  const o = opts || {};
  const f = figma.createFrame();
  f.name = name;
  f.layoutMode = o.dir === 'h' ? 'HORIZONTAL' : 'VERTICAL';
  f.itemSpacing = o.gap === undefined ? 8 : o.gap;
  f.paddingLeft = f.paddingRight = o.px === undefined ? 0 : o.px;
  f.paddingTop = f.paddingBottom = o.py === undefined ? 0 : o.py;
  if (o.pl !== undefined) f.paddingLeft = o.pl;
  if (o.pr !== undefined) f.paddingRight = o.pr;
  if (o.pt !== undefined) f.paddingTop = o.pt;
  if (o.pb !== undefined) f.paddingBottom = o.pb;
  // 주축은 방향이 정한다 — 세로 프레임은 높이가 주축, 가로 프레임은 폭이 주축이다.
  // 한쪽으로만 적으면 가로 프레임에서 폭·높이 고정이 서로 뒤바뀐다.
  const horizontal = f.layoutMode === 'HORIZONTAL';
  const primaryFixed = horizontal ? !!o.w : !!o.h;
  const counterFixed = horizontal ? !!o.h : !!o.w;
  f.primaryAxisSizingMode = primaryFixed ? 'FIXED' : 'AUTO';
  f.counterAxisSizingMode = counterFixed ? 'FIXED' : 'AUTO';
  f.counterAxisAlignItems = o.align || 'MIN';
  f.primaryAxisAlignItems = o.justify || 'MIN';
  f.fills = o.fill ? [solid(o.fill, o.fillOpacity)] : [];
  if (o.stroke) {
    f.strokes = [solid(o.stroke)];
    f.strokeWeight = o.strokeWeight || 1;
    f.strokeAlign = 'INSIDE';
  }
  if (o.radius !== undefined) f.cornerRadius = o.radius;
  if (o.w || o.h) f.resize(o.w || f.width, o.h || f.height);
  f.clipsContent = o.clip === undefined ? true : o.clip;
  return f;
}

/** 자식을 담고 가로/세로 늘림 옵션을 함께 적용한다. */
function add(parent, child, opts) {
  const o = opts || {};
  parent.appendChild(child);
  if (o.stretch) child.layoutAlign = 'STRETCH';
  if (o.grow) child.layoutGrow = 1;
  return child;
}

// ---------------------------------------------------------------- 컨트롤 목업
function fieldLabel(p, s) {
  return text(s, { size: 11, weight: 'medium', color: p.textMuted });
}

/** 입력 칸 — 값이 없으면 placeholder 톤으로 그린다. */
function input(p, value, opts) {
  const o = opts || {};
  const f = frame('input', {
    dir: 'h',
    w: o.w || 240,
    h: 30,
    px: 10,
    align: 'CENTER',
    fill: p.bgElevated,
    stroke: p.borderDefault,
    radius: 6,
  });
  add(f, text(value, { size: 12, color: o.placeholder ? p.textMuted : p.textPrimary }));
  return f;
}

/** 드롭다운 — 오른쪽 갈매기까지 포함한 한 덩어리. */
function select(p, value, opts) {
  const o = opts || {};
  const f = frame('select', {
    dir: 'h',
    w: o.w || 240,
    h: 30,
    px: 10,
    align: 'CENTER',
    justify: 'SPACE_BETWEEN',
    fill: p.bgElevated,
    stroke: p.borderDefault,
    radius: 6,
  });
  add(f, text(value, { size: 12, color: p.textPrimary }));
  add(f, text('⌄', { size: 12, color: p.textMuted }));
  return f;
}

function checkbox(p, label, checked) {
  const f = frame('checkbox', { dir: 'h', gap: 6, align: 'CENTER' });
  const box = frame('box', {
    w: 13,
    h: 13,
    radius: 3,
    fill: checked ? p.accent : p.bgElevated,
    stroke: checked ? p.accent : p.borderStrong,
    align: 'CENTER',
    justify: 'CENTER',
  });
  if (checked) add(box, text('✓', { size: 9, weight: 'bold', color: '#ffffff' }));
  add(f, box);
  add(f, text(label, { size: 11, color: p.textMuted }));
  return f;
}

/** 디자인 배지 — 축·범례·타이틀이 공유하는 접기 버튼. */
function designBadge(p) {
  const f = frame('디자인 배지', {
    dir: 'h',
    px: 6,
    py: 2,
    radius: 4,
    fill: p.bgElevated,
    stroke: p.borderDefault,
    align: 'CENTER',
  });
  add(f, text('디자인', { size: 11, color: p.textMuted }));
  return f;
}

function sectionHeader(p, title, withBadge) {
  const f = frame('헤더', { dir: 'h', gap: 6, align: 'CENTER' });
  add(f, text(title, { size: 12, weight: 'bold', color: p.textPrimary }));
  if (withBadge) add(f, designBadge(p));
  return f;
}

/**
 * 설정 절 — `CollapsibleSection` 의 모양 그대로.
 *
 * Grafana 패턴이라 **상자가 없다**: 배경도 테두리도 없고 절 사이는 아래쪽 1px 선으로만
 * 갈린다. 머리는 갈매기(펼침이면 90° 회전) + 굵은 제목이다.
 */
function section(p, title, withBadge) {
  const f = frame('절 ' + title, { gap: 0, w: 352 });

  const head = frame('머리', { dir: 'h', gap: 6, px: 4, py: 8, w: 352, align: 'CENTER' });
  add(head, text('⌄', { size: 11, color: p.textMuted }));
  add(head, text(title, { size: 12, weight: 'bold', color: p.textPrimary }));
  if (withBadge) add(head, designBadge(p));
  add(f, head, { stretch: true });

  const body = frame('본문', { gap: 8, pl: 4, pr: 4, pb: 12, w: 352 });
  add(f, body, { stretch: true });

  const rule = figma.createRectangle();
  rule.name = '절 구분선';
  rule.resize(352, 1);
  rule.fills = [solid(p.borderDefault)];
  add(f, rule, { stretch: true });

  // 호출부는 본문에 넣는다 — 머리·구분선은 이 함수가 소유한다.
  f.__body = body;
  return f;
}

/** 절의 본문에 넣는다. `section()` 이 돌려준 프레임에는 머리와 구분선이 함께 들어 있다. */
function inSection(sec, child, opts) {
  return add(sec.__body, child, opts);
}

function labeledRow(p, label, control) {
  const f = frame('행 ' + label, { gap: 5 });
  add(f, fieldLabel(p, label));
  add(f, control);
  return f;
}

/** 슬라이더 — 트랙 + 채운 구간 + 손잡이. */
function slider(p, pct, width) {
  const w = width || 200;
  // 손잡이가 트랙 위에 겹쳐야 하므로 오토 레이아웃을 쓰지 않는다.
  const f = figma.createFrame();
  f.name = '슬라이더';
  f.resize(w, 16);
  f.fills = [];
  f.clipsContent = false;
  const track = figma.createRectangle();
  track.name = '트랙';
  track.resize(w, 4);
  track.y = 6;
  track.cornerRadius = 2;
  track.fills = [solid(p.borderStrong)];
  f.appendChild(track);
  const fill = figma.createRectangle();
  fill.name = '채움';
  fill.resize(Math.max(4, w * pct), 4);
  fill.y = 6;
  fill.cornerRadius = 2;
  fill.fills = [solid(p.accent)];
  f.appendChild(fill);
  const knob = figma.createEllipse();
  knob.name = '손잡이';
  knob.resize(12, 12);
  knob.x = Math.max(0, w * pct - 6);
  knob.y = 2;
  knob.fills = [solid('#ffffff')];
  knob.strokes = [solid(p.accent)];
  knob.strokeWeight = 2;
  f.appendChild(knob);
  return f;
}

function pillButton(p, label) {
  const f = frame('버튼 ' + label, {
    dir: 'h',
    px: 8,
    py: 4,
    radius: 6,
    fill: p.bgElevated,
    stroke: p.borderDefault,
    align: 'CENTER',
  });
  add(f, text(label, { size: 11, color: p.textSecondary }));
  return f;
}

function iconButton(p, glyph) {
  const f = frame('아이콘 ' + glyph, {
    w: 20,
    h: 20,
    radius: 4,
    align: 'CENTER',
    justify: 'CENTER',
  });
  add(f, text(glyph, { size: 11, color: p.textMuted }));
  return f;
}

// ---------------------------------------------------------------- 옵션 컬럼
function optionsColumn(p) {
  // 실제 컬럼은 우측 고정 폭 360px(`optionsWidth` 기본값)에 `pr-1` 뿐이다 — 카드도 배경도
  // 없다. 절 사이 간격은 `space-y-0.5`(2px)이고 구분은 절 자신의 아래 선이 맡는다.
  const col = frame('옵션 컬럼', { gap: 2, pr: 4, w: 360 });

  // 1. 패널 옵션 — 타이틀 + 디자인 배지 + 타이틀 바 토글.
  const s1 = section(p, '패널 옵션', false);
  const titleRow = frame('타이틀 행', { gap: 5 });
  const titleLabelRow = frame('라벨 행', { dir: 'h', gap: 6, align: 'CENTER' });
  add(titleLabelRow, fieldLabel(p, '타이틀'));
  add(titleLabelRow, designBadge(p));
  add(titleRow, titleLabelRow);
  add(titleRow, input(p, '라인 차트', { w: 344 }));
  inSection(s1, titleRow);
  inSection(s1, checkbox(p, '타이틀 바 보이기', true));
  add(col, s1, { stretch: true });

  // 2. X축 — 레이블 · 가져올 데이터(범위) · 갱신 주기.
  const s2 = section(p, 'X축', true);
  inSection(s2, labeledRow(p, '레이블', input(p, '예: 시간', { w: 344, placeholder: true })));
  const xRange = frame('범위', { dir: 'h', gap: 8 });
  add(xRange, labeledRow(p, '가져올 데이터', select(p, '기간(상대)', { w: 168 })));
  add(xRange, labeledRow(p, ' ', input(p, '10분', { w: 168 })));
  inSection(s2, xRange);
  inSection(s2, labeledRow(p, '갱신 주기(ms)', input(p, '1000', { w: 344 })));
  add(col, s2, { stretch: true });

  // 3. Y축 — 레이블 · 단위 · 최소/최대.
  const s3 = section(p, 'Y축', true);
  inSection(s3, labeledRow(p, '레이블', input(p, '예: 온도', { w: 344, placeholder: true })));
  const yPair = frame('단위/자릿수', { dir: 'h', gap: 8 });
  add(yPair, labeledRow(p, '단위', select(p, '자동(바이트)', { w: 168 })));
  add(yPair, labeledRow(p, '소수점 자릿수', input(p, '2', { w: 168 })));
  inSection(s3, yPair);
  const yBound = frame('최소/최대', { dir: 'h', gap: 8 });
  add(yBound, labeledRow(p, '최소', input(p, '자동', { w: 168, placeholder: true })));
  add(yBound, labeledRow(p, '최대', input(p, '자동', { w: 168, placeholder: true })));
  inSection(s3, yBound);
  add(col, s3, { stretch: true });

  // 4. 그래프 스타일.
  const s4 = section(p, '그래프 스타일', false);
  inSection(s4, labeledRow(p, '그래프 스타일', select(p, '라인', { w: 344 })));
  inSection(s4, checkbox(p, '스택킹', false));
  add(col, s4, { stretch: true });

  // 5. 그래프 영역 — 슬라이더 + 값 + 초기화 + 힌트.
  const s5 = section(p, '그래프 영역', false);
  const sizeRow = frame('크기 행', { dir: 'h', gap: 8, align: 'CENTER' });
  add(sizeRow, slider(p, 1, 190));
  add(sizeRow, text('100%', { size: 11, color: p.textMuted }));
  add(sizeRow, pillButton(p, '초기화'));
  inSection(s5, sizeRow);
  add(
    s5,
    text('미리보기나 패널 배치 편집 모드에서 그래프를 끌어 옮길 수 있습니다.', {
      size: 11,
      color: p.textMuted,
      width: 344,
      lineHeight: 15,
    }),
  );
  add(col, s5, { stretch: true });

  // 6. 범례 — 구성 체크 3개 + 위치.
  const s6 = section(p, '범례', true);
  inSection(s6, fieldLabel(p, '구성'));
  const comp = frame('구성', { dir: 'h', gap: 12 });
  add(comp, checkbox(p, '이름', true));
  add(comp, checkbox(p, '라인', true));
  add(comp, checkbox(p, '마지막 값', false));
  inSection(s6, comp);
  inSection(s6, labeledRow(p, '범례 위치', select(p, '하단', { w: 344 })));
  add(col, s6, { stretch: true });

  // 7. 툴팁.
  const s7 = section(p, '툴팁', false);
  const tip = frame('툴팁 옵션', { dir: 'h', gap: 12 });
  add(tip, checkbox(p, '사용', true));
  add(tip, checkbox(p, '단일 값', false));
  inSection(s7, tip);
  add(col, s7, { stretch: true });

  return col;
}

// ---------------------------------------------------------------- 차트 목업
/** 라인 차트 미리보기 — 절대 배치(오토 레이아웃 밖)로 그린다. */
function chartMock(p, w, h) {
  const box = figma.createFrame();
  box.name = '차트';
  box.resize(w, h);
  box.fills = [solid(p.bgSurface)];
  box.strokes = [solid(p.borderDefault)];
  box.strokeWeight = 1;
  box.cornerRadius = 12;
  box.clipsContent = true;

  // 패널 타이틀.
  const title = text('라인 차트', { size: 13, weight: 'bold', color: p.textPrimary });
  title.x = 16;
  title.y = 14;
  box.appendChild(title);

  // 플롯 영역.
  const padL = 62;
  const padT = 46;
  const padR = 132;
  const padB = 34;
  const pw = w - padL - padR;
  const ph = h - padT - padB;

  for (let i = 0; i <= 4; i++) {
    const g = figma.createRectangle();
    g.name = '격자';
    g.resize(pw, 1);
    g.x = padL;
    g.y = padT + (ph / 4) * i;
    g.fills = [solid(p.borderDefault, 0.6)];
    box.appendChild(g);
  }
  const axis = figma.createRectangle();
  axis.name = 'Y축';
  axis.resize(1, ph);
  axis.x = padL;
  axis.y = padT;
  axis.fills = [solid(p.borderStrong)];
  box.appendChild(axis);

  // 시리즈 세 줄 — 위상만 달리한 같은 파형.
  const points = 40;
  SERIES_COLORS.forEach((color, idx) => {
    let d = '';
    for (let i = 0; i <= points; i++) {
      const x = padL + (pw / points) * i;
      const phase = (idx * Math.PI) / 3;
      const y = padT + ph / 2 - Math.sin((i / points) * Math.PI * 2 + phase) * (ph / 3);
      d += (i === 0 ? 'M ' : ' L ') + x.toFixed(1) + ' ' + y.toFixed(1);
    }
    const v = figma.createVector();
    v.name = '시리즈 ' + (idx + 1);
    box.appendChild(v);
    v.vectorPaths = [{ windingRule: 'NONE', data: d }];
    v.strokes = [solid(color)];
    v.strokeWeight = 2;
    v.strokeCap = 'ROUND';
    v.fills = [];
  });

  // 축 눈금.
  ['1.2 G', '0.9 G', '0.6 G', '0.3 G', '0'].forEach((label, i) => {
    const t = text(label, { size: 10, color: p.textMuted });
    t.x = padL - 8 - t.width;
    t.y = padT + (ph / 4) * i - 6;
    box.appendChild(t);
  });
  ['07:45', '08:00', '08:15', '08:30'].forEach((label, i) => {
    const t = text(label, { size: 10, color: p.textMuted });
    t.x = padL + (pw / 4) * i - t.width / 2 + 20;
    t.y = padT + ph + 8;
    box.appendChild(t);
  });

  // 범례 — 우측 세로 배치(구분선 + 항목).
  const sep = figma.createRectangle();
  sep.name = '범례 구분선';
  sep.resize(1, ph + 20);
  sep.x = w - padR + 12;
  sep.y = padT - 10;
  sep.fills = [solid(p.borderDefault)];
  box.appendChild(sep);

  ['en0', 'lo0', 'utun4'].forEach((name, i) => {
    const line = figma.createRectangle();
    line.name = '범례 선';
    line.resize(12, 2);
    line.cornerRadius = 1;
    line.x = w - padR + 26;
    line.y = padT + ph / 2 - 18 + i * 18 + 5;
    line.fills = [solid(SERIES_COLORS[i])];
    box.appendChild(line);

    const t = text(name, { size: 11, color: p.textPrimary });
    t.x = w - padR + 44;
    t.y = padT + ph / 2 - 18 + i * 18;
    box.appendChild(t);
  });

  return box;
}

// ---------------------------------------------------------------- 미리보기 / 데이터 소스
function previewPane(p, w, h) {
  const pane = frame('미리보기', { gap: 10, w: w, h: h });

  const bar = frame('툴바', { dir: 'h', justify: 'SPACE_BETWEEN', align: 'CENTER', w: w, h: 20 });
  add(bar, text('패널 스타일 미리보기', { size: 11, weight: 'medium', color: p.textMuted }));
  const tools = frame('도구', { dir: 'h', gap: 2, align: 'CENTER' });
  add(tools, iconButton(p, '−'));
  add(tools, text('100%', { size: 10, weight: 'medium', color: p.textMuted }));
  add(tools, iconButton(p, '+'));
  add(tools, text('│', { size: 10, color: p.borderDefault }));
  add(tools, iconButton(p, '⤢'));
  add(tools, text('│', { size: 10, color: p.borderDefault }));
  add(tools, iconButton(p, '↺'));
  add(tools, text('│', { size: 10, color: p.borderDefault }));
  add(tools, iconButton(p, '‹'));
  add(bar, tools);
  add(pane, bar, { stretch: true });

  const shell = frame('미리보기 상자', {
    px: 12,
    py: 12,
    w: w,
    h: h - 30,
    fill: p.bgElevated,
    stroke: p.borderDefault,
    radius: 12,
  });
  add(shell, chartMock(p, w - 24, h - 54));
  add(pane, shell, { stretch: true });
  return pane;
}

function dataSourcePane(p, w, h) {
  const pane = frame('데이터 소스', {
    gap: 10,
    px: 14,
    py: 14,
    w: w,
    h: h,
    fill: p.bgSurface,
    stroke: p.borderDefault,
    radius: 10,
  });
  add(pane, text('데이터 소스', { size: 12, weight: 'bold', color: p.textPrimary }));

  const row = frame('행', { dir: 'h', gap: 10 });
  add(row, labeledRow(p, '소스', select(p, '시스템 지표', { w: 220 })));
  add(row, labeledRow(p, '에이전트', select(p, 'host-01', { w: 220 })));
  add(row, labeledRow(p, '조회 방식', select(p, '실시간', { w: 220 })));
  add(row, labeledRow(p, '폴링 주기', input(p, '5000 ms', { w: 220 })));
  add(pane, row);

  const table = frame('시리즈 표', {
    w: w - 28,
    stroke: p.borderDefault,
    radius: 8,
    fill: p.bgElevated,
    fillOpacity: 0.5,
  });
  const head = frame('머리', {
    dir: 'h',
    px: 12,
    py: 8,
    gap: 24,
    w: w - 28,
    fill: p.bgElevated,
  });
  ['measurement', '분류', '대상', '표기', '이름'].forEach((h) => {
    add(head, text(h, { size: 11, weight: 'medium', color: p.textMuted }));
  });
  add(table, head, { stretch: true });
  [
    ['bytes_recv', 'network', 'en0', 'bytes', 'en0'],
    ['bytes_recv', 'network', 'lo0', 'bytes', 'lo0'],
    ['bytes_recv', 'network', 'utun4', 'bytes', 'utun4'],
  ].forEach((cells) => {
    const r = frame('행', { dir: 'h', px: 12, py: 8, gap: 24, w: w - 28 });
    cells.forEach((c) => add(r, text(c, { size: 11, color: p.textSecondary })));
    add(table, r, { stretch: true });
  });
  add(pane, table, { stretch: true });
  return pane;
}

// ---------------------------------------------------------------- 화면
function buildScreen(p) {
  // 실제 배치는 **미리보기가 왼쪽, 옵션이 오른쪽**이다. flex order 로 뒤집혀 있어
  // (미리보기 order-1 · 스플리터 order-2 · 옵션 order-3) JSX 순서만 보면 반대로 읽힌다.
  // 옵션 컬럼은 고정 360px, 미리보기 컬럼이 남는 폭을 채운다.
  const W = 1440;
  const H = 900;

  const page = frame('그래프 패널 설정 — ' + p.name, {
    w: W,
    h: H,
    fill: p.bgPrimary,
    gap: 0,
  });

  // 페이지 컨테이너 — 둥근 테두리 + surface 배경.
  const shell = frame('설정 페이지', {
    gap: 0,
    w: W - 32,
    h: H - 32,
    fill: p.bgSurface,
    stroke: p.borderDefault,
    radius: 16,
  });
  shell.x = 16;
  shell.y = 16;

  // 헤더 — 뒤로가기 + 제목, 오른쪽 닫기.
  const header = frame('헤더', {
    dir: 'h',
    w: W - 32,
    pl: 20,
    pr: 20,
    pt: 16,
    pb: 12,
    justify: 'SPACE_BETWEEN',
    align: 'CENTER',
  });
  const headLeft = frame('제목', { dir: 'h', gap: 8, align: 'CENTER' });
  add(headLeft, text('←', { size: 16, color: p.textMuted }));
  add(headLeft, text('패널 설정', { size: 16, weight: 'bold', color: p.textPrimary }));
  add(header, headLeft);
  add(header, text('✕', { size: 14, color: p.textMuted }));
  add(shell, header, { stretch: true });

  // 본문 — [미리보기 컬럼(flex-1)] [스플리터] [옵션 컬럼(360)].
  const contentW = W - 32;
  const body = frame('본문', {
    dir: 'h',
    gap: 12,
    pl: 20,
    pr: 20,
    pb: 20,
    w: contentW,
    h: H - 32 - 60,
  });

  const leftW = contentW - 40 - 12 - 12 - 360;
  const leftCol = frame('미리보기 컬럼', { gap: 12, w: leftW, h: H - 32 - 60 - 20 });
  // 세로 경계 기본값은 0.5 — 미리보기가 절반, 데이터 소스가 나머지다.
  const halfH = (H - 32 - 60 - 20 - 12) / 2;
  add(leftCol, previewPane(p, leftW, halfH), { stretch: true });
  add(leftCol, dataSourcePane(p, leftW, halfH), { stretch: true });
  add(body, leftCol);

  // 스플리터 — 가운데 짧은 손잡이만 보인다.
  const split = frame('스플리터', { w: 8, h: H - 32 - 60 - 20, align: 'CENTER', justify: 'CENTER' });
  const grip = figma.createRectangle();
  grip.name = '손잡이';
  grip.resize(2, 48);
  grip.cornerRadius = 1;
  grip.fills = [solid(p.borderDefault)];
  add(split, grip);
  add(body, split);

  add(body, optionsColumn(p));
  add(shell, body, { stretch: true });

  page.layoutMode = 'NONE';
  page.appendChild(shell);
  return page;
}

/** 디자인 배지의 열린 상태 — 화면에서는 접혀 있어 따로 그리지 않으면 보이지 않는다. */
function buildPopoverSheet(p) {
  const sheet = frame('디자인 배지 — 열림', {
    gap: 24,
    px: 24,
    py: 24,
    w: 720,
    fill: p.bgPrimary,
  });
  add(sheet, text('디자인 배지 (열린 상태)', { size: 14, weight: 'bold', color: p.textPrimary }));

  function popover(title, rows, extra) {
    const wrap = frame(title, { gap: 8 });
    add(wrap, sectionHeader(p, title, true));
    const pop = frame('팝오버', {
      gap: 6,
      px: 10,
      py: 10,
      w: 288,
      fill: p.bgPrimary,
      stroke: p.borderDefault,
      radius: 6,
    });
    const head = frame('머리', { dir: 'h', gap: 8, align: 'CENTER' });
    add(head, text('글꼴', { size: 11, color: p.textMuted, width: 80 }));
    add(head, text('크기', { size: 11, color: p.textMuted, width: 56 }));
    add(head, text('색', { size: 11, color: p.textMuted, width: 24 }));
    if (extra) add(head, text(extra, { size: 11, color: p.textMuted, width: 24 }));
    add(pop, head, { stretch: true });
    rows.forEach((label) => {
      const r = frame('행 ' + label, { dir: 'h', gap: 8, align: 'CENTER' });
      add(r, text(label, { size: 11, color: p.textMuted, width: 80 }));
      add(r, select(p, '상속', { w: 96 }));
      add(r, input(p, '10', { w: 44 }));
      const sw = frame('색', { w: 26, h: 26, radius: 6, fill: p.textMuted, stroke: p.borderDefault });
      add(r, sw);
      if (extra) add(r, checkbox(p, '', false));
      add(pop, r, { stretch: true });
    });
    add(wrap, pop);
    return wrap;
  }

  const row = frame('세 배지', { dir: 'h', gap: 20 });
  add(row, popover('X축', ['레이블', '값'], '굵'));
  add(row, popover('범례', ['범례 글자']));
  add(row, popover('타이틀', ['타이틀 글자'], '굵'));
  add(sheet, row);
  return sheet;
}

// ---------------------------------------------------------------- DOM 트리 → 프레임
//
// 손으로 옮긴 구조는 반드시 실제와 갈라진다. `extract.js` 가 화면에서 읽어 온 좌표·색·글자를
// 그대로 프레임으로 옮기면 그 갈림이 사라진다 — 여기서는 배치를 **해석하지 않고** 받는다.
// 그래서 오토 레이아웃을 쓰지 않는다: 이미 계산된 좌표를 다시 레이아웃에 맡기면 값이 바뀐다.

/** CSS 굵기 → 준비된 세 자족 중 하나. */
function weightFont(w) {
  if (w >= 700) return FONT.bold;
  if (w >= 500) return FONT.medium;
  return FONT.regular;
}

function paintOf(c) {
  if (!c) return [];
  const p = solid(c.hex);
  if (c.alpha !== undefined && c.alpha < 1) p.opacity = c.alpha;
  return [p];
}

/** 노드 하나를 그린다. 좌표는 부모 기준으로 바꿔 넣는다. */
function drawNode(node, parent, originX, originY) {
  const x = node.x - originX;
  const y = node.y - originY;

  // 글자만 든 노드는 텍스트로 만든다 — 사각형으로 두면 글자가 사라진다.
  if (node.text) {
    const t = figma.createText();
    t.name = node.text.slice(0, 24);
    t.fontName = weightFont(node.fontWeight || 400);
    t.fontSize = Math.max(1, node.fontSize || 12);
    t.textAutoResize = 'NONE';
    t.characters = node.text;
    t.fills = paintOf(node.color) ;
    if (node.align === 'center') t.textAlignHorizontal = 'CENTER';
    else if (node.align === 'right' || node.align === 'end') t.textAlignHorizontal = 'RIGHT';
    t.textAlignVertical = 'CENTER';
    parent.appendChild(t);
    t.resize(Math.max(1, node.w), Math.max(1, node.h));
    t.x = x;
    t.y = y;
    return t;
  }

  const f = figma.createFrame();
  f.name = node.testid || node.tag;
  f.layoutMode = 'NONE';
  f.resize(Math.max(1, node.w), Math.max(1, node.h));
  f.x = x;
  f.y = y;
  f.fills = paintOf(node.bg);
  f.cornerRadius = node.radius || 0;
  // 아이콘은 벡터를 옮기지 않고 자리만 남긴다 — 자리표시자임을 이름으로 밝힌다.
  f.clipsContent = false;
  if (node.icon) {
    f.name = 'icon(' + f.name + ')';
    f.opacity = 0.45;
    f.fills = paintOf(node.color) ;
    f.cornerRadius = 3;
    parent.appendChild(f);
    return f;
  }
  parent.appendChild(f);

  // 테두리는 변마다 두께가 다를 수 있다(절 구분선은 아래쪽 1px 뿐이다).
  // Figma 프레임의 stroke 는 네 변이 한 값이므로, 한 변만 있으면 얇은 사각형으로 그린다.
  if (node.border && node.border.color) {
    const b = node.border;
    const same = b.top === b.left && b.left === b.bottom && b.bottom === b.right && b.top > 0;
    if (same) {
      f.strokes = paintOf(b.color);
      f.strokeWeight = b.top;
      f.strokeAlign = 'INSIDE';
    } else {
      const edges = [
        ['top', 0, 0, node.w, b.top],
        ['bottom', 0, node.h - b.bottom, node.w, b.bottom],
        ['left', 0, 0, b.left, node.h],
        ['right', node.w - b.right, 0, b.right, node.h],
      ];
      for (const [name, ex, ey, ew, eh] of edges) {
        if (!(eh > 0) || !(ew > 0)) continue;
        const r = figma.createRectangle();
        r.name = '테두리 ' + name;
        r.resize(Math.max(1, ew), Math.max(1, eh));
        r.x = ex;
        r.y = ey;
        r.fills = paintOf(b.color);
        f.appendChild(r);
      }
    }
  }

  for (const c of node.children || []) drawNode(c, f, node.x, node.y);
  return f;
}

function buildFromDom(tree) {
  const page = figma.createFrame();
  page.name = '그래프 패널 설정 — 실제 화면 (' + (tree.theme || 'system') + ')';
  page.layoutMode = 'NONE';
  page.resize(Math.max(1, tree.w), Math.max(1, tree.h));
  page.fills = [];
  page.clipsContent = false;
  drawNode(tree.root, page, tree.root.x, tree.root.y);
  return page;
}

// ---------------------------------------------------------------- 실행
function place(nodes) {
  let x = 0;
  for (const n of nodes) {
    figma.currentPage.appendChild(n);
    n.x = x;
    n.y = 0;
    x += n.width + 80;
  }
  figma.currentPage.selection = nodes;
  figma.viewport.scrollAndZoomIntoView(nodes);
}

function buildMock() {
  const dark = buildScreen(PALETTES.dark);
  const light = buildScreen(PALETTES.light);
  const popover = buildPopoverSheet(PALETTES.dark);
  place([dark, light, popover]);
  return '목업 아트보드 3개를 만들었습니다 (' + FONT.regular.family + ' 사용).';
}

figma.showUI(__html__, { width: 420, height: 340 });

figma.ui.onmessage = async (msg) => {
  try {
    if (!FONT) FONT = await pickFont();
    if (msg.type === 'build-dom') {
      const frame = buildFromDom(msg.tree);
      place([frame]);
      let count = 0;
      (function n(x) {
        count++;
        (x.children || []).forEach(n);
      })(msg.tree.root);
      figma.closePlugin('실제 화면 ' + count + '개 노드를 옮겼습니다.');
      return;
    }
    if (msg.type === 'build-mock') {
      figma.closePlugin(buildMock());
      return;
    }
  } catch (e) {
    figma.closePlugin('실패: ' + e.message);
  }
};
