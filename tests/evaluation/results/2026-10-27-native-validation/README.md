# Go / llama.cpp 接入设计的原生验证

结论：**Go/cgo 直接调用同一份 llama.cpp + Metal 可以复现 Python 串行模型结果，但没有测到延迟收益；本次多序列批处理不推荐启用。** 这是独立验证程序的结果，尚未接入 ODS HTTP。接入方案见 [设计](../../../../docs/superpowers/specs/2026-10-27-llama-backend-design.md)。

## 准确率与一致性

| 检查 | 结果 | 判断 |
|---|---|---|
| HF 与 GGUF tokenizer：1,045 条提示 + A–Z | token IDs 全部一致 | 通过 |
| Go 字符串模板与 Python 提示 bytes | 1,028 条一致，17 条结构化输入跳过 | 仅模板拼接通过 |
| Go 串行 vs 历史 Python：989 条 | winner 989/989 一致；最大概率绝对差 2.22e-16 | 通过 |
| 串行 A/B/A | 同一 A 的概率差为 0 | 通过 |
| 多序列 vs 串行：1,045 条 | winner 全部一致；最大概率差 0.000517159 | **未过 0.0001 容差** |
| 不连续选样、重复 A 分配两个 sequence | 最大概率差 0.000160207 | **未过同一容差** |

989 条来自之前固定的自建 504 + 外部 TokSuite 485，未改提示或温度。Go 串行与批处理的成绩均为自建 **435/504（86.31%）**、TokSuite **366/485（75.46%）**，与 Python 相同。批处理虽未改变这批答案，但概率没有达到事先设定的一致性门槛，不能声称完全等价。原生 batch/serial 最大候选 logit 差为 0.0056324；这不足以单独证明状态污染或确定其数值变化原因。

另有语音性能集的 36 个独立问题和 20 个边界组合，共 1,045 条 native 输入。边界含对象键顺序、大整数、浮点、null、空描述、Unicode/特殊 token、数组、2/26 候选和结构化问题。它们没有新增准确率金标准。**Go renderer 的 options 由 Python 参考生成**，所以此次没有验证 Go 的 options/defaults/parser；结构化输入只验证了 Python 提示到 GGUF 的 tokenizer 与 native 运行。1,028 条模板对比也不能解释为整个输入编码器已完成。

真实 batch 按 2/3/4 题轮换，长度无需相等；最后余下一题。实际分别覆盖 232/348/464/1 条输入。全部 native 前向完成，输出覆盖完整且 logits/probabilities 有限。串行和批处理都保留每条原始候选 logits、概率，便于重算，不只保存最终通过标记。[逐条输出](parity.jsonl)、[独立复核](audit.json)

## 延迟：同一 Go 验证程序的同步原生调用

单位 ms，p95。每组 120 次，6 次预热，单个调用者；每组轮换三条相同历史长度的语音模板。

| 历史轮数 | 判断数 | Go 串行 | Go 多序列 batch | ≤100ms |
|---:|---:|---:|---:|---|
| 0 | 1 | 47.29 | 51.65 | 两者都满足 |
| 4 | 1 | 84.83 | 89.09 | 两者都满足 |
| 12 | 1 | 131.17 | 134.06 | 两者都超出 |
| 0 | 3 | 141.13 | 156.15 | 两者都超出 |
| 4 | 3 | 252.75 | 257.44 | 两者都超出 |
| 12 | 3 | 389.63 | 431.76 | 两者都超出 |

计时包括 native 状态清理、decode、等待 GPU 完成、候选 logits 复制和 softmax；不含加载、提示渲染、分词、HTTP、排队。三问串行计时覆盖三次前向；三问 batch 是三个独立 sequence 一次提交。batch 模式的单问仍使用为四序列预分配的 context，所以它与串行 context 的内存配置不同，不能把差异解释为 cgo 开销。

此前 Python HTTP 同工作负载单问 p95 为 47.04/86.25/131.58 ms，三问为 138.67/254.37/395.91 ms，量级与这里相近；两次测量时间、计时范围和常驻进程不同，不能据其微小差值声称 Go 比 Python 更快或更慢。[旧端到端结果](../2026-10-27-voice-latency/README.md)

本次没有发现 batch 优势，不能把“Python 逐题串行”简单推演成“真正 batch 必然快”。只测一个参数配置，不能推断所有 llama.cpp 优化都无效；flash attention、不同 microbatch、上下文池、其他量化或模型均未在此验证。没有 HTTP/4 clients、ASR/TTS 竞争或开放流量测试，也没有生产 SLA 结论。

## 固定环境与审阅

Apple M6、16 GiB、12 逻辑 CPU；2B Q8_0、4 CPU threads、25/25 层卸载 Metal，flash attention disabled。沿用 [原模型清单](../2026-10-27-chinese-decision-2b/model-manifest.json) 和 temperature 1.074872374534607。串行 context：n_ctx16384、n_seq_max1、n_batch512、n_ubatch512。batch：n_ctx65536、n_ctx_seq16384、n_seq_max4、n_batch8192、n_ubatch512。没有 prefix sharing 或手工 padding，每个作业前清除完整混合状态。[GPU/context 日志](native.log)

预设迁移门槛是 100% winner 一致、概率绝对差 ≤1e-4，未按结果放宽。`summary.json` 中 parity_pass=false，`audit.py` 返回 1，是**批处理验证未过门槛的研究结果**，不是忽略的测试失败。串行单独通过。[汇总](summary.json)、[1,440 条延迟观测](latency.jsonl)

设计经独立 builder check，无需用户补充关键决定。审阅还要求显式检查所有 logits 有限、把 A/B/A 纳入总门槛，已加入保留的工具；实际测量二进制的源码另存 [measurement.go.txt](measurement.go.txt)。独立 audit 复核了落盘 logits 全部有限、覆盖率、成绩、概率差及 p95，并将测量程序记录的 A/B/A=0 和重复输入差值纳入门槛；这两组重复输出没有另存，未作独立重算。未重新跑相同推理来制造新的计时样本。[版本与文件哈希](metadata.json)

## 复现

先准备 [已有固定 Python/Metal 环境](../2026-10-27-chinese-decision-2b/SETUP.md)。头文件来自固定的 `llama_cpp_python-0.3.36.tar.gz`，本机解包位置为 `dist/llama-cpp-python-cpu/vendor/llama.cpp/`；目录名 cpu 是历史实验名称，源码与 Metal 库同源。不要改用任意最新 llama.h。

```sh
mkdir -p dist/native-validation
dist/chinese-decision-venv/bin/python tests/evaluation/native_probe/prepare.py \
  --upstream dist/chinese-decision-upstream --model dist/chinese-decision-2b-q8 \
  --output dist/native-validation/reference.json
CGO_CFLAGS="-I$PWD/dist/llama-cpp-python-cpu/vendor/llama.cpp/include -I$PWD/dist/llama-cpp-python-cpu/vendor/llama.cpp/ggml/include" \
CGO_LDFLAGS="-L$PWD/dist/chinese-decision-venv/lib/python3.13/site-packages/llama_cpp/lib -Wl,-rpath,$PWD/dist/chinese-decision-venv/lib/python3.13/site-packages/llama_cpp/lib -lllama" \
  go build -tags llamavalidation -o dist/native-validation/probe ./tests/evaluation/native_probe
dist/native-validation/probe \
  -reference dist/native-validation/reference.json \
  -model dist/chinese-decision-2b-q8/chinese-decision-2b-Q8_0.gguf \
  -output dist/native-validation/new-run -samples 120 -ubatch 512
dist/chinese-decision-venv/bin/python tests/evaluation/native_probe/audit.py \
  --reference dist/native-validation/reference.json --results dist/native-validation/new-run
```

输出目录必须不存在。验证工具用 build tag 隔离，常规 ODS 构建不链接这些本机库；它是设计验证工具，没有实现生产排队、取消或关闭协议。保留源码和数据是为了复现，此次未把实验程序作为生产 Engine 接入。
