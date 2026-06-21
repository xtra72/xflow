# syntax=docker/dockerfile:1.7
# xflow 멀티스테이지 Dockerfile
# - Stage 1 (web-builder): Node.js 20-alpine 으로 web/dist 빌드
# - Stage 2 (go-builder): golang:1.25-alpine 으로 3개 Go 바이너리 빌드 (CGO_ENABLED=0)
# - Stage 3 (runtime): distroless static-debian12:nonroot 로 최소 런타임 구성
#
# 빌드: docker build --build-arg VERSION=v0.1.0 -t xflow:v0.1.0 .
# 실행: docker run --rm -p 8081:8081 xflow:v0.1.0

# ============================================================================
# Stage 1: 프론트엔드 빌더 (web/dist 생성)
# ============================================================================
FROM node:20-alpine AS web-builder

WORKDIR /web

# 의존성 캐시 최적화: package*.json 만 먼저 복사하여 npm ci 캐싱
COPY web/package.json web/package-lock.json ./
RUN npm ci --no-audit --no-fund

# 소스 복사 후 빌드
COPY web/ ./
RUN npm run build

# ============================================================================
# Stage 2: Go 바이너리 빌더 (xflowd, xflow, xflow-agent)
# ============================================================================
FROM golang:1.25-alpine AS go-builder

# 빌드 시점에 주입되는 버전 정보 (CI/Makefile/직접 빌드 모두 동일 인터페이스)
ARG VERSION=dev

WORKDIR /src

# 모듈 캐시 최적화: go.mod, go.sum 만 먼저 복사
COPY go.mod go.sum ./
RUN go mod download

# 전체 소스 복사 후 3개 바이너리 정적 빌드
COPY . .

# CGO_ENABLED=0: 정적 바이너리 (distroless static 호환)
# -trimpath: 재현 가능 빌드 (절대 경로 제거)
# -s -w: 디버그 심볼/심볼 테이블 제거 (바이너리 크기 축소)
# -X main.Version=...: 버전 문자열 주입
RUN CGO_ENABLED=0 go build \
    -trimpath \
    -ldflags "-s -w -X main.Version=${VERSION}" \
    -o /out/xflowd ./cmd/xflowd \
 && CGO_ENABLED=0 go build \
    -trimpath \
    -ldflags "-s -w -X main.Version=${VERSION}" \
    -o /out/xflow ./cmd/xflow \
 && CGO_ENABLED=0 go build \
    -trimpath \
    -ldflags "-s -w -X main.Version=${VERSION}" \
    -o /out/xflow-agent ./cmd/xflow-agent

# ============================================================================
# Stage 3: 런타임 (distroless, nonroot)
# ============================================================================
FROM gcr.io/distroless/static-debian12:nonroot

# 메타데이터 라벨 (OCI 표준)
LABEL org.opencontainers.image.source="https://github.com/xtra72/xflow"
LABEL org.opencontainers.image.description="xflow IoT Flow Engine"
LABEL org.opencontainers.image.licenses="Apache-2.0"

WORKDIR /app

# Go 바이너리 복사 (PATH 등록을 위해 /usr/local/bin 사용)
COPY --from=go-builder /out/xflowd /usr/local/bin/xflowd
COPY --from=go-builder /out/xflow /usr/local/bin/xflow
COPY --from=go-builder /out/xflow-agent /usr/local/bin/xflow-agent

# 프론트엔드 정적 자산 복사 (xflow.yaml 의 web_ui.dir 와 일치)
COPY --from=web-builder /web/dist /app/web/dist

# 기본 설정 파일 (deploy/xflow.yaml 의 web_ui.dir 를 컨테이너 경로에 맞게 사용)
# 운영 시에는 -v 로 호스트 설정 마운트 권장: -v /etc/xflow/xflow.yaml:/app/xflow.yaml:ro
COPY deploy/xflow.yaml /app/xflow.yaml

# xflowd HTTP 서버 기본 포트 (deploy/xflow.yaml server.port 와 동일)
EXPOSE 8081

# distroless:nonroot 사용자 (UID 65532) 로 실행
# nonroot 사용자는 distroless 이미지에 기본 포함됨

# 기본 명령: xflowd 데몬 시작
ENTRYPOINT ["/usr/local/bin/xflowd"]
CMD ["--config", "/app/xflow.yaml"]
