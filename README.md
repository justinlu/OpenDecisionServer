# Open Decision Server

Open Decision Server（ODS）为业务系统提供意图分类、功能路由、真假判断和等级评分，使用 Jev-compatible System One HTTP API。

当前 Apple Silicon 默认模型为 **Liquid d1-2B Q8_0**，推理链路为 **Go HTTP → cgo → 同进程 llama.cpp → Metal**。一个 ODS 进程只加载启动配置选定的一个模型；客户端请求不会加载或切换模型。Chinese Decision 和 Julia 保留为可选引擎。

## 当前服务状态

本机已部署 d1-2B，以下两个地址由同一进程、同一模型提供服务：

| 用途 | 地址 |
| --- | --- |
| 默认 API | `http://127.0.0.1:8080/v1/systemone` |
| 保留的原入口 | `http://127.0.0.1:18083/v1/systemone` |
| 就绪检查 | `http://127.0.0.1:8080/health/ready` |
| 实际模型信息 | `http://127.0.0.1:8080/v1/models` |

这两个端口当前都返回 `d1-2b`。原 Julia、Chinese 的独立进程已退出，旧版本和恢复配置保留。部署、重启验证和回滚记录见 [d1 默认服务验收](tests/evaluation/results/2026-10-28-d1-default/README.md)。

**调用当前 d1 服务只需 `state` 和 `questions`，省略 `model`。** 响应中的 `model` 标识实际使用的模型，便于日志和评估追踪。

## 模型与接入状态

| 模型 | 服务端 `-engine` | 响应模型名 | 当前 ODS 能力 | 平台与运行时 |
| --- | --- | --- | --- | --- |
| Liquid d1-2B Q8_0（默认） | `d1-2b` | `d1-2b` | 文字决策；已接入并部署 | macOS ARM64，cgo / llama.cpp / Metal |
| Chinese Decision 2B Q8_0 | `chinese-decision` | `chinese-decision-2b` | 文字决策；可显式选择 | macOS ARM64，cgo / llama.cpp / Metal |
| Julia-1 | `julia` | `julia-1` | 文字决策；保留 CPU 和容器方案 | macOS ARM64 / Linux x86-64，ONNX Runtime CPU |
| Liquid d1-Omni-2B | 尚无此引擎选项 | 不在当前服务中列出 | 音频、图像已有隔离实验；未接入生产 ODS | 参见 [多模态实验](tests/evaluation/results/2026-10-28-liquid-d1-gguf/README.md) |

表中列出的是 **ODS 已开放的能力**。当前 d1-2B 服务仅接受文字，未开放图像、音频、视频或多标签分类。不能通过请求填写 `d1-omni-2b` 来启用 Omni。

模型选择依据见 [同集对照](tests/evaluation/results/2026-10-28-d1-default/README.md)：银行核心两阶段路由，d1 为 130/130，Chinese 为 120/130；完整业务标签则 Chinese 多答对一条。d1 在当前优先场景表现更好，并非所有任务都领先。该回归集不是独立的线上准确率认证。

## 配置在哪里生效

当前使用 **命令行启动参数** 配置服务。本机托管部署将这些参数持久化在 launchd 的 `ProgramArguments` 中：

```text
~/Library/LaunchAgents/app.powermobile.voiceai.decision.plist
```

进程启动时读取参数、加载一个模型并预热，成功后才就绪。切换模型需要准备对应工件、修改服务端启动参数并重启；没有热切换、请求选模、多模型同时常驻或自动主备降级。

目前没有应用级 JSON/YAML 配置文件，也没有 `-config` 选项。以下参数用途不同：

| 参数 | 用途 |
| --- | --- |
| `-engine` | 选择唯一加载的推理引擎，默认 `d1-2b` |
| `-llama-model` | d1 / Chinese 的 GGUF 权重路径 |
| `-llama-lib` | 与选定原生引擎匹配的动态库路径 |
| `-llama-config` | **仅 Chinese** 的模板、分词器和校准工件目录；不是整个服务的配置文件 |
| `-model` | **仅 Julia** 的 ONNX 图路径；不是请求里的模型名，也不是 GGUF 路径 |
| `-tokenizer` / `-ort-lib` | **仅 Julia** 的分词器和 ONNX Runtime 库路径 |
| `-http` | 监听地址；逗号分隔的多个地址共用同一个模型实例 |

命令行相对路径按进程工作目录解析；以下开发命令均从仓库根目录执行。本机 launchd 发布包使用绝对路径和独立工作目录，不依赖开发目录。

### 通用配置

| 参数 | 默认值 | 说明 |
| --- | --- | --- |
| `-http` | `:8080` | 默认绑定所有本机接口；仅本机使用时显式配置 `127.0.0.1:8080` |
| `-max-questions` | `64` | 每请求问题数上限；原生引擎最多 64 问 |
| `-log-requests` | `false` | 记录请求元数据，不记录 state、instructions 或 criteria 内容 |

### d1 / Chinese 原生配置

| 参数 | 默认值 | d1-2B | Chinese Decision |
| --- | --- | --- | --- |
| `-llama-model` | 随引擎选择，见下方启动命令 | 固定 Q8_0 权重 | 固定 Q8_0 权重 |
| `-llama-lib` | 随引擎选择，见下方启动命令 | `libods_d1.dylib` C ABI 桥 | `libllama.dylib` |
| `-llama-config` | 空 | 不使用 | 空时使用 GGUF 所在目录 |
| `-llama-threads` | `4` | CPU 辅助线程 | CPU 辅助线程 |
| `-llama-max-waiting` | `32` | 等待请求数，另有一个活动请求 | 等待请求数，另有一个活动请求 |
| `-llama-max-tokens` | `65536` | **单请求**所有题目编译后的 token 总量上限 | 活动及等待请求的**合计预留** token 预算 |

两种原生引擎都串行执行推理，`-max-runs` 不会为它们增加 GPU worker。d1 每道完整题目必须在一次 physical batch 中完成，最多 8192 tokens；Chinese 单题最多 16384 tokens，内部可按 512 tokens 分块。均不会静默截断超限输入。

d1 和 Chinese 使用各自固定、匹配 ABI 的 SDK；不能互换库、头文件或修改来源标记绕过校验。同一可执行文件只加载所选引擎的库。原生路径没有 CPU 自动回退。

### Julia CPU 配置

| 参数 | 默认值 | 说明 |
| --- | --- | --- |
| `-model` | `models/julia-1/model.onnx` | ONNX 图，需保留配套外部权重 |
| `-tokenizer` | `models/julia-1/tokenizer.json` | 分词器 |
| `-ort-lib` | macOS：`lib/darwin-arm64/libonnxruntime.dylib`；Linux：`lib/linux-amd64/libonnxruntime.so` | 原生运行库 |
| `-ort-threads` | `4` | Session 内部线程数 |
| `-max-runs` | `4` | 同时运行的 Session.Run 数量 |
| `-max-waiting` | `128` | 等待推理槽的请求数 |
| `-max-length` | `1024` | 编码序列长度上限 |
| `-head-length` | `256` | 问题与候选的 token 预算 |

## 准备与启动

需要 Go 1.26+、cgo 和 C/C++ 编译器。原生 Metal 构建还需要 CMake、curl 和 Python 3 准备工具；部署后的 Go 服务不运行 Python / Transformers 或独立 llama-server。模型权重、SDK 二进制和构建结果不提交 Git，准备脚本校验固定版本和哈希。

以下为三套**择一运行**的启动配置；替换已有服务时先停止旧实例。本机已有默认服务时，可以直接调用，无需再启动另一个原生模型实例。多实例同时初始化曾触发 Metal 内存不足。

### d1-2B：默认原生服务

```sh
# 准备固定模型与原生构建环境；首次下载也包含实验用多模态工件
bash scripts/prepare-liquid-d1.sh
# 当前 Go 可执行文件还保留 Chinese 引擎，需要其匹配的编译头文件
bash scripts/prepare-llama.sh
bash scripts/prepare-d1-native.sh
bash scripts/build-d1.sh

./bin/ods-metal \
  -engine d1-2b \
  -llama-model dist/liquid-d1/models/d1-2B-Q8_0.gguf \
  -llama-lib lib/darwin-arm64/d1/lib/libods_d1.dylib \
  -http 127.0.0.1:8080 \
  -llama-threads 4 \
  -llama-max-waiting 32 \
  -llama-max-tokens 65536
```

这些是 d1 的默认工件路径。准备完成后可简写为：

```sh
./bin/ods-metal -http 127.0.0.1:8080
```

启动会校验权重、SDK 来源及补丁、桥版本、动态库清单和 Metal 预热。部署时需要权重、SDK 的 `lib/`、相邻的来源/补丁/桥标记、库哈希清单及许可证；当前纯文字路径不加载 projector。

### Chinese Decision：保留的原生引擎

```sh
bash scripts/prepare-llama.sh
bash scripts/download-chinese-model.sh
bash scripts/build-llama.sh

./bin/ods-metal \
  -engine chinese-decision \
  -llama-model models/chinese-decision-2b/chinese-decision-2b-Q8_0.gguf \
  -llama-config models/chinese-decision-2b \
  -llama-lib lib/darwin-arm64/llama/lib/libllama.dylib \
  -http 127.0.0.1:8080 \
  -llama-threads 4 \
  -llama-max-waiting 32 \
  -llama-max-tokens 65536
```

模板、tokenizer 和 `decision_config.json` 必须与固定权重配套；不能只复制 GGUF。SDK 从 llama-cpp-python 固定源码包构建，不依赖 Python 推理服务。`LLAMA_SDK` 可指定另一构建安装目录，同时需通过 `-llama-lib` 指定相应库。

新版候选 SDK 的 Chinese 数值兼容验证见 [兼容性报告](tests/evaluation/results/2026-10-28-chinese-parity-fix/README.md)。当前 Chinese 引擎仍绑定其原有 SDK；兼容实验不表示已完成所有引擎的共享 runtime / 多模型管理。

### Julia：CPU 服务

```sh
make deps
make build

./bin/ods \
  -engine julia \
  -model models/julia-1/model.onnx \
  -tokenizer models/julia-1/tokenizer.json \
  -ort-lib lib/darwin-arm64/libonnxruntime.dylib \
  -http 127.0.0.1:8080 \
  -ort-threads 4 \
  -max-runs 4 \
  -max-waiting 128 \
  -max-length 1024 \
  -head-length 256
```

Linux 将 `-ort-lib` 换为 `.so` 路径。普通 `make build` 不包含 Metal 支持，运行时必须显式选择 `-engine julia`；省略后会尝试默认 d1 并提示使用原生构建。

### Linux x86-64 容器

```sh
./scripts/download-model.sh
docker build --platform linux/amd64 -t open-decision-server .
docker run --rm -p 127.0.0.1:8080:8080 open-decision-server
```

当前 Dockerfile 显式选择 Julia CPU，并下载固定 ONNX Runtime 库。该镜像没有 d1 / Chinese Metal 支持。

## 调用当前 d1 服务

检查服务：

```sh
curl -fsS http://127.0.0.1:8080/health/ready
curl -fsS http://127.0.0.1:8080/v1/models
```

银行功能导航示例，客户端省略 `model`：

```sh
curl -fsS http://127.0.0.1:8080/v1/systemone \
  -H 'Content-Type: application/json' \
  -d '{
    "state": "帮我打开取款功能",
    "questions": {
      "intent": {
        "type": "choice",
        "instructions": "用户现在要进入办理，还是只咨询业务信息？",
        "criteria": {
          "navigate": "现在打开、进入或使用银行功能",
          "inquire": "仅了解规则、费用或操作方法"
        }
      }
    }
  }'
```

Python 标准库示例同样只配置服务地址：

```python
import json
from urllib.request import Request, urlopen

request = Request(
    "http://127.0.0.1:8080/v1/systemone",
    data=json.dumps({
        "state": "帮我打开取款功能",
        "questions": {
            "intent": {
                "type": "choice",
                "instructions": "用户现在要进入办理，还是只咨询业务信息？",
                "criteria": {
                    "navigate": "现在打开、进入或使用银行功能",
                    "inquire": "仅了解规则、费用或操作方法",
                },
            },
        },
    }, ensure_ascii=False).encode("utf-8"),
    headers={"Content-Type": "application/json"},
)
with urlopen(request, timeout=30) as response:
    result = json.load(response)
    print(result["answers"]["intent"]["choice"])
    print(result["model"])  # 实际模型标识，用于追踪
```

### 请求与响应约定

| 项目 | d1-2B | Chinese Decision | Julia |
| --- | --- | --- | --- |
| 请求 `model` | 可省略，使用服务端已加载模型 | 旧解析器仍要求 `chinese-decision-2b` | 旧解析器仍要求 `julia-1` 或其别名 |
| `choice` 候选数 | 2–255 | 2–26 | 2–20 |
| `score` 等级数 | 2–10 | 2–26 | 2–10 |
| `score` 返回值 | 按校准概率读出中位等级 | 按概率计算期望等级 | 按概率计算期望等级 |
| 并发执行 | 一个原生 worker 串行处理 | 一个原生 worker 串行处理 | 默认最多 4 次 Session.Run 并发 |

三种题型：`noul` 为真假概率，`choice` 为互斥候选选择，`score` 为等级评分。响应包含 `model`、`answers` 和 `usage`；`usage.output_tokens` 为 0，因为这些模型不生成回答文本。

旧引擎的必填 `model` 是解析器兼容遗留，完全统一其省略行为尚未实现；它不提供请求选模。当前 d1 若显式携带不匹配的模型名会返回 404，不会改用其他模型。某些 SDK 会自动填入 `jev-latest`，不能把它直接当作当前 d1 的默认请求；可使用上面的无模型名 HTTP 请求，或调整 SDK 的兼容参数。

Julia 的 `jev-latest`、`jev-1.13.0`、`typesafe/jev-1.13` 均为 `julia-1` 的别名，只在 Julia 引擎下有效。

当前 API 未启用服务端认证；兼容客户端附带的 Authorization / Bearer 头。

常见状态码：

| 状态码 | 含义 |
| --- | --- |
| `400` | JSON 格式错误 |
| `404` | 显式请求的模型名与当前模型不匹配 |
| `413` | 请求体超过 1 MiB |
| `422` | 字段、题型、候选数或 token 预算不合法，或使用未开放的媒体能力 |
| `429` | 推理队列或相应预留预算已满 |
| `500` | 内部推理错误 |
| `503` | 尚未就绪、请求取消或模型不可用 |

启动失败不会自动改用其他模型。原生推理发生后端错误时会撤销就绪状态并结束 worker；取消请求可以让调用方提前返回，但资源会保留至 GPU 工作结束。服务退出先停止准入，再 drain 和释放资源。

## 本机托管服务与维护

当前服务由 `app.powermobile.voiceai.decision` launchd 作业管理；原 `app.powermobile.voiceai.decision-chinese` 作业已停用。程序、权重和原生库位于独立 release，路径记录在 [activation.json](tests/evaluation/results/2026-10-28-d1-default/activation.json)。

切换启动配置时，需要一起核对 `-engine`、权重、配套工件和 SDK 路径，并确认目标 release 包含这些文件。当前在线 d1 发布包只携带其运行所需工件；保留源码中的其他引擎不等于该发布包已打包全部模型。

停止 launchd 作业后，须等待旧 PID 真正退出再重新注册。`bootout` 返回不保证进程已完成退出；部署工具已有相应等待及回滚处理。初次迁移工具 `scripts/deploy-local-d1.py` 的 stage 流程要求原 Julia / Chinese 两作业均已加载、启用，不能把它作为当前在线 d1 的通用升级命令直接重复执行。

如需恢复迁移前的服务，可使用记录中的 release 路径：

```sh
python3 scripts/deploy-local-d1.py rollback --release '<activation.json 中的 release 路径>'
```

回滚会恢复原 Julia 和 Chinese 两项服务；它不是切换默认模型的日常配置接口。

## 监控与验证

`GET /metrics` 提供 HTTP 请求/错误计数、活动推理、排队和累计推理次数；还包含对应引擎的资源及错误指标。d1 的 worker 串行；Julia 的并发计数描述 Session.Run。Chinese 另有预留 token 等指标，不能把某个引擎的指标当作所有引擎都有。

测试与离线审计：

```sh
make test
make race
go vet ./...
python3 -m unittest discover -s tests/evaluation -p 'test_*.py'
python3 tests/evaluation/results/2026-10-28-d1-default/audit.py
```

对已经运行的 d1 做小规模延迟评估，输出目录必须是新目录：

```sh
python3 tests/evaluation/benchmark_latency.py \
  --url http://127.0.0.1:8080/v1/systemone \
  --model d1-2b --score-readout median \
  --clients 1 --samples 20 --warmup 2 \
  --output /tmp/ods-d1-latency-new
```

这里的评测工具显式记录模型身份，并校验 d1 的中位等级读出；普通业务客户端仍可省略 `model`。评测会产生真实推理负载，结果应按实际硬件、历史长度、题数和并发解释。

`make parity` 只检查 Julia 官方 ONNX 对照；`make bench` / `cmd/odsbench` 的内置请求仍固定为 Julia 的 `jev-latest`，不能直接用来压测当前 d1 服务。Julia / Chinese 的历史性能对照见 [语音延迟评测](tests/evaluation/results/2026-10-27-voice-latency/README.md)，不能视为当前 d1 性能指标。

更多证据与设计：

- [d1 原生验收、本机部署与同集模型比较](tests/evaluation/results/2026-10-28-d1-default/README.md)
- [158 条银行原始文字评估](tests/evaluation/results/2026-10-28-d1-banking-text/README.md)
- [官方 d1 用法核查及 BF16 音频对照](docs/research/2026-10-28-d1-official-usage-audit.md)
- [Chinese 原生服务验收](tests/evaluation/results/2026-10-27-ods-native/README.md)
- [统一原生多模型设计（尚未完整实施）](docs/superpowers/specs/2026-10-28-unified-native-model-routing-design.md)
- [银行音频数据集](tests/evaluation/datasets/banking-audio-v1/README.md)、[音频评估](tests/evaluation/results/2026-10-28-banking-audio-v1/README.md)、[两阶段路由](tests/evaluation/results/2026-10-28-banking-audio-cascade/README.md)、[业务分级实验](tests/evaluation/results/2026-10-28-banking-business-hierarchy/README.md)

## 上游与许可证

- [Liquid d1-2B](https://huggingface.co/LiquidAI/d1-2B)、[d1-Omni-2B](https://huggingface.co/LiquidAI/d1-Omni-2B)
- [Julia-1 ONNX](https://huggingface.co/SupersonicLabs/Julia-1-ONNX)
- [TypeSafe System One OpenAPI](https://api.typesafe.ai/openapi.json)
- [ONNX Runtime Go binding](https://github.com/microsoft/onnxruntime/tree/main/go)

本项目采用 Apache-2.0。模型、llama.cpp、ONNX Runtime 等工件各自保留其上游许可证；部署时保留相应许可文件。
