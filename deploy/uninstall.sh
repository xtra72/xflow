#!/usr/bin/env bash
# xflowd uninstaller for Raspberry Pi
# Usage: sudo ./uninstall.sh
set -euo pipefail

INSTALL_DIR="/opt/xflow"
CONFIG_DIR="/etc/xflow"
SERVICE_USER="xflow"

info()  { echo -e "\033[1;34m[INFO]\033[0m  $*"; }
ok()    { echo -e "\033[1;32m[OK]\033[0m    $*"; }
error() { echo -e "\033[1;31m[ERROR]\033[0m $*" >&2; exit 1; }

[ "$(id -u)" -eq 0 ] || error "Root privileges required. Run with: sudo $0"

# --- Stop and disable service ---
if systemctl is-active --quiet xflowd 2>/dev/null; then
    info "Stopping xflowd service"
    systemctl stop xflowd
fi
if systemctl is-enabled --quiet xflowd 2>/dev/null; then
    info "Disabling xflowd service"
    systemctl disable xflowd
fi
if [ -f /etc/systemd/system/xflowd.service ]; then
    info "Removing systemd service"
    rm -f /etc/systemd/system/xflowd.service
    systemctl daemon-reload
fi
ok "Service removed"

# --- Remove files ---
info "Removing installation directory: ${INSTALL_DIR}"
rm -rf "${INSTALL_DIR}"

# CLI tools
for cli in xflow xflow-agent; do
    if [ -f "/usr/local/bin/${cli}" ]; then
        info "Removing CLI: ${cli}"
        rm -f "/usr/local/bin/${cli}"
    fi
done
ok "Binaries removed"

# --- Config removal (ask) ---
if [ -d "${CONFIG_DIR}" ]; then
    read -r -p "Remove configuration (${CONFIG_DIR})? [y/N] " confirm
    if [[ "${confirm}" =~ ^[Yy]$ ]]; then
        rm -rf "${CONFIG_DIR}"
        ok "Configuration removed"
    else
        info "Configuration preserved at ${CONFIG_DIR}"
    fi
fi

# --- Remove user ---
if id "${SERVICE_USER}" &>/dev/null; then
    read -r -p "Remove system user (${SERVICE_USER})? [y/N] " confirm
    if [[ "${confirm}" =~ ^[Yy]$ ]]; then
        userdel "${SERVICE_USER}"
        ok "User removed"
    else
        info "User preserved"
    fi
fi

echo ""
ok "xflowd uninstalled"
