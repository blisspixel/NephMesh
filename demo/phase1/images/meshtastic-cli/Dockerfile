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

# The Meshtastic CLI and PyYAML, installed once at build time and pinned, so
# nothing in the demo pulls from PyPI at runtime. This removes three
# problems the runtime-pip approach had: supply-chain exposure on every Job
# run, dependence on flaky pod egress, and the need for internet access from
# workload pods. Phase 4 publishes the equivalent image to a registry with a
# digest pin; the demo builds it locally and loads it into the cluster.

FROM python:3.13-slim

ARG MESHTASTIC_VERSION=2.7.11
ARG PYYAML_VERSION=6.0.2

# Install as an unprivileged user's site packages so the running container
# never needs root and matches the pods' runAsNonRoot securityContext.
RUN useradd --create-home --uid 65532 app
USER 65532
ENV HOME=/home/app \
    PATH=/home/app/.local/bin:/usr/local/bin:/usr/bin:/bin
RUN pip install --no-cache-dir --user \
        "meshtastic[cli]==${MESHTASTIC_VERSION}" \
        "PyYAML==${PYYAML_VERSION}"

# A no-op default; the Job and demo scripts set their own commands.
ENTRYPOINT ["meshtastic"]
CMD ["--help"]
