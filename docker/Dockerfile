# Base: Debian slim with dev tools and CLIs
# Includes: Go 1.24, Rust (stable), Python, Node, C/C++, Java, jq, git
# Adds user: devuser (uid 1000)
# Default working directory: /src

FROM debian:bookworm-slim AS base

LABEL org.opencontainers.image.source="local" \
      org.opencontainers.image.description="Slim Debian dev image with Go, Rust, Python, Node, C/C++, Java, jq, git, and common AI CLIs"

## Versions and config
ARG DEBIAN_FRONTEND=noninteractive
ARG GO_VERSION=1.24.0
ARG NODE_MAJOR=22
ARG NODE_VERSION=22.9.0

# Ensure predictable shell behavior for RUN chains
SHELL ["/bin/bash", "-o", "pipefail", "-c"]

############################################
# Dev target: Split steps into many layers #
############################################
FROM base AS dev

# System packages in one pass (apt: update + install)
RUN set -eux; \
    apt-get update; \
    icu_pkg=$(apt-cache search -n '^libicu[0-9][0-9]$' | awk '{print $1}' | sort -V | tail -n1); \
    lttng_pkg=$(apt-cache search -n '^liblttng-ust[0-9].*$' | awk '{print $1}' | sort -V | tail -n1); \
    apt-get install -y --no-install-recommends \
       apt-utils ca-certificates curl wget gnupg gnupg2 dirmngr unzip xz-utils tar \
       git jq \
       build-essential pkg-config clang libclang-dev \
       python3 python3-pip python3-venv python3-dev \
       default-jdk \
       openssh-client \
       sshpass \
       coreutils \
       bash-completion iproute2 procps lsof htop net-tools psmisc tree rsync \
       bzip2 zip nano vim-tiny less lsb-release apt-transport-https dialog \
       libc6 libgcc1 libkrb5-3 libgssapi-krb5-2 libstdc++6 zlib1g locales sudo \
       ncdu man-db strace manpages manpages-dev init-system-helpers libssl3 iptables \
       supervisor inotify-tools tinyproxy dnsmasq zsh \
       "$icu_pkg" "$lttng_pkg"; \
    rm -rf /var/lib/apt/lists/*

 # (config moved later to speed rebuilds)

# Locales configuration
ENV LANG=en_US.UTF-8 LC_ALL=en_US.UTF-8 LANGUAGE=en_US:en
RUN set -euxo pipefail && sed -i 's/^# \?en_US.UTF-8 UTF-8/en_US.UTF-8 UTF-8/' /etc/locale.gen
RUN set -euxo pipefail && locale-gen

# Create dev user early to configure shell
 

# Node.js via official tarball + package managers
RUN set -eux; \
    arch=$(dpkg --print-architecture); \
    case "$arch" in amd64) NODEARCH=x64 ;; arm64) NODEARCH=arm64 ;; *) echo "Unsupported architecture: $arch" >&2; exit 1 ;; esac; \
    curl -fsSL "https://nodejs.org/dist/v${NODE_VERSION}/node-v${NODE_VERSION}-linux-${NODEARCH}.tar.xz" -o /tmp/node.tar.xz; \
    tar -xJf /tmp/node.tar.xz -C /usr/local --strip-components=1; \
    rm -f /tmp/node.tar.xz; \
    node --version; \
    npm -g install npm@latest yarn pnpm; \
    npm cache clean --force

# Playwright + Chromium (browsers path global and readable for all users)
ENV PLAYWRIGHT_BROWSERS_PATH=/ms-playwright
RUN set -euxo pipefail && \
    mkdir -p "$PLAYWRIGHT_BROWSERS_PATH" && chmod -R a+rx "$PLAYWRIGHT_BROWSERS_PATH" && \
    npm -g install playwright && \
    npx playwright install --with-deps chromium && \
    chmod -R a+rx "$PLAYWRIGHT_BROWSERS_PATH"

# Go 1.24 (system-wide in /usr/local/go)
ENV PATH="/usr/local/go/bin:${PATH}"
RUN set -euxo pipefail && arch=$(dpkg --print-architecture) && \
    case "$arch" in amd64) GOARCH=amd64 ;; arm64) GOARCH=arm64 ;; *) echo "Unsupported architecture: $arch" >&2; exit 1 ;; esac && \
    curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-${GOARCH}.tar.gz" -o /tmp/go.tgz
RUN set -euxo pipefail && rm -rf /usr/local/go && tar -C /usr/local -xzf /tmp/go.tgz
RUN set -euxo pipefail && rm -f /tmp/go.tgz
RUN set -euxo pipefail && go version

# Rust (stable) via rustup in system dirs so all users see it
ENV RUSTUP_HOME=/usr/local/rustup \
    CARGO_HOME=/usr/local/cargo \
    PATH="/usr/local/cargo/bin:/usr/local/go/bin:${PATH}"
RUN set -euxo pipefail && curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs -o /tmp/rustup.sh
RUN set -euxo pipefail && sh /tmp/rustup.sh -y --default-toolchain stable
RUN set -euxo pipefail && rustup default stable
RUN set -euxo pipefail && chmod -R a+rX /usr/local/rustup /usr/local/cargo
RUN set -euxo pipefail && rustc --version && cargo --version

## Python: keep pip current (no pipx for non-Python CLIs)
RUN set -euxo pipefail && python3 -m pip --version

## AI CLIs via npm (global)
# - OpenAI Codex CLI
# - Google Gemini CLI
# - Claude Code CLI
# - Moonrepo CLI
RUN set -euxo pipefail && npm -g install @openai/codex @google/gemini-cli claude-code @moonrepo/cli && npm cache clean --force

## Users, shells, bootstrap, workspace (late config layer)
RUN set -euxo pipefail && useradd -m -u 1000 -s /bin/bash devuser
RUN set -euxo pipefail && usermod -s /usr/bin/zsh root && usermod -s /usr/bin/zsh devuser
ENV SHELL=/usr/bin/zsh
# Oh My Zsh for root
RUN set -euxo pipefail && \
    export RUNZSH=no CHSH=no KEEP_ZSHRC=yes && \
    curl -fsSL https://raw.githubusercontent.com/ohmyzsh/ohmyzsh/master/tools/install.sh -o /tmp/install-ohmyzsh.sh && \
    sh /tmp/install-ohmyzsh.sh && \
    rm -f /tmp/install-ohmyzsh.sh
# Oh My Zsh for devuser
RUN set -euxo pipefail && \
    git clone --depth=1 https://github.com/ohmyzsh/ohmyzsh.git /home/devuser/.oh-my-zsh && \
    cp /home/devuser/.oh-my-zsh/templates/zshrc.zsh-template /home/devuser/.zshrc && \
    chown -R devuser:devuser /home/devuser/.oh-my-zsh /home/devuser/.zshrc && \
    printf '\nexport HTTP_PROXY=http://127.0.0.1:8888\nexport HTTPS_PROXY=http://127.0.0.1:8888\nexport NO_PROXY=localhost,127.0.0.1,::1\nexport http_proxy=http://127.0.0.1:8888\nexport https_proxy=http://127.0.0.1:8888\nexport no_proxy=localhost,127.0.0.1,::1\n' >> /home/devuser/.zshrc && \
    printf '\n# Auto-cd to /src for interactive zsh\nif [ -t 1 ] && [ -d /src ]; then cd /src; fi\n' >> /home/devuser/.zshrc && \
    printf '# Auto-cd to /src for login zsh\nif [ -d /src ]; then cd /src; fi\n' > /home/devuser/.zprofile && \
    chown devuser:devuser /home/devuser/.zprofile
RUN set -euxo pipefail && printf '\nexport HTTP_PROXY=http://127.0.0.1:8888\nexport HTTPS_PROXY=http://127.0.0.1:8888\nexport NO_PROXY=localhost,127.0.0.1,::1\nexport http_proxy=http://127.0.0.1:8888\nexport https_proxy=http://127.0.0.1:8888\nexport no_proxy=localhost,127.0.0.1,::1\n' >> /home/devuser/.bashrc && \
    printf '\n# Auto-cd to /src for interactive bash\nif [ -t 1 ] && [ -d /src ]; then cd /src; fi\n' >> /home/devuser/.bashrc && \
    printf '# Auto-cd to /src for login bash\nif [ -f ~/.bashrc ]; then . ~/.bashrc; fi\nif [ -d /src ]; then cd /src; fi\n' > /home/devuser/.bash_profile && \
    chown devuser:devuser /home/devuser/.bashrc /home/devuser/.bash_profile
RUN set -euxo pipefail && printf 'export HTTP_PROXY=http://127.0.0.1:8888\nexport HTTPS_PROXY=http://127.0.0.1:8888\nexport NO_PROXY=localhost,127.0.0.1,::1\nexport http_proxy=http://127.0.0.1:8888\nexport https_proxy=http://127.0.0.1:8888\nexport no_proxy=localhost,127.0.0.1,::1\n' > /etc/profile.d/10-proxy.sh && chmod 0644 /etc/profile.d/10-proxy.sh
RUN set -euxo pipefail && printf 'export HTTP_PROXY=http://127.0.0.1:8888\nexport HTTPS_PROXY=http://127.0.0.1:8888\nexport NO_PROXY=localhost,127.0.0.1,::1\nexport http_proxy=http://127.0.0.1:8888\nexport https_proxy=http://127.0.0.1:8888\nexport no_proxy=localhost,127.0.0.1,::1\n' >> /etc/zsh/zshenv
# Bootstrap proxy on root interactive shells
COPY scripts/bootstrap.sh /usr/local/sbin/bootstrap.sh
RUN chmod +x /usr/local/sbin/bootstrap.sh
RUN set -euxo pipefail && \
    printf '\n# Bootstrap services synchronously (root interactive shells) to tighten env before prompt\nif [ -t 1 ] && [ "$(id -u)" -eq 0 ]; then /usr/local/sbin/bootstrap.sh || true; fi\n# Ensure TTY sane in case anything tweaked it (echo, ixon)\nif [ -t 1 ]; then stty sane -ixon 2>/dev/null || true; fi\n' >> /etc/zsh/zshrc && \
    printf '\n# Bootstrap services synchronously (root interactive shells) to tighten env before prompt\nif [ -t 1 ] && [ "$(id -u)" -eq 0 ]; then /usr/local/sbin/bootstrap.sh || true; fi\n# Ensure TTY sane in case anything tweaked it (echo, ixon)\nif [ -t 1 ]; then stty sane -ixon 2>/dev/null || true; fi\n' >> /etc/bash.bashrc
# Tinyproxy/dnsmasq config and allowlist (late)
RUN set -euxo pipefail && \
    mkdir -p /var/log/tinyproxy /var/log/supervisor && \
    touch /var/log/tinyproxy/tinyproxy.log && \
    chown tinyproxy:tinyproxy /var/log/tinyproxy /var/log/tinyproxy/tinyproxy.log && \
    chmod 0644 /var/log/tinyproxy/tinyproxy.log
RUN set -euxo pipefail && sed -i 's/^# \?Port .*/Port 8888/' /etc/tinyproxy/tinyproxy.conf
RUN set -euxo pipefail && \
    if grep -qE '^[# ]*LogFile ' /etc/tinyproxy/tinyproxy.conf; then \
      sed -i -E 's|^[# ]*LogFile .*|LogFile "/var/log/tinyproxy/tinyproxy.log"|' /etc/tinyproxy/tinyproxy.conf; \
    else \
      printf '\nLogFile "/var/log/tinyproxy/tinyproxy.log"\n' >> /etc/tinyproxy/tinyproxy.conf; \
    fi
RUN set -euxo pipefail && \
    if grep -qE '^[# ]*Listen ' /etc/tinyproxy/tinyproxy.conf; then \
      sed -i 's|^[# ]*Listen .*|Listen 127.0.0.1|' /etc/tinyproxy/tinyproxy.conf; \
    else \
      printf '\nListen 127.0.0.1\n' >> /etc/tinyproxy/tinyproxy.conf; \
    fi
RUN set -euxo pipefail && \
    grep -qE '^[# ]*Allow 127\.0\.0\.1' /etc/tinyproxy/tinyproxy.conf || \
    printf 'Allow 127.0.0.1\n' >> /etc/tinyproxy/tinyproxy.conf
RUN set -euxo pipefail && sed -i 's|^# \?User .*$|User tinyproxy|' /etc/tinyproxy/tinyproxy.conf || true
RUN set -euxo pipefail && sed -i 's|^# \?Group .*$|Group tinyproxy|' /etc/tinyproxy/tinyproxy.conf || true
RUN set -euxo pipefail && \
    grep -q '^Filter ' /etc/tinyproxy/tinyproxy.conf || \
    printf '\nFilter "/etc/shai/allowed_domains.conf"\nFilterDefaultDeny Yes\nFilterURLs On\n#FilterExtended On\n' >> /etc/tinyproxy/tinyproxy.conf
RUN set -euxo pipefail && printf '%s\n' \
    'no-resolv' \
    'bind-interfaces' \
    'listen-address=127.0.0.1' \
    'conf-dir=/etc/dnsmasq.d,*.conf' \
    > /etc/dnsmasq.conf
RUN set -euxo pipefail && mkdir -p /etc/dnsmasq.d /etc/shai && chown -R tinyproxy:tinyproxy /etc/shai || true
COPY shai-allowed-domains.conf /etc/shai/allowed_domains.conf
COPY scripts/generate-dnsmasq-allowlist.sh /usr/local/sbin/generate-dnsmasq-allowlist.sh
RUN chmod +x /usr/local/sbin/generate-dnsmasq-allowlist.sh && \
    /usr/local/sbin/generate-dnsmasq-allowlist.sh /etc/shai/allowed_domains.conf /etc/dnsmasq.d/allowlist.conf
# Egress setup and supervisord configs (late)
COPY scripts/dev-egress-setup.sh /usr/local/sbin/dev-egress-setup.sh
RUN chmod +x /usr/local/sbin/dev-egress-setup.sh
COPY supervisord/supervisord.conf /etc/supervisor/supervisord.conf
COPY supervisord/supervisord-daemon.conf /etc/supervisor/supervisord-daemon.conf
COPY supervisord/tinyproxy.conf /etc/supervisor/conf.d/tinyproxy.conf
# Workspace
RUN set -euxo pipefail && mkdir -p /src
WORKDIR /src

# Ensure toolchains are on PATH and add global symlinks (dev stage)
RUN set -euxo pipefail && \
    printf 'export PATH=/usr/local/go/bin:/usr/local/cargo/bin:$PATH\n' > /etc/profile.d/10-lang-paths.sh && \
    chmod 0644 /etc/profile.d/10-lang-paths.sh && \
    printf 'export PATH=/usr/local/go/bin:/usr/local/cargo/bin:$PATH\n' >> /etc/zsh/zshenv && \
    ln -sf /usr/local/go/bin/go /usr/local/bin/go && \
    ln -sf /usr/local/go/bin/gofmt /usr/local/bin/gofmt && \
    ln -sf /usr/local/cargo/bin/cargo /usr/local/bin/cargo && \
    ln -sf /usr/local/cargo/bin/rustc /usr/local/bin/rustc && \
    ln -sf /usr/local/cargo/bin/rustup /usr/local/bin/rustup

# Default user remains root as requested (devuser also available)
# To switch default user, uncomment the next line:
# USER devuser

# Final sanity prints (optional; can be removed to slim image layers)
RUN set -euxo pipefail && node --version
RUN set -euxo pipefail && npm --version
RUN set -euxo pipefail && yarn --version
RUN set -euxo pipefail && pnpm --version
RUN set -euxo pipefail && python3 --version
RUN set -euxo pipefail && java -version || true
RUN set -euxo pipefail && git --version
RUN set -euxo pipefail && jq --version
RUN set -euxo pipefail && codex -h >/dev/null 2>&1 || true
RUN set -euxo pipefail && gemini -h >/dev/null 2>&1 || true
RUN set -euxo pipefail && claude -h >/dev/null 2>&1 || claude-code -h >/dev/null 2>&1 || true

EXPOSE 8888
ENTRYPOINT ["/usr/bin/supervisord","-n","-c","/etc/supervisor/supervisord.conf"]

############################################
# Prod target: Collapse steps into one RUN #
############################################
FROM base AS prod

ENV PATH="/usr/local/go/bin:${PATH}"
ENV RUSTUP_HOME=/usr/local/rustup \
    CARGO_HOME=/usr/local/cargo \
    PATH="/usr/local/cargo/bin:/usr/local/go/bin:${PATH}"

RUN set -euxo pipefail \
    && apt-get update \
    && icu_pkg=$(apt-cache search -n '^libicu[0-9][0-9]$' | awk '{print $1}' | sort -V | tail -n1) \
    && lttng_pkg=$(apt-cache search -n '^liblttng-ust[0-9].*$' | awk '{print $1}' | sort -V | tail -n1) \
    && apt-get install -y --no-install-recommends \
       apt-utils ca-certificates curl wget gnupg gnupg2 dirmngr unzip xz-utils tar \
       git jq bash-completion iproute2 procps lsof htop net-tools psmisc tree rsync \
       bzip2 zip nano vim-tiny less lsb-release apt-transport-https dialog \
       libc6 libgcc1 libkrb5-3 libgssapi-krb5-2 libstdc++6 zlib1g locales sudo \
       ncdu man-db strace manpages manpages-dev init-system-helpers libssl3 zsh iptables supervisor inotify-tools \
       build-essential pkg-config clang libclang-dev \
       python3 python3-pip python3-venv python3-dev \
       default-jdk \
       openssh-client \
       sshpass \
       coreutils \
       tinyproxy \
       "$icu_pkg" "$lttng_pkg" \
    && arch=$(dpkg --print-architecture) \
    && case "$arch" in amd64) GOARCH=amd64 ;; arm64) GOARCH=arm64 ;; *) echo "Unsupported architecture: $arch" >&2; exit 1 ;; esac \
    && curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-${GOARCH}.tar.gz" -o /tmp/go.tgz \
    && rm -rf /usr/local/go \
    && tar -C /usr/local -xzf /tmp/go.tgz \
    && rm -f /tmp/go.tgz \
    && curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs -o /tmp/rustup.sh \
    && sh /tmp/rustup.sh -y --default-toolchain stable \
    && rustup default stable \
    && chmod -R a+rX /usr/local/rustup /usr/local/cargo \
    && python3 -m pip --version \
    && arch=$(dpkg --print-architecture) \
    && case "$arch" in amd64) NODEARCH=x64 ;; arm64) NODEARCH=arm64 ;; *) echo "Unsupported architecture: $arch" >&2; exit 1 ;; esac \
    && curl -fsSL "https://nodejs.org/dist/v${NODE_VERSION}/node-v${NODE_VERSION}-linux-${NODEARCH}.tar.xz" -o /tmp/node.tar.xz \
    && tar -xJf /tmp/node.tar.xz -C /usr/local --strip-components=1 \
    && rm -f /tmp/node.tar.xz \
    && npm -g install npm@latest yarn pnpm \
    && npm -g install @openai/codex @google/gemini-cli claude-code @moonrepo/cli \
    && npm cache clean --force \
    && npm -g install playwright \
    && npx playwright install --with-deps chromium \
    && icu_pkg=$(apt-cache search -n '^libicu[0-9][0-9]$' | awk '{print $1}' | sort -V | tail -n1) \
    && lttng_pkg=$(apt-cache search -n '^liblttng-ust[0-9].*$' | awk '{print $1}' | sort -V | tail -n1) \
    && echo "Installing ICU package: $icu_pkg, LTTng package: $lttng_pkg" \
    && true \
    && sed -i 's/^# \?en_US.UTF-8 UTF-8/en_US.UTF-8 UTF-8/' /etc/locale.gen \
    && locale-gen \
    && useradd -m -u 1000 -s /bin/bash devuser \
    && usermod -s /usr/bin/zsh root \
    && usermod -s /usr/bin/zsh devuser \
    && export RUNZSH=no CHSH=no KEEP_ZSHRC=yes \
    && curl -fsSL https://raw.githubusercontent.com/ohmyzsh/ohmyzsh/master/tools/install.sh -o /tmp/install-ohmyzsh.sh \
    && sh /tmp/install-ohmyzsh.sh \
    && rm -f /tmp/install-ohmyzsh.sh \
    && git clone --depth=1 https://github.com/ohmyzsh/ohmyzsh.git /home/devuser/.oh-my-zsh \
    && cp /home/devuser/.oh-my-zsh/templates/zshrc.zsh-template /home/devuser/.zshrc \
    && chown -R devuser:devuser /home/devuser/.oh-my-zsh /home/devuser/.zshrc \
    && printf '\nexport HTTP_PROXY=http://127.0.0.1:8888\nexport HTTPS_PROXY=http://127.0.0.1:8888\nexport NO_PROXY=localhost,127.0.0.1,::1\nexport http_proxy=http://127.0.0.1:8888\nexport https_proxy=http://127.0.0.1:8888\nexport no_proxy=localhost,127.0.0.1,::1\n' >> /home/devuser/.zshrc \
    && printf '\n# Auto-cd to /src for interactive zsh\nif [ -t 1 ] && [ -d /src ]; then cd /src; fi\n' >> /home/devuser/.zshrc \
    && printf '# Auto-cd to /src for login zsh\nif [ -d /src ]; then cd /src; fi\n' > /home/devuser/.zprofile \
    && printf '\nexport HTTP_PROXY=http://127.0.0.1:8888\nexport HTTPS_PROXY=http://127.0.0.1:8888\nexport NO_PROXY=localhost,127.0.0.1,::1\nexport http_proxy=http://127.0.0.1:8888\nexport https_proxy=http://127.0.0.1:8888\nexport no_proxy=localhost,127.0.0.1,::1\n' >> /home/devuser/.bashrc \
    && printf '\n# Auto-cd to /src for interactive bash\nif [ -t 1 ] && [ -d /src ]; then cd /src; fi\n' >> /home/devuser/.bashrc \
    && printf '# Auto-cd to /src for login bash\nif [ -f ~/.bashrc ]; then . ~/.bashrc; fi\nif [ -d /src ]; then cd /src; fi\n' > /home/devuser/.bash_profile \
    && chown devuser:devuser /home/devuser/.bashrc /home/devuser/.bash_profile /home/devuser/.zprofile \
    && printf 'export HTTP_PROXY=http://127.0.0.1:8888\nexport HTTPS_PROXY=http://127.0.0.1:8888\nexport NO_PROXY=localhost,127.0.0.1,::1\nexport http_proxy=http://127.0.0.1:8888\nexport https_proxy=http://127.0.0.1:8888\nexport no_proxy=localhost,127.0.0.1,::1\n' > /etc/profile.d/10-proxy.sh \
    && chmod 0644 /etc/profile.d/10-proxy.sh \
    && printf 'export HTTP_PROXY=http://127.0.0.1:8888\nexport HTTPS_PROXY=http://127.0.0.1:8888\nexport NO_PROXY=localhost,127.0.0.1,::1\nexport http_proxy=http://127.0.0.1:8888\nexport https_proxy=http://127.0.0.1:8888\nexport no_proxy=localhost,127.0.0.1,::1\n' >> /etc/zsh/zshenv \
    && printf 'export PATH=/usr/local/go/bin:/usr/local/cargo/bin:$PATH\n' > /etc/profile.d/10-lang-paths.sh \
    && chmod 0644 /etc/profile.d/10-lang-paths.sh \
    && printf 'export PATH=/usr/local/go/bin:/usr/local/cargo/bin:$PATH\n' >> /etc/zsh/zshenv \
    && printf '\nexport PATH="/usr/local/go/bin:/usr/local/cargo/bin:$PATH"\n' >> /home/devuser/.zshrc \
    && printf '\nexport PATH="/usr/local/go/bin:/usr/local/cargo/bin:$PATH"\n' >> /home/devuser/.bashrc \
    && sed -i 's/^# \?Port .*/Port 8888/' /etc/tinyproxy/tinyproxy.conf \
    && if grep -qE '^[# ]*Listen ' /etc/tinyproxy/tinyproxy.conf; then \
         sed -i 's|^[# ]*Listen .*|Listen 127.0.0.1|' /etc/tinyproxy/tinyproxy.conf; \
       else \
         printf '\nListen 127.0.0.1\n' >> /etc/tinyproxy/tinyproxy.conf; \
       fi \
    && grep -qE '^[# ]*Allow 127\.0\.0\.1' /etc/tinyproxy/tinyproxy.conf || printf 'Allow 127.0.0.1\n' >> /etc/tinyproxy/tinyproxy.conf \
    && sed -i 's|^# \?User .*$|User tinyproxy|' /etc/tinyproxy/tinyproxy.conf || true \
    && sed -i 's|^# \?Group .*$|Group tinyproxy|' /etc/tinyproxy/tinyproxy.conf || true \
    && grep -q '^Filter ' /etc/tinyproxy/tinyproxy.conf || printf '\nFilter "/etc/shai/allowed_domains.conf"\nFilterDefaultDeny Yes\nFilterURLs On\n#FilterExtended On\n' >> /etc/tinyproxy/tinyproxy.conf \
    && mkdir -p /etc/shai && chown -R tinyproxy:tinyproxy /etc/shai || true \
    && mkdir -p /var/log/tinyproxy && touch /var/log/tinyproxy/tinyproxy.log \
    && chown tinyproxy:tinyproxy /var/log/tinyproxy /var/log/tinyproxy/tinyproxy.log \
    && chmod 0644 /var/log/tinyproxy/tinyproxy.log \
    && if grep -qE '^[# ]*LogFile ' /etc/tinyproxy/tinyproxy.conf; then sed -i -E 's|^[# ]*LogFile .*|LogFile "/var/log/tinyproxy/tinyproxy.log"|' /etc/tinyproxy/tinyproxy.conf; else printf '\nLogFile "/var/log/tinyproxy/tinyproxy.log"\n' >> /etc/tinyproxy/tinyproxy.conf; fi \
    && ln -sf /usr/local/go/bin/go /usr/local/bin/go \
    && ln -sf /usr/local/go/bin/gofmt /usr/local/bin/gofmt \
    && ln -sf /usr/local/cargo/bin/cargo /usr/local/bin/cargo \
    && ln -sf /usr/local/cargo/bin/rustc /usr/local/bin/rustc \
    && ln -sf /usr/local/cargo/bin/rustup /usr/local/bin/rustup \
    && mkdir -p /src \
    && rm -rf /var/lib/apt/lists/* \
    && node --version && npm --version && yarn --version && pnpm --version \
    && go version \
    && rustc --version && cargo --version \
    && python3 --version \
    && java -version || true \
    && git --version && jq --version \
    && codex -h >/dev/null 2>&1 || true \
    && gemini -h >/dev/null 2>&1 || true \
    && claude -h >/dev/null 2>&1 || claude-code -h >/dev/null 2>&1 || true

COPY scripts/dev-egress-setup.sh /usr/local/sbin/dev-egress-setup.sh
RUN chmod +x /usr/local/sbin/dev-egress-setup.sh
COPY supervisord/supervisord.conf /etc/supervisor/supervisord.conf
COPY supervisord/tinyproxy.conf /etc/supervisor/conf.d/tinyproxy.conf
COPY shai-allowed-domains.conf /etc/shai/allowed_domains.conf
EXPOSE 8888
ENTRYPOINT ["/usr/bin/supervisord","-n","-c","/etc/supervisor/supervisord.conf"]
ENV PLAYWRIGHT_BROWSERS_PATH=/ms-playwright
RUN set -euxo pipefail && mkdir -p "$PLAYWRIGHT_BROWSERS_PATH" && chmod -R a+rx "$PLAYWRIGHT_BROWSERS_PATH"
RUN set -euxo pipefail && \
    printf 'export PATH=/usr/local/go/bin:/usr/local/cargo/bin:$PATH\n' > /etc/profile.d/10-lang-paths.sh && \
    chmod 0644 /etc/profile.d/10-lang-paths.sh && \
    printf 'export PATH=/usr/local/go/bin:/usr/local/cargo/bin:$PATH\n' >> /etc/zsh/zshenv && \
    printf '\nexport PATH="/usr/local/go/bin:/usr/local/cargo/bin:$PATH"\n' >> /home/devuser/.zshrc && \
    printf '\nexport PATH="/usr/local/go/bin:/usr/local/cargo/bin:$PATH"\n' >> /home/devuser/.bashrc && \
    chown devuser:devuser /home/devuser/.bashrc /home/devuser/.zshrc && \
    ln -sf /usr/local/go/bin/go /usr/local/bin/go && \
    ln -sf /usr/local/go/bin/gofmt /usr/local/bin/gofmt && \
    ln -sf /usr/local/cargo/bin/cargo /usr/local/bin/cargo && \
    ln -sf /usr/local/cargo/bin/rustc /usr/local/bin/rustc && \
    ln -sf /usr/local/cargo/bin/rustup /usr/local/bin/rustup
