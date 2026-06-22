FROM --platform=$BUILDPLATFORM ollama/ollama AS bake

ARG MODEL_VERSION="v1"

ENV OLLAMA_MODELS=/models

COPY Modelfile /build/Modelfile

RUN ollama serve & sleep 3 && \
    ollama pull qwen2.5-coder:1.5b-instruct-q4_K_M && \
    ollama create kwatch-triage -f /build/Modelfile && \
    pkill ollama

FROM ollama/ollama

ARG MODEL_VERSION="v1"

LABEL model.version="$MODEL_VERSION"
LABEL model.name="kwatch-triage"
LABEL model.base="qwen2.5-coder:1.5b-instruct-q4_K_M"
LABEL description="Kubernetes incident triage model — baked Modelfile over Qwen2.5-Coder-1.5B"

ENV OLLAMA_MODELS=/models HOME=/home/nonroot

RUN mkdir -p /home/nonroot && \
    echo "nonroot:x:1000:1000:nonroot:/home/nonroot:/bin/sh" >> /etc/passwd && \
    echo "nonroot:x:1000:" >> /etc/group
COPY --from=bake --chown=1000:1000 /models /models
USER 1000:1000
