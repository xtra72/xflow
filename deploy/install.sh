#!/usr/bin/env bash
# xflowd installer (Linux: Debian/Ubuntu, x86-64 / arm64 / Raspberry Pi)
# Usage: sudo ./install.sh
#
# 환경변수 (비대화식/자동화용):
#   XFLOW_TLS=yes|no          TLS 프롬프트를 건너뛰고 강제 지정 (기본: 대화식이면 질문, 아니면 no)
#   XFLOW_TLS_HOSTS="host,ip" 자체 서명 인증서 SAN 목록(쉼표 구분). 미지정 시 hostname/IP 자동 감지.
set -euo pipefail

INSTALL_DIR="/opt/xflow"
CONFIG_DIR="/etc/xflow"
DATA_DIR="${INSTALL_DIR}/data"
CERT_DIR="${INSTALL_DIR}/certs"
SERVICE_USER="xflow"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
TLS_ENABLED=0

# --- Helpers ---
info()  { echo -e "\033[1;34m[INFO]\033[0m  $*"; }
ok()    { echo -e "\033[1;32m[OK]\033[0m    $*"; }
input() { echo -e "\033[1;36m[INPUT]\033[0m $*"; }
error() { echo -e "\033[1;31m[ERROR]\033[0m $*" >&2; exit 1; }

# enable_tls_in_config: server.tls 블록만 정확히 패치(enabled/cert_file/key_file).
# cors.enabled 등 동일 키 오탐을 피하기 위해 "  tls:" 진입~다음 2-space 키까지로 범위를 제한한다.
enable_tls_in_config() {
    local cfg="$1" cert="$2" key="$3" tmp
    tmp="$(mktemp)"
    awk -v cert="$cert" -v key="$key" '
        /^  tls:[[:space:]]*$/ { intls=1; print; next }
        intls==1 {
            if ($0 ~ /^  [a-z_]+:/ || $0 ~ /^[a-z_]/) { intls=0; print; next }
            if ($0 ~ /^    enabled:/)   { print "    enabled: true"; next }
            if ($0 ~ /^    cert_file:/) { print "    cert_file: \"" cert "\""; next }
            if ($0 ~ /^    key_file:/)  { print "    key_file: \"" key "\""; next }
            print; next
        }
        { print }
    ' "$cfg" > "$tmp" && cat "$tmp" > "$cfg"
    rm -f "$tmp"
    grep -A1 '^  tls:' "$cfg" | grep -q 'enabled: true' \
        || info "주의: config 의 tls 블록 구조가 예상과 달라 자동 활성화되지 않았습니다. ${cfg} 에서 server.tls 를 수동 설정하세요."
}

# setup_tls: (선택) TLS 사용 여부를 묻고, 승인 시 자체 서명 인증서를 생성 + config 활성화.
setup_tls() {
    local cert_file="${CERT_DIR}/cert.pem" key_file="${CERT_DIR}/key.pem"
    local want_tls="${XFLOW_TLS:-}"

    if [ -z "$want_tls" ]; then
        if [ -t 0 ]; then
            input "HTTPS(TLS)를 사용하시겠습니까? 자체 서명 인증서를 생성합니다. [y/N]: "
            read -r ans || ans=""
            case "${ans,,}" in y|yes) want_tls=yes ;; *) want_tls=no ;; esac
        else
            want_tls=no
            info "비대화식 설치: TLS 비활성(HTTP). 활성화하려면 XFLOW_TLS=yes 로 재실행."
        fi
    fi

    if [ "$want_tls" != "yes" ]; then
        info "TLS 비활성(HTTP). 이후 ${CONFIG_DIR}/xflow.yaml 의 server.tls 로 수동 활성화 가능."
        return 0
    fi

    command -v openssl >/dev/null 2>&1 || error "openssl 미설치 — 자체 서명 인증서 생성에 필요합니다."

    # 기존 인증서 처리
    if [ -f "$cert_file" ] && [ -f "$key_file" ]; then
        local regen=no
        if [ -t 0 ]; then
            input "기존 인증서가 있습니다(${cert_file}). 재생성할까요? [y/N]: "
            read -r rg || rg=""
            case "${rg,,}" in y|yes) regen=yes ;; esac
        fi
        if [ "$regen" != "yes" ]; then
            info "기존 인증서 유지"
            enable_tls_in_config "${CONFIG_DIR}/xflow.yaml" "$cert_file" "$key_file"
            TLS_ENABLED=1
            return 0
        fi
    fi

    # SAN 기본값 자동 감지 (FQDN + 모든 IP + localhost)
    local fqdn ips san_default san
    fqdn="$(hostname -f 2>/dev/null || hostname 2>/dev/null || echo xflow)"
    ips="$(hostname -I 2>/dev/null || true)"
    san_default="${fqdn},localhost,127.0.0.1"
    for ip in $ips; do san_default="${san_default},${ip}"; done

    san="${XFLOW_TLS_HOSTS:-}"
    if [ -z "$san" ]; then
        if [ -t 0 ]; then
            input "인증서 SAN(호스트/IP, 쉼표 구분) [${san_default}]: "
            read -r san || san=""
            san="${san:-$san_default}"
        else
            san="$san_default"
        fi
    fi

    # openssl SAN config 생성 (IP/DNS 자동 분류)
    mkdir -p "$CERT_DIR"
    local cnf; cnf="$(mktemp)"
    {
        printf '[req]\ndistinguished_name = dn\nx509_extensions = v3\nprompt = no\n'
        printf '[dn]\nCN = %s\nO = xflow\n' "$fqdn"
        printf '[v3]\nbasicConstraints = critical, CA:FALSE\n'
        printf 'keyUsage = critical, digitalSignature, keyEncipherment\n'
        printf 'extendedKeyUsage = serverAuth\nsubjectAltName = @alt\n'
        printf '[alt]\n'
        local dns_i=1 ip_i=1 entry
        IFS=',' read -ra _entries <<< "$san"
        for entry in "${_entries[@]}"; do
            entry="$(echo "$entry" | xargs)"
            [ -z "$entry" ] && continue
            if [[ "$entry" =~ ^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$ ]] || [[ "$entry" == *:* ]]; then
                printf 'IP.%d = %s\n' "$ip_i" "$entry"; ip_i=$((ip_i+1))
            else
                printf 'DNS.%d = %s\n' "$dns_i" "$entry"; dns_i=$((dns_i+1))
            fi
        done
    } > "$cnf"

    info "자체 서명 인증서 생성 중 (SAN: ${san})"
    openssl req -x509 -newkey rsa:2048 -sha256 -days 825 -nodes \
        -keyout "$key_file" -out "$cert_file" -config "$cnf" 2>/dev/null \
        || { rm -f "$cnf"; error "인증서 생성 실패"; }
    rm -f "$cnf"
    chmod 644 "$cert_file"; chmod 600 "$key_file"
    ok "인증서 생성: ${cert_file}"

    enable_tls_in_config "${CONFIG_DIR}/xflow.yaml" "$cert_file" "$key_file"
    TLS_ENABLED=1
    ok "TLS 활성화: ${CONFIG_DIR}/xflow.yaml"
}

# --- Pre-checks ---
[ "$(id -u)" -eq 0 ] || error "Root privileges required. Run with: sudo $0"
[ -f "${SCRIPT_DIR}/xflowd" ] || error "xflowd binary not found in ${SCRIPT_DIR}"

# --- Create system user ---
if ! id "${SERVICE_USER}" &>/dev/null; then
    info "Creating system user: ${SERVICE_USER}"
    useradd --system --no-create-home --shell /usr/sbin/nologin "${SERVICE_USER}"
    ok "User created"
else
    info "User ${SERVICE_USER} already exists"
fi

# --- Create directories ---
info "Creating directories"
mkdir -p "${INSTALL_DIR}" "${CONFIG_DIR}" "${DATA_DIR}" "${INSTALL_DIR}/plugins"

# --- Install binaries ---
info "Installing xflowd binary"
cp "${SCRIPT_DIR}/xflowd" "${INSTALL_DIR}/xflowd"
chmod 755 "${INSTALL_DIR}/xflowd"
ok "Binary installed: ${INSTALL_DIR}/xflowd"

# CLI tools -> /usr/local/bin (PATH accessible)
for cli in xflow xflow-agent; do
    if [ -f "${SCRIPT_DIR}/${cli}" ]; then
        info "Installing CLI: ${cli}"
        cp "${SCRIPT_DIR}/${cli}" /usr/local/bin/${cli}
        chmod 755 /usr/local/bin/${cli}
        ok "CLI installed: /usr/local/bin/${cli}"
    fi
done

# --- Install web UI ---
if [ -d "${SCRIPT_DIR}/web/dist" ]; then
    info "Installing web UI"
    rm -rf "${INSTALL_DIR}/web/dist"
    mkdir -p "${INSTALL_DIR}/web"
    cp -r "${SCRIPT_DIR}/web/dist" "${INSTALL_DIR}/web/dist"
    ok "Web UI installed: ${INSTALL_DIR}/web/dist"
else
    info "No web/dist found, skipping web UI"
fi

# --- Install config (preserve existing) ---
if [ ! -f "${CONFIG_DIR}/xflow.yaml" ]; then
    info "Installing default configuration"
    cp "${SCRIPT_DIR}/xflow.yaml" "${CONFIG_DIR}/xflow.yaml"
    ok "Config installed: ${CONFIG_DIR}/xflow.yaml"
else
    info "Config already exists, skipping (compare with ${SCRIPT_DIR}/xflow.yaml)"
fi

# --- TLS setup (optional, interactive) ---
setup_tls

# --- Environment file ---
if [ ! -f "${CONFIG_DIR}/env" ]; then
    info "Creating environment file"
    JWT_SECRET=$(openssl rand -base64 32 2>/dev/null || head -c 32 /dev/urandom | base64)
    cat > "${CONFIG_DIR}/env" <<ENVEOF
# xflowd environment variables
# Viper prefix: XFLOW_ + key (dot -> underscore)
XFLOW_AUTH_JWT_SECRET=${JWT_SECRET}
ENVEOF
    chmod 600 "${CONFIG_DIR}/env"
    ok "Env file created: ${CONFIG_DIR}/env"
fi

# --- Install systemd service ---
info "Installing systemd service"
cp "${SCRIPT_DIR}/xflowd.service" /etc/systemd/system/xflowd.service
systemctl daemon-reload
ok "Service installed"

# --- Set ownership ---
info "Setting permissions"
chown -R "${SERVICE_USER}:${SERVICE_USER}" "${INSTALL_DIR}"
chown -R "${SERVICE_USER}:${SERVICE_USER}" "${CONFIG_DIR}"
ok "Permissions set"

# --- Done ---
echo ""
ok "xflowd installation complete!"
echo ""
echo "  Next steps:"
echo "    1. Edit config:    sudo nano ${CONFIG_DIR}/xflow.yaml"
echo "    2. Set JWT secret: sudo nano ${CONFIG_DIR}/env"
echo "    3. Enable service: sudo systemctl enable xflowd"
echo "    4. Start service:  sudo systemctl start xflowd"
echo "    5. Check status:   sudo systemctl status xflowd"
echo "    6. View logs:      sudo journalctl -u xflowd -f"
echo ""
if [ "${TLS_ENABLED}" -eq 1 ]; then
    echo "  Web UI: https://$(hostname -I | awk '{print $1}'):8081"
    echo "  (자체 서명 인증서 — 브라우저 경고는 정상입니다. 신뢰하려면 인증서를 클라이언트에 등록하세요.)"
    echo "  공개 도메인이라면 Let's Encrypt 등 CA 서명 인증서 사용을 권장합니다."
else
    echo "  Web UI: http://$(hostname -I | awk '{print $1}'):8081"
fi
echo ""
