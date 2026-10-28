# d1-2B CPU inference image. Multi-arch via buildx (linux/amd64, linux/arm64).
# All inputs are pinned and hash-verified; see tests/evaluation/liquid-d1-models.json.

FROM debian:bookworm-slim AS llama-src
RUN apt-get update && apt-get install -y --no-install-recommends curl ca-certificates patch \
    && rm -rf /var/lib/apt/lists/*
ARG LLAMA_REVISION=d05706b4f32a545814856e98ca518292585443b9
ARG LLAMA_ARCHIVE_SHA256=65b13ebdf4e03cefe58f27096e9af698dc3d41ec6f63f6cd9b4a32ce02e525cf
RUN curl -fL --retry 3 -o /tmp/llama.tar.gz "https://codeload.github.com/ggml-org/llama.cpp/tar.gz/${LLAMA_REVISION}" \
    && echo "${LLAMA_ARCHIVE_SHA256}  /tmp/llama.tar.gz" | sha256sum -c - \
    && mkdir -p /llama \
    && tar -xzf /tmp/llama.tar.gz --strip-components=1 -C /llama \
    && rm /tmp/llama.tar.gz
COPY tests/evaluation/patches/llama-d05706b-qwen35-late-crop.patch /tmp/d1.patch
RUN cd /llama && patch --batch --fuzz=0 -p1 -i /tmp/d1.patch \
    && echo "${LLAMA_ARCHIVE_SHA256}" > /llama-source.sha256 \
    && sha256sum /tmp/d1.patch | cut -d' ' -f1 > /llama-patch.sha256

FROM debian:bookworm-slim AS bridge-build
RUN apt-get update && apt-get install -y --no-install-recommends cmake g++ make \
    && rm -rf /var/lib/apt/lists/*
COPY --from=llama-src /llama /llama
COPY --from=llama-src /llama-source.sha256 /llama-patch.sha256 /markers/
COPY native/d1 /native/d1
RUN cmake -S /native/d1 -B /build -DLLAMA_SOURCE_DIR=/llama -DGGML_NATIVE=OFF \
    && cmake --build /build --target ods_d1 ods_d1_error_test -j "$(nproc)"
RUN /build/bin/ods_d1_error_test
RUN mkdir -p /sdk/lib \
    && cp -a /build/bin/*.so* /sdk/lib/ \
    && cd /sdk/lib && sha256sum $(ls | sort) > SHA256SUMS \
    && cp /markers/llama-source.sha256 /sdk/source.sha256 \
    && cp /markers/llama-patch.sha256 /sdk/patch.sha256 \
    && cat /native/d1/bridge.cpp /native/d1/bridge.h /native/d1/CMakeLists.txt | sha256sum | cut -d' ' -f1 > /sdk/bridge.sha256 \
    && cp /llama/LICENSE /sdk/LICENSE-llama.cpp

FROM golang:1.27-bookworm AS go-build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=1 go build -tags llama -o /out/ods ./cmd/ods

FROM debian:bookworm-slim AS model
RUN apt-get update && apt-get install -y --no-install-recommends curl ca-certificates \
    && rm -rf /var/lib/apt/lists/*
ARG D1_MODEL_REVISION=63be75e6586882defeb706ed4bda812d90256a6d
ARG D1_MODEL_SHA256=073313d92b53b0ac8193db6e906cb3c78a98183d244b15b7b8f5426115c6738c
RUN curl -fL --retry 3 -o /d1-2B-Q8_0.gguf "https://huggingface.co/LiquidAI/d1-2B-GGUF/resolve/${D1_MODEL_REVISION}/d1-2B-Q8_0.gguf" \
    && echo "${D1_MODEL_SHA256}  /d1-2B-Q8_0.gguf" | sha256sum -c -

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates libstdc++6 curl \
    && rm -rf /var/lib/apt/lists/*
COPY --from=go-build /out/ods /usr/local/bin/ods
COPY --from=bridge-build /sdk /opt/ods/d1
COPY --from=model /d1-2B-Q8_0.gguf /models/d1-2b/d1-2B-Q8_0.gguf
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s --start-period=120s CMD curl -fsS http://127.0.0.1:8080/health/ready || exit 1
ENTRYPOINT ["/usr/local/bin/ods","-engine","d1-2b","-http",":8080","-llama-model","/models/d1-2b/d1-2B-Q8_0.gguf","-llama-lib","/opt/ods/d1/lib/libods_d1.so"]
