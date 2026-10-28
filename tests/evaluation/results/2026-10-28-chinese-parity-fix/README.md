# Chinese Decision：新版 SDK 数值兼容补齐

日期：2026-10-28。延续 [首次验证](../2026-10-28-llama-upgrade-chinese/README.md)，保持 Go＋cgo 单进程方向。本轮交付隔离的、可重建的候选 SDK 和验证证据；没有替换生产 SDK，也没有实施 d1 的生产 cgo／多模型路由。

## 原因与修复

默认 d05706b 使用 `crop_before_nextn()`，在最后一层 FFN 之前选取输出 token 行。旧版 Qwen3.5 图只在 `embeddings_nextn_masked` 时提前裁剪，默认一直计算全部行，最后归一化后才选取行。两者数学意图相同，最后 FFN 的矩阵形状和 Metal 数值路径不同。

先用新版 `llama_set_embeddings_nextn(ctx, true, false)` 单独强制保留全部隐藏态，66 条诊断输入的 logits 和概率全部恢复旧值；默认路径仍复现最大0.0008434699469788143的概率差。由此定位裁剪位置，而不是改变权重、提示、校准温度或门槛。

最终候选使用 [两处条件补丁](../../patches/llama-d05706b-qwen35-late-crop.patch) 恢复旧版 Qwen3.5 裁剪行为，**不启用 nextn 隐藏态输出**。补丁不修改 LFM3 图、tokenizer 或公开 C ABI；它是本地兼容补丁，不是上游已发布修复。兼容结论针对本文固定的 Chinese Decision Q8_0、设置和输入集，不保证所有 Qwen3.5 权重／设备逐位一致。

诊断最初错误地查找 C 符号名，而该 staging API 实际有 C++ linkage，因而未进入推理；异常清理又触发 Metal 断言。随后按固定提交的实际符号修正诊断，并补上错误分支 context 释放。失败记录 `diagnostic-full-hidden.log` 不计入结果；成功诊断在 `diagnostic-full-hidden-v2/`。该硬编码 C++ 符号仅限探针诊断，生产方案不依赖它。

## 固定条件与数值结果

源提交 d05706b4f32a545814856e98ca518292585443b9，归档 SHA-256 与之前完全相同，另加可审查的两处本地补丁。`build-inputs.json` 和 `environment.json` 记录源、补丁、测量程序、权重及动态库哈希。

同一份1069条输入、token IDs、Q8_0权重、温度1.074872374534607；context=16384、batch=ubatch=512、seq=1、4线程、全层Metal、flash attention disabled。每题清理 hybrid memory。旧版沿用上轮按旧头文件编译的探针，候选按新头文件重新编译；两份实测源码及程序哈希均保留，默认计算路径一致。两个独立 Go/cgo 程序顺序执行，不在同一测试进程加载两版 Metal。

**严格迁移门槛通过：`strict_upgrade_pass=true`。** 同场新旧结果如下：

| 检查 | 结果 |
|---|---|
| 1,069条提示及A–Z分词 | 全部一致 |
| 全部候选原始logits／概率 | 最大绝对差均为0 |
| 逐条winner | 1,069/1,069一致，原先变化的那条已恢复 |
| 24条生产结构化／26候选／长输入goldens | 原生输入与结果一致，包含803-token分块输入 |
| A/B/A状态隔离 | 两版重复A的概率差均为0 |
| 自建集 | 两版均435/504（86.31%） |
| TokSuite | 两版均366/485（75.46%） |

1,069条中35条复杂输入仍直接使用冻结提示，探针未重新实现其结构化renderer。本轮验证的是这些生产golden的冻结提示、token和原生计算；不能替代将来生产engine全链路的parser、队列和生命周期验收。与更早历史概率参考的最大差为2.22e-16；本文的“差为0”指本轮旧版与候选新版之间的对照。

## 同场原生延迟

Apple M6／16GiB，先候选新版、后旧版，独立进程顺序运行。每组6次预热、120次正式采样，保存720次／版本；nearest-rank p95，单位ms。计入状态清理、decode、GPU同步、logits复制和softmax，不含模型加载、分词、HTTP或排队。

| 历史对数 | 问数 | 旧版p95 | 候选新版p95 | 新／旧 |
|---:|---:|---:|---:|---:|
| 0 | 1 | 77.05 | 74.03 | 0.961 |
| 0 | 3 | 226.45 | 218.46 | 0.965 |
| 4 | 1 | 129.60 | 125.92 | 0.972 |
| 4 | 3 | 386.77 | 372.09 | 0.962 |
| 12 | 1 | 216.85 | 203.85 | 0.940 |
| 12 | 3 | 618.19 | 617.16 | 0.998 |

没有触发p95超过10%退化的调查项；本次观测约低0.2%–6%。只有一轮顺序测量，不认证性能增益，也不作为同时运行多个模型或语音服务时的SLA。

## d1／Omni冒烟复测

同一候选构建的实验server顺序加载两个模型，沿用原有固定工件、设置及媒体，不重新生成语音。它只是验证SDK补丁影响的对照工具，不是产品HTTP后端。

| 模型 | 固定样本 | 结果 | 与此前概率最大差 |
|---|---|---|---:|
| d1-Omni-2B | 18请求／20问题：中英文文本、合成语音、图像、音频＋图像、口述问题、静音／音调 | 20/20，无请求错误 | 0 |
| d1-2B | 1图像请求／3问题 | 3/3，无请求错误 | 0 |

这些是回归诊断，不是新准确率基准；没有用20条合成／固定媒体样本替代真实说话人或业务质量评估，也没有重跑d1的989条准确率集。`media-comparison.json` 保存与此前结果逐题比较，`*-media/` 保存原始响应及读出。

生产旧服务18083未替换，本任务的原版Omni实验端点18084已恢复。

## 重建与审计

```sh
bash scripts/prepare-liquid-d1-compat.sh
```

输出：`dist/chinese-parity-fix/source`、`build/bin` 和 `sdk/{include,lib}`。SDK 的 `source.sha256`、`patch.sha256` 和 `lib/SHA256SUMS` 分别记录归档、补丁和库身份；动态库依赖使用 `@loader_path`。这不是旧 SDK 目录的替换品，当前生产身份检查会拒绝它。

SDK 包含 `mtmd.h`、`mtmd-helper.h` 和 `libmtmd`，已通过独立 C 头文件编译／链接冒烟检查，打包库与实测库逐文件哈希一致。Go 常规测试与24项Python评测工具测试通过；原始失败门槛与诊断模式拒绝均有回归覆盖。独立审查已复算数值、延迟及媒体结果。

编译探针使用匹配头文件及库：

```sh
CGO_ENABLED=1 \
 CGO_CFLAGS="-I$PWD/dist/chinese-parity-fix/source/include -I$PWD/dist/chinese-parity-fix/source/ggml/include" \
 CGO_LDFLAGS="-L$PWD/dist/chinese-parity-fix/build/bin -Wl,-rpath,$PWD/dist/chinese-parity-fix/build/bin -lllama" \
 go build -tags llamavalidation -o dist/chinese-parity-fix/probe-patched ./tests/evaluation/native_probe

# output 必须是尚不存在的新目录。运行期间避免其他模型占用 GPU。
dist/chinese-parity-fix/probe-patched \
 -reference tests/evaluation/results/2026-10-28-chinese-parity-fix/reference.json \
 -model dist/chinese-decision-2b-q8/chinese-decision-2b-Q8_0.gguf \
 -output dist/chinese-parity-fix/fresh-run -serial-only -samples 120

python3 tests/evaluation/native_probe/audit_upgrade.py \
 --results tests/evaluation/results/2026-10-28-chinese-parity-fix --require-pass
```

审计最后一条命令检查仓库中已保存的新旧结果；重新测量时需将新旧 `parity.jsonl`、`latency.jsonl`、`summary.json` 和相同 `reference.json` 放入新的结果根目录的 `old/`、`new/` 后，传该根目录。不得只替换一份摘要。

`--require-pass` 在原先门槛失败时返回非零；不传该参数仍可审计合法的失败研究结果。审计拒绝 `full_hidden=true` 的诊断路径，并在拒绝重跑前清理旧审计产物，防止留下旧的成功结论。新增回归测试分别验证原始失败数据确实被拒绝，以及相同真实观测只有默认模式能通过。

## 产品接入边界

这一轮关闭数值兼容差异，不把候选 SDK 说成已经集成好的产品。完整 Go engine 仍需按新头文件编译，并一起更新 SDK／补丁身份保护；还需验收生产 parser、取消和退出、共享 backend 生命周期。Omni 的进程内媒体适配、配置驱动的多模型注册和同时常驻内存验收仍待实施。

新的 [统一原生设计](../../../../docs/superpowers/specs/2026-10-28-unified-native-model-routing-design.md) 明确单进程所有权、配置后重启、启用项和请求选模，并补齐 LFM3 单题必须在一个 physical ubatch 完成的约束。实验 HTTP 端点继续仅作为对照，不纳入产品部署。
