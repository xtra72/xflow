import { StrictMode } from 'react';
import ReactDOM from 'react-dom/client';

import '@/index.css';
import App from '@/App';

// 인증/갱신 인터셉터는 client.ts 가 모듈 적재 시점에 스스로 붙인다 — 봉투
// 인터셉터보다 먼저 등록되어야 하기 때문이다(client.ts 주석 참조). 여기서 직접
// 붙이면 순서가 뒤집혀 갱신이 죽는다. 이 부수효과 import 는 라우트가 지연 적재라
// 어떤 서비스 모듈도 아직 불러오지 않은 상태에서 렌더가 시작되는 경우까지
// 덮으려고 남겨 둔다.
import '@/services/api/client';

ReactDOM.createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <App />
  </StrictMode>,
);
