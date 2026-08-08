#!/bin/sh
# Resolve the runtime identity, then exec Montainer as PID 1.
#
# The identity is taken from the world volume rather than baked into the image,
# so an existing server keeps working no matter who owns its files. Ownership is
# only rewritten when the operator names a different identity via PUID/PGID.
set -eu

app=/app/montainer

warn() {
    printf 'montainer-entrypoint: %s\n' "$*" >&2
}

# An operator-selected identity is authoritative. Nothing here can improve on
# it, and a non-root process cannot chown anyway, so hand over immediately
# rather than failing on storage this container was never able to repair.
if [ "$(id -u)" -ne 0 ]; then
    exec env LD_LIBRARY_PATH="${INSTANCE_DIR:-/app/instance}" "$app" "$@"
fi

# Normalized so the world path below matches what find prints while walking it.
instance_dir=${INSTANCE_DIR:-/app/instance}
while [ "$instance_dir" != "/" ] && [ "${instance_dir%/}" != "$instance_dir" ]; do
    instance_dir=${instance_dir%/}
done
worlds_dir="${instance_dir%/}/worlds"
configs_dir=${CONFIG_DIR:-/app/configs}
resources_dir=${RESOURCE_PACKS_DIR:-/app/resource_packs}
logs_dir=${LOG_DIR:-/app/logs}

for directory in "$instance_dir" "$worlds_dir" "$configs_dir" "$resources_dir" "$logs_dir"; do
    mkdir -p -- "$directory" || {
        warn "could not create $directory"
        exit 1
    }
done

validate_id() {
    case "$2" in
        '' | *[!0-9]*)
            warn "$1 must be a non-negative integer, got '$2'"
            exit 1
            ;;
    esac
}
[ -z "${PUID:-}" ] || validate_id PUID "$PUID"
[ -z "${PGID:-}" ] || validate_id PGID "$PGID"

# Whoever owns the world data is who Bedrock should be, unless told otherwise.
data_uid=$(stat -c %u -- "$worlds_dir")
data_gid=$(stat -c %g -- "$worlds_dir")
uid=${PUID:-$data_uid}
gid=${PGID:-$data_gid}

printf 'montainer-entrypoint: running Montainer and Bedrock as %s:%s\n' "$uid" "$gid"

if [ "$uid" -eq 0 ] && [ "$gid" -eq 0 ]; then
    exec env LD_LIBRARY_PATH="$instance_dir" "$app" "$@"
fi

# Ownership changes are best-effort. Root-squashed NFS, SMB, and read-only
# mounts routinely refuse chown while already granting the access Bedrock
# needs, and refusing to boot over that is worse than letting the server try.
align() {
    chown "$@" 2>/dev/null || warn "could not change ownership of ${*}; continuing"
}

# Image content must follow the identity: Bedrock's binary and shared libraries
# live here. The world volume is pruned because it is user data, and on an
# established server it is by far the largest tree under this path.
find "$instance_dir" -path "$worlds_dir" -prune -o \
    -exec chown "$uid:$gid" {} + 2>/dev/null \
    || warn "could not change ownership of $instance_dir; continuing"

# Top level only. A fresh Docker volume is created root-owned and needs this;
# data already inside it belongs to the user and is left alone.
align "$uid:$gid" "$worlds_dir" "$configs_dir" "$resources_dir" "$logs_dir"

# Reached only when PUID/PGID name an identity the data does not already use.
# Asking for a specific identity is a request to make it work, so this is the
# one case where rewriting existing data is what the operator actually wants.
if [ "$uid" != "$data_uid" ] || [ "$gid" != "$data_gid" ]; then
    align -R "$uid:$gid" "$worlds_dir" "$configs_dir" "$resources_dir" "$logs_dir"
fi

exec setpriv \
    --reuid="$uid" \
    --regid="$gid" \
    --clear-groups \
    --bounding-set=-all \
    --inh-caps=-all \
    --ambient-caps=-all \
    --no-new-privs \
    env LD_LIBRARY_PATH="$instance_dir" "$app" "$@"
