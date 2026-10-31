#!/bin/sh
set -eu

output=${1:?Pass an output directory}
mkdir -p "$output"
output=$(cd "$output" && pwd)
compiler=${CC:-cc}
"$compiler" -std=c11 -O2 -Wall -Wextra -Werror -fPIC -shared \
    -Wl,-z,relro,-z,now -o "$output/bedrock-socket-reuse.so" \
    compat/bedrock_socket_reuse.c -ldl -pthread
"$compiler" -std=c11 -O2 -Wall -Wextra -Werror \
    -o "$output/bedrock-socket-reuse-test" compat/bedrock_socket_reuse_test.c

# Establish that the kernel reproduces the failure before checking the fix.
env -u LD_PRELOAD "$output/bedrock-socket-reuse-test" baseline
LD_PRELOAD="$output/bedrock-socket-reuse.so" "$output/bedrock-socket-reuse-test" enabled
