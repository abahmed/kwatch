 # ── Stage 1: bake the model (full ollama image, build-time only) ──
  FROM --platform=$BUILDPLATFORM ollama/ollama AS bake
  ARG MODEL_VERSION="dev"
  ENV OLLAMA_MODELS=/models
  COPY Modelfile /build/Modelfile
  RUN ollama serve & sleep 3 && \
      ollama pull qwen2.5-coder:1.5b-instruct-q4_K_M && \
      ollama create kwatch-triage -f /build/Modelfile && \
      pkill ollama

  # ── Stage 2: strip GPU runners from the ollama runtime (CPU-only) ──
  FROM ollama/ollama AS prune
  RUN rm -rf /usr/lib/ollama/cuda_* /usr/lib/ollama/*jetpack* /usr/lib/ollama/rocm*

  # ── Stage 3: slim final — CUDA libs never land in these layers ──
  FROM debian:bookworm-slim
  ARG MODEL_VERSION="v1"
  LABEL model.version="$MODEL_VERSION" \
        model.name="kwatch-triage" \
        model.base="qwen2.5-coder:1.5b-instruct-q4_K_M" \
        description="kwatch-triage over Qwen2.5-Coder-1.5B (CPU-only slim)"

  RUN apt-get update && \
      apt-get install -y --no-install-recommends ca-certificates libgomp1 libstdc++6 && \
      rm -rf /var/lib/apt/lists/*

  ENV OLLAMA_MODELS=/models HOME=/home/nonroot

  COPY --from=prune /bin/ollama /bin/ollama
  COPY --from=prune /usr/lib/ollama /usr/lib/ollama

  RUN mkdir -p /home/nonroot/.ollama && \
      echo "nonroot:x:1000:1000:nonroot:/home/nonroot:/bin/sh" >> /etc/passwd && \
      echo "nonroot:x:1000:" >> /etc/group && \
      chown -R 1000:1000 /home/nonroot
  COPY --from=bake --chown=1000:1000 /models /models
  USER 1000:1000

  ENTRYPOINT ["/bin/ollama"]
  CMD ["serve"]
