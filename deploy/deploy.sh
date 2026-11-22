#!/usr/bin/env bash
set -euo pipefail

root_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
env_file="${PMAIL_ENV_FILE:-$root_dir/.env}"
project_name="${PMAIL_PROJECT_NAME:-}"
YES=false
PURGE=false
FILL_MISSING=false
UPGRADE_VERSION="${UPGRADE_VERSION:-}"
INIT_REVERSE_PROXY=false
INIT_WEB_PUBLIC=false
INIT_WEB_BIND=""
INIT_WEB_PORT=""
OLD_CONFIG=""
FORCE_PROJECT=false

usage() {
    cat <<USAGE
用法：$0 <命令> [选项]

命令：
  init                         生成 .env，支持交互式初始化
  check                        检查 Docker、端口、磁盘和必填配置
  up                           构建或拉取镜像并启动
  upgrade                      备份、构建、启动，失败自动回滚
  backup                      备份 PostgreSQL
  restore <file>               恢复 PostgreSQL 备份
  dns                          查询域名 DNS 期望值与实际值
  migrate-from-sqlite <path>   从 SQLite 迁移
  migrate-from-mysql <dsn>     从 MySQL 迁移
  logs                         查看日志
  status                       查看状态
  down                         停止服务
  uninstall                    停止并删除容器；--purge 同时删除卷

通用选项：
  --yes                        非交互模式/确认危险操作
  --project <name>             覆盖 Compose 项目名
  --domain <name>              主邮件域名
  --domains <list>             附加域名，逗号分隔
  --web-domain <name>          Web 域名
  --ssl-type <0|1|2|3>         证书模式
  --reverse-proxy              init 时使用外部反向代理（新建 .env）
  --web-bind <ip>              反代上游的宿主机 IPv4 地址（默认 172.17.0.1）
  --web-port <port>            反代上游的宿主机 HTTP 端口（默认 8080）
  --web-public                 直接公开 Web；SSL 模式 0/1/2 同时启用 Web HTTPS
  --admin-account <name>       管理员账号
  --admin-password <password>  可选：直接创建管理员；不指定则使用首次注册密钥
  --purge                      uninstall 时删除数据卷
  --fill-missing               init 时补齐镜像选项，保留现有凭据配置
  --version <tag>              upgrade 使用的镜像 tag（预构建镜像模式）
                               也可设置 UPGRADE_VERSION，--version 优先；默认 latest
  --old-config <dir>           旧版 config/ 目录，用于迁移域名和 DKIM
  --force-project              跳过项目目录冲突拒绝（仍必须二次确认）
  -h, --help                   显示帮助
USAGE
}

random_hex() {
    od -An -N "$1" -tx1 /dev/urandom | tr -d '[:space:]'
}

set_env_value() {
    local key="$1" value="$2" tmp
    tmp="$(mktemp)"
    awk -v key="$key" -v value="$value" '
        BEGIN { done=0 }
        $0 ~ "^" key "=" { print key "=" value; done=1; next }
        { print }
        END { if (!done) print key "=" value }
    ' "$env_file" > "$tmp"
    chmod 600 "$tmp"
    mv "$tmp" "$env_file"
    if [[ "$key" == PMAIL_IMAGE ]]; then
        export PMAIL_IMAGE="$value"
    fi
}

parse_options() {
    while (($#)); do
        case "$1" in
            --yes) YES=true; shift ;;
            --purge) PURGE=true; shift ;;
            --fill-missing) FILL_MISSING=true; shift ;;
            --version) UPGRADE_VERSION="$2"; shift 2 ;;
            --old-config) OLD_CONFIG="$2"; shift 2 ;;
            --force-project) FORCE_PROJECT=true; shift ;;
            --project) project_name="$2"; shift 2 ;;
            --domain) INIT_DOMAIN="$2"; shift 2 ;;
            --domains) INIT_DOMAINS="$2"; shift 2 ;;
            --web-domain) INIT_WEB_DOMAIN="$2"; shift 2 ;;
            --ssl-type) INIT_SSL_TYPE="$2"; shift 2 ;;
            --reverse-proxy) INIT_REVERSE_PROXY=true; shift ;;
            --web-bind|--web-port)
                [[ $# -ge 2 && -n "$2" && "$2" != --* ]] || { echo "$1 缺少参数。" >&2; exit 2; }
                if [[ "$1" == --web-bind ]]; then INIT_WEB_BIND="$2"; else INIT_WEB_PORT="$2"; fi
                shift 2
                ;;
            --web-public) INIT_WEB_PUBLIC=true; shift ;;
            --admin-account) INIT_ADMIN_ACCOUNT="$2"; shift 2 ;;
            --admin-password) INIT_ADMIN_PASSWORD="$2"; shift 2 ;;
            -h|--help) usage; exit 0 ;;
            -*) echo "未知选项：$1" >&2; usage >&2; exit 2 ;;
            *) echo "多余位置参数：$1（该命令只接受一个位置参数）" >&2; usage >&2; exit 2 ;;
        esac
    done
}

load_project() {
    if [[ ! -f "$env_file" ]]; then
        echo "未找到 $env_file，请先运行 deploy/deploy.sh init。" >&2
        exit 1
    fi
    if grep -Eq '^[[:space:]]*(export[[:space:]]+)?COMPOSE_FILE[[:space:]]*=[^#]*docker-compose\.caddy\.yml' "$env_file"; then
        echo "docker-compose.caddy.yml 已移除，请删除 $env_file 中引用它的 COMPOSE_FILE 行，再重试。" >&2
        exit 1
    fi
    if [[ "${COMPOSE_FILE:-}" == *docker-compose.caddy.yml* ]]; then
        echo "docker-compose.caddy.yml 已移除，请取消 shell 中引用它的 COMPOSE_FILE 环境变量，再重试。" >&2
        exit 1
    fi
    if [[ -z "$project_name" ]]; then
        project_name="$(sed -n 's/^PMAIL_PROJECT_NAME=//p' "$env_file" | tail -n 1)"
        project_name="${project_name:-pmail}"
    fi
    if [[ ! "$project_name" =~ ^[a-z0-9][a-z0-9_-]*$ ]]; then
        echo "PMAIL_PROJECT_NAME 格式无效。" >&2
        exit 2
    fi
    local configured_image env_image
    configured_image="$(sed -n 's/^PMAIL_IMAGE=//p' "$env_file" | tail -n 1)"
    env_image="${PMAIL_IMAGE:-}"
    if [[ -z "$configured_image" ]]; then
        configured_image="${env_image:-dingding229/pmail-pro:latest}"
    fi
    export PMAIL_IMAGE="$configured_image"
}

print_project_context() {
    echo "项目：$project_name  目录：$root_dir"
}

check_project_safety() {
    local current other found=false containers
    current="$root_dir"
    if ! containers="$(docker ps -a --filter "label=com.docker.compose.project=$project_name" --format '{{.Label "com.docker.compose.project.working_dir"}}' 2>/dev/null)"; then
        echo "无法检查 Docker 项目归属，拒绝执行可能改动容器的命令。请确认 Docker 可用后重试。" >&2
        return 1
    fi
    while IFS= read -r other; do
        [[ -n "$other" && "$other" != "$current" ]] || continue
        found=true
        echo "检测到项目 $project_name 已在其他目录运行：$other" >&2
    done <<< "$containers"
    if [[ "$found" == true ]]; then
        if [[ "$FORCE_PROJECT" != true ]]; then
            echo "拒绝操作：请使用 --project 指定独立项目名，或在原目录执行；需要跳过保护时显式使用 --force-project。" >&2
            return 1
        fi
        local answer
        read -r -p "确认操作项目 $project_name 并可能影响其他目录容器？请输入 yes：" answer
        [[ "$answer" == yes ]] || { echo "未确认，停止操作。" >&2; return 1; }
    fi
    compose config --quiet
}

compose() {
    local -a cmd=(docker compose --project-directory "$root_dir" --env-file "$env_file" -p "$project_name")
    if [[ "${1:-}" == build ]]; then
        # The base file is pull-only. Include the source overlay only for builds,
        # retaining any file list and path separator configured for this project.
        local compose_env files separator
        compose_env="$("${cmd[@]}" config --environment)" || return
        files="$(compose_env_value COMPOSE_FILE '' <<< "$compose_env")"
        separator="$(compose_env_value COMPOSE_PATH_SEPARATOR : <<< "$compose_env")"
        if [[ -z "$files" ]]; then
            files=docker-compose.yml
            if [[ -f "$root_dir/docker-compose.override.yml" ]]; then
                files="${files}${separator}docker-compose.override.yml"
            fi
        fi
        COMPOSE_FILE="${files}${separator}docker-compose.build.yml" "${cmd[@]}" "$@"
    else
        "${cmd[@]}" "$@"
    fi
}

valid_ipv4() {
    local octet
    [[ "$1" =~ ^([0-9]{1,3})\.([0-9]{1,3})\.([0-9]{1,3})\.([0-9]{1,3})$ ]] || return 1
    for octet in "${BASH_REMATCH[@]:1}"; do
        (( 10#$octet <= 255 )) || return 1
    done
}

valid_port() {
    [[ "$1" =~ ^[1-9][0-9]{0,4}$ ]] && (( $1 <= 65535 ))
}

init_env() {
    if [[ -e "$env_file" ]]; then
        if [[ "$INIT_REVERSE_PROXY" == true || -n "$INIT_WEB_BIND" || -n "$INIT_WEB_PORT" ]]; then
            echo "已有 .env：反代初始化选项仅用于新建配置，未修改现有部署模式。" >&2
        fi
        local existing_project
        existing_project="$(sed -n 's/^PMAIL_PROJECT_NAME=//p' "$env_file" | tail -n 1)"
        if [[ -n "$existing_project" && "$project_name" != "$existing_project" ]]; then
            echo "已有 .env 的项目名为 $existing_project，保留该值，不写入请求的 $project_name。" >&2
            project_name="$existing_project"
        elif [[ -z "$existing_project" && -n "$project_name" ]]; then
            set_env_value PMAIL_PROJECT_NAME "$project_name"
        fi
        if [[ "$FILL_MISSING" != true ]]; then
            if [[ -z "$(sed -n 's/^PMAIL_IMAGE=//p' "$env_file" | tail -n 1)" ]]; then
                set_env_value PMAIL_IMAGE "${PMAIL_IMAGE:-pmail-${project_name}:local}"
            fi
            echo ".env 已存在，未覆盖。最终项目名：$project_name" >&2
            return 0
        fi
        [[ -n "$(sed -n 's/^PMAIL_IMAGE=//p' "$env_file" | tail -n 1)" ]] || set_env_value PMAIL_IMAGE "${PMAIL_IMAGE:-pmail-${project_name}:local}"
        echo "保留现有密码和密钥配置；未显式设置的凭据继续由数据卷管理。"
        return 0
    fi
    local domain domains web_domain ssl_type web_bind https_enabled=2 admin_account admin_password db_password secret_key value
    local web_http_port=80 web_https_port=443
    domain="${INIT_DOMAIN:-${PMAIL_DOMAIN:-}}"
    domains="${INIT_DOMAINS:-${PMAIL_DOMAINS:-}}"
    web_domain="${INIT_WEB_DOMAIN:-${PMAIL_WEB_DOMAIN:-}}"
    ssl_type="${INIT_SSL_TYPE:-${PMAIL_SSL_TYPE:-3}}"
    web_bind="127.0.0.1"
    admin_account="${INIT_ADMIN_ACCOUNT:-${PMAIL_ADMIN_ACCOUNT:-admin}}"
    admin_password="${INIT_ADMIN_PASSWORD:-}"
    if [[ "$YES" != true ]]; then
        read -r -p "主邮件域名 [${domain:-example.com}]: " value
        domain="${value:-${domain:-example.com}}"
        read -r -p "附加域名（逗号分隔，可留空） [$domains]: " value
        domains="${value:-$domains}"
        read -r -p "Web 域名 [${web_domain:-mail.$domain}]: " value
        web_domain="${value:-${web_domain:-mail.$domain}}"
        read -r -p "证书模式 0=ACME HTTP-01, 1=自有证书, 2=ACME DNS-01, 3=反代仅 HTTP [$ssl_type]: " value
        ssl_type="${value:-$ssl_type}"
        if [[ "$INIT_REVERSE_PROXY" != true ]]; then
            read -r -p "Web 是否由外部反向代理（如 Nginx Proxy Manager）提供？[y/N] " value
            if [[ "$value" =~ ^[Yy]$ ]]; then
                INIT_REVERSE_PROXY=true
            fi
        fi
        if [[ "$INIT_REVERSE_PROXY" == true ]]; then
            if [[ -z "$INIT_WEB_BIND" ]]; then
                read -r -p "反代上游的宿主机 IPv4 地址 [172.17.0.1]: " value
                INIT_WEB_BIND="${value:-172.17.0.1}"
            fi
        elif [[ "$INIT_WEB_PUBLIC" != true ]]; then
            read -r -p "是否对公网开放 Web？[y/N]: " value
            if [[ "$value" =~ ^[Yy]$ ]]; then
                INIT_WEB_PUBLIC=true
            fi
        fi
        read -r -p "管理员账号 [$admin_account]: " value
        admin_account="${value:-$admin_account}"
    else
        [[ -n "$domain" ]] || { echo "--domain 或 PMAIL_DOMAIN 必填。" >&2; exit 2; }
    fi
    [[ -n "$domain" ]] || { echo "主邮件域名不能为空。" >&2; exit 2; }
    [[ "$ssl_type" =~ ^[0-3]$ ]] || { echo "SSL 类型必须为 0、1、2 或 3。" >&2; exit 2; }
    if [[ "$INIT_REVERSE_PROXY" == true ]]; then
        web_bind="${INIT_WEB_BIND:-172.17.0.1}"
        web_http_port="${INIT_WEB_PORT:-8080}"
        web_https_port=8443
        ssl_type=0
        web_domain="${web_domain:-mail.$domain}"
        valid_ipv4 "$web_bind" || { echo "--web-bind 必须是有效的 IPv4 地址。" >&2; return 2; }
        if ! valid_port "$web_http_port" || [[ "$web_http_port" == 80 || "$web_http_port" == 443 || "$web_http_port" == "$web_https_port" ]]; then
            echo "--web-port 必须是 1–65535 中的有效端口，且不能与 80、443 或 8443 冲突。" >&2
            return 2
        fi
    elif [[ -n "$INIT_WEB_BIND" || -n "$INIT_WEB_PORT" ]]; then
        echo "--web-bind 和 --web-port 仅适用于 --reverse-proxy。" >&2
        return 2
    elif [[ "$INIT_WEB_PUBLIC" == true ]]; then
        web_bind="0.0.0.0"
        if [[ "$ssl_type" != 3 ]]; then
            https_enabled=0
        fi
    fi
    web_domain="${web_domain:-mail.$domain}"
    db_password="$(random_hex 24)"
    secret_key="$(random_hex 32)"
    cp "$root_dir/.env.example" "$env_file"
    chmod 600 "$env_file"
    set_env_value PMAIL_PROJECT_NAME "$project_name"
    set_env_value PMAIL_IMAGE "${PMAIL_IMAGE:-pmail-${project_name}:local}"
    set_env_value PMAIL_DOMAIN "$domain"
    set_env_value PMAIL_DOMAINS "$domains"
    set_env_value PMAIL_WEB_DOMAIN "$web_domain"
    set_env_value PMAIL_SSL_TYPE "$ssl_type"
    set_env_value PMAIL_WEB_BIND "$web_bind"
    set_env_value PMAIL_HTTPS_ENABLED "$https_enabled"
    set_env_value PMAIL_HOST_HTTP_PORT "$web_http_port"
    set_env_value PMAIL_HOST_HTTPS_PORT "$web_https_port"
    set_env_value PMAIL_ADMIN_ACCOUNT "$admin_account"
    set_env_value PMAIL_ADMIN_PASSWORD "$admin_password"
    set_env_value POSTGRES_PASSWORD "$db_password"
    set_env_value PMAIL_SECRET_KEY "$secret_key"
    cat <<INFO
初始化完成。最终项目名：$project_name
请配置以下 DNS：
  MX、SPF、DKIM、DMARC；邮件服务器公网 IP 还应配置 PTR/rDNS。
INFO
    if [[ -n "$admin_password" ]]; then
        printf '首次启动将使用你提供的密码创建管理员 %s。已有用户不会被修改。\n' "$admin_account"
    else
        printf '启动后使用 `docker compose exec pmail cat /work/config/registration_key`，打开 https://%s/ 完成首次注册。\n' "$web_domain"
    fi
}

compose_env_value() {
    local key="$1" fallback="$2" value
    value="$(awk -v key="$key" 'index($0, key "=") == 1 { print substr($0, length(key) + 2); exit }')"
    printf '%s\n' "${value:-$fallback}"
}

check_default_bridge_bind() {
    if command -v ip >/dev/null; then
        if ip -o -4 addr show | awk '{ split($4, addr, "/"); if (addr[1] == "172.17.0.1") found=1 } END { exit !found }'; then
            return 0
        fi
    elif docker network inspect bridge --format '{{range .IPAM.Config}}{{println .Gateway}}{{end}}' 2>/dev/null | grep -Fx '172.17.0.1' >/dev/null; then
        return 0
    fi
    echo "宿主机未检测到 172.17.0.1。请用 ip -4 addr show docker0 确认网桥地址；初始化时改用 --web-bind <ip>，已有 .env 则更新 PMAIL_WEB_BIND。" >&2
    return 1
}

check() {
    load_project
    command -v docker >/dev/null || { echo "缺少 docker。" >&2; exit 1; }
    docker version >/dev/null
    docker compose version >/dev/null
    local rendered port
    rendered="$(compose config)" || return
    if grep -Eq '^[[:space:]]+host_ip: "?172\.17\.0\.1"?$' <<< "$rendered"; then
        check_default_bridge_bind || return 1
    fi
    if command -v ss >/dev/null; then
        while IFS= read -r port; do
            if valid_port "$port" && ss -H -ltn "sport = :$port" | grep . >/dev/null; then
                echo "端口 $port 已被占用（可能是本项目容器）。"
            fi
        done < <(sed -nE 's/^[[:space:]]+published: "?([0-9]+)"?$/\1/p' <<< "$rendered")
    else
        echo "提示：未安装 ss，无法检查宿主机端口占用；请安装 iproute2。"
    fi
    df -Pk "$root_dir" | awk 'NR==2 { if ($4 < 1048576) { print "磁盘可用空间少于 1GiB。"; exit 1 } }'
    if command -v nc >/dev/null; then
        if nc -z -w 3 1.1.1.1 25; then
            echo "25 端口出站连通性：TCP 探测成功。"
        else
            echo "提示：TCP 25 出站探测失败，云厂商可能封禁了 25 端口。"
        fi
    else
        echo "提示：未安装 nc，跳过 25 端口出站探测。"
    fi
    echo "部署前检查通过。"
}

wait_healthy() {
    local status
    for _ in $(seq 1 60); do
        status="$(compose ps --format '{{.State}} {{.Health}}' pmail 2>/dev/null || true)"
        if [[ "$status" == 'running healthy' ]]; then
            echo "PMail healthy。"
            return 0
        fi
        sleep 2
    done
    echo "PMail 未在超时内进入 healthy。" >&2
    compose ps
    return 1
}

verify_running() {
    local container status health restart_before restart_after
    container="$(compose ps -q pmail 2>/dev/null || true)"
    if [[ -z "$container" ]]; then
        echo "PMail 容器不存在，服务未运行。" >&2
        return 1
    fi
    status="$(docker inspect -f '{{.State.Status}}' "$container" 2>/dev/null || true)"
    health="$(docker inspect -f '{{if .State.Health}}{{.State.Health.Status}}{{else}}none{{end}}' "$container" 2>/dev/null || true)"
    restart_before="$(docker inspect -f '{{.RestartCount}}' "$container" 2>/dev/null || true)"
    echo "PMail 运行校验：status=$status health=$health restart_count=$restart_before"
    if [[ "$status" != running ]]; then
        echo "PMail 未处于 running 状态。" >&2
        return 1
    fi
    for _ in $(seq 1 30); do
        [[ "$health" == starting ]] || break
        sleep 3
        status="$(docker inspect -f '{{.State.Status}}' "$container" 2>/dev/null || true)"
        health="$(docker inspect -f '{{if .State.Health}}{{.State.Health.Status}}{{else}}none{{end}}' "$container" 2>/dev/null || true)"
        echo "PMail 健康检查等待：status=$status health=$health"
        [[ "$status" == running ]] || break
    done
    if [[ "$status" != running || "$health" != healthy ]]; then
        echo "PMail 未在 90 秒内进入 running/healthy 状态。" >&2
        return 1
    fi
    sleep 10
    status="$(docker inspect -f '{{.State.Status}}' "$container" 2>/dev/null || true)"
    health="$(docker inspect -f '{{if .State.Health}}{{.State.Health.Status}}{{else}}none{{end}}' "$container" 2>/dev/null || true)"
    restart_after="$(docker inspect -f '{{.RestartCount}}' "$container" 2>/dev/null || true)"
    echo "PMail 稳定性校验：status=$status health=$health restart_count=$restart_after"
    if [[ "$status" != running || "$health" != healthy || "$restart_after" != "$restart_before" ]]; then
        echo "PMail 在稳定性窗口内退出、重启或变为不健康。" >&2
        return 1
    fi
}

exec_postgres() {
    # shellcheck disable=SC2016
    compose exec -T postgres sh -ec '
        if [ -n "${POSTGRES_PASSWORD_FILE:-}" ]; then
            PGPASSWORD="$(cat "$POSTGRES_PASSWORD_FILE")"
        else
            PGPASSWORD="${POSTGRES_PASSWORD:?}"
        fi
        export PGPASSWORD
        exec "$@"
    ' sh "$@"
}

restore_database() {
    local file="$1" restore_rc
    # shellcheck disable=SC2016
    if exec_postgres sh -ec 'exec pg_restore -h 127.0.0.1 -U "$POSTGRES_USER" -d "$POSTGRES_DB" --clean --if-exists --single-transaction --exit-on-error --no-owner --no-privileges' < "$file"; then
        return 0
    else
        restore_rc=$?
        printf '数据库恢复失败，备份文件：%s，请人工恢复（退出码：%s）。\n' "$file" "$restore_rc" >&2
        return "$restore_rc"
    fi
}

rollback_upgrade() {
    local previous="$1" backup_file="$2"
    set_env_value PMAIL_IMAGE "$previous"
    compose down
    compose up -d --wait postgres
    restore_database "$backup_file" || return $?
    # Bring the entire configured stack back.
    if ! compose up -d --wait; then
        echo "回滚后服务仍不健康，请人工处理。" >&2
        compose logs --tail=50 pmail || true
        return 1
    fi
    if ! verify_running; then
        echo "回滚后服务仍不健康，请人工处理。" >&2
        compose logs --tail=50 pmail || true
        return 1
    fi
    echo "升级失败，已回滚到 $previous 并恢复升级前备份，服务健康。"
}

backup() (
    umask 077
    local dir="$root_dir/backups" stamp tmp file
    mkdir -p "$dir"
    stamp="$(date -u +%Y%m%dT%H%M%SZ)"
    file="$dir/${project_name}-${stamp}.dump"
    tmp="$(mktemp "$dir/.${project_name}-${stamp}.XXXXXXXX")"
    trap 'rm -f -- "$tmp"' EXIT
    # shellcheck disable=SC2016
    exec_postgres sh -ec 'exec pg_dump -h 127.0.0.1 -U "$POSTGRES_USER" -d "$POSTGRES_DB" --format=custom' > "$tmp"
    mv -- "$tmp" "$file"
    trap - EXIT
    echo "备份完成：$file"
)

restore() {
    local file="$1"
    compose up -d --wait postgres
    compose stop pmail
    restore_database "$file" || return $?
    compose up -d
    wait_healthy
}

is_registry_image() {
    [[ "$1" == */* ]]
}

image_exists() {
    docker image inspect "$1" >/dev/null 2>&1
}

local_image_cleanup() {
    local images prefix
    prefix="pmail-${project_name}"
    images="$(docker image ls --format '{{.Repository}}:{{.Tag}}' "$prefix" | awk -F: '$2 ~ /^[0-9]{14}$/ {print}' | sort -r | tail -n +3)"
    if [[ -n "$images" ]]; then
        while IFS= read -r image; do
            [[ -n "$image" ]] && docker image rm "$image" >/dev/null 2>&1 || true
        done <<< "$images"
    fi
}

up() {
    check
    local image
    image="${PMAIL_IMAGE:-pmail-${project_name}:local}"
    if is_registry_image "$image"; then
        compose pull pmail init-secrets
    else
        compose build pmail
    fi
    if ! compose up -d --wait || ! verify_running; then
        echo "PMail 启动失败，最近日志如下：" >&2
        compose logs --tail=50 pmail || true
        return 1
    fi
}

upgrade() {
    check
    local previous tag backup_file image repo version
    previous="${PMAIL_IMAGE:-pmail-${project_name}:local}"
    backup_file="$(backup | sed -n 's/^备份完成：//p')"
    if is_registry_image "$previous"; then
        repo="${previous%:*}"
        version="${UPGRADE_VERSION:-latest}"
        image="$repo:$version"
        set_env_value PMAIL_IMAGE "$image"
        if ! compose pull pmail init-secrets || ! compose up -d --wait || ! verify_running; then
            echo "预构建镜像升级失败，恢复旧 tag 和数据库备份。" >&2
            rollback_upgrade "$previous" "$backup_file"
            return 1
        fi
        echo "预构建镜像升级完成：$image；升级前备份：$backup_file"
        return 0
    fi
    tag="pmail-${project_name}:$(date -u +%Y%m%d%H%M%S)"
    set_env_value PMAIL_IMAGE "$tag"
    if ! compose build --pull pmail || ! compose up -d --wait || ! verify_running; then
        echo "本地构建升级失败，开始回滚镜像和数据库。" >&2
        rollback_upgrade "$previous" "$backup_file"
        return 1
    fi
    local_image_cleanup
    echo "本地构建升级完成：$tag；升级前备份：$backup_file"
}

api_error_fields() {
    local payload="$1"
    if command -v jq >/dev/null; then
        printf '%s' "$payload" | jq -r '[.errorNo // 0, .errorMsg // ""] | @tsv' 2>/dev/null && return 0
    fi
    if command -v python3 >/dev/null; then
        printf '%s' "$payload" | python3 -c '
import json, sys
value = json.load(sys.stdin)
print("{}\t{}".format(value.get("errorNo", 0), value.get("errorMsg", "")))
' && return 0
    fi
    local error_no=0 error_msg=""
    if [[ "$payload" =~ "errorNo"[[:space:]]*:[[:space:]]*([0-9]+) ]]; then
        error_no="${BASH_REMATCH[1]}"
    fi
    error_msg="$(printf '%s' "$payload" | sed -n 's/.*"errorMsg"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p')"
    printf '%s\t%s\n' "$error_no" "$error_msg"
}

dns() {
    local base token domains domain_rows formatter id name dns_json response_file http_code error_no error_msg
    local compose_env port binding
    base="${PMAIL_API_URL:-}"
    if [[ -z "$base" ]]; then
        compose_env="$(compose config --environment)" || return
        port="$(compose_env_value PMAIL_HTTP_PORT 80 <<< "$compose_env")"
        binding="$(compose port pmail "$port")" || return
        base="http://${binding/#0.0.0.0:/127.0.0.1:}"
    fi
    token="${PMAIL_API_TOKEN:-}"
    [[ -n "$token" ]] || { echo "请设置 PMAIL_API_TOKEN 后重试；Token 只在内存中使用。" >&2; exit 2; }
    command -v curl >/dev/null || { echo "缺少 curl。" >&2; exit 1; }
    response_file="$(mktemp)"
    if ! http_code="$(curl -sS -o "$response_file" -w '%{http_code}' -H "Authorization: Bearer $token" "$base/api/domain/list")"; then
        rm -f "$response_file"
        echo "域名列表查询失败（curl 无法连接 API）。" >&2
        return 1
    fi
    domains="$(cat "$response_file")"
    rm -f "$response_file"
    IFS=$'\t' read -r error_no error_msg <<< "$(api_error_fields "$domains")"
    if [[ ! "$http_code" =~ ^2[0-9][0-9]$ ]]; then
        echo "API Token 无效或无权限（HTTP $http_code${error_msg:+ / $error_msg}）。" >&2
        return 1
    fi
    case "$error_no" in
        ""|0|null) ;;
        *)
            echo "API Token 无效或无权限（HTTP $http_code${error_msg:+ / $error_msg}）。" >&2
            return 1
            ;;
    esac

    formatter=""
    domain_rows=""
    if command -v jq >/dev/null; then
        if domain_rows="$(printf '%s' "$domains" | jq -r 'if (.data | type) == "array" then .data[]? else (.data.list // [])[]? end | [.id, .name] | @tsv')"; then
            formatter=jq
        fi
    fi
    if [[ -z "$formatter" ]] && command -v python3 >/dev/null; then
        if domain_rows="$(printf '%s' "$domains" | python3 -c '
import json, sys
payload = json.load(sys.stdin)
data = payload.get("data", [])
if isinstance(data, dict):
    data = data.get("list", [])
for item in data or []:
    if item.get("id") is not None:
        print("{}\t{}".format(item.get("id"), item.get("name", "")))
')"; then
            formatter=python
        fi
    fi
    if [[ -z "$formatter" ]]; then
        if ! command -v grep >/dev/null || ! command -v sed >/dev/null; then
            echo "无法解析域名列表：需要 jq、python3 或 grep/sed。" >&2
            return 1
        fi
        local data_segment ids names
        data_segment="$(printf '%s' "$domains" | grep -oE '"data"[[:space:]]*:[[:space:]]*\[[^]]*\]' | head -n 1 || true)"
        if [[ -z "$data_segment" ]]; then
            data_segment="$(printf '%s' "$domains" | grep -oE '"data"[[:space:]]*:[[:space:]]*\{[^}]*\}' | grep -oE '"list"[[:space:]]*:[[:space:]]*\[[^]]*\]' | head -n 1 || true)"
        fi
        ids="$(printf '%s' "$data_segment" | grep -oE '"id"[[:space:]]*:[[:space:]]*[0-9]+' | grep -oE '[0-9]+' || true)"
        names="$(printf '%s' "$data_segment" | grep -oE '"name"[[:space:]]*:[[:space:]]*"[^"]*"' | sed -E 's/.*:[[:space:]]*"([^"]*)"/\1/' || true)"
        while IFS= read -r id; do
            [[ -n "$id" ]] || continue
            name="$(printf '%s\n' "$names" | head -n 1)"
            names="$(printf '%s\n' "$names" | tail -n +2)"
            printf '%s\t%s\n' "$id" "${name:-ID $id}"
        done <<< "$ids" >"${TMPDIR:-/tmp}/pmail-dns-rows.$$.tmp"
        domain_rows="$(cat "${TMPDIR:-/tmp}/pmail-dns-rows.$$.tmp")"
        rm -f "${TMPDIR:-/tmp}/pmail-dns-rows.$$.tmp"
        formatter=bash
    fi
    [[ -n "$domain_rows" ]] || { echo "域名列表中没有可用的 data 域名 ID。" >&2; return 1; }

    while IFS=$'\t' read -r id name; do
        [[ -n "$id" ]] || continue
        if ! dns_json="$(curl -fsS -H "Authorization: Bearer $token" -H 'Content-Type: application/json' -d "{\"id\":$id}" "$base/api/domain/dns")"; then
            echo "域名 $name (id=$id) 的 DNS 查询失败。" >&2
            return 1
        fi
        printf '域名：%s (id=%s)\n' "$name" "$id"
        case "$formatter" in
            jq)
                printf '类型\t主机名\t期望值\t当前是否匹配\n'
                printf '%s' "$dns_json" | jq -r 'if (.data | type) == "array" then .data[]? else (.data.list // [])[]? end | "\(.type)\t\(.name)\t\(.expected)\t\(.match)"'
                ;;
            python)
                printf '类型\t主机名\t期望值\t当前是否匹配\n'
                printf '%s' "$dns_json" | python3 -c '
import json, sys
payload = json.load(sys.stdin)
data = payload.get("data", [])
if isinstance(data, dict):
    data = data.get("list", [])
for item in data or []:
    print("{}\t{}\t{}\t{}".format(item.get("type", ""), item.get("name", ""), item.get("expected", ""), item.get("match", False)))
'
                ;;
            bash)
                printf '%s\n' "$dns_json"
                ;;
        esac
        printf '\n'
    done <<< "$domain_rows"
}

read_old_config() {
    local config_file="$1"
    [[ -r "$config_file" ]] || { echo "无法读取旧配置：$config_file" >&2; return 1; }
    if command -v jq >/dev/null; then
        jq -r '[.domain // "", ((.domains // []) | join(",")), (.webDomain // "")] | @tsv' "$config_file"
    elif command -v python3 >/dev/null; then
        python3 - "$config_file" <<'PYINNER'
import json, sys
with open(sys.argv[1], encoding="utf-8") as f:
    value = json.load(f)
print(value.get("domain", ""), ",".join(value.get("domains", []) or []), value.get("webDomain", ""), sep="\t")
PYINNER
    else
        echo "需要 jq 或 python3 解析旧 config.json。" >&2
        return 1
    fi
}

migrate_old_config() {
    local old_dir="$1" config_file="$1/config.json" values old_domain old_domains old_web
    [[ -d "$old_dir" ]] || { echo "旧 config 目录不存在：$old_dir" >&2; return 1; }
    if [[ -r "$config_file" ]]; then
        values="$(read_old_config "$config_file")"
        IFS=$'\t' read -r old_domain old_domains old_web <<< "$values"
        if [[ -n "$old_domain" ]]; then
            if [[ "$YES" != true ]]; then
                read -r -p "使用旧配置域名 $old_domain / $old_domains / $old_web 写入 .env？[Y/n]: " answer
                if [[ "$answer" =~ ^[Nn]$ ]]; then
                    old_domain=""; old_domains=""; old_web=""
                fi
            fi
            if [[ -n "$old_domain" ]]; then
                set_env_value PMAIL_DOMAIN "$old_domain"
                set_env_value PMAIL_DOMAINS "$old_domains"
                set_env_value PMAIL_WEB_DOMAIN "${old_web:-$old_domain}"
            fi
        fi
    fi
    if [[ -r "$old_dir/dkim/dkim.priv" ]]; then
        compose run --rm --no-deps -v "$old_dir:/migration/old-config:ro" pmail /bin/sh -ec 'mkdir -p /work/config/dkim && cp /migration/old-config/dkim/dkim.priv /work/config/dkim/dkim.priv && chmod 600 /work/config/dkim/dkim.priv'
        echo "已复制旧 DKIM 私钥到 pmail_config/config/dkim/dkim.priv。"
    else
        echo "警告：旧配置中没有 dkim/dkim.priv，DKIM 需要重新配置。" >&2
    fi
}

migrate() {
    local kind="$1" source="$2"
    if [[ -n "$OLD_CONFIG" ]]; then
        OLD_CONFIG="$(realpath "$OLD_CONFIG")"
        [[ -d "$OLD_CONFIG" ]] || { echo "旧 config 目录不存在：$OLD_CONFIG" >&2; exit 2; }
    fi
    if [[ "$kind" == sqlite ]]; then
        source="$(realpath "$source")"
        [[ -r "$source" ]] || { echo "SQLite 文件不可读：$source" >&2; exit 2; }
    fi
    compose stop pmail >/dev/null 2>&1 || true
    compose up -d --wait postgres
    [[ -n "$OLD_CONFIG" ]] && migrate_old_config "$OLD_CONFIG"
    if [[ "$kind" == sqlite ]]; then
        compose run --rm --no-deps -v "$source:/migration/source:ro" pmail /work/pmail migrate --from-type sqlite --from-dsn /migration/source
    else
        compose run --rm --no-deps pmail /work/pmail migrate --from-type mysql --from-dsn "$source"
    fi
    compose up -d pmail
    wait_healthy
    echo "迁移完成提醒：旧版 Token 头格式不再可用，请登录后创建新的 API Token。"
}

uninstall() {
    if [[ "$PURGE" != true && "$YES" != true ]]; then
        read -r -p "只删除容器，保留数据卷。继续？[y/N]: " value
        [[ "$value" =~ ^[Yy]$ ]] || exit 0
    fi
    compose down
    if [[ "$PURGE" == true ]]; then
        if [[ "$YES" != true ]]; then
            read -r -p "确认删除项目数据卷（pg_data、pmail_config、secrets），数据不可恢复？[y/N]: " value
            [[ "$value" =~ ^[Yy]$ ]] || exit 0
        fi
        compose down -v
    fi
}

main() {
    local command="${1:-}" position=""
    [[ $# -gt 0 ]] && shift
    if [[ "${1:-}" == "--help" || "${1:-}" == "-h" ]]; then
        usage
        exit 0
    fi
    case "$command" in
        restore|migrate-from-sqlite|migrate-from-mysql)
            [[ $# -gt 0 && "${1#-}" == "$1" ]] || { echo "该命令需要一个位置参数。" >&2; usage >&2; exit 2; }
            position="$1"
            shift
            ;;
    esac
    parse_options "$@"
    if [[ "$command" == init ]]; then
        project_name="${project_name:-${PMAIL_PROJECT_NAME:-pmail}}"
        print_project_context
    else
        load_project
        print_project_context
    fi
    case "$command" in
        init|check|status|logs) ;;
        *) check_project_safety ;;
    esac
    case "$command" in
        init) init_env ;;
        check) check ;;
        up) up ;;
        upgrade) upgrade ;;
        backup) backup ;;
        restore) [[ -f "$position" ]] || { echo "备份文件不存在：$position" >&2; exit 2; }; restore "$position" ;;
        dns) dns ;;
        migrate-from-sqlite) migrate sqlite "$position" ;;
        migrate-from-mysql) migrate mysql "$position" ;;
        logs) compose logs -f --tail=200 ;;
        status) compose ps ;;
        down) compose down ;;
        uninstall) uninstall ;;
        -h|--help) usage ;;
        *) usage >&2; exit 2 ;;
    esac
}

main "$@"
