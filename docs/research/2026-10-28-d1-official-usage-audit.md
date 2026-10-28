# d1 中文银行音频：官方用法核查与参考实现对照

2026-10-28。问题：当前低准确率是否主要来自模型使用方法或 GGUF 接入错误？

结论：已核查的请求形式符合官方用法；小面板的官方 BF16 与 GGUF 结果也非常接近。没有发现能解释大幅低分的接入错误，也没有证据说明改英文、拆成真假题或继续减少候选就能解决。这个结论限于当前短中文 TTS 面板，不能证明所有后端数值等价或模型适合上线。

## 已核实的官方事实

- 决策模型适合已知候选的分类和路由；互斥单选用 `choice`，真假用 `noul`。我们区分咨询／办理、再识别业务，属于适用任务。开放咨询回答仍需另一个能力；决策本身不生成回答。[官方指南](https://docs.liquid.ai/guides/decision-model-guide#classification)、[类型规范](https://docs.liquid.ai/lfm/models/decision-models#primitives)。
- 官方明确支持中文语音，但提示较长口述内容、尤其非拉丁文字语言有准确率损失。不能把这一警示直接归因到本次几秒短句。[固定模型卡](https://huggingface.co/LiquidAI/d1-Omni-2B/blob/4cccbfd5ba3405ee0f973134bd03e0d9d60ef1d8/README.md#L105)。
- 官方表格区分“把问题读出来”和“把待判断的内容读出来”。前者只下降 0.5 分，并不代表我们的语音业务内容也只下降 0.5 分。官方音频 state 的部分测试下降明显；表中 effective accuracy 把随机水平记为 0，不能与我们的普通准确率直接比较。[模型卡音频测评](https://huggingface.co/LiquidAI/d1-Omni-2B/blob/4cccbfd5ba3405ee0f973134bd03e0d9d60ef1d8/README.md#L529)。
- `set_size=16` 是共享一个 state 的问题分组大小，不是每题候选数；源码允许 `1`。没有必须凑齐 16 题或这样必然更准确的要求。[runner.py](https://huggingface.co/LiquidAI/d1-Omni-2B/blob/4cccbfd5ba3405ee0f973134bd03e0d9d60ef1d8/runner.py#L99)。
- 候选的完整区别应写进 `criteria` 描述。默认描述模式有描述时仅呈现描述，不能依赖内部函数名补足语义；没有必须英文的规定。独立真假题的概率也不是跨业务归一化分布。[prompt.py](https://huggingface.co/LiquidAI/d1-Omni-2B/blob/4cccbfd5ba3405ee0f973134bd03e0d9d60ef1d8/prompt.py#L397)、[描述渲染](https://huggingface.co/LiquidAI/d1-Omni-2B/blob/4cccbfd5ba3405ee0f973134bd03e0d9d60ef1d8/prompt.py#L547)。

## 对现有实现的核查

现有录音为 16 kHz、单声道、PCM16 WAV；客户声音放 `files/media`，文字任务放 `instructions`，`state` 可为空或包含历史，不注入当前音频转写。格式对应 [官方媒体读取源码](https://huggingface.co/LiquidAI/d1-Omni-2B/blob/4cccbfd5ba3405ee0f973134bd03e0d9d60ef1d8/media.py#L170) 和 [GGUF 示例](https://huggingface.co/LiquidAI/d1-Omni-2B-GGUF/blob/main/README.md)。Q8_0、F16 projector、batch/ubatch 8192 对应官方示例。GGUF 具体 repo 与文件哈希已固定在 [模型清单](../../tests/evaluation/liquid-d1-models.json)，源码固定为 d05706b4f32a545814856e98ca518292585443b9。

源码审计还核对了采样率、mel 参数、音频边界标记、每个录音的编码器状态清零、单题注意力／位置规则、候选别名与概率读出。没有发现明确错接。特别地，不能套用通用 Whisper 的动态范围裁剪假设：[LFM3 原生路径](https://github.com/ggml-org/llama.cpp/blob/d05706b4f32a545814856e98ca518292585443b9/tools/mtmd/mtmd-audio.cpp#L535)的 `no_padding` 分支跳过该处理。源码核查不等于逐层数值等价证明。[原生媒体实现](https://github.com/ggml-org/llama.cpp/blob/d05706b4f32a545814856e98ca518292585443b9/tools/mtmd/clip.cpp#L6541)、[原生注意力](https://github.com/ggml-org/llama.cpp/blob/d05706b4f32a545814856e98ca518292585443b9/src/llama-graph.cpp#L478)、[HF 编码器](https://huggingface.co/LiquidAI/d1-Omni-2B/blob/4cccbfd5ba3405ee0f973134bd03e0d9d60ef1d8/modeling_lfm3_omni.py#L421)。

## 实测证据

从已见的 banking-audio-v1 固定诊断面板选 14 条办理语义和 8 条办理／咨询语义；每条文字一次、两种 TTS 声线各一次。GGUF 264 次，官方 Transformers BF16 132 次，无请求错误。相同 ID 对照使用相同问题、候选、历史及录音字节，HF 直接走官方 `system_one(..., media=..., set_size=1)`。完整方法、原始结果和身份核验见 [实验记录](../../tests/evaluation/results/2026-10-28-d1-usage-probe/README.md)。

| 音频诊断 | GGUF Q8_0 | 官方 BF16 |
| --- | --- | --- |
| 具体业务，原中文题 | 10/28 | 10/28 |
| 具体业务，短英文题及描述 | 11/28 | 11/28 |
| 咨询／办理，原中文 choice | 11/16 | 12/16 |
| 是否办理，英文 noul | 9/16 | 9/16 |

对应文字均全对。132 个共同观测仅 2 个最终选项不同，最大候选概率绝对差约 0.031；不是完全数值相同，也不是量化无影响的证明。框架、计算精度及权重表示同时变化，无法用这次对照分离各项原因；但未出现“官方实现能答对、GGUF 大面积答错”的现象。

GGUF 额外探针：具体业务短中文 11/28，英文加场景 10/28，均未明显改善；咨询／办理短英文 choice 为 14/16，值得在独立样本验证，但目前只有 8 个语义、两声线，不能按 16 个独立用户样本或上线准确率报告。先前完整评估仍是基线；此面板不能替换它。

## 工程判断与下一步建议

1. 保留清晰、互斥的 `choice`，描述明确写出“现在进入办理”与“仅了解信息”的区别；带上真实已有对话历史。不把 `set_size`、英语或 `noul` 当作官方提分开关。
2. 保留咨询／办理与业务识别的产品分工。层级应对应用户实际交互：先给大类页面再获取下一句新信息；同一句音频反复走树不会增加信息，先前的离线层级实验也没有验证真实多轮效果。
3. 暂不把当前 Omni 的原生中文音频分类定为主路径。下一轮用同一批音频做“ASR 转写 → d1-2B 文本决策”对照，另留人工转写作为文本上限，分别记录转写错误和决策错误。这是待测建议，并非已证实优胜；仍可保持 Go＋cgo 单进程部署目标。
4. 增加真人、不同口音、现场噪声及未见语义的保留集；题目开发与最终评分分开。声线重复不能当作独立语义样本，报置信区间时按语义聚类。拒识阈值在开发集校准，同时看错开功能率、漏办理率、覆盖率及延迟。

本次仅增加研究与可复核实验，不更改生产推理架构。BF16/HTTP 服务只作参考测量；生产仍沿用用户确定的 Go＋cgo 单进程方向。
