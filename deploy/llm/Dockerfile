FROM --platform=$BUILDPLATFORM ollama/ollama AS bake

ARG MODEL_VERSION="triage-dev"

ENV OLLAMA_MODELS=/models

COPY Modelfile /build/Modelfile

RUN ollama serve & sleep 3 && \
    ollama pull qwen2.5-coder:1.5b-instruct-q4_K_M && \
    ollama create kwatch-triage -f /build/Modelfile && \
    pkill ollama

FROM ollama/ollama

ARG MODEL_VERSION="triage-dev"

LABEL model.version="$MODEL_VERSION"
LABEL model.name="kwatch-triage"
LABEL model.base="qwen2.5-coder:1.5b-instruct-q4_K_M"
LABEL description="Kubernetes incident triage model — baked Modelfile over Qwen2.5-Coder-1.5B"

ENV OLLAMA_MODELS=/models

COPY --from=bake /models /models
