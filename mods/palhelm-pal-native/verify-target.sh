#!/usr/bin/env bash
set -euo pipefail

linux_dir="${PALWORLD_LINUX_DIR:-/home/hunter/palworld/data/Pal/Binaries/Linux}"
game="${linux_dir}/PalServer-Linux-Shipping"
ue4ss="${linux_dir}/ue4ss/libUE4SS.so"
layout="${linux_dir}/ue4ss/MemberVariableLayout.ini"
headers="${linux_dir}/ue4ss/UHTHeaderDump/Pal/Public"

expected_game_sha="788649fa1592160faa7bcf07ccd16d474ebeaae954717bc32284b5a43028d8e7"
expected_ue4ss_sha="b614f4f0555319be60a45fd5c47e7ca8089a3222babf49a42d4bdbd187855fb1"

actual_game_sha="$(sha256sum "${game}" | awk '{print $1}')"
actual_ue4ss_sha="$(sha256sum "${ue4ss}" | awk '{print $1}')"

if [[ "${actual_game_sha}" != "${expected_game_sha}" ]]; then
    echo "Palworld executable mismatch; bridge must remain disabled" >&2
    exit 1
fi
if [[ "${actual_ue4ss_sha}" != "${expected_ue4ss_sha}" ]]; then
    echo "UE4SS library mismatch; bridge must remain disabled" >&2
    exit 1
fi
if [[ ! -s "${layout}" ]]; then
    echo "MemberVariableLayout.ini missing; bridge must remain disabled" >&2
    exit 1
fi

utility_header="$(rg -l 'GetInitializedCharacterSaveParemter\(' "${headers}" -g 'PalUtility*.h' | head -n 1)"
manager_header="$(rg -l 'SpawnNewCharacter\(' "${headers}" -g 'PalCharacterManager*.h' | head -n 1)"
if [[ -z "${utility_header}" || -z "${manager_header}" ]]; then
    echo "required reflected Palworld API missing; bridge must remain disabled" >&2
    exit 1
fi

echo "Pinned Palworld and UE4SS target verified; capability probe may be staged"
echo "Native addresses from PalServer-Linux-Shipping.sym remain prohibited"
