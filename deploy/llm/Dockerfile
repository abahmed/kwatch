# syntax=docker/dockerfile:1

FROM debian:bookworm-slim AS model

RUN apt-get update && \
    apt-get install -y --no-install-recommends \
        curl \
        ca-certificates && \
    rm -rf /var/lib/apt/lists/*

RUN mkdir -p /models && \
    curl -fL \
      "https://huggingface.co/openbmb/MiniCPM5-1B-GGUF/resolve/main/MiniCPM5-1B-Q4_K_M.gguf" \
      -o /models/model.gguf


FROM ghcr.io/ggml-org/llama.cpp:server

ARG MODEL_VERSION=v1

LABEL model.version="${MODEL_VERSION}" \
      model.name="kwatch-triage" \
      model.base="MiniCPM5-1B-Q4_K_M" \
      description="kwatch-triage over MiniCPM5-1B using llama.cpp"

COPY --from=model /models/model.gguf /models/model.gguf

EXPOSE 8080

ENTRYPOINT ["/app/llama-server"]

CMD ["--host","0.0.0.0","--port","8080","--model","/models/model.gguf","--threads","1","--ctx-size","4096","--jinja","--temp","0.7","--top-p","0.95","--repeat-penalty","1.1","--n-predict","256", "--chat-template-kwargs", "{\"enable_thinking\":false}"]
