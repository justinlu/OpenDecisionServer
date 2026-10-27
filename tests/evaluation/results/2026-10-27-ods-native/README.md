# ODS 原生 Chinese Decision 2B Metal 验收

日期：2026-10-27。首版串行后端已接入，运行时为 **Go HTTP → cgo → 同进程 llama.cpp → Metal**，使用独立动态库与 GGUF，没有 Python 服务。默认引擎仍为 Julia；选择新后端必须传 `-engine chinese-decision`。方案见 [设计](../../../../docs/superpowers/specs/2026-10-27-llama-backend-design.md)，启动见 [README](../../../../README.md#chinese-decision-on-apple-silicon)。

## 准确率与契约

- 固定 989 条 HTTP 请求全部成功；自建 **435/504（86.31%）**，外部 TokSuite **366/485（75.46%）**，与原 Python 服务相同。989 条 winner 全一致，最大候选概率绝对差 **2.22e-16**，低于预设 1e-4。[逐条记录](accuracy/predictions.jsonl)、[比较](python-comparison.json)
- 新生产编码器对原 989 条提示 bytes、GGUF token IDs 与固定 Python/HF 参考逐项一致。[真实 Metal 测试日志](native-parity.log)
- 24 条 Python 金标准覆盖保序对象、大整数、浮点、null/空描述、Unicode/特殊 token、2/26 选项、结构化问题、score legend，以及跨 512-token 的长输入分块。原生概率通过，HTTP 的 model/usage/probabilities/confidence/choice/noul/score/legend 也全部通过。[HTTP 契约](structured-contract.json)
- 纯 Go 内容测试另有固定随机种子的近千个有限 binary64 浮点数参考；重复键、非法 Unicode、选项/问题上限和 16,384/16,385 token 边界均有测试。实际 Metal 测试覆盖超长拒绝、A/B/A、取消后安全 Close。16,384 上界接受用 fake native 测试，没有声称实测过 16k-token 的端到端吞吐。
- 原 Julia 官方 **100/100** ONNX 对照通过，25 次 batch Run；标准 Go、race、vet 及原 16 项 Python 评测测试通过。[Julia 日志](julia-parity.log)

这是固定诊断集的迁移一致性证据，不是新的真实语音业务准确率估计；未改数据、标签、提示或温度。

## 完整 HTTP 延迟

单位 ms，p95。Apple M6、16 GiB、4 CPU 辅助线程、25/25 层 Metal、单 context/worker。每组 120 次正式观测和 6 次预热，共 1,440 次正式请求，**零错误**。历史轮数指用户/助手一对发言。

| 历史轮数 | 每请求问题数 | 同时在途请求 | 历史 Python | 新 ODS |
|---:|---:|---:|---:|---:|
| 0 | 1 | 1 | 47.04 | 48.48 |
| 0 | 1 | 4 | 181.86 | 187.94 |
| 0 | 3 | 1 | 138.67 | 141.85 |
| 0 | 3 | 4 | 554.74 | 566.66 |
| 4 | 1 | 1 | 86.25 | 85.28 |
| 4 | 1 | 4 | 340.91 | 335.01 |
| 4 | 3 | 1 | 254.37 | 252.96 |
| 4 | 3 | 4 | 1002.56 | 995.05 |
| 12 | 1 | 1 | 131.58 | 130.61 |
| 12 | 1 | 4 | 520.54 | 516.84 |
| 12 | 3 | 1 | 395.91 | 387.80 |
| 12 | 3 | 4 | 1557.21 | 1566.26 |

无历史和 4 轮历史的单问、单客户端符合本次 100ms 预算；多问、12 轮历史以及 4 个在途请求仍超预算。**这次替换没有带来明显提速**；主要交付是统一 ODS 运行时和可验证的资源管理。真实多序列 batch 在前一轮[原生实验](../2026-10-27-native-validation/README.md)既无提速，又未过概率容差，因此本版禁用。

计时从客户端发请求到读完响应并验证，包含 Go 编码、分词、排队、同步推理和响应。不含服务启动/权重加载。旧 Python 和新 ODS 在不同时间测量，原生库也重新从同一固定源码构建，常驻进程布局不完全相同；不能根据微小差异宣称统计显著的快慢。单轮闭环负载不是实际到达分布或生产 SLA；没有并行 ASR/TTS、开放流量、网络部署或跨日重复测试。[原始观测](latency/samples.jsonl)、[分组指标](latency/summary.json)

## 资源、失败处理与审阅

实际计数为 **2,525 个 HTTP 决策请求、4,038 次原生题目推理**，包含准确率989、结构化24、性能正式1440及72次预热，以及服务自身一次 warmup。与预期逐项相符；最大 native 并发为1，结束时 queue/reserved tokens/active 均为0，零拒绝、零原生错误。进程峰值 RSS 约 **2.25 GiB**，只描述此次短/中输入负载。[计数与资源](metrics-after.txt)、[独立重算](acceptance.json)

GPU device free/total 是推荐 Metal 工作集额度及剩余额度，不是物理内存。初始化日志记录配置、模型/模板/tokenizer/原生库哈希和实际 GPU 卸载。[运行日志](runtime.log)、[来源和哈希](metadata.json)

已将 SDK 搬到主工作区默认目录，并从默认模型路径启动新二进制完成真实推理；检查服务加载库未见 Python。普通二进制选择 native 模式时明确报构建提示并退出1。作者固定版 checker 也直接验证了24条结构化响应。[部署冒烟](deployment-smoke.json)

独立代码审阅发现的 Serve 异常丢失退出状态、并列概率对象按 Go map 排序的问题均已用 RED→GREEN 回归修复。Chinese 响应保留候选概率顺序；Julia 元数据、别名和响应行为保持兼容。停止接收后先撤销排队任务，等待 HTTP 使用者与原生 worker 完成，再释放资源；HTTP timeout 不触发提前 native free。队列 token 预算包括运行中的请求，取消不会提前释放其仍在使用的数据。Metal DSO 保留到进程退出，context/model/backend 在 drain 后释放。

## 复现

构建与模型准备使用主 README 的脚本；Python只作为测试客户端/金标准导出工具。

```sh
./bin/ods-metal -engine chinese-decision -http 127.0.0.1:18083
# 另一个终端，输出目录必须不存在：
python3 tests/evaluation/evaluate.py --url http://127.0.0.1:18083/v1/systemone --model chinese-decision-2b --output /tmp/ods-native/accuracy
python3 tests/evaluation/check_chinese_contract.py --url http://127.0.0.1:18083/v1/systemone --output /tmp/ods-native/structured-contract.json
python3 tests/evaluation/benchmark_latency.py --url http://127.0.0.1:18083/v1/systemone --model chinese-decision-2b --samples 120 --clients 1 4 --warmup 6 --budget-ms 100 --output /tmp/ods-native/latency
# 重新核算本仓库保存的完整记录：
python3 tests/evaluation/audit_ods_native.py --results tests/evaluation/results/2026-10-27-ods-native
```

真实 Metal 测试的参数、来源包/SDK/二进制哈希与构建信息见 metadata。测试服务在验收结束后已正常退出；没有替换用户正在使用的其他监听进程。
