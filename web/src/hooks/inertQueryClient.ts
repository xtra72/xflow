// `QueryClientProvider` 가 없을 때 쓰는 **비활성 대체 클라이언트**.
//
// 여러 패널이 공유하는 데이터 훅은 소스 종류와 무관하게 **항상** 호출된다(훅 규칙).
// 그런데 패널 단위 테스트 다수는 데이터 계층을 모킹해 걷어내고 Provider 없이 렌더한다 —
// 그 자리에서 react-query 컨텍스트를 요구하면 소스와 무관한 패널 렌더가 전부 깨진다.
//
// 컨텍스트가 없으면 이 클라이언트를 쓰되 `enabled: false` 로 두어 **어떤 요청도 나가지
// 않는다**. 사용 형태는 다음과 같다:
//
//   const client = useContext(QueryClientContext);
//   const query = useQuery({ ..., enabled: client !== undefined }, client ?? inertQueryClient());
//
// 하나의 싱글턴을 나눠 쓰는 이유: 훅마다 자기 것을 만들면 비활성 클라이언트가 훅 수만큼
// 생기고, 각각이 캐시·타이머를 들고 있게 된다. 어차피 아무 요청도 하지 않으므로 하나면 된다.

import { QueryClient } from '@tanstack/react-query';

let inertClient: QueryClient | undefined;

/** Provider 부재 시 쓰는 비활성 클라이언트(싱글턴). */
export function inertQueryClient(): QueryClient {
  inertClient ??= new QueryClient();
  return inertClient;
}
