# d1 官方用法诊断面板

用途：定位中文音频低分是否来自请求形式或 GGUF 路径。不是新的准确率验收集。来源 banking-audio-v1 已见样本，14 个业务语义、8 个咨询／办理语义；每条文字及两声线。业务和门控有重复语义，不把所有调用计作独立样本。

- `gguf`：264 调用，四种题目变体；现有 Q8_0＋F16 projector，固定 llama.cpp d05706b4f32a545814856e98ca518292585443b9，启动参数见 `scripts/run-liquid-d1.sh`。HTTP 是实验接口，未改变生产 Go＋cgo 设计。
- `hf-bf16`：132 调用，固定取 GGUF 面板的业务 baseline_zh/short_en、门控 baseline_zh/noul_en。官方 Transformers `system_one`，MPS BF16，set_size=1、未 compile、threads=4，`torch.inference_mode()`。模型加载不计入每请求时间；无统一预热，时间只作诊断，不能当性能排名。
- 每个目录保存 `protocol.json`、`predictions.jsonl`（原始响应、评分、输入摘要、时间）、`summary.json`、`metadata.json`、实际运行脚本快照 `measurement.py.txt`。输入文件由 protocol 和冻结数据集重建；音频任务不含当前转写和金标。
- HF 固定 revision `4cccbfd5ba3405ee0f973134bd03e0d9d60ef1d8`，`verified-files.json` 记录实际 39 个下载文件哈希。下载时逐项对官方 API 的 Git blob SHA-1 或 LFS SHA-256 核验；权重 SHA-256 为 `1553fff04c54377ff527d2016fc479425ef60a0b3bc7a2e61d32f80a75c32aaf`。`reference-versions.json` 是执行环境版本。权重未入库。
- HF `response_body` 保留官方响应；仅给既有评分器的副本补模型别名，无修改概率或选择。
- `comparison.json` 将 HF 结果按 ID 与 GGUF 匹配，统计选项差异及同名候选概率最大绝对差；132 条只有 2 条选择不同。概率差不能独立归因为量化，框架与精度也不同。

| 音频题目 | GGUF | HF BF16 |
| --- | --- | --- |
| 业务／原中文 | 10/28 | 10/28 |
| 业务／短英文 | 11/28 | 11/28 |
| 门控／原中文 choice | 11/16 | 12/16 |
| 门控／英文 noul | 9/16 | 9/16 |

各后端对应文字题全对。GGUF 其余变体：业务短中文 11/28、英文加场景 10/28；门控短英文 choice 14/16、中文 noul 9/16（文字 7/8）。这是探针结果，未检验显著性，不能挑最好一项替换完整集指标或宣称泛化提升。

重算审计（不运行模型）：

```sh
python3 tests/evaluation/d1_usage_probe.py audit --output tests/evaluation/results/2026-10-28-d1-usage-probe/gguf
python3 tests/evaluation/d1_usage_probe.py audit --output tests/evaluation/results/2026-10-28-d1-usage-probe/hf-bf16
python3 tests/evaluation/d1_usage_probe.py compare --output tests/evaluation/results/2026-10-28-d1-usage-probe
```

重跑先准备清单里的 native 模型和官方 HF 固定 revision（代码会执行 `trust_remote_code`，须检查官方固定源码）。安装 reference-versions 所列依赖；原实验临时 venv 通过只读 .pth 复用本机既有通用包，环境路径不是可移植依赖声明。`run --backend hf --model <固定快照> --output <新目录>`；GGUF 端点为 18084。命令见 `d1_usage_probe.py --help`。两者均要求新目录，避免覆盖测量。模型及运行库版本有变时需重新记录身份。

结论及一手资料：[研究记录](../../../../docs/research/2026-10-28-d1-official-usage-audit.md)。本次没有证明中文不受支持、Q8 必然退化或英文必然更好。
