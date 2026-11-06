#!/bin/sh
# Kite agent installer for Linux. The hub serves this script at /install.sh:
#
#   curl -fsSL <hub>/install.sh | sudo sh -s -- --hub <hub> --token <token> [--name <name>]
#   curl -fsSL <hub>/install.sh | sudo sh -s -- --uninstall
#
# Running it again upgrades the agent. Test hooks: KITE_PREFIX (install
# under another root), KITE_SYSTEMCTL, KITE_ARCH, KITE_SKIP_ROOT_CHECK=1.
set -eu

PREFIX=${KITE_PREFIX:-}
SYSTEMCTL=${KITE_SYSTEMCTL:-systemctl}
BIN=$PREFIX/usr/local/bin/kite-agent
ENV_DIR=$PREFIX/etc/kite
UNIT=$PREFIX/etc/systemd/system/kite-agent.service

die() {
	echo "kite install: $*" >&2
	exit 1
}

usage() {
	echo "usage: install.sh --hub <url> --token <token> [--name <name>]"
	echo "       install.sh --uninstall"
}

hub=
token=
name=
uninstall=
while [ $# -gt 0 ]; do
	case $1 in
	--hub)
		[ $# -ge 2 ] || die "--hub needs a value"
		hub=$2
		shift 2
		;;
	--token)
		[ $# -ge 2 ] || die "--token needs a value"
		token=$2
		shift 2
		;;
	--name)
		[ $# -ge 2 ] || die "--name needs a value"
		name=$2
		shift 2
		;;
	--uninstall)
		uninstall=1
		shift
		;;
	-h | --help)
		usage
		exit 0
		;;
	*) die "unknown option: $1" ;;
	esac
done

# Every check runs before anything is written, so a refusal leaves no trace.
if [ "${KITE_SKIP_ROOT_CHECK:-}" != 1 ] && [ "$(id -u)" != 0 ]; then
	die "run as root (for example with sudo)"
fi
command -v "$SYSTEMCTL" >/dev/null 2>&1 || die "systemd is required (systemctl not found)"

if [ -n "$uninstall" ]; then
	"$SYSTEMCTL" disable --now kite-agent >/dev/null 2>&1 || true
	rm -f "$UNIT" "$BIN"
	rm -rf "$ENV_DIR"
	"$SYSTEMCTL" daemon-reload
	echo "Kite agent removed."
	exit 0
fi

[ -n "$hub" ] || die "--hub is required"
[ -n "$token" ] || die "--token is required"
hub=${hub%/}

machine=${KITE_ARCH:-$(uname -m)}
case $machine in
x86_64 | amd64) arch=amd64 ;;
aarch64 | arm64) arch=arm64 ;;
*) die "unsupported CPU: $machine" ;;
esac

url="$hub/download/kite-agent-linux-$arch"
tmp=$(mktemp)
trap 'rm -f "$tmp"' EXIT
if command -v curl >/dev/null 2>&1; then
	curl -fsSL -o "$tmp" "$url" || die "download failed: $url"
elif command -v wget >/dev/null 2>&1; then
	wget -qO "$tmp" "$url" || die "download failed: $url"
else
	die "curl or wget is required"
fi

mkdir -p "$(dirname "$BIN")" "$ENV_DIR" "$(dirname "$UNIT")"
# Copy next to the binary, then rename: replacing a running agent is safe.
cp "$tmp" "$BIN.new"
chmod 755 "$BIN.new"
mv -f "$BIN.new" "$BIN"

(
	umask 077
	{
		echo "HUB_URL=$hub"
		echo "AGENT_TOKEN=$token"
		if [ -n "$name" ]; then
			echo "AGENT_NAME=$name"
		fi
	} >"$ENV_DIR/agent.env"
)
chmod 600 "$ENV_DIR/agent.env"

cat >"$UNIT" <<'UNIT'
[Unit]
Description=Kite monitoring agent
After=network-online.target docker.service
Wants=network-online.target

[Service]
EnvironmentFile=/etc/kite/agent.env
ExecStart=/usr/local/bin/kite-agent
Restart=always
RestartSec=10
Nice=10
IOSchedulingClass=idle

[Install]
WantedBy=multi-user.target
UNIT

"$SYSTEMCTL" daemon-reload
"$SYSTEMCTL" enable kite-agent
"$SYSTEMCTL" restart kite-agent
echo "Kite agent installed; it should appear on the dashboard within a few seconds."
