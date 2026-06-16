# kwatch-triage model

`kwatch-triage` is a baked Ollama model for kubernetes incident triage. It wraps
Qwen2.5-Coder-1.5B-Instruct (Apache-2.0) with a SYSTEM prompt and few-shot
examples via the `Modelfile`.

## Build

```sh
make llm-image   # local build: ghcr.io/abahmed/kwatch-llm:dev
PUSH=1 make llm-image   # build + push
TAG=v0.11.0 PUSH=1 make llm-image   # release build
```

CI publishes the image with the release version on every GitHub release
(`publish-llm.yml`). The `VERSION` file tracks the model revision (bump when the
Modelfile changes); it's baked as the `MODEL_VERSION` label.

## Run

```sh
docker run --rm -p 11434:11434 ghcr.io/abahmed/kwatch-llm:dev
curl localhost:11434/api/tags   # shows kwatch-triage
```

## License

The base model (Qwen2.5-Coder-1.5B) is Apache-2.0.
The Modelfile and this packaging are under the kwatch project license (MIT).
