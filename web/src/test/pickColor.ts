// 색을 고르는 시험 동작 — 공용 `ColorPicker` 를 지나는 한 갈래.
//
// 색 칸은 더 이상 네이티브 `<input type="color">` 가 아니라 팝오버를 여는 단추다.
// `fireEvent.change(색칸, …)` 은 단추에 아무 일도 일으키지 않고 **조용히 통과한다** —
// 그래서 이 도우미를 쓰지 않고 옛 형태를 남기면 시험이 초록인 채로 아무것도 재지 않는다.
//
// @spec SPEC-COLOR-001 §결정 1 (M9)

import { fireEvent, screen } from '@testing-library/react';

/**
 * 색 칸을 열어 16진 값을 넣고 닫는다.
 *
 * 닫는 것(Esc)은 팝오버가 하나만 떠 있게 하기 위해서다. `fireEvent.click` 은
 * `mousedown` 을 내지 않아 바깥 클릭으로 닫히지 않으므로, 두 칸을 잇달아 열면
 * `colorpicker-hex` 조회가 둘로 갈라진다.
 */
export function pickColorByTestId(testId: string, hex: string): void {
  fireEvent.click(screen.getByTestId(testId));
  fireEvent.change(screen.getByTestId('colorpicker-hex'), { target: { value: hex } });
  fireEvent.keyDown(document, { key: 'Escape' });
}

/** 접근성 이름으로 여는 같은 동작. */
export function pickColorByLabel(label: string, hex: string): void {
  fireEvent.click(screen.getByLabelText(label));
  fireEvent.change(screen.getByTestId('colorpicker-hex'), { target: { value: hex } });
  fireEvent.keyDown(document, { key: 'Escape' });
}
