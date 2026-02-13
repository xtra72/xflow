---
id: SPEC-API-001
type: acceptance
version: "1.0.0"
status: draft
created: "2026-02-13"
updated: "2026-02-13"
author: xtra
---

# SPEC-API-001: REST API Service System - 인수 테스트 기준

## 1. API Server Core (P0)

### AC-API-001-01: 서버 시작 및 종료
```gherkin
기능: API 서버 생명주기 관리

  시나리오: 서버 정상 시작
    주어진 유효한 서버 설정(포트: 8080)이 제공되었을 때
    그리고 의존 서비스(DB, Engine)가 정상 상태일 때
    만약 Server.Start()를 호출하면
    그러면 HTTP 리스너가 설정된 포트에서 시작되어야 한다
    그리고 서버 상태가 "Running"으로 전이되어야 한다
    그리고 ListenAddr()가 ":8080"을 반환해야 한다

  시나리오: 서버 graceful shutdown
    주어진 서버가 Running 상태이고
    그리고 5개의 요청이 처리 중일 때
    만약 Server.Stop()을 호출하면
    그러면 새로운 요청 수신을 중단해야 한다
    그리고 진행 중인 5개의 요청이 완료될 때까지 대기해야 한다
    그리고 설정된 타임아웃(30초) 내에 서버가 종료되어야 한다
    그리고 서버 상태가 "Stopped"로 전이되어야 한다

  시나리오: OS 시그널에 의한 종료
    주어진 서버가 Running 상태일 때
    만약 SIGTERM 시그널을 수신하면
    그러면 graceful shutdown 프로세스가 시작되어야 한다
    그리고 진행 중인 요청 완료 후 서버가 종료되어야 한다
```

### AC-API-001-02: Health Check 엔드포인트
```gherkin
기능: 서버 상태 확인 엔드포인트

  시나리오: liveness 확인 - 정상
    주어진 API 서버가 Running 상태일 때
    만약 GET /health 요청을 보내면
    그러면 200 OK를 반환해야 한다
    그리고 응답에 {"status": "ok"} 형식이 포함되어야 한다
    그리고 인증 없이 접근 가능해야 한다

  시나리오: readiness 확인 - 정상
    주어진 API 서버가 Running 상태이고
    그리고 DB 연결이 활성이고 Engine이 초기화된 상태일 때
    만약 GET /ready 요청을 보내면
    그러면 200 OK를 반환해야 한다
    그리고 각 의존 서비스의 상태가 포함되어야 한다

  시나리오: readiness 확인 - DB 연결 실패
    주어진 API 서버가 Running 상태이고
    그리고 DB 연결이 끊어진 상태일 때
    만약 GET /ready 요청을 보내면
    그러면 503 Service Unavailable을 반환해야 한다
    그리고 DB 상태가 "unhealthy"로 표시되어야 한다
```

---

## 2. Router & Versioning (P0)

### AC-API-001-03: API 버전 프리픽스
```gherkin
기능: API 버전 관리

  시나리오: v1 API 접근
    주어진 API 서버가 Running 상태일 때
    만약 GET /api/v1/flows 요청을 보내면
    그러면 200 OK와 플로우 목록을 반환해야 한다

  시나리오: 버전 없는 API 접근 거부
    주어진 API 서버가 Running 상태일 때
    만약 GET /flows 요청을 보내면
    그러면 404 Not Found를 반환해야 한다

  시나리오: 정적 파일 서빙
    주어진 API 서버가 Running 상태이고
    그리고 web/dist/ 디렉토리에 빌드 결과물이 있을 때
    만약 GET / 요청을 보내면
    그러면 Web Dashboard의 index.html을 반환해야 한다
```

---

## 3. Middleware Stack (P0)

### AC-API-001-04: 요청 ID 미들웨어
```gherkin
기능: 요청별 고유 식별자

  시나리오: 요청 ID 자동 생성
    주어진 API 서버가 Running 상태일 때
    만약 X-Request-ID 헤더 없이 요청을 보내면
    그러면 응답에 X-Request-ID 헤더가 UUID v4 형식으로 포함되어야 한다
    그리고 요청 로그에 동일한 request_id가 기록되어야 한다

  시나리오: 클라이언트 제공 요청 ID 전파
    주어진 API 서버가 Running 상태일 때
    만약 X-Request-ID: "abc-123" 헤더와 함께 요청을 보내면
    그러면 응답의 X-Request-ID가 "abc-123"이어야 한다
```

### AC-API-001-05: 인증 미들웨어
```gherkin
기능: JWT/API 키 기반 인증

  시나리오: 유효한 JWT 토큰으로 접근
    주어진 유효한 JWT 액세스 토큰이 있을 때
    만약 Authorization: Bearer <token> 헤더와 함께 GET /api/v1/flows 요청을 보내면
    그러면 200 OK와 플로우 목록을 반환해야 한다

  시나리오: 만료된 JWT 토큰으로 접근
    주어진 만료된 JWT 액세스 토큰이 있을 때
    만약 Authorization: Bearer <expired_token> 헤더와 함께 요청을 보내면
    그러면 401 Unauthorized를 반환해야 한다
    그리고 에러 코드가 "UNAUTHORIZED"이어야 한다

  시나리오: 유효한 API 키로 접근
    주어진 유효한 API 키가 있을 때
    만약 X-API-Key: <valid_key> 헤더와 함께 요청을 보내면
    그러면 인증이 성공하고 요청이 처리되어야 한다

  시나리오: 인증 없이 보호 엔드포인트 접근
    주어진 인증 헤더가 없을 때
    만약 GET /api/v1/flows 요청을 보내면
    그러면 401 Unauthorized를 반환해야 한다

  시나리오: 권한 부족으로 접근 거부
    주어진 Viewer 역할의 JWT 토큰이 있을 때
    만약 DELETE /api/v1/flows/:id 요청을 보내면
    그러면 403 Forbidden을 반환해야 한다
    그리고 에러 코드가 "FORBIDDEN"이어야 한다
```

### AC-API-001-06: CORS 미들웨어
```gherkin
기능: 교차 출처 요청 처리

  시나리오: 허용된 출처에서의 preflight 요청
    주어진 CORS 허용 출처에 "http://localhost:3000"이 설정되었을 때
    만약 Origin: http://localhost:3000 헤더와 함께 OPTIONS 요청을 보내면
    그러면 200 OK를 반환해야 한다
    그리고 Access-Control-Allow-Origin 헤더가 포함되어야 한다

  시나리오: 허용되지 않은 출처에서의 요청
    주어진 CORS 허용 출처에 "http://evil.com"이 없을 때
    만약 Origin: http://evil.com 헤더와 함께 요청을 보내면
    그러면 CORS 헤더가 응답에 포함되지 않아야 한다
```

### AC-API-001-07: 레이트 리밋 미들웨어
```gherkin
기능: 클라이언트별 요청 제한

  시나리오: 레이트 리밋 미초과
    주어진 레이트 리밋이 100 req/min으로 설정되었을 때
    만약 1분 내에 50개의 요청을 보내면
    그러면 모든 요청이 정상 처리되어야 한다

  시나리오: 레이트 리밋 초과
    주어진 레이트 리밋이 100 req/min으로 설정되었을 때
    만약 1분 내에 101번째 요청을 보내면
    그러면 429 Too Many Requests를 반환해야 한다
    그리고 Retry-After 헤더가 포함되어야 한다
    그리고 에러 코드가 "RATE_LIMIT_EXCEEDED"이어야 한다
```

### AC-API-001-08: 에러 핸들링 미들웨어
```gherkin
기능: panic 복구 및 에러 처리

  시나리오: 핸들러 panic 복구
    주어진 핸들러에서 panic이 발생했을 때
    만약 해당 엔드포인트에 요청을 보내면
    그러면 500 Internal Server Error를 반환해야 한다
    그리고 표준 에러 응답 형식 {success: false, error: {...}}을 반환해야 한다
    그리고 스택 트레이스가 클라이언트 응답에 노출되지 않아야 한다
    그리고 서버가 계속 동작해야 한다

  시나리오: 타임아웃 초과
    주어진 요청 타임아웃이 30초로 설정되었을 때
    만약 처리에 35초가 걸리는 요청을 보내면
    그러면 408 Request Timeout을 반환해야 한다
```

### AC-API-001-09: 보안 미들웨어
```gherkin
기능: 보안 정보 보호

  시나리오: 인증 토큰 로그 마스킹
    주어진 JWT 토큰이 포함된 요청이 처리될 때
    만약 요청 로그를 확인하면
    그러면 Authorization 헤더 값이 마스킹되어야 한다
    그리고 평문 토큰이 로그에 기록되지 않아야 한다

  시나리오: 내부 오류 시 시스템 정보 은닉
    주어진 핸들러에서 내부 오류가 발생했을 때
    만약 500 에러 응답을 확인하면
    그러면 내부 스택 트레이스가 포함되지 않아야 한다
    그리고 파일 경로나 시스템 정보가 노출되지 않아야 한다
```

---

## 4. Flow Handler (P0)

### AC-API-001-10: 플로우 CRUD
```gherkin
기능: 플로우 생성, 조회, 수정, 삭제

  시나리오: 플로우 목록 조회 (페이지네이션)
    주어진 25개의 플로우가 등록되어 있을 때
    만약 GET /api/v1/flows?page=1&size=10 요청을 보내면
    그러면 200 OK를 반환해야 한다
    그리고 data에 10개의 플로우가 포함되어야 한다
    그리고 meta.pagination.total이 25이어야 한다
    그리고 meta.pagination.total_pages가 3이어야 한다

  시나리오: 플로우 목록 필터링
    주어진 Running 상태 플로우 5개, Stopped 상태 플로우 3개가 있을 때
    만약 GET /api/v1/flows?status=running 요청을 보내면
    그러면 data에 5개의 플로우만 반환되어야 한다

  시나리오: 플로우 상세 조회
    주어진 ID가 "flow-001"인 플로우가 있을 때
    만약 GET /api/v1/flows/flow-001 요청을 보내면
    그러면 200 OK를 반환해야 한다
    그리고 플로우 정의, 상태, 노드 목록이 포함되어야 한다

  시나리오: 존재하지 않는 플로우 조회
    주어진 ID가 "nonexistent"인 플로우가 없을 때
    만약 GET /api/v1/flows/nonexistent 요청을 보내면
    그러면 404 Not Found를 반환해야 한다
    그리고 에러 코드가 "NOT_FOUND"이어야 한다

  시나리오: 플로우 생성 (JSON)
    주어진 유효한 플로우 정의 JSON이 있을 때
    만약 POST /api/v1/flows 요청을 보내면
    그러면 201 Created를 반환해야 한다
    그리고 생성된 플로우 정보(ID 포함)가 반환되어야 한다

  시나리오: 유효하지 않은 플로우 정의로 생성
    주어진 필수 필드가 누락된 플로우 정의가 있을 때
    만약 POST /api/v1/flows 요청을 보내면
    그러면 422 Validation Failed를 반환해야 한다
    그리고 필드별 검증 에러 상세가 포함되어야 한다

  시나리오: 실행 중인 플로우 삭제 시도
    주어진 Running 상태의 플로우 "flow-001"이 있을 때
    만약 DELETE /api/v1/flows/flow-001 요청을 보내면
    그러면 409 Conflict를 반환해야 한다
    그리고 "실행 중인 플로우는 삭제할 수 없습니다" 메시지가 포함되어야 한다
```

### AC-API-001-11: 플로우 실행 제어
```gherkin
기능: 플로우 배포, 시작, 중지, 재시작

  시나리오: 플로우 배포
    주어진 Created 상태의 플로우 "flow-001"이 있을 때
    만약 POST /api/v1/flows/flow-001/deploy 요청을 보내면
    그러면 200 OK를 반환해야 한다
    그리고 플로우 상태가 "Deployed"로 변경되어야 한다

  시나리오: 플로우 시작
    주어진 Deployed 상태의 플로우 "flow-001"이 있을 때
    만약 POST /api/v1/flows/flow-001/start 요청을 보내면
    그러면 200 OK를 반환해야 한다
    그리고 플로우 상태가 "Running"으로 변경되어야 한다

  시나리오: 플로우 중지
    주어진 Running 상태의 플로우 "flow-001"이 있을 때
    만약 POST /api/v1/flows/flow-001/stop 요청을 보내면
    그러면 200 OK를 반환해야 한다
    그리고 플로우가 graceful stop 되어야 한다
    그리고 상태가 "Stopped"로 변경되어야 한다

  시나리오: 플로우 재시작
    주어진 Running 상태의 플로우 "flow-001"이 있을 때
    만약 POST /api/v1/flows/flow-001/restart 요청을 보내면
    그러면 200 OK를 반환해야 한다
    그리고 플로우가 stop 후 start 되어야 한다
    그리고 최종 상태가 "Running"이어야 한다

  시나리오: 플로우 런타임 설정 변경
    주어진 Running 상태의 플로우 "flow-001"이 있을 때
    만약 PUT /api/v1/flows/flow-001/config 요청을 {"backpressure_threshold": 1000} 본문으로 보내면
    그러면 200 OK를 반환해야 한다
    그리고 플로우가 재시작 없이 설정이 반영되어야 한다

  시나리오: 플로우 상태 조회
    주어진 Running 상태의 플로우 "flow-001"이 있을 때
    만약 GET /api/v1/flows/flow-001/status 요청을 보내면
    그러면 200 OK를 반환해야 한다
    그리고 현재 상태, 실행 시간, 노드별 처리 통계가 포함되어야 한다
```

---

## 5. Agent Handler (P0)

### AC-API-001-12: Agent CRUD 및 생명주기 제어
```gherkin
기능: Agent 관리

  시나리오: Agent 목록 조회
    주어진 3개의 Agent가 등록되어 있을 때
    만약 GET /api/v1/agents 요청을 보내면
    그러면 200 OK와 3개의 Agent 목록을 반환해야 한다

  시나리오: Agent 상세 조회 (Info + Stats)
    주어진 Running 상태의 MQTT Agent "agent-001"이 있을 때
    만약 GET /api/v1/agents/agent-001 요청을 보내면
    그러면 200 OK를 반환해야 한다
    그리고 Agent의 Info(타입, 설정, 상태)와 Stats(메시지 수, 연결 상태)가 포함되어야 한다

  시나리오: Agent 생성
    주어진 유효한 MQTT Agent 설정이 있을 때
    만약 POST /api/v1/agents 요청을 보내면
    그러면 201 Created를 반환해야 한다
    그리고 생성된 Agent 정보가 반환되어야 한다

  시나리오: 참조 중인 Agent 삭제 시도
    주어진 Agent "agent-001"이 2개의 플로우에서 참조 중일 때
    만약 DELETE /api/v1/agents/agent-001 요청을 보내면
    그러면 409 Conflict를 반환해야 한다
    그리고 "다른 플로우에서 참조 중인 Agent는 삭제할 수 없습니다" 메시지가 포함되어야 한다

  시나리오: Agent 시작/중지/재시작
    주어진 Stopped 상태의 Agent "agent-001"이 있을 때
    만약 POST /api/v1/agents/agent-001/start 요청을 보내면
    그러면 200 OK를 반환해야 한다
    그리고 Agent 상태가 "Running"으로 변경되어야 한다

  시나리오: Agent 런타임 설정 변경
    주어진 Running 상태의 Agent "agent-001"이 있을 때
    만약 PUT /api/v1/agents/agent-001/config 요청을 {"polling_interval": "5s"} 본문으로 보내면
    그러면 200 OK를 반환해야 한다
    그리고 Agent가 재시작 없이 설정이 반영되어야 한다

  시나리오: Agent 통계 조회
    주어진 Running 상태의 Agent "agent-001"이 있을 때
    만약 GET /api/v1/agents/agent-001/stats 요청을 보내면
    그러면 200 OK를 반환해야 한다
    그리고 수신/송신 메시지 수, 연결 상태, 업타임이 포함되어야 한다
```

---

## 6. Node Handler (P1)

### AC-API-001-13: 노드 카탈로그 및 관리
```gherkin
기능: 노드 타입 및 인스턴스 관리

  시나리오: 노드 타입 목록 조회
    주어진 10개의 내장 노드 타입과 2개의 플러그인 노드 타입이 있을 때
    만약 GET /api/v1/nodes 요청을 보내면
    그러면 200 OK와 12개의 노드 타입 목록을 반환해야 한다

  시나리오: 노드 타입 상세 조회
    주어진 "filter" 노드 타입이 있을 때
    만약 GET /api/v1/nodes/filter 요청을 보내면
    그러면 200 OK를 반환해야 한다
    그리고 입/출력 포트 정보, 설정 스키마, 설명이 포함되어야 한다

  시나리오: 플로우 내 노드 목록 조회
    주어진 플로우 "flow-001"에 5개의 노드가 있을 때
    만약 GET /api/v1/flows/flow-001/nodes 요청을 보내면
    그러면 200 OK와 5개의 노드 인스턴스 목록을 반환해야 한다

  시나리오: 노드 런타임 설정 변경
    주어진 플로우 "flow-001"의 필터 노드 "node-003"이 Running 상태일 때
    만약 PUT /api/v1/flows/flow-001/nodes/node-003/config 요청을 {"condition": "temp > 30"} 본문으로 보내면
    그러면 200 OK를 반환해야 한다
    그리고 노드가 재시작 없이 필터 조건이 변경되어야 한다
```

---

## 7. Monitoring Handler (P1)

### AC-API-001-14: 모니터링 및 관찰성
```gherkin
기능: 시스템 모니터링 및 로그 레벨 제어

  시나리오: Prometheus 메트릭 노출
    주어진 API 서버가 Running 상태일 때
    만약 GET /api/v1/monitor/metrics 요청을 보내면
    그러면 200 OK와 Prometheus text format 메트릭을 반환해야 한다
    그리고 인증 없이 접근 가능해야 한다

  시나리오: 시스템 건강 상태 조회
    주어진 API 서버, DB, Engine이 모두 정상일 때
    만약 GET /api/v1/monitor/health 요청을 보내면
    그러면 200 OK를 반환해야 한다
    그리고 각 구성 요소의 상태가 포함되어야 한다

  시나리오: 시스템 상태 개요 조회
    주어진 3개의 플로우가 Running, 5개의 Agent가 활성일 때
    만약 GET /api/v1/monitor/status 요청을 보내면
    그러면 200 OK를 반환해야 한다
    그리고 실행 중 플로우 수(3), 활성 Agent 수(5), 서버 업타임이 포함되어야 한다

  시나리오: 컴포넌트 로그 레벨 변경
    주어진 Admin 권한의 사용자가 인증된 상태일 때
    만약 PUT /api/v1/observe/level?component=agent.mqtt&level=debug 요청을 보내면
    그러면 200 OK를 반환해야 한다
    그리고 agent.mqtt 컴포넌트의 로그 레벨이 debug로 변경되어야 한다
    그리고 서버 재시작 없이 즉시 반영되어야 한다

  시나리오: SSE 이벤트 스트리밍
    주어진 인증된 사용자가 있을 때
    만약 GET /api/v1/events 요청을 보내면
    그러면 SSE 연결이 수립되어야 한다
    그리고 Content-Type이 "text/event-stream"이어야 한다
    그리고 플로우 상태 변경, Agent 이벤트 등이 실시간으로 전달되어야 한다
```

---

## 8. Plugin Handler (P1)

### AC-API-001-15: 플러그인 관리
```gherkin
기능: 플러그인 설치 및 관리

  시나리오: 플러그인 목록 조회
    주어진 2개의 플러그인이 설치되어 있을 때
    만약 GET /api/v1/plugins 요청을 보내면
    그러면 200 OK와 2개의 플러그인 목록을 반환해야 한다

  시나리오: 사용 중인 플러그인 제거 시도
    주어진 플러그인 "custom-transform"이 제공하는 노드 타입이 플로우에서 사용 중일 때
    만약 DELETE /api/v1/plugins/custom-transform 요청을 보내면
    그러면 409 Conflict를 반환해야 한다
```

---

## 9. DTO & Validation (P0)

### AC-API-001-16: 표준 응답 형식 및 유효성 검증
```gherkin
기능: API 응답 형식 및 입력 검증

  시나리오: 성공 응답 형식
    주어진 API 서버가 정상 동작할 때
    만약 GET /api/v1/flows 요청을 보내면
    그러면 응답이 {success: true, data: [...], meta: {...}} 형식이어야 한다

  시나리오: 에러 응답 형식
    주어진 존재하지 않는 리소스에 접근할 때
    만약 GET /api/v1/flows/nonexistent 요청을 보내면
    그러면 응답이 {success: false, error: {code: "NOT_FOUND", message: "..."}} 형식이어야 한다

  시나리오: 유효성 검증 실패
    주어진 이름 필드가 비어있는 플로우 생성 요청이 있을 때
    만약 POST /api/v1/flows 요청을 보내면
    그러면 422 Validation Failed를 반환해야 한다
    그리고 에러 details에 필드별 검증 에러가 포함되어야 한다

  시나리오: 잘못된 JSON 형식
    주어진 유효하지 않은 JSON 문자열이 요청 본문에 포함될 때
    만약 POST /api/v1/flows 요청을 보내면
    그러면 400 Bad Request를 반환해야 한다
    그리고 파싱 에러 상세가 포함되어야 한다

  시나리오: 페이지네이션 메타 정보
    주어진 25개의 항목이 있고 page=2, size=10으로 조회할 때
    만약 GET /api/v1/flows?page=2&size=10 요청을 보내면
    그러면 meta.pagination에 {page: 2, size: 10, total: 25, total_pages: 3}이 포함되어야 한다
    그리고 data에 10개의 항목이 포함되어야 한다
```

---

## 10. WebSocket Handler (P1)

### AC-API-001-17: WebSocket 실시간 스트리밍
```gherkin
기능: WebSocket 연결 및 실시간 데이터 전송

  시나리오: WebSocket 연결 수립
    주어진 유효한 JWT 토큰이 있을 때
    만약 GET /api/v1/ws 요청으로 WebSocket 업그레이드를 시도하면
    그러면 101 Switching Protocols로 WebSocket 연결이 수립되어야 한다

  시나리오: 인증 없이 WebSocket 연결 시도
    주어진 인증 토큰이 없을 때
    만약 GET /api/v1/ws 요청으로 WebSocket 업그레이드를 시도하면
    그러면 401 Unauthorized를 반환해야 한다

  시나리오: 플로우 상태 실시간 수신
    주어진 WebSocket 연결이 수립되어 있고
    그리고 플로우 상태 채널을 구독한 상태일 때
    만약 플로우 "flow-001"의 상태가 Running에서 Stopped로 변경되면
    그러면 WebSocket을 통해 상태 변경 이벤트가 전달되어야 한다

  시나리오: heartbeat 처리
    주어진 WebSocket 연결이 수립된 상태일 때
    만약 서버가 ping을 전송하면
    그러면 클라이언트의 pong 응답을 확인해야 한다
    그리고 pong 응답이 없으면 연결을 종료해야 한다

  시나리오: 연결 끊김 시 리소스 정리
    주어진 WebSocket 연결이 3개의 채널을 구독 중일 때
    만약 연결이 끊어지면
    그러면 3개의 채널 구독이 모두 해제되어야 한다
    그리고 연결 관련 goroutine이 정리되어야 한다
```

---

## 11. Error Types (P0)

### AC-API-001-18: 에러 매핑 및 응답
```gherkin
기능: 도메인 에러에서 HTTP 에러로 변환

  시나리오: 도메인 NotFound 에러 -> 404
    주어진 FlowManager.GetFlow()이 xferr.ErrNotFound를 반환할 때
    만약 GET /api/v1/flows/:id 요청을 처리하면
    그러면 404 Not Found를 반환해야 한다
    그리고 에러 코드가 "NOT_FOUND"이어야 한다

  시나리오: 도메인 Conflict 에러 -> 409
    주어진 FlowManager.DeleteFlow()이 xferr.ErrConflict를 반환할 때
    만약 DELETE /api/v1/flows/:id 요청을 처리하면
    그러면 409 Conflict를 반환해야 한다

  시나리오: 미매핑 에러 -> 500
    주어진 핸들러에서 예상치 못한 에러가 반환될 때
    만약 해당 에러가 APIError로 매핑되지 않으면
    그러면 500 Internal Server Error를 반환해야 한다
    그리고 에러 코드가 "INTERNAL_ERROR"이어야 한다
```

---

## 12. 성능 기준

### AC-API-001-19: 성능 요구사항
```gherkin
기능: API 성능 목표

  시나리오: CRUD 엔드포인트 응답 시간
    주어진 단일 인스턴스 API 서버가 Running 상태일 때
    만약 100개의 동시 GET /api/v1/flows 요청을 보내면
    그러면 p95 응답 시간이 50ms 미만이어야 한다

  시나리오: 서버 시작 시간
    주어진 유효한 서버 설정이 있을 때
    만약 Server.Start()를 호출하면
    그러면 1초 이내에 요청을 수신할 준비가 되어야 한다

  시나리오: graceful shutdown 시간
    주어진 100개의 요청이 처리 중일 때
    만약 Server.Stop()을 호출하면
    그러면 30초 이내에 모든 요청을 완료하고 종료해야 한다
```

---

## 13. 엣지 케이스

### AC-API-001-20: 엣지 케이스 처리
```gherkin
기능: 경계 조건 처리

  시나리오: 빈 목록 응답
    주어진 등록된 플로우가 없을 때
    만약 GET /api/v1/flows 요청을 보내면
    그러면 200 OK와 빈 배열 {success: true, data: []}을 반환해야 한다
    그리고 meta.pagination.total이 0이어야 한다

  시나리오: 잘못된 페이지네이션 파라미터
    주어진 page=-1 또는 size=0으로 요청할 때
    만약 GET /api/v1/flows?page=-1&size=0 요청을 보내면
    그러면 기본값(page=1, size=20)으로 처리해야 한다

  시나리오: 매우 큰 페이지 크기 요청
    주어진 size=10000으로 요청할 때
    만약 GET /api/v1/flows?size=10000 요청을 보내면
    그러면 최대 허용 크기(100)로 제한하여 처리해야 한다

  시나리오: 동시 플로우 시작 요청
    주어진 Deployed 상태의 플로우 "flow-001"에 대해
    만약 2개의 POST /api/v1/flows/flow-001/start 요청이 동시에 도달하면
    그러면 하나는 성공하고 다른 하나는 409 Conflict를 반환해야 한다
    그리고 플로우 상태가 일관되어야 한다

  시나리오: Content-Type 불일치
    주어진 Content-Type: text/plain 헤더로 JSON 본문을 보낼 때
    만약 POST /api/v1/flows 요청을 보내면
    그러면 415 Unsupported Media Type을 반환해야 한다

  시나리오: 대용량 요청 본문
    주어진 10MB 크기의 플로우 정의 JSON이 있을 때
    만약 POST /api/v1/flows 요청을 보내면
    그러면 413 Payload Too Large를 반환해야 한다
```

---

## 14. Definition of Done

- [ ] 모든 P0 엔드포인트가 구현되어 요청/응답이 동작한다
- [ ] 모든 P0 요구사항(REQ-API-001-XX-XX)에 대한 테스트가 통과한다
- [ ] 테스트 커버리지가 85% 이상이다
- [ ] 표준 응답 엔벨로프가 모든 API 응답에 적용되어 있다
- [ ] 인증/인가 미들웨어가 보호 엔드포인트에 적용되어 있다
- [ ] CORS, 레이트 리밋, 에러 핸들링 미들웨어가 동작한다
- [ ] health/ready 엔드포인트가 서버 상태를 정확히 반영한다
- [ ] graceful shutdown이 진행 중인 요청을 완료한 후 종료한다
- [ ] golangci-lint 경고가 0건이다
- [ ] go test -race 에서 데이터 레이스가 0건이다
- [ ] 에러 응답에 내부 시스템 정보가 노출되지 않는다
- [ ] API 응답 시간(p95)이 50ms 미만이다 (CRUD 엔드포인트)

---

*SPEC ID: SPEC-API-001*
*버전: 1.0.0*
*상태: draft*
*최종 수정: 2026-02-13*
