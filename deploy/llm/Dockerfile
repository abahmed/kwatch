# ── Stage 1: download artifacts ──
FROM --platform=$BUILDPLATFORM debian:bookworm-slim AS download

RUN apt-get update && apt-get install -y --no-install-recommends curl ca-certificates && \
    rm -rf /var/lib/apt/lists/*

ARG TARGETOS TARGETARCH
ARG LLAMACPP_VERSION="b5098"

# Map arch to llama.cpp release asset suffix
RUN case "$TARGETARCH" in \
      amd64)  ASSET="ubuntu-x64" ;; \
      arm64)  ASSET="ubuntu-arm64" ;; \
      *)      echo "unsupported arch: $TARGETARCH"; exit 1 ;; \
    esac && \
    curl -fsSL "https://github.com/ggml-org/llama.cpp/releases/download/${LLAMACPP_VERSION}/llama-b${LLAMACPP_VERSION}-bin-${ASSET}.tar.gz" \
    | tar -xz --strip=1 -C /usr/local/bin/ "llama-server"

# MiniCPM5-1B Q4_K_M GGUF from OpenBMB HF repo
RUN mkdir -p /models && \
    curl -fsSL -o /models/model.gguf \
    "https://huggingface.co/openbmb/MiniCPM5-1B-GGUF/resolve/main/MiniCPM5-1B-Q4_K_M.gguf"

# ── Stage 2: slim final ──
FROM debian:bookworm-slim

ARG MODEL_VERSION="v1"
LABEL model.version="$MODEL_VERSION" \
      model.name="kwatch-triage" \
      model.base="minicpm5:1b-instruct-q4_K_M" \
      description="kwatch-triage over MiniCPM5-1B (llama-server CPU)"

RUN apt-get update && \
    apt-get install -y --no-install-recommends libstdc++6 libgomp1 ca-certificates && \
    rm -rf /var/lib/apt/lists/* && \
    adduser --uid 1000 --disabled-password --gecos "" nonroot

COPY --from=download /usr/local/bin/llama-server /usr/local/bin/llama-server
COPY --from=download /models/model.gguf /models/model.gguf

USER nonroot

EXPOSE 8080

ENTRYPOINT ["llama-server"]
CMD ["--host", "127.0.0.1", \
     "--port", "8080", \
     "--model", "/models/model.gguf", \
     "--threads", "1", \
     "--ctx-size", "8192", \
     "--jinja", \
     "--temp", "0.7", \
     "--top-p", "0.95", \
     "--repeat-penalty", "1.1", \
     "--n-predict", "256"]
