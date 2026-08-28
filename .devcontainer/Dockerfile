ARG GO_IMAGE=golang:1.24.13
FROM ${GO_IMAGE}

ENV PATH="/opt/minimega-python/bin:/usr/local/go/bin:/go/bin:${PATH}"

RUN DEBIAN_FRONTEND=noninteractive apt-get update \
  && apt-get install -y --no-install-recommends \
    build-essential \
    libpcap-dev \
    python3-build \
    python3-venv \
    sudo \
  && python3 -m venv --system-site-packages /opt/minimega-python \
  && printf '%s\n' \
    'export PATH="/opt/minimega-python/bin:/usr/local/go/bin:/go/bin:$PATH"' \
    > /etc/profile.d/minimega-path.sh \
  && rm -rf /var/lib/apt/lists/*

ARG USERNAME=vscode
ARG USER_UID=1000
ARG USER_GID=${USER_UID}

RUN groupadd --gid "${USER_GID}" "${USERNAME}" \
  && useradd --uid "${USER_UID}" --gid "${USER_GID}" \
    --create-home --shell /bin/bash "${USERNAME}" \
  && echo "${USERNAME} ALL=(root) NOPASSWD:ALL" \
    > "/etc/sudoers.d/${USERNAME}" \
  && chmod 0440 "/etc/sudoers.d/${USERNAME}"

USER ${USERNAME}
