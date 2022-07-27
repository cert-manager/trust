#!/usr/bin/env bash

# Copyright 2021 The cert-manager Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -o errexit
set -o nounset
set -o pipefail

KUBE_ROOT=$(dirname "${BASH_SOURCE}")/..

HELM_DOCS_BIN="${KUBE_ROOT}/bin/helm-docs"

TEMP_FILE=$(mktemp)
trap '{ rm -f -- "$TEMP_FILE"; }' EXIT

$HELM_DOCS_BIN ${KUBE_ROOT}/deploy/charts/trust -d -l error > ${TEMP_FILE}

if ! cmp -s "${KUBE_ROOT}/deploy/charts/trust/README.md" "${TEMP_FILE}"; then
  echo "Helm chart README.md is out of date."
  echo "Please run './hack/update-helm-docs.sh'."
  exit 1
fi

