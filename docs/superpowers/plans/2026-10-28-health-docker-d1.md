# Health 聚合端点 + d1-2b Docker 部署 实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 新增聚合 `GET /health` 端点，并把 Docker 部署从 julia/ONNX 切换为 d1-2b（GGUF，Linux CPU），支持 linux/amd64 + linux/arm64，附带 docker-compose.yml。

**Architecture:** d1 引擎通过 dlopen 加载 pinned llama.cpp bridge DSO。本计划给 bridge/CMake/cgo 构建标签加 Linux 平台分流（macOS 行为不变），然后用多阶段 Dockerfile 从 pinned 源码完整构建 Linux SDK、Go 二进制和 pinned 模型，compose 单服务暴露 8080。

**Tech Stack:** Go 1.27 (cgo/dlopen)、CMake、llama.cpp d05706b（pinned + patch）、Docker buildx、docker compose。

**Spec:** `docs/superpowers/specs/2026-10-28-health-docker-design.md`

## Global Constraints

- pinned llama.cpp 源码 archive SHA-256：`65b13ebdf4e03cefe58f27096e9af698dc3d41ec6f63f6cd9b4a32ce02e525cf`（revision `d05706b4f32a545814856e98ca518292585443b9`）
- pinned 补丁：`tests/evaluation/patches/llama-d05706b-qwen35-late-crop.patch`，SHA-256 `b57f2ca9d79d57af1aa425c4cf0f308ae2d317a89f5085cb3d9ca527f50b79ae`
- pinned 模型：`https://huggingface.co/LiquidAI/d1-2B-GGUF/resolve/63be75e6586882defeb706ed4bda812d90256a6d/d1-2B-Q8_0.gguf`，SHA-256 `073313d92b53b0ac8193db6e906cb3c78a98183d244b15b7b8f5426115c6738c`
- 镜像内路径：SDK `/opt/ods/d1/`（`lib/*.so*` + 标记文件在其上一层）、模型 `/models/d1-2b/d1-2B-Q8_0.gguf`
- 不新增任何 Go 第三方依赖；macOS 本地 d1 构建与运行行为不变（无 Metal 仍硬失败）
- Dockerfile 不得依赖 `dist/`、`lib/`、`bin/` 等本地产物（.dockerignore 已排除，必须全部在构建期内下载/编译）
- Go 构建标签语法支持括号：`//go:build llama && cgo && ((darwin && arm64) || linux)` 是合法表达式

## Review Focus

1. **CPU 推理延迟**：容器内 2B Q8 纯 CPU 推理比 Metal 慢一个数量级，CI 的 64-client 冒烟和调用方客户端都可能遇到长延迟——期望是请求最终成功而非崩溃或 hang。→ Task 3 Step 4 的真实 POST 冒烟钉住。
2. **模型加载窗口期**：启动后数秒内 `/health` 与 `/v1/systemone` 必须返回 503 而不是 hang 或 200。→ Task 1 Step 1 的未就绪用例钉住。
3. **Linux SDK 含 symlink 的 `.so`**：`libggml.so` 等是指向 `libggml.so.0.27.0` 的符号链接，SHA256SUMS 对链接和目标各记一行（哈希相同），verifySDK 必须接受。→ Task 2 Step 6 的 fake-SDK 测试钉住。
4. **amd64 本机无法运行验证**：只能编译 + 构建期自检，运行时验证交给 CI。→ Task 5 Step 3 钉住 CI 路径。
5. **healthcheck start_period 不足**：CPU 模型加载慢于 Metal，start_period 太短会让 compose 误报 unhealthy。→ Task 4 Step 2 的启动验证钉住。

---

### Task 1: 聚合 `GET /health` 端点

**Files:**
- Modify: `internal/api/http/server.go`（Handler() 内注册；新增 health 方法）
- Test: `internal/api/http/server_test.go`

**Interfaces:**
- Consumes: 现有 `Server.isReady()`、`Server.Service.ModelInfo()`（`decision.Model`，`internal/decision/model.go:48`）
- Produces: `GET /health`：就绪 → 200 `{"status":"ok","ready":true,"engine":<ModelInfo().ID>,"model":<ModelInfo().Listings()[0]>}`；未就绪 → 503 `{"status":"unavailable","ready":false}`（无 engine/model 字段）

- [ ] **Step 1: 写失败测试**

在 `server_test.go` 追加（风格对齐现有测试，fixedEngine 已存在）：

```go
func TestHealthAggregatedEndpoint(t *testing.T) {
	api := &Server{Service: &decision.Service{Engine: fixedEngine{}, Admission: decision.NewAdmission(4, 128)}, MaxQuestions: 64}
	api.SetReady(true)
	handler := api.Handler()
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest("GET", "/health", nil))
	if w.Code != 200 {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["status"] != "ok" || body["ready"] != true || body["engine"] != "julia-1" {
		t.Fatalf("body: %v", body)
	}
	if _, ok := body["model"].(map[string]any)["name"]; !ok {
		t.Fatalf("model: %v", body["model"])
	}
	api.SetReady(false)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest("GET", "/health", nil))
	if w.Code != 503 {
		t.Fatalf("status %d", w.Code)
	}
	body = map[string]any{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["status"] != "unavailable" || body["ready"] != false {
		t.Fatalf("body: %v", body)
	}
	if _, present := body["engine"]; present {
		t.Fatalf("engine must be omitted when unavailable: %v", body)
	}
}
```

注意：`Service.Model` 为零值时 `ModelInfo()` 返回 `JuliaModel()`，所以 engine 断言为 `"julia-1"`。

- [ ] **Step 2: 跑测试确认失败**

Run: `go test ./internal/api/http -run TestHealthAggregatedEndpoint -v`
Expected: FAIL（404，路由不存在）

- [ ] **Step 3: 实现 health handler**

在 `server.go` 的 `Handler()` 中 `/health/ready` 之后注册 `mux.HandleFunc("GET /health", s.health)`，并新增 `func (s *Server) health(w http.ResponseWriter, _ *http.Request)`：未就绪写 503 `{"status":"unavailable","ready":false}`；就绪从 `s.Service.ModelInfo()` 取 ID 与 `Listings()[0]`，写 200。复用现有 `writeJSON`。

- [ ] **Step 4: 跑测试确认通过**

Run: `go test ./internal/api/http -v`
Expected: PASS（含全部既有用例）

- [ ] **Step 5: Commit**

```bash
git add internal/api/http/server.go internal/api/http/server_test.go
git commit -m "feat: add aggregated GET /health endpoint"
```

---

### Task 2: d1 引擎 Linux 平台支持

**Files:**
- Modify: `native/d1/bridge.cpp:47-51`（GPU 硬性要求段）
- Modify: `native/d1/CMakeLists.txt:7-8,16-17`（RPATH 与 Metal）
- Modify: `internal/engine/d1/native_llama.go:1`、`internal/engine/d1/native_stub.go:1,8`（构建标签与报错文案）
- Modify: `internal/engine/d1/config.go:13,67`（BridgeSHA256 常量与 verifySDK 平台清单）
- Test: `internal/engine/d1/config_test.go`（追加 fake-SDK 用例）

**Interfaces:**
- Consumes: 无（本任务是平台层）
- Produces: `verifySDK` 在 linux 接受 `.so` 清单；`BridgeSHA256` 新值；linux 下 `-tags llama` 可编译 d1 cgo 绑定（`openNative(Options) (native, error)` 签名不变）

- [ ] **Step 1: bridge.cpp 平台分流**

将 `ods_d1_open` 中的 GPU 检查（47-49 行）与 `mp.devices`/`mp.n_gpu_layers=99`（51 行）包进 `#if defined(__APPLE__)`，`#else` 分支：`r->gpu = "cpu"`，`mp.n_gpu_layers = 0`，不设置 `mp.devices`。其余逻辑（模型加载、decision type 校验、context 参数、错误处理）不动。macOS 行为逐字节不变。

- [ ] **Step 2: CMakeLists.txt 平台分流**

把 `GGML_METAL`、`GGML_METAL_EMBED_LIBRARY` 两个 FORCE set 和 `CMAKE_INSTALL_RPATH "@loader_path"` 移入 `if(APPLE)` 块；`else()` 分支设 `set(CMAKE_INSTALL_RPATH "$ORIGIN")`。其余 FORCE 项（`BUILD_SHARED_LIBS`、`LLAMA_BUILD_SERVER` 等）保持全局。

- [ ] **Step 3: 重算并更新 BridgeSHA256**

Run: `cat native/d1/bridge.cpp native/d1/bridge.h native/d1/CMakeLists.txt | shasum -a 256`
把输出填入 `config.go:13` 的 `BridgeSHA256`。
Run: `go test ./internal/engine/d1 -run TestBridgeSourceIdentity -v`
Expected: PASS（该测试用同样顺序哈希 native 目录三个文件，与常量比对）

- [ ] **Step 4: 放宽 cgo 构建标签**

`native_llama.go:1` 改为 `//go:build llama && cgo && ((darwin && arm64) || linux)`；`native_stub.go:1` 改为 `//go:build !llama || !cgo || ((!darwin || !arm64) && !linux)`，并把 stub 报错文案改为 `d1-2B requires cgo on darwin/arm64 or linux: see Dockerfile or scripts/build-d1.sh`。`native_integration_test.go` 的标签保持 `llama && darwin && arm64 && cgo` 不变。
Run: `go build ./... && go vet ./... && GOOS=linux GOARCH=amd64 go build ./internal/engine/d1`（后者走 stub 验证标签互斥无冲突，cgo 关时 linux 也能编译）
Expected: 全部成功

- [ ] **Step 5: 写 verifySDK 平台分流的失败测试**

在 `config_test.go` 追加 `TestVerifySDKAcceptsPlatformManifest`：在 `t.TempDir()` 构造 fake SDK——上层写 `source.sha256`/`patch.sha256`/`bridge.sha256`（值取对应常量），`lib/` 下按 `runtime.GOOS` 创建空文件：`darwin` 为 `libods_d1.dylib libllama.dylib libggml.dylib libggml-base.dylib libggml-metal.dylib`，`linux` 为 `libods_d1.so libllama.so libggml.so libggml-base.so libggml-cpu.so libllama-common.so libmtmd.so`，另加一个 versioned 副本（如 linux 的 `libggml.so.0.27.0`）模拟符号链接展开后的重复哈希行；写 `SHA256SUMS`（对每文件算 sha256）；断言 `verifySDK(<lib 下 bridge 库路径>)` 返回 nil。

- [ ] **Step 6: 跑测试确认失败**

Run: `go test ./internal/engine/d1 -run TestVerifySDKAcceptsPlatformManifest -v`（linux 清单场景在本机可用 `GOOS` 不可换，测试内按 `runtime.GOOS` 选清单，故本机验证 darwin 分支；linux 分支由 Docker 容器启动验证——见 Task 3）
Expected: 本机 darwin 分支当前就应 PASS（清单与现状一致）；若 FAIL 说明 fake SDK 构造有误，先修测试。真正的"失败驱动"体现在：把测试清单临时换成 linux 名会 FAIL（现状校验写死 .dylib）——实现后用条件编译/GOOS 分流使两边都过。

- [ ] **Step 7: 实现 verifySDK 平台分流**

`config.go` 把 67 行写死的清单换成按 `runtime.GOOS` 选择：`darwin` 保持现有 5 个 `.dylib` 名；`linux` 为 Step 5 列出的 7 个 `.so` 名；其他 GOOS 返回明确错误。manifest 解析与逐文件哈希逻辑不动。
Run: `go test ./internal/engine/d1 -v`
Expected: PASS

- [ ] **Step 8: 重新生成 macOS SDK 标记并验证本地行为不变**

BridgeSHA256 变了，本机 `lib/darwin-arm64/d1/bridge.sha256` 已失效。
Run: `bash scripts/prepare-d1-native.sh && bash scripts/build-d1.sh`
Expected: 构建成功；随后 `bin/ods-metal -engine d1-2b` 能启动且 `/health/ready` 变 200（模型在 `dist/liquid-d1/models/`，已在 .dockerignore 外）。若 prepare 脚本耗时过长，至少必须完成标记重生成 + `go build -tags llama` 成功。

- [ ] **Step 9: Commit**

```bash
git add native/d1 internal/engine/d1
git commit -m "feat: support linux cpu build for d1 engine"
```

---

### Task 3: 多阶段 Dockerfile（d1-2b）

**Files:**
- Modify: `Dockerfile`（整体替换）

**Interfaces:**
- Consumes: Task 2 的 Linux 构建能力；pinned 值见 Global Constraints
- Produces: 镜像 `ods`，`docker run -p 8080:8080 ods` 可直接服务；ENTRYPOINT：`/usr/local/bin/ods -engine d1-2b -http :8080 -llama-model /models/d1-2b/d1-2B-Q8_0.gguf -llama-lib /opt/ods/d1/lib/libods_d1.so`

- [ ] **Step 1: 写 Dockerfile**

五个阶段，不用 `$BUILDPLATFORM` 交叉技巧（跨架构由 buildx/QEMU 承担，慢但正确）：

1. `llama-src`（`debian:bookworm-slim` + `curl ca-certificates patch`）：下载 `https://codeload.github.com/ggml-org/llama.cpp/tar.gz/d05706b4f32a545814856e98ca518292585443b9`，校验 archive SHA-256（Global Constraints），解到 `/llama`；COPY 补丁文件并 `patch --batch --fuzz=0 -p1`。
2. `bridge-build`（`debian:bookworm-slim` + `cmake g++ make`）：`COPY --from=llama-src` 源码 + `COPY native/d1`；`cmake -S native/d1 路径 -B build -DLLAMA_SOURCE_DIR=/llama`，`cmake --build build --target ods_d1 ods_d1_error_test -j`；执行 `ods_d1_error_test` 自检；按 `scripts/prepare-d1-native.sh` 的布局组装 `/sdk`：`lib/`（全部 `*.so*`）、`SHA256SUMS`、`source.sha256`、`patch.sha256`、`bridge.sha256`（内容为 Task 2 的新常量）、`LICENSE-llama.cpp`。注意 `sha256sum` 会跟随符号链接，versioned 与 unversioned 名各占一行，与 darwin SDK 行为一致。
3. `go-build`（`golang:1.27-bookworm`）：`go mod download` 后 COPY 全部源码，`CGO_ENABLED=1 go build -tags llama -o /out/ods ./cmd/ods`（cgo 只需 `native/d1/bridge.h` 与系统 dlfcn.h，不需要 SDK 头文件；二进制运行时才 dlopen）。
4. `model`（`debian:bookworm-slim` + `curl ca-certificates`）：下载 pinned GGUF（URL 见 Global Constraints），`sha256sum -c` 校验。
5. runtime（`debian:bookworm-slim` + `ca-certificates libstdc++6 curl`，`--no-install-recommends` + 清 apt 缓存）：COPY 二进制到 `/usr/local/bin/ods`、`/sdk` 到 `/opt/ods/d1`、模型到 `/models/d1-2b/`；`EXPOSE 8080`；`HEALTHCHECK --interval=30s --timeout=5s --start-period=120s CMD curl -fsS http://127.0.0.1:8080/health/ready || exit 1`；ENTRYPOINT 见 Interfaces。

- [ ] **Step 2: 构建 linux/arm64 镜像（本机原生架构）**

Run: `docker buildx build --platform linux/arm64 -t ods:d1 --load .`
Expected: 成功；`ods_d1_error_test` 阶段输出通过。首次构建含 llama.cpp 编译 + 1.8GB 下载，耐心等。

- [ ] **Step 3: 容器启动 + 就绪探针**

Run: `docker run -d --name ods-d1 -p 18080:8080 ods:d1`，然后轮询 `curl -s http://127.0.0.1:18080/health/ready` 直到 200。
Expected: 启动初期 `/health` 返回 503 `unavailable`，模型加载完成后 `/health` 返回 200 且含 `"engine":"d1-2b"`，`/health/ready` 200。若启动失败，`docker logs ods-d1` 排查（最常见：SDK 清单缺 `.so` 名 → 回 Task 2 Step 7 调整清单）。

- [ ] **Step 4: 真实推理冒烟（Review Focus #1）**

Run: `curl -s -X POST http://127.0.0.1:18080/v1/systemone -H 'Content-Type: application/json' -d '{"model":"d1-2b","state":"user asks about refund","questions":{"route":{"type":"choice","criteria":{"billing":"refund or payment","support":"technical help"}}}}'`
Expected: 200 且 answers.route.choice 为 `billing` 或 `support` 之一（CPU 上可能需十几秒，属预期）。完成后 `docker rm -f ods-d1`。

- [ ] **Step 5: amd64 编译验证（QEMU，仅验证能构建 + 构建期自检）**

Run: `docker buildx build --platform linux/amd64 -t ods:d1-amd64 .`（不 --load 到本机运行，或 load 后不跑推理）
Expected: 构建与 `ods_d1_error_test` 通过。此步很慢（QEMU 编译），可用 `--target bridge-build` 提前验证关键阶段；运行时验证留给 CI（Review Focus #4）。

- [ ] **Step 6: Commit**

```bash
git add Dockerfile
git commit -m "feat: multi-stage dockerfile for d1-2b cpu inference"
```

---

### Task 4: docker-compose.yml

**Files:**
- Create: `docker-compose.yml`

**Interfaces:**
- Consumes: Task 3 的镜像构建
- Produces: `docker compose up --build` 一条命令起服务于 8080

- [ ] **Step 1: 写 docker-compose.yml**

单服务 `ods`：`build: .`；`ports: ["8080:8080"]`；`restart: unless-stopped`；healthcheck：`test: ["CMD","curl","-fsS","http://127.0.0.1:8080/health/ready"]`、`interval: 30s`、`timeout: 5s`、`start_period: 120s`、`retries: 5`；文件尾部放注释掉的示例：volume 覆盖 `/models`（注明仅接受同一 pinned 模型文件）与 `deploy.resources` 限制。

- [ ] **Step 2: 校验并启动验证（Review Focus #5）**

Run: `docker compose config -q && docker compose up -d --build && docker compose ps`
Expected: 配置合法；容器在 start_period 内从 `starting` 变为 `healthy`（CPU 加载 1.8GB 模型，若超 120s 则上调 start_period 后重验）。然后 `curl -s http://127.0.0.1:8080/health` 见 `"engine":"d1-2b"`。`docker compose down` 收尾。

- [ ] **Step 3: Commit**

```bash
git add docker-compose.yml
git commit -m "feat: add docker-compose for one-command deployment"
```

---

### Task 5: CI 缓存与 README 更新

**Files:**
- Modify: `.github/workflows/ci.yml:29-43`
- Modify: `README.md:176-180`（Docker 章节）

**Interfaces:**
- Consumes: Task 3 的 Dockerfile
- Produces: CI 的 "Build Linux container" 步骤带 GHA 层缓存；README 的 Docker 说明与实际一致

- [ ] **Step 1: CI 加 buildx + 缓存**

在 "Build Linux container" 前加 `docker/setup-buildx-action@v3`；构建改为 `docker buildx build --platform linux/amd64 -t ods-ci --load --cache-from type=gha --cache-to type=gha,mode=max .`。就绪轮询从 90 次改为 300 次（CPU 模型加载慢）。`odsbench` 冒烟参数不动。

- [ ] **Step 2: README 更新**

把 176-180 行的 Docker 章节改写为：`docker compose up --build` 为推荐方式；说明镜像为 d1-2B CPU 推理（linux/amd64 + linux/arm64）、内含 pinned 1.8GB 模型、首次构建需下载编译较久；保留裸 `docker build` 等价命令。删掉"该镜像没有 d1 支持"的旧描述。

- [ ] **Step 3: 校验（Review Focus #4）**

Run: `python3 -c "import yaml;yaml.safe_load(open('.github/workflows/ci.yml'))"`（或 actionlint 若已安装）+ 通读 README 渲染效果。
Expected: YAML 合法。CI 的实际运行验证随下次 push 发生；在 commit message 中注明 linux/amd64 运行时验证由 CI 承担。

- [ ] **Step 4: Commit**

```bash
git add .github/workflows/ci.yml README.md
git commit -m "ci: cache docker layers and bump readiness timeout; docs: d1 docker deployment"
```
