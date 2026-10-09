# Run the TUI over SSH

[Back to README](../README.md) · [CLI reference](cli.md)

SSH mode serves public catalog browsing. It accepts public keys from an
`authorized_keys` file and requires a terminal. Any SSH username works; password
login is not the access mechanism. Library/Libby/Hardcover account screens are
not available over SSH. All sessions use one server-selected theme and share the
catalog client, request throttle, and cache.

## Run manually

Build the binary and prepare an authorized-keys file containing the public keys
of users you want to admit. Run from the repository root, for example:

```sh
go build -o shelfcheck .
./shelfcheck serve -addr 127.0.0.1:2222 -authorized-keys /path/to/authorized_keys
```

The default address is `:2222` (all interfaces). Select an address appropriate for
the intended network. The server refuses to start without at least one valid
public key. The default authorization file is `~/.ssh/authorized_keys`; the
default server host key is `.ssh/shelfcheck_ed25519` relative to the working
directory, created on first run. Preserve that key to retain the server identity.

Connect from another terminal:

```sh
ssh -p 2222 localhost
ssh -p 2222 -t localhost dune
```

Use `-t` when passing a search command so SSH allocates a terminal. For a remote
server, replace `localhost` with its hostname or IP. Verify the SSH host key when
connecting. Ctrl+C or SIGTERM on the server triggers graceful shutdown. Idle
sessions disconnect after an hour by default.

The server accepts `-library`, `-home`, `-anywhere`, and `-theme`, plus `-addr`,
`-host-key`, `-authorized-keys`, and `-idle-timeout` (Go durations such as `30m`).
It does not accept the local TUI's `-covers` flag or per-client theme changes.

## Linux systemd installation over Tailscale

The supplied [installer](../deploy/install.sh) expects Linux, systemd, a running
Tailscale installation with an IPv4 address, and root privileges. Build on the
target host or provide a binary compatible with its OS and architecture.

Prepare these three files together in `deploy/`:

```sh
go build -o deploy/shelfcheck .
cp /path/to/approved_public_keys deploy/authorized_keys
sudo bash deploy/install.sh
```

`deploy/shelfcheck.service` is already supplied. Keep private keys out of
`authorized_keys`. The installer copies the binary to `/usr/local/bin/shelfcheck`,
public keys and an address file to `/etc/shelfcheck`, and the service unit to
`/etc/systemd/system/shelfcheck.service`. It enables and restarts the service.
If UFW is active, it adds an allow rule on `tailscale0`. The listen address is the
machine's current Tailscale IPv4 address with port `2222`.

To choose another port:

```sh
sudo env SHELFCHECK_PORT=2223 bash deploy/install.sh
```

The service runs as a dynamic user with a persistent state directory. Its host
key lives at `/var/lib/shelfcheck/host_ed25519`; its authorized keys are in
`/etc/shelfcheck/authorized_keys`. The default unit uses the default Cincinnati
catalog and home branch. To change catalog options, use a systemd override for
`ExecStart`, retaining the supplied address and key paths.

```sh
systemctl status shelfcheck.service
journalctl -u shelfcheck.service -n 50 --no-pager
ssh -p 2222 -t YOUR_TAILSCALE_IP dune
```

For an update, rebuild `deploy/shelfcheck` and rerun the installer; it restarts
the service. The generated binary and approved keys in `deploy/` are local
deployment inputs, not source documentation: do not commit them. If the Tailscale
IP changes, rerun the installer to regenerate `/etc/shelfcheck/env`.
