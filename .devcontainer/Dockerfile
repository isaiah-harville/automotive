FROM mcr.microsoft.com/devcontainers/base:ubuntu-24.04

RUN apt-get update \
    && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
        build-essential \
        ca-certificates \
        can-utils \
        curl \
        gettext-base \
        iproute2 \
        jq \
        make \
        pkg-config \
        shellcheck \
        unzip \
        xz-utils \
    && rm -rf /var/lib/apt/lists/*
