#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
source_dir="${repo_root}/mods/palhelm-pal-native"
ue4ss_dir="${UE4SS_DIR:-/home/hunter/palworld/data/Pal/Binaries/Linux/ue4ss}"
builder="${PALHELM_NATIVE_BUILDER:-palhelm/ue4ss-native-builder:bookworm}"

mkdir -p "${source_dir}/dist"

docker build --tag "${builder}" --file "${source_dir}/Dockerfile.builder" "${source_dir}"
docker run --rm \
    --volume "${source_dir}:/source:ro" \
    --volume "${source_dir}/dist:/output" \
    --volume "${ue4ss_dir}:/ue4ss:ro" \
    "${builder}" \
    -std=c++20 -O2 -fPIC -fvisibility=hidden -fno-exceptions -fno-rtti \
    -stdlib=libc++ -nostdlib++ \
    -I/source/include /source/src/main.cpp \
    -shared -L/ue4ss -lUE4SS \
    -Wl,--no-undefined -Wl,-z,defs -Wl,-z,relro -Wl,-z,now \
    -o /output/main.so
