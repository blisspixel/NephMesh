# Copyright 2026 The NephMesh Authors.
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

# A reproducible environment for `kpt fn render`. kpt needs git on PATH and,
# to run KRM functions, the docker CLI plus a mounted Docker socket. Pinning
# kpt here means local Windows or macOS runs and CI render the packages with
# the exact same toolchain. Base is docker:cli (Alpine with the docker CLI).

FROM docker:28-cli

ARG KPT_VERSION=v1.0.0-beta.67
RUN apk add --no-cache git curl \
    && curl -sL "https://github.com/kptdev/kpt/releases/download/${KPT_VERSION}/kpt_linux_amd64-${KPT_VERSION#v}.tar.gz" \
       | tar -xz -C /usr/local/bin kpt \
    && kpt version

WORKDIR /work
ENTRYPOINT ["kpt"]
CMD ["fn", "render", "--help"]
