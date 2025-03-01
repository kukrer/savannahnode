#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

# Changes to the minimum golang version must also be replicated in
# scripts/build_savannahnode.sh (here)
# scripts/local.Dockerfile
# Dockerfile
# README.md
# go.mod
go_version_minimum="1.18.1"

go_version() {
    go version | sed -nE -e 's/[^0-9.]+([0-9.]+).+/\1/p'
}

version_lt() {
    # Return true if $1 is a lower version than than $2,
    local ver1=$1
    local ver2=$2
    # Reverse sort the versions, if the 1st item != ver1 then ver1 < ver2
    if  [[ $(echo -e -n "$ver1\n$ver2\n" | sort -rV | head -n1) != "$ver1" ]]; then
        return 0
    else
        return 1
    fi
}

if version_lt "$(go_version)" "$go_version_minimum"; then
    echo "Savannahnode requires Go >= $go_version_minimum, Go $(go_version) found." >&2
    exit 1
fi

# Savnnahnode root folder
SAVANNAHNODE_PATH=$( cd "$( dirname "${BASH_SOURCE[0]}" )"; cd .. && pwd )
# Load the versions
source "$SAVANNAHNODE_PATH"/scripts/versions.sh
# Load the constants
source "$SAVANNAHNODE_PATH"/scripts/constants.sh

echo "Building Savannahnode..."
go build -ldflags "-X github.com/kukrer/savannahnode/version.GitCommit=$git_commit $static_ld_flags" -o "$savannahnode_path" "$SAVANNAHNODE_PATH/main/"*.go
