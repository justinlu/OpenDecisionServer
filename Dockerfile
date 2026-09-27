FROM --platform=linux/amd64 golang:1.27-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN ./scripts/download-ort.sh linux amd64 && CGO_ENABLED=1 go build -o /out/ods ./cmd/ods

FROM --platform=linux/amd64 debian:bookworm-slim
WORKDIR /app
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates libstdc++6 libgomp1 \
    && rm -rf /var/lib/apt/lists/*
COPY --from=build /out/ods /usr/local/bin/ods
COPY --from=build /src/lib/linux-amd64 /opt/ods/lib
COPY models/julia-1 /models/julia-1
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/ods","-http",":8080","-model","/models/julia-1/model.onnx","-tokenizer","/models/julia-1/tokenizer.json","-ort-lib","/opt/ods/lib/libonnxruntime.so"]
