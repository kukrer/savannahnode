#!/usr/bin/env bash
set -e
set -o nounset
set -o pipefail

# e.g.,
# ./scripts/build.sh
# ./scripts/tests.e2e.sh ./build/savannahnode
# ENABLE_WHITELIST_VTX_TESTS=true ./scripts/tests.e2e.sh ./build/savannahnode
if ! [[ "$0" =~ scripts/tests.e2e.sh ]]; then
  echo "must be run from repository root"
  exit 255
fi

SAVANNAHNODE_PATH="${1-}"
if [[ -z "${SAVANNAHNODE_PATH}" ]]; then
  echo "Missing SAVANNAHNODE_PATH argument!"
  echo "Usage: ${0} [SAVANNAHNODE_PATH]" >> /dev/stderr
  exit 255
fi

ENABLE_WHITELIST_VTX_TESTS=${ENABLE_WHITELIST_VTX_TESTS:-false}

#################################
# download avalanche-network-runner
# https://github.com/ava-labs/avalanche-network-runner
# TODO: migrate to upstream avalanche-network-runner
GOARCH=$(go env GOARCH)
GOOS=$(go env GOOS)
NETWORK_RUNNER_VERSION=1.1.0
DOWNLOAD_PATH=/tmp/avalanche-network-runner.tar.gz
DOWNLOAD_URL="https://github.com/ava-labs/avalanche-network-runner/releases/download/v${NETWORK_RUNNER_VERSION}/avalanche-network-runner_${NETWORK_RUNNER_VERSION}_${GOOS}_${GOARCH}.tar.gz"

rm -f ${DOWNLOAD_PATH}
rm -f /tmp/avalanche-network-runner

echo "downloading avalanche-network-runner ${NETWORK_RUNNER_VERSION} at ${DOWNLOAD_URL}"
curl --fail -L ${DOWNLOAD_URL} -o ${DOWNLOAD_PATH}

echo "extracting downloaded avalanche-network-runner"
tar xzvf ${DOWNLOAD_PATH} -C /tmp
/tmp/avalanche-network-runner -h

GOPATH="$(go env GOPATH)"
PATH="${GOPATH}/bin:${PATH}"

#################################
echo "building e2e.test"
# to install the ginkgo binary (required for test build and run)
go install -v github.com/onsi/ginkgo/v2/ginkgo@v2.1.4
ACK_GINKGO_RC=true ginkgo build ./tests/e2e
./tests/e2e/e2e.test --help

#################################
# run "avalanche-network-runner" server
echo "launch avalanche-network-runner in the background"
/tmp/avalanche-network-runner \
server \
--log-level debug \
--port=":12342" \
--disable-grpc-gateway 2> /dev/null &
PID=${!}

#################################
# By default, it runs all e2e test cases!
# Use "--ginkgo.skip" to skip tests.
# Use "--ginkgo.focus" to select tests.
#
# to run only ping tests:
# --ginkgo.focus "\[Local\] \[Ping\]"
#
# to run only X-Chain whitelist vtx tests:
# --ginkgo.focus "\[X-Chain\] \[WhitelistVtx\]"
#
# to run only X-Chain transfer tests:
# --ginkgo.focus "\[X-Chain\] \[Virtuous Transfer Tx AVAX\]"
#
# to skip all "Local" tests
# --ginkgo.skip "\[Local\]"
#
# set "--enable-whitelist-vtx-tests" to explicitly enable/disable whitelist vtx tests
echo "running e2e tests against the local cluster with ${SAVANNAHNODE_PATH}"
./tests/e2e/e2e.test \
--ginkgo.v \
--log-level debug \
--network-runner-grpc-endpoint="0.0.0.0:12342" \
--network-runner-avalanchego-path=${SAVANNAHNODE_PATH} \
--network-runner-avalanchego-log-level="WARN" \
--test-keys-file=tests/test.insecure.secp256k1.keys \
--enable-whitelist-vtx-tests=${ENABLE_WHITELIST_VTX_TESTS} \
&& EXIT_CODE=$? || EXIT_CODE=$?

kill ${PID}

if [[ ${EXIT_CODE} -gt 0 ]]; then
  echo "FAILURE with exit code ${EXIT_CODE}"
  exit ${EXIT_CODE}
else
  echo "ALL SUCCESS!"
fi
