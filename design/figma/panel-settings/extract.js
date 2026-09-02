// 실행 중인 화면에서 **실제 레이아웃을 그대로 뽑는다** — 브라우저 콘솔에 붙여 넣는다.
//
// 손으로 옮겨 적은 구조는 반드시 실제와 갈라진다(조건부 렌더·flex order·사용자가 끌어
// 놓은 폭까지 코드만 읽어서는 알 수 없다). 그래서 그리는 값을 화면에서 직접 읽는다.
//
// 쓰는 법
//   1. 패널 설정 화면(/panels/:id/settings)을 연다. 보고 싶은 상태 그대로 둔다.
//   2. DevTools 콘솔에 이 파일 전체를 붙여 넣고 실행한다.
//   3. panel-settings-dom.json 이 내려받아진다.
//   4. Figma 플러그인을 열고 그 JSON 을 붙여 넣는다.

(() => {
  const root =
    document.querySelector('[data-panel-settings-content]')?.closest('.rounded-2xl') ??
    document.querySelector('[data-panel-settings-content]') ??
    document.body;
  if (!root) return console.error('패널 설정 화면을 찾지 못했습니다.');

  const base = root.getBoundingClientRect();
  const SKIP = new Set(['SCRIPT', 'STYLE', 'NOSCRIPT', 'BR']);

  /** rgb(a) 문자열 → { hex, alpha }. 투명이면 null. */
  function color(v) {
    const m = /rgba?\(([^)]+)\)/.exec(v || '');
    if (!m) return null;
    const [r, g, b, a] = m[1].split(',').map((n) => parseFloat(n));
    if (a === 0) return null;
    const hex =
      '#' + [r, g, b].map((c) => Math.round(c).toString(16).padStart(2, '0')).join('');
    return { hex, alpha: a === undefined ? 1 : a };
  }

  /** 자식 엘리먼트 없이 글자만 든 노드인가 — 그 글자를 텍스트 노드로 만든다. */
  function ownText(el) {
    let s = '';
    for (const n of el.childNodes) {
      if (n.nodeType === 3) s += n.nodeValue;
      else if (n.nodeType === 1) return null;
    }
    s = s.replace(/\s+/g, ' ').trim();
    return s || null;
  }

  function walk(el, depth) {
    if (SKIP.has(el.tagName)) return null;
    const cs = getComputedStyle(el);
    if (cs.display === 'none' || cs.visibility === 'hidden' || cs.opacity === '0') return null;
    const r = el.getBoundingClientRect();
    if (r.width < 1 || r.height < 1) return null;
    // 화면 밖(스크롤로 잘린 부분)도 담는다 — 설정 컬럼은 스크롤되며, 디자인에는 전체가 필요하다.

    const node = {
      tag: el.tagName.toLowerCase(),
      testid: el.getAttribute('data-testid') || undefined,
      x: Math.round((r.left - base.left) * 10) / 10,
      y: Math.round((r.top - base.top) * 10) / 10,
      w: Math.round(r.width * 10) / 10,
      h: Math.round(r.height * 10) / 10,
      bg: color(cs.backgroundColor),
      radius: parseFloat(cs.borderTopLeftRadius) || 0,
      children: [],
    };

    const bw = parseFloat(cs.borderTopWidth) || 0;
    const bl = parseFloat(cs.borderLeftWidth) || 0;
    const bb = parseFloat(cs.borderBottomWidth) || 0;
    const br = parseFloat(cs.borderRightWidth) || 0;
    if (bw || bl || bb || br) {
      node.border = {
        color: color(cs.borderTopColor) || color(cs.borderLeftColor),
        top: bw,
        left: bl,
        bottom: bb,
        right: br,
      };
    }

    const t = ownText(el);
    if (t) {
      node.text = t;
      node.fontSize = Math.round(parseFloat(cs.fontSize) * 10) / 10;
      node.fontWeight = parseInt(cs.fontWeight, 10) || 400;
      node.color = color(cs.color);
      node.align = cs.textAlign;
    }

    // SVG 는 통째로 사각형 자리표시자로 둔다 — 아이콘 벡터까지 옮기면 JSON 이 폭증한다.
    if (el.tagName.toLowerCase() === 'svg') {
      node.icon = true;
      return node;
    }

    for (const c of el.children) {
      const cn = walk(c, depth + 1);
      if (cn) node.children.push(cn);
    }
    return node;
  }

  const tree = {
    w: Math.round(base.width),
    h: Math.round(base.height),
    theme: document.documentElement.getAttribute('data-theme') || 'system',
    capturedAt: new Date().toISOString(),
    root: walk(root, 0),
  };

  const json = JSON.stringify(tree);
  const blob = new Blob([json], { type: 'application/json' });
  const a = document.createElement('a');
  a.href = URL.createObjectURL(blob);
  a.download = 'panel-settings-dom.json';
  a.click();
  URL.revokeObjectURL(a.href);

  let count = 0;
  (function n(x) {
    count++;
    x.children?.forEach(n);
  })(tree.root);
  console.log(`뽑았습니다: ${count}개 노드, ${tree.w}x${tree.h}, 테마 ${tree.theme}`);
  console.log('panel-settings-dom.json 을 Figma 플러그인에 붙여 넣으세요.');
})();
