#!/usr/bin/env bash
# Installs or updates shelfcheck as a systemd service listening only on this
# machine's Tailscale address. Run with sudo from the directory holding the
# shelfcheck binary, shelfcheck.service and authorized_keys:
#
#   sudo ./install.sh
set -euo pipefail
cd "$(dirname "$0")"

[[ $EUID -eq 0 ]] || { echo "run with sudo" >&2; exit 1; }
for f in shelfcheck shelfcheck.service authorized_keys; do
	[[ -s $f ]] || { echo "missing $f" >&2; exit 1; }
done

ts_ip=$(tailscale ip -4 | head -1)
[[ -n $ts_ip ]] || { echo "no Tailscale IPv4 address; is tailscale up?" >&2; exit 1; }
port=${SHELFCHECK_PORT:-2222}

install -m 0755 shelfcheck /usr/local/bin/shelfcheck
install -d -m 0755 /etc/shelfcheck
install -m 0644 authorized_keys /etc/shelfcheck/authorized_keys
printf 'SHELFCHECK_ADDR=%s:%s\n' "$ts_ip" "$port" > /etc/shelfcheck/env
install -m 0644 shelfcheck.service /etc/systemd/system/shelfcheck.service

if command -v ufw >/dev/null && ufw status | grep -q '^Status: active'; then
	ufw allow in on tailscale0 to any port "$port" proto tcp comment 'shelfcheck via Tailscale'
fi

systemctl daemon-reload
systemctl enable shelfcheck.service
systemctl restart shelfcheck.service
sleep 2
systemctl --no-pager --lines=5 status shelfcheck.service
echo
echo "shelfcheck is listening on $ts_ip:$port (Tailscale only)."
