#!/usr/bin/env bash
set -euo pipefail

root_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
env_file="$root_dir/.env"
project_name="${PMAIL_PROJECT_NAME:-}"

load_project() {
    if [[ ! -f "$env_file" ]]; then
        echo "未找到 .env，请先运行 deploy/deploy.sh init。" >&2
        exit 1
    fi
    if [[ -z "$project_name" ]]; then
        # Only read the simple project-name assignment; Compose, not Bash, parses the rest.
        project_name="$(sed -n 's/^PMAIL_PROJECT_NAME=\([a-z0-9][a-z0-9_-]*\)$/\1/p' "$env_file" | tail -n 1)"
        project_name="${project_name:-pmail}"
    fi
    if [[ ! "$project_name" =~ ^[a-z0-9][a-z0-9_-]*$ ]]; then
        echo "PMAIL_PROJECT_NAME 必须以小写字母或数字开头，只能包含小写字母、数字、下划线和连字符。" >&2
        exit 2
    fi
}

compose() {
    docker compose --project-directory "$root_dir" --env-file "$env_file" -p "$project_name" "$@"
}

random_hex() {
    od -An -N "$1" -tx1 /dev/urandom | tr -d '[:space:]'
}

init_env() {
    if [[ -e "$env_file" ]]; then
        echo ".env 已存在，未覆盖。"
        return
    fi
    local db_password secret_key admin_password
    db_password="$(random_hex 24)"
    secret_key="$(random_hex 32)"
    admin_password="$(random_hex 24)"
    (
        umask 077
        set -o noclobber
        sed -e "s/^POSTGRES_PASSWORD=$/POSTGRES_PASSWORD=$db_password/" \
            -e "s/^PMAIL_SECRET_KEY=$/PMAIL_SECRET_KEY=$secret_key/" \
            -e "s/^PMAIL_ADMIN_PASSWORD=$/PMAIL_ADMIN_PASSWORD=$admin_password/" \
            "$root_dir/.env.example" > "$env_file"
    )
    echo "已生成权限为 0600 的 .env。请修改邮件域名及端口绑定，并妥善保存管理员密码和密钥。"
}

backup() (
    umask 077
    local dir="$root_dir/backups" stamp tmp basename file
    mkdir -p "$dir"
    stamp="$(date -u +%Y%m%dT%H%M%SZ)"
    tmp="$(mktemp "$dir/.${project_name}-${stamp}.XXXXXXXX")"
    trap 'rm -- "$tmp"' EXIT
    compose exec -T postgres sh -ec \
        'PGPASSWORD="$POSTGRES_PASSWORD" exec pg_dump -h 127.0.0.1 -U "$POSTGRES_USER" -d "$POSTGRES_DB" --format=custom' > "$tmp"
    basename="${tmp##*/}"
    file="$dir/${basename#.}.dump"
    mv -- "$tmp" "$file"
    trap - EXIT
    echo "备份完成：$file"
)

restore() {
    local file="$1" running
    compose up -d --wait postgres
    running="$(compose ps --status running -q pmail)"
    if [[ -n "$running" ]]; then
        compose stop pmail
    fi
    if ! compose exec -T postgres sh -ec \
        'PGPASSWORD="$POSTGRES_PASSWORD" exec pg_restore -h 127.0.0.1 -U "$POSTGRES_USER" -d "$POSTGRES_DB" --clean --if-exists --single-transaction --exit-on-error --no-owner --no-privileges' < "$file"; then
        echo "恢复失败；PMail 不会自动重启。请查看错误并确认数据库状态。" >&2
        return 1
    fi
    if [[ -n "$running" ]]; then
        compose up -d pmail
    fi
    echo "恢复完成：$file"
}

case "${1:-}" in
    init) init_env ;;
    up) load_project; compose build; compose up -d ;;
    upgrade) load_project; backup; compose build --pull; compose up -d ;;
    backup) load_project; backup ;;
    restore)
        [[ $# -eq 2 && -f "$2" && -r "$2" ]] || { echo "用法：$0 restore <backup.dump>" >&2; exit 2; }
        load_project
        restore "$2"
        ;;
    logs) load_project; compose logs -f --tail=200 ;;
    status) load_project; compose ps ;;
    down) load_project; compose down ;;
    *) echo "用法：$0 {init|up|upgrade|backup|restore <file>|logs|status|down}" >&2; exit 2 ;;
esac
