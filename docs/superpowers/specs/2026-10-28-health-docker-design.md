# Health 聚合端点 + d1-2b Docker 部署 设计

日期：2026-10-28
状态：已评审（builder check 一轮），待批准

## 意图

让 OpenDecisionServer 可以通过 `docker compose up` 一条命令部署，默认跑 d1-2b 引擎（GGUF），并提供一个聚合的 `/health` 端点供监控和容器健康检查使用。

## 现状

- `/health/live` 和 `/health/ready` 已存在（`internal/api/http/server.go:41-48`）。
- 现有 `Dockerfile` 绑死 julia/ONNX + linux/amd64，与默认引擎 d1-2b 不符。
- d1 引擎目前仅支持 macOS：cgo 绑定构建标签为 `llama && darwin && arm64 && cgo`（`internal/engine/d1/native_llama.go:1`）；`native/d1/CMakeLists.txt` 硬编码 Metal 和 `@loader_path` RPATH。
- **bridge.cpp 硬性要求 GPU**：`ods_d1_open` 在无 GPU 设备时抛 `"Metal GPU unavailable"`（`native/d1/bridge.cpp:47-48`），纯 CPU Linux 容器必须改这里。
- d1 启动时做完整性校验：模型 SHA-256、SDK 标记（source.sha256 / patch.sha256 / bridge.sha256 / lib/SHA256SUMS），且 `verifySDK` 清单写死 `.dylib` 文件名（`internal/engine/d1/config.go:67`）。
- pinned 输入：llama.cpp 源码 `d05706b4…`（archive SHA-256 `65b13ebd…`）、补丁 `tests/evaluation/patches/llama-d05706b-qwen35-late-crop.patch`、模型 `LiquidAI/d1-2B-GGUF` 的 `d1-2B-Q8_0.gguf`（SHA-256 `073313d9…`，1.8GB），均记录在 `tests/evaluation/liquid-d1-models.json`。
- `cmd/ods/main.go` 的 `nativePaths` 默认是 darwin 路径，容器 ENTRYPOINT 必须显式传 `-llama-model` / `-llama-lib`。
- `.github/workflows/ci.yml` 每次 push/PR 构建 Docker 镜像并跑 30s/64-client odsbench 冒烟。

## 目标

- 新增 `GET /health` 聚合端点。
- 多阶段 `Dockerfile` 从 pinned 源码构建 Linux 版 d1，模型打进镜像。
- 支持 linux/amd64 和 linux/arm64 两种构建（`TARGETARCH`）。
- `docker-compose.yml` 一条命令启动，带 healthcheck。
- 不破坏现有 macOS 本地构建流程和行为（macOS 上无 Metal 仍然硬失败）。

## 非目标 / 反目标

- Linux 镜像为 CPU 推理，不做 CUDA/Metal 容器。
- mmproj（视觉投影）GGUF 不进镜像：文本路径不需要它，bridge 本来就拒绝 mtmd 任务。
- 不做多模型热切换：volume 挂载仅用于替换为**同一个 pinned 模型文件**（启动强校验 SHA-256），不支持挂别的模型。
- 失败条件：镜像构建需要本地产物才能成功；或容器内 `/health/ready` 无法变 200。

## 方案（已选定）

考虑过三种：A) 全源码构建 Linux 镜像；B) 只容器化 julia；C) 仅 linux/arm64。用户选定 **A + 多架构（amd64 + arm64）**。模型分发用户确认**打进镜像**（约 2.3GB 镜像，构建期下载 1.8GB 并校验 SHA-256，Docker 层缓存保证重建不重复下载）。

## 设计细节

### 1. `GET /health` 端点（`internal/api/http/server.go`）

返回 JSON：

```json
{
  "status": "ok",            // "ok" | "unavailable"
  "ready": true,
  "engine": "d1-2b",
  "model": { ... }           // Service.ModelInfo() 的第一个 Listing，未就绪时省略
}
```

- 就绪（`isReady()`）→ 200 + `status: "ok"`；未就绪 → 503 + `status: "unavailable"`。
- 引擎名取 `Service.ModelInfo().ID`（已确认为 `"d1-2b"`，`internal/decision/model.go:12`），无需给 Server 加字段。
- 现有 `/health/live`、`/health/ready` 保留不动。
- `internal/api/http/server_test.go` 补用例：就绪 200、未就绪 503。

### 2. Linux 构建支持（核心改动）

**`native/d1/bridge.cpp`**：编译期平台分流——Apple 保持"无 Metal GPU 硬失败"现状；Linux 走 CPU 路径（不做 GPU 设备检查，`n_gpu_layers=0`）。用 `#ifdef __APPLE__` 或 CMake 定义的宏实现。

**`native/d1/CMakeLists.txt`**：`GGML_METAL` / `GGML_METAL_EMBED_LIBRARY` / RPATH 按 `APPLE` 条件设置；Linux 关闭 Metal、RPATH 用 `$ORIGIN`，其余 FORCE 项不变。

**`internal/engine/d1/native_llama.go`**：构建标签放宽到支持 Linux（纯 dlopen 代码，无需改逻辑）；`native_stub.go` 反向标签同步调整。

**`internal/engine/d1/config.go`**：
- `verifySDK` 的平台分流：macOS 校验 `.dylib` 清单（含 libggml-metal.dylib），Linux 校验 `.so` 清单（无 Metal 库）。
- `BridgeSHA256` 常量在 bridge.cpp / CMakeLists.txt 变更后重新计算更新。**注意**：这会同时使现有 `lib/darwin-arm64/d1/bridge.sha256` 失效，macOS 本地 SDK 需重新生成标记（重跑 `scripts/prepare-d1-native.sh` 或手动重写该文件）。

**`cmd/ods`**：不动代码；容器 ENTRYPOINT 显式传 `-engine d1-2b -llama-model /models/d1-2b/d1-2B-Q8_0.gguf -llama-lib /opt/ods/d1/lib/libods_d1.so`。

### 3. Dockerfile（替换现有）

多阶段：

1. **llama-src**：下载 pinned 源码 tarball（校验 `65b13ebd…`）、打补丁。
2. **bridge-build**：cmake 构建 llama.cpp + `ods_d1` bridge（Linux CPU）；按 `prepare-d1-native.sh` 的布局产出 SDK：`lib/*.so` + `SHA256SUMS` + `source.sha256` + `patch.sha256` + `bridge.sha256` + `LICENSE-llama.cpp`。
3. **go-build**：`CGO_ENABLED=1 go build -tags llama`，`CGO_CFLAGS` 指向 SDK include。
4. **model**：从 HuggingFace 下载 `d1-2B-Q8_0.gguf`（pinned revision `63be75e6…`），校验 SHA-256。
5. **runtime**：`debian:bookworm-slim` + ca-certificates + libgomp1 + libstdc++6 + **curl**（healthcheck 需要，slim 镜像不带）；拷入二进制、`/opt/ods/d1/` SDK、`/models/d1-2b/`；`HEALTHCHECK` 调 `/health/ready`；`EXPOSE 8080`。

多架构策略：优先 `$BUILDPLATFORM` 原生交叉编译（Go 交叉 + CMake 工具链文件）；`ods_d1_error_test` 构建期自检在 `TARGETARCH` ≠ 构建机架构时用 QEMU/binfmt 跑（二进制小、开销可忽略）。**降级预案**：若 ggml 的 x86 CPU 变体交叉编译不可行，amd64 阶段整体退回 QEMU 模拟构建（慢但正确），不允许交付"只编译过、没跑过测试"的产物。

### 4. docker-compose.yml

单服务 `ods`：build 本仓库；`ports: 8080:8080`；healthcheck（`curl -f http://localhost:8080/health/ready`，start_period 120s 给足 CPU 模型加载时间）；`restart: unless-stopped`；注释掉的可选 volume 覆盖 `/models`（仅同 pinned 文件）与资源限制示例。

### 5. CI（我的决定，用户未作答）

保留现有"每次 push/PR 构建 Docker + 冒烟"的行为，给构建加 GitHub Actions 层缓存（`actions/cache` 或 registry cache），缓解 llama.cpp 编译 + 1.8GB 模型下载的开销。附带收益：CI 的 ubuntu-24.04 runner 是 amd64，可作为 linux/amd64 的运行时验证（本机只能验证 arm64）。若缓存未命中的时长不可接受，后续再把 Docker job 改为仅 main/release——这次不动触发条件。

### 6. 验证

- `go build ./...` 与 `go test ./internal/...`（不含需原生库的集成测试）在 macOS 保持绿；macOS 本地 d1 启动行为不变。
- 本机 `docker buildx build --platform linux/arm64` 完整构建、启动容器，`/health`、`/health/ready`、`POST /v1/systemone` 冒烟通过。
- linux/amd64：编译 + 构建期自检通过即交付；运行时验证依赖 CI（在设计中写明此限制）。

## 留给实现者的决定

- 构建标签的具体文件拆分方式（保证 macOS 行为不变）。
- bridge.cpp 平台分流用 `#ifdef __APPLE__` 还是 CMake 宏。
- compose 中资源限制、日志配置的具体默认值。
