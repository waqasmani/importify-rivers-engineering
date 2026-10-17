# Deployment

REA Importify runs entirely on your own server: PostgreSQL, the Go API (with an optional separate
worker) and Caddy serving the dashboard. There is no vendor account, licence server, billing or
telemetry. These instructions were written for Ubuntu 24.04 LTS and Debian 12; other Linux
distributions work with their own Docker packages.

## Requirements

- 2 CPU cores, 2 GB RAM, 10 GB disk (more for large repository analyses and attachments).
- Docker Engine 24+ with the Compose plugin.
- For a public HTTPS address: a DNS name pointing at the server and ports 80 and 443 reachable.

## 1. Install Docker (Ubuntu)

```bash
sudo apt-get update
sudo apt-get install -y ca-certificates curl git
sudo install -m 0755 -d /etc/apt/keyrings
sudo curl -fsSL https://download.docker.com/linux/ubuntu/gpg -o /etc/apt/keyrings/docker.asc
sudo chmod a+r /etc/apt/keyrings/docker.asc
echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.asc] https://download.docker.com/linux/ubuntu $(. /etc/os-release && echo "$VERSION_CODENAME") stable" \
  | sudo tee /etc/apt/sources.list.d/docker.list > /dev/null
sudo apt-get update
sudo apt-get install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
sudo usermod -aG docker "$USER"
newgrp docker
docker compose version
```

On Debian replace both `ubuntu` occurrences in the URLs with `debian`.

## 2. Get REA and create the environment file

```bash
sudo mkdir -p /opt/rea && sudo chown "$USER": /opt/rea
git clone https://github.com/waqasmani/importify-rivers-engineering.git /opt/rea
cd /opt/rea

# Local trial on this machine (plain HTTP on http://localhost):
deploy/scripts/init-env.sh
# …or a public server with automatic HTTPS:
# deploy/scripts/init-env.sh rea.example.com
```

`init-env.sh` writes `deploy/.env` (mode 600) with a random database password, encryption key and
one-time setup token, and prints the token. Review the other options in that file; they are
documented in [configuration.md](configuration.md).

## 3. Start the stack

```bash
docker compose -f deploy/docker-compose.yml up -d --build --wait
docker compose -f deploy/docker-compose.yml ps
```

The first build downloads base images, Go modules and npm packages, then compiles everything
(about 3–6 minutes). `--wait` returns when the database and API report healthy.

For a public server, open the firewall:

```bash
sudo ufw allow 80/tcp && sudo ufw allow 443/tcp
```

## 4. First-run setup

Open the address from `REA_PUBLIC_URL` (for example `http://localhost` or `https://rea.example.com`).
The setup page asks for the setup token:

```bash
grep '^REA_SETUP_TOKEN=' deploy/.env | cut -d= -f2
```

Enter the organization name, the first workspace and the owner account (password of at least 12
characters). The token stops working once setup has completed.

Then, in the dashboard:

1. **Settings → Users** to invite colleagues, **Roles** to adjust permissions.
2. **Settings → Browser extension** to pair Chrome (see [extension.md](extension.md)).
3. Optionally **Settings → AI providers** to connect a local Ollama or another provider. Every
   feature works without AI.

## HTTPS options

| Situation | `REA_SITE_ADDRESS` | `REA_PUBLIC_URL` |
| --- | --- | --- |
| Public domain, automatic certificates | `rea.example.com` | `https://rea.example.com` |
| This machine only, no TLS | `http://localhost` | `http://localhost` |
| This machine, local TLS (trust Caddy's root CA) | `https://localhost` | `https://localhost` |
| Behind an existing reverse proxy that terminates TLS | `:80` | `https://rea.example.com` |

For the last case, also set `REA_BIND_ADDRESS=127.0.0.1` and a free `REA_HTTP_PORT`, and see
`deploy/proxy/nginx.conf.example`. To trust Caddy's local CA:

```bash
docker compose -f deploy/docker-compose.yml cp web:/data/caddy/pki/authorities/local/root.crt ./caddy-root.crt
sudo cp caddy-root.crt /usr/local/share/ca-certificates/caddy-local.crt && sudo update-ca-certificates
```

The browser extension accepts `https://` servers and, for development only, `http://localhost`.

## Optional separate worker

Analysis runs and AI jobs execute inside the API by default. To run them in their own container:

```bash
sed -i 's/^REA_WORKER_MODE=.*/REA_WORKER_MODE=off/' deploy/.env
docker compose -f deploy/docker-compose.yml --profile worker up -d
```

## Backups

`deploy/scripts/backup.sh` writes `deploy/backups/rea-<UTC timestamp>/` containing a PostgreSQL
custom-format dump, an archive of the data directory (attachments, snapshots, uploaded sources),
a manifest and SHA-256 checksums. It keeps the newest `REA_BACKUP_KEEP` backups.

```bash
deploy/scripts/backup.sh
deploy/scripts/verify-backup.sh deploy/backups/rea-20261017-021500
```

`verify-backup.sh` restores the dump into a temporary database, checks the schema version and core
tables, lists the data archive and drops the temporary database. The live data is untouched.

Schedule nightly backups and copy them off the server:

```bash
( crontab -l 2>/dev/null; echo '15 2 * * * cd /opt/rea && deploy/scripts/backup.sh >> /var/log/rea-backup.log 2>&1' ) | crontab -
rsync -a /opt/rea/deploy/backups/ backup-host:/srv/backups/rea/
```

`deploy/.env` is not part of the backup. Store `REA_SECRET_KEY` somewhere safe; without it,
encrypted AI provider keys in a restored database cannot be decrypted (re-enter them).

Administrators can also create and download database-only backups under **Settings → System**,
and export all records as JSON Lines (passwords, sessions and secrets excluded).

## Restore

```bash
deploy/scripts/verify-backup.sh deploy/backups/rea-20261017-021500
deploy/scripts/restore.sh deploy/backups/rea-20261017-021500
```

`restore.sh` checks the checksums, asks you to type `RESTORE`, stops the application, recreates
the database, restores the dump and the data directory, and starts the stack again. The API
applies any newer migrations at start-up, so a backup from an older version can be restored into
a newer one. `deploy/scripts/smoke-test.sh` runs the full backup → verify → restore cycle on a
disposable stack and is what CI uses.

## Upgrades

```bash
cd /opt/rea
deploy/scripts/backup.sh
git pull
docker compose -f deploy/docker-compose.yml up -d --build --wait
curl -fsS http://localhost/readyz
```

Migrations run automatically. They are forward-only in production; every migration also has a
tested rollback (`rea-api migrate down`) for emergencies.

## Operator commands

```bash
# Reset a password (reads the new password from stdin and signs the user out everywhere)
printf '%s' 'a new long passphrase' | docker compose -f deploy/docker-compose.yml exec -T api rea-api reset-password owner@example.com
# Migration status
docker compose -f deploy/docker-compose.yml exec api rea-api migrate status
# Logs
docker compose -f deploy/docker-compose.yml logs -f api
```

## Without Docker

1. Install PostgreSQL 18 from the PostgreSQL project repository:

   ```bash
   sudo apt-get install -y postgresql-common
   sudo /usr/share/postgresql-common/pgdg/apt.postgresql.org.sh -y
   sudo apt-get install -y postgresql-18
   sudo -u postgres psql -c "CREATE ROLE rea LOGIN PASSWORD 'change-me'" -c "CREATE DATABASE rea OWNER rea"
   ```

2. Build (needs Go 1.27, Node.js 22 and pnpm 10): `make deps build-go build-web`.
3. Install the files and configuration:

   ```bash
   sudo useradd --system --home /var/lib/rea --shell /usr/sbin/nologin rea
   sudo install -d -o rea -g rea -m 750 /var/lib/rea /opt/rea/bin /etc/rea
   sudo install -m 755 bin/rea-api bin/rea-worker /opt/rea/bin/
   sudo cp -r apps/web/dist /opt/rea/web
   sudo tee /etc/rea/rea.env > /dev/null <<EOF2
   REA_DATABASE_URL=postgres://rea:change-me@127.0.0.1:5432/rea?sslmode=disable
   REA_SECRET_KEY=$(openssl rand -base64 32)
   REA_PUBLIC_URL=https://rea.example.com
   REA_HTTP_ADDR=127.0.0.1:8080
   REA_DATA_DIR=/var/lib/rea
   REA_WEB_DIR=/opt/rea/web
   REA_TRUSTED_PROXIES=127.0.0.1
   EOF2
   sudo chmod 640 /etc/rea/rea.env && sudo chgrp rea /etc/rea/rea.env
   sudo cp deploy/systemd/rea-api.service /etc/systemd/system/
   sudo systemctl daemon-reload && sudo systemctl enable --now rea-api
   sudo cat /var/lib/rea/setup-token
   ```

4. Put Caddy or nginx with TLS in front of `127.0.0.1:8080`.

## Offline and air-gapped operation

Running REA needs no internet access. Only these optional features reach out, and only when you
enable them: Let's Encrypt certificates for a public domain, external AI providers you configure,
and server-side fetching of public URLs (`REA_SERVER_FETCH_ENABLED`). A local Ollama keeps AI on
your network.

Building the images needs Docker Hub, the Go module proxy and the npm registry. For an air-gapped
host, build on a connected machine and move the images:

```bash
docker compose -f deploy/docker-compose.yml build
docker save rea-importify/api:1.0.0 rea-importify/web:1.0.0 postgres:18.6-alpine3.24 | gzip > rea-images.tar.gz
# on the offline host, with the repository checked out:
gunzip -c rea-images.tar.gz | docker load
docker compose -f deploy/docker-compose.yml up -d --no-build
```

Behind a TLS-intercepting proxy, pass its CA certificate to the build:
`docker build --secret id=extra_ca,src=/path/proxy-ca.crt …` (see the top of `Dockerfile`).

## Troubleshooting

| Symptom | Check |
| --- | --- |
| `docker compose up` stops with "run deploy/scripts/init-env.sh" | `deploy/.env` is missing or a required secret is empty. |
| Browser shows a certificate warning | With `https://localhost`, trust Caddy's root CA (above). With a domain, check DNS and that ports 80/443 reach the server; see `docker compose logs web`. |
| Sign-in seems to succeed but you land on the sign-in page again | `REA_PUBLIC_URL` uses `https` while the browser uses `http`, so the secure session cookie is dropped. Make them match. |
| Many users locked out at once behind a proxy | Client IPs are not forwarded. Set `REA_TRUSTED_PROXIES` (Compose sets the Docker subnet) and `REA_UPSTREAM_TRUSTED_PROXIES` for proxies in front of Caddy. |
| Extension says the server is unreachable | See [extension.md](extension.md#troubleshooting): HTTPS is required except on localhost, and custom extension IDs must be in `REA_ALLOWED_ORIGINS`. |
| Analyses stay queued | With `REA_WORKER_MODE=off`, start the worker profile. Check `docker compose logs api worker`. |
| "pg_dump is not installed" in System settings | Only when running without Docker; install `postgresql-client-18` or use `deploy/scripts/backup.sh`. |
| AI tools say no provider is configured | Expected until an administrator adds one under **Settings → AI providers**. |
