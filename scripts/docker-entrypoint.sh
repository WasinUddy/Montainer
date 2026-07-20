#!/bin/sh
set -eu

target_uid=10001
target_gid=10001
running_pid=
scratch_dir=
export LC_ALL=C

cleanup() {
    if [ -n "$scratch_dir" ]; then
        rm -rf -- "$scratch_dir"
        scratch_dir=
    fi
}

fatal() {
    printf 'montainer-entrypoint: %s\n' "$*" >&2
    cleanup
    exit 1
}

validate_nonroot_identity() {
    current_uid=$(id -u)
    current_gid=$(id -g)
    if [ "$current_uid" -ne "$target_uid" ] || [ "$current_gid" -ne "$target_gid" ]; then
        fatal "explicit non-root mode requires UID/GID ${target_uid}:${target_gid}"
    fi
    if ! awk '
        $1 == "Groups:" {
            seen=1
            for (i=2; i<=NF; i++) if ($i == 0) exit 1
        }
        END { if (!seen) exit 1 }
    ' /proc/self/status; then
        fatal 'explicit non-root mode must not include supplementary group 0'
    fi
    if ! awk '
        $1 ~ /^Cap(Inh|Prm|Eff|Bnd|Amb):$/ {
            seen++
            if ($2 != "0000000000000000") exit 1
        }
        END { if (seen != 5) exit 1 }
    ' /proc/self/status; then
        fatal 'explicit non-root mode requires the OCI runtime to drop all capabilities (Docker: --cap-drop=ALL; Kubernetes: capabilities.drop: [ALL])'
    fi
}

# Docker starts this image as root so legacy volumes can be repaired. Normal
# application and health-check execution always pass through this boundary.
# An operator-selected non-root identity cannot clear its own capability
# bounding set, so require the OCI runtime to have dropped it already.
exec_hardened() {
    current_uid=$(id -u)
    if [ "$current_uid" -eq 0 ]; then
        exec setpriv \
            --reuid="$target_uid" \
            --regid="$target_gid" \
            --clear-groups \
            --bounding-set=-all \
            --inh-caps=-all \
            --ambient-caps=-all \
            --no-new-privs \
            "$@"
    fi

    validate_nonroot_identity
    exec setpriv \
        --inh-caps=-all \
        --ambient-caps=-all \
        --no-new-privs \
        "$@"
}

if [ "${1:-}" = __healthcheck ]; then
    [ "$#" -eq 1 ] || fatal '__healthcheck does not accept arguments'
    exec_hardened curl \
        --fail \
        --silent \
        --show-error \
        --connect-timeout 2 \
        --max-time 4 \
        --output /dev/null \
        http://127.0.0.1:8000/healthz
fi

terminate() {
    signal_name=$1
    exit_code=$2
    trap - HUP INT TERM
    if [ -n "$running_pid" ]; then
        kill "-$signal_name" "-$running_pid" 2>/dev/null \
            || kill "-$signal_name" "$running_pid" 2>/dev/null \
            || true
        wait "$running_pid" 2>/dev/null || true
        running_pid=
    fi
    cleanup
    exit "$exit_code"
}

trap 'terminate HUP 129' HUP
trap 'terminate INT 130' INT
trap 'terminate TERM 143' TERM

# Run longer filesystem operations in their own process group. While this shell
# is PID 1, its signal traps can then terminate and reap the entire operation
# instead of leaving Docker to SIGKILL a partially migrated scan.
run_guarded() {
    setsid "$@" &
    running_pid=$!
    if wait "$running_pid"; then
        guarded_status=0
    else
        guarded_status=$?
    fi
    running_pid=
    return "$guarded_status"
}

run_for_identity() {
    run_identity=$1
    shift
    if [ "$run_identity" = target ]; then
        run_guarded setpriv \
            --reuid="$target_uid" \
            --regid="$target_gid" \
            --clear-groups \
            --bounding-set=-all \
            --inh-caps=-all \
            --ambient-caps=-all \
            --no-new-privs \
            "$@"
        return
    fi
    run_guarded "$@"
}

resolve_data_path() {
    raw_path=$1
    [ -n "$raw_path" ] || fatal 'persistent data paths must not be empty'
    case "$raw_path" in
        *[[:cntrl:]]*) fatal 'persistent data paths must not contain control characters' ;;
    esac
    case "$raw_path" in
        /*) candidate_path=$raw_path ;;
        ./*) candidate_path="/app/${raw_path#./}" ;;
        *) candidate_path="/app/$raw_path" ;;
    esac
    [ ! -L "$candidate_path" ] || fatal "refusing symlinked data directory ${candidate_path}"
    lexical_path=$(realpath -m -s -- "$candidate_path") \
        || fatal "could not normalize persistent data path ${candidate_path}"
    resolved_path=$(realpath -m -- "$candidate_path") \
        || fatal "could not resolve persistent data path ${candidate_path}"
    [ "$resolved_path" = "$lexical_path" ] \
        || fatal "refusing data directory with a symbolic-link component ${candidate_path}"
    [ "$resolved_path" != / ] || fatal 'refusing to use the filesystem root as a persistent data directory'
    printf '%s\n' "$resolved_path"
}

validate_data_root() {
    root_label=$1
    root_path=$2
    case "$root_path" in
        /app | /app/dist | /app/dist/* | /app/montainer | \
        /bin | /bin/* | /boot | /boot/* | /dev | /dev/* | /etc | /etc/* | \
        /home | /lib | /lib/* | /lib64 | /lib64/* | /opt | /proc | /proc/* | \
        /root | /root/* | /run | /run/* | /sbin | /sbin/* | /sys | /sys/* | \
        /tmp | /usr | /usr/* | /var | /var/cache | /var/lib | /var/log)
            fatal "${root_label} resolves to protected path ${root_path}; use a dedicated data directory"
            ;;
    esac
}

reject_overlapping_roots() {
    left_label=$1
    left_path=$2
    right_label=$3
    right_path=$4
    case "$left_path" in
        "$right_path" | "$right_path"/*)
            fatal "${left_label} ${left_path} overlaps ${right_label} ${right_path}; configured data roots must be disjoint"
            ;;
    esac
    case "$right_path" in
        "$left_path"/*)
            fatal "${left_label} ${left_path} overlaps ${right_label} ${right_path}; configured data roots must be disjoint"
            ;;
    esac
}

ensure_directory() {
    ensure_path=$1
    [ ! -L "$ensure_path" ] || fatal "refusing symlinked data directory ${ensure_path}"
    mkdir -p -- "$ensure_path" || fatal "could not create persistent data directory ${ensure_path}"
    [ -d "$ensure_path" ] && [ ! -L "$ensure_path" ] \
        || fatal "persistent data path ${ensure_path} is not a directory"
}

escape_find_pattern() {
    printf '%s\n' "$1" | sed 's/[][\\*?]/\\&/g'
}

# Mutating scans prune every nested mount reported by the kernel. -xdev alone
# is insufficient because Docker volumes and bind mounts can share a device ID.
migrate_legacy_entries() {
    ownership_path=$1
    mount_targets=$(findmnt -R -l -n -o TARGET --target "$ownership_path") \
        || fatal "could not inspect mount boundaries below ${ownership_path}"

    set -- "$ownership_path" -xdev -mindepth 1
    while IFS= read -r nested_mount; do
        case "$nested_mount" in
            *\\x[0-9A-Fa-f][0-9A-Fa-f]*)
                fatal "refusing escaped or control-character mount target below ${ownership_path}"
                ;;
        esac
        case "$nested_mount" in
            "$ownership_path") ;;
            "$ownership_path"/*)
                nested_pattern=$(escape_find_pattern "$nested_mount")
                set -- "$@" -path "$nested_pattern" -prune -o
                ;;
        esac
    done <<EOF
$mount_targets
EOF
    # Do not chown multiply linked non-directories: a bind-mounted hardlink can
    # share an inode with a file outside the configured data root. If such an
    # entry is not already accessible, the recursive preflight fails safely.
    set -- "$@" \
        \( -type d -o \( \( -type f -o -type l \) -links 1 \) \) \
        \( -uid 0 -o -gid 0 \) \
        -exec chown --no-dereference "${target_uid}:${target_gid}" {} +
    run_guarded find "$@"
}

has_directory_access() {
    access_identity=$1
    access_path=$2
    access_mode=$3

    case "$access_mode" in
        read)
            run_for_identity "$access_identity" sh -c '[ -r "$1" ] && [ -x "$1" ]' sh "$access_path" \
                || return 1
            set -- "$access_path" \
                \( -type d \( ! -readable -o ! -executable \) \
                -o -type f ! -readable \
                -o ! \( -type d -o -type f -o -type l \) \) \
                -print -quit
            ;;
        instance)
            run_for_identity "$access_identity" sh -c '[ -r "$1" ] && [ -w "$1" ] && [ -x "$1" ]' sh "$access_path" \
                || return 1
            instance_resources=$(escape_find_pattern "${access_path%/}/resource_packs")
            set -- "$access_path" \
                \( -type d \( ! -readable -o ! -writable -o ! -executable \) \
                -o -type f \( ! -readable -o ! -writable \) \
                -o \( -type l ! -path "${instance_resources}/*" \) \
                -o ! \( -type d -o -type f -o -type l \) \) \
                -print -quit
            ;;
        write)
            run_for_identity "$access_identity" sh -c '[ -r "$1" ] && [ -w "$1" ] && [ -x "$1" ]' sh "$access_path" \
                || return 1
            set -- "$access_path" \
                \( -type d \( ! -readable -o ! -writable -o ! -executable \) \
                -o -type f \( ! -readable -o ! -writable \) \
                -o ! \( -type d -o -type f \) \) \
                -print -quit
            ;;
        *) fatal "unsupported data access mode ${access_mode}" ;;
    esac

    : > "$scratch_dir/access-report"
    : > "$scratch_dir/access-errors"
    if ! run_for_identity "$access_identity" find "$@" \
        > "$scratch_dir/access-report" 2> "$scratch_dir/access-errors"; then
        return 1
    fi
    [ ! -s "$scratch_dir/access-report" ] && [ ! -s "$scratch_dir/access-errors" ]
}

verify_directory_access() {
    verify_identity=$1
    verify_path=$2
    verify_mode=$3
    if ! has_directory_access "$verify_identity" "$verify_path" "$verify_mode"; then
        if [ -s "$scratch_dir/access-report" ]; then
            inaccessible_path=$(sed -n '1p' "$scratch_dir/access-report")
            printf 'montainer-entrypoint: inaccessible path: %s\n' "$inaccessible_path" >&2
        elif [ -s "$scratch_dir/access-errors" ]; then
            sed -n '1p' "$scratch_dir/access-errors" >&2
        fi
        fatal "${verify_path} does not provide ${verify_mode} access to the runtime identity; stop the server, back up the mount, repair its ownership or ACL, and retry"
    fi
}

migrate_directory() {
    migrate_path=$1
    migrate_mode=$2
    ensure_directory "$migrate_path"

    printf 'montainer-entrypoint: checking legacy ownership under %s\n' "$migrate_path"
    migration_failed=false
    if ! run_guarded chown --no-dereference "${target_uid}:${target_gid}" "$migrate_path"; then
        migration_failed=true
    fi
    if ! migrate_legacy_entries "$migrate_path"; then
        migration_failed=true
    fi

    # Root-squashed or ACL-managed storage can deny chown while already
    # granting the target identity everything it needs. Continue only after a
    # complete read/write traversal proves that it is safe.
    if [ "$migration_failed" = true ] && has_directory_access target "$migrate_path" "$migrate_mode"; then
        printf 'montainer-entrypoint: %s is already accessible; denied ownership changes were ignored\n' "$migrate_path"
        return
    fi
    verify_directory_access target "$migrate_path" "$migrate_mode"
}

case "${MONTAINER_AUTO_CHOWN:-true}" in
    true | false) ;;
    *) fatal 'MONTAINER_AUTO_CHOWN must be true or false' ;;
esac

# Reject a wrong or insufficiently confined explicit identity before it can
# create or otherwise mutate anything in configured storage.
if [ "$(id -u)" -ne 0 ]; then
    validate_nonroot_identity
fi

scratch_dir=$(mktemp -d /tmp/montainer-entrypoint.XXXXXX) \
    || fatal 'could not create entrypoint scratch directory'

instance_path=$(resolve_data_path "${INSTANCE_DIR:-/app/instance}")
worlds_path=$(resolve_data_path "${instance_path%/}/worlds")
configs_path=$(resolve_data_path "${CONFIG_DIR:-/app/configs}")
resources_path=$(resolve_data_path "${RESOURCE_PACKS_DIR:-/app/resource_packs}")
logs_path=$(resolve_data_path "${LOG_DIR:-/app/logs}")

validate_data_root INSTANCE_DIR "$instance_path"
validate_data_root CONFIG_DIR "$configs_path"
validate_data_root RESOURCE_PACKS_DIR "$resources_path"
validate_data_root LOG_DIR "$logs_path"
reject_overlapping_roots INSTANCE_DIR "$instance_path" CONFIG_DIR "$configs_path"
reject_overlapping_roots INSTANCE_DIR "$instance_path" RESOURCE_PACKS_DIR "$resources_path"
reject_overlapping_roots INSTANCE_DIR "$instance_path" LOG_DIR "$logs_path"
reject_overlapping_roots CONFIG_DIR "$configs_path" RESOURCE_PACKS_DIR "$resources_path"
reject_overlapping_roots CONFIG_DIR "$configs_path" LOG_DIR "$logs_path"
reject_overlapping_roots RESOURCE_PACKS_DIR "$resources_path" LOG_DIR "$logs_path"

if [ "$(id -u)" -eq 0 ]; then
    if [ "${MONTAINER_AUTO_CHOWN:-true}" = true ]; then
        migrate_directory "$worlds_path" write
        migrate_directory "$configs_path" write
        migrate_directory "$resources_path" read
        migrate_directory "$logs_path" write
        migrate_directory "$instance_path" instance
    else
        ensure_directory "$worlds_path"
        ensure_directory "$configs_path"
        ensure_directory "$resources_path"
        ensure_directory "$logs_path"
        ensure_directory "$instance_path"
        verify_directory_access target "$worlds_path" write
        verify_directory_access target "$configs_path" write
        verify_directory_access target "$resources_path" read
        verify_directory_access target "$logs_path" write
        verify_directory_access target "$instance_path" instance
    fi

    cleanup
    trap - HUP INT TERM
    export HOME=/home/montainer USER=montainer LOGNAME=montainer
    exec_hardened env LD_LIBRARY_PATH="$instance_path" /app/montainer "$@"
fi

# An explicit Docker/Kubernetes user cannot migrate ownership. Validate every
# configured root as that operator-selected identity before Bedrock can touch
# LevelDB, then preserve the selected identity for the application process.
ensure_directory "$worlds_path"
ensure_directory "$configs_path"
ensure_directory "$resources_path"
ensure_directory "$logs_path"
ensure_directory "$instance_path"
verify_directory_access current "$worlds_path" write
verify_directory_access current "$configs_path" write
verify_directory_access current "$resources_path" read
verify_directory_access current "$logs_path" write
verify_directory_access current "$instance_path" instance
cleanup
trap - HUP INT TERM
export HOME=/home/montainer USER=montainer LOGNAME=montainer
exec_hardened env LD_LIBRARY_PATH="$instance_path" /app/montainer "$@"
