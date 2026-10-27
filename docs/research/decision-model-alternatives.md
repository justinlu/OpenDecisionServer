# Julia-1 的中文决策模型替代候选

核查日期：2026-10-27。当前服务器为 Go + Julia-1 ONNX CPU；评测已固定为504条自建上下文诊断和485条外部TokSuite。本文仅调研，未下载候选权重、未运行候选模型、未修改推理实现。

## 结论与优先级（工程判断）

第一组应比较 Chinese Decision 0.8B / 2B / 4B 的官方Q8_0：0.8B用于CPU资源参考，2B作为折中候选，4B作为该家族质量上限参考。它们有真正的中文决策评估，任务也覆盖choice/noul/score。Laya Multilingual作为较小的多语言对照，但其作者披露的零样本、布尔和评分弱项使它不能被视为确定升级。现阶段没有证据证明任何候选在本仓库989条用例上优于Julia-1，尤其不能推断中文微调后仍保留英文或拼音能力。

## 已核实的候选

| 模型／发布者 | 规模与部署 | 语言证据与局限 | 当前适配判断 |
|---|---|---|---|
| Chinese Decision / quincyqiang | 0.8B、2B、4B；合并BF16和作者Q8_0 GGUF；Q8权重约0.812 / 2.01 / 4.48 GB；权重Apache-2.0 | 实际中文CJ-Bench；模型卡标中文；未找到该系列的英文／拼音分项验证 | 最优先实测；作者服务提供 `/v1/systemone`，CPU可用，但不是现有Julia ONNX图的替换文件 |
| Laya Multilingual / convaiinnovations | 322M，mmBERT-base＋决策头；Apache-2.0；作者包支持CPU、MPS、CUDA及ONNX选项 | 面向多语言，包含中文标记；作者报告多语言评估，也明确披露typed-decisions零样本仅34.2%、ordinal score偏置、noul弱阳性和过度自信 | 轻量对照，不能只凭“多语言”承诺升级；需固定checkpoint避免拼音自动路由到英语模型 |
| OpenJev / alanhuangyoo（HF为alanhuangya） | 1.7B、4B、8B；Qwen＋pointer head；PyTorch CPU/MPS/CUDA；权重Apache-2.0 | 官方4B卡明确English only；主要包含英文决策和浏览器步骤评估 | 中文任务优先级低；未核实官方ONNX/GGUF工件 |
| Kev / jaredpalmer | 0.8B、4B等；Qwen＋LoRA／pointer head；PyTorch、Apple MLX；权重Apache-2.0 | 官方模型卡English；4B验证上下文8K，接受上限不等于准确率验证 | 英文决策对照，不能由Qwen底座推导中文能力 |
| Lev / interfaze-ai | 4B＋LoRA；末位候选读出及candidate head；权重Apache-2.0 | 官方English only；承认单词／数字微小修改和细粒度评分较弱；实时部署建议CUDA | 当前CPU中文工作负载优先级低 |
| OpenJev / openjev（独立的另一个发布者） | 27B，Qwen3.8-27B；FP8、MLX、GGUF；权重CC-BY-NC-4.0 | 有德法印中四语XNLI共240题合计82.5%，没有中文独立结果 | 资源与许可均不适合作为当前轻量服务首选；不能与alanhuangya项目混淆 |

表中事实来源：[Chinese Decision论文v2](https://arxiv.org/html/2610.23170v2)、[0.8B模型卡](https://huggingface.co/quincyqiang/chinese-decision-0.8b)、[2B GGUF模型卡](https://huggingface.co/quincyqiang/chinese-decision-2b-gguf)、[4B模型卡](https://huggingface.co/quincyqiang/chinese-decision-4b)、[Laya模型卡](https://huggingface.co/convaiinnovations/laya-multilingual)、[Laya作者部署说明](https://github.com/NandhaKishorM/laya)、[alanhuangya OpenJev-4B](https://huggingface.co/alanhuangya/OpenJev-4B)、[Kev-4B](https://huggingface.co/jaredpalmer/kev-4b)、[Kev-0.8B](https://huggingface.co/jaredpalmer/kev-0.8b)、[Lev](https://huggingface.co/interfaze-ai/lev)、[另一个OpenJev](https://huggingface.co/openjev/openjev)。权重许可证不等于所有训练数据均允许任意用途；Chinese Decision、Lev、alanhuangya OpenJev的作者均另披露数据／教师条款，具体采用前应保留相应说明。

## Chinese Decision：最匹配的中文候选

作者：[yanqiangmiffy/chinese-decision](https://github.com/yanqiangmiffy/chinese-decision)，模型发布者`quincyqiang`。论文v1为2026-10-16，v2为2026-10-23；v2明确发布完成的0.8B、2B、4B，不能再把2B描述为早期未完成检查点。[论文元数据](https://arxiv.org/abs/2610.23170)

它使用Qwen3.5文本主干，对候选字母A–Z的末位logits做归一化，一次前向，不生成答案文本。作者交付模型的输入上限16,384 tokens、单题2–26选项，训练覆盖2–16；三种题型语义与本服务相近。[4B作者卡](https://huggingface.co/quincyqiang/chinese-decision-4b)

| 作者CJ-Bench报告（BF16） | 0.8B | 2B | 4B |
|---|---:|---:|---:|
| 全量micro ACC | 83.11% | 84.11% | 85.97% |
| 通用领域ACC | 72.26% | 73.96% | 75.69% |

来源：[论文v2结果表](https://arxiv.org/html/2610.23170v2)。总计307,900个决策，其中医疗200,000、通用100,000、法律4,300、金融3,600，因此不能把85.97%当成通用业务判断准确率。4B的score准确率仍仅60.43%；论文基线含导入的历史成绩，且只有单训练运行，不是所有模型都在作者环境重新训练／实测的同条件比较。上述数字也不能直接减去本仓库Julia-1的57.54%来推断收益。

量化没有消除所有预测变化：作者对比中，合并和Q8各改变约0.4%–1.1%的top-1，整体分数接近并不意味着每条答案／概率相同。[论文分析](https://arxiv.org/html/2610.23170v2)、[2B量化全量对照](https://huggingface.co/quincyqiang/chinese-decision-2b-gguf)

CPU证据也有明确边界：论文Xeon Gold5318Y、16线程、25个预分词输入的Q8_0 p50为427 / 818 / 2004ms，p95为7047 / 12885 / 31481ms（依次0.8B / 2B / 4B）。这是特定机器和长短输入集合，不是本机HTTP延迟，更不是吞吐保证。GGUF文件大小不等于运行内存或显存需求。[论文Table15](https://arxiv.org/html/2610.23170v2)

作者GGUF服务走llama-cpp-python，保留提示编译、候选字母token校验、校准温度与逐题状态重置，支持 `/v1/systemone`。不能只将当前 `model.onnx` 换成GGUF，或用普通生成结果代替候选概率。[2B官方部署和接口](https://huggingface.co/quincyqiang/chinese-decision-2b-gguf)

## Laya 的重要反证

作者将`laya`、`laya-typed-decisions`标为英语模型；多语言的是`convaiinnovations/laya-multilingual`。不能把英语专用微调版76.6%的成绩移到多语言版：多语言版在同一typed-decisions上的零样本值是34.2%，接近31.8%随机参考。作者还披露score首等级偏置、noul正例P(true)偏低、原始概率未校准。速度宣传常来自T4，不能视为CPU指标。[多语言模型卡的Limits](https://huggingface.co/convaiinnovations/laya-multilingual)、[英语typed版模型卡](https://huggingface.co/convaiinnovations/laya-typed-decisions)

因此，它适合评估轻量多语言编码器能否覆盖部分任务；如果需要训练，必须新增训练／校准数据并保留当前测试集，不能在这989行上训练再回报测试提升。这是工程建议，不是已经完成的训练实验。

## 核验时的版本记录

以下由Hugging Face官方API于2026-10-27读取；时间为UTC，更新时间可以只反映README变更，不等于重新训练。

| 模型 | main修订 | lastModified |
|---|---|---|
| quincyqiang/chinese-decision-0.8b | f40e4bbdc96fda656221d156a5c7fd9c6788090c | 2026-10-18T03:43:24Z |
| quincyqiang/chinese-decision-2b | 530b01cae46ed9f1fb2b7939c4bd477d30f16c43 | 2026-10-21T16:28:09Z |
| quincyqiang/chinese-decision-4b | be8a49a63d17dd3a64ccd5c2fef6c637f2911085 | 2026-10-21T17:44:11Z |
| convaiinnovations/laya-multilingual | 1720e3e3357cfe1e281542e223f8273b0890ca34 | 2026-10-03T18:00:08Z |
| interfaze-ai/lev | 8721908e50dffd5fe8726507c09498e3def5b2f6 | 2026-10-15T06:53:35Z |
| jaredpalmer/kev-4b | 6cfce5c2fa4b4bd64026336ab649c5ca78857d52 | 2026-10-01T17:18:26Z |

官方API形式为 `https://huggingface.co/api/models/<上述模型ID>`；下载与对比时还需分别固定所选GGUF仓库修订、文件哈希、代码修订和温度，而不是把BF16仓库修订误用到GGUF仓库。

## 后续实测建议（尚未执行）

先在独立端口使用作者推理实现，验证本仓库请求契约，再复用固定504+485案例。逐来源、输入形式、提示语言、choice/noul/score报告正确率、错误率和延迟；拼音固定checkpoint，不依赖自动语言路由。保留原题、候选顺序和描述，不根据测试结果反复改提示。作者CPU尾延迟可能超过当前脚本30秒默认值，可用 `--timeout 120` 并明确记录，所有超时仍留在分母。

优先顺序是一次候选验证的建议，不是未经运行的性能结论。尤其要检查英文退化、拼音、数字规则、否定和改口，以及低置信度是否真能筛出错误。服务实现的有序score仍返回期望等级，正确率比较应继续使用概率argmax。
