#!/usr/bin/env bash
# xflowd installer for Raspberry Pi (Debian/Ubuntu)
# Usage: sudo ./install.sh
set -euo pipefail

INSTALL_DIR="/opt/xflow"
CONFIG_DIR="/etc/xflow"
DATA_DIR="${INSTALL_DIR}/data"
SERVICE_USER="xflow"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

# --- Helpers ---
info()  { echo -e "\033[1;34m[INFO]\033[0m  $*"; }
ok()    { echo -e "\033[1;32m[OK]\033[0m    $*"; }
error() { echo -e "\033[1;31m[ERROR]\033[0m $*" >&2; exit 1; }

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
echo "  Web UI: http://$(hostname -I | awk '{print $1}'):8081"
echo ""
