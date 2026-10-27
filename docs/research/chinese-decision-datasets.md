# 中文与混合文本决策评测数据调研

核查日期：2026-10-27。目标是寻找可直接适配 Julia-1 的 choice／noul／score 测试数据，覆盖中文、英文、数字、拼音及混用。

## 已核实的事实

| 作者来源 | 覆盖与接口适配 | 许可与使用限制 |
|---|---|---|
| [TokSuite Chinese](https://huggingface.co/datasets/toksuite/toksuite_chinese) | 四选一，提供标准答案；含中文原文、全拼音、部分拼音、中英混用、繁体、输入噪声等。固定版本的13个正式配置为485条，不是每组都40条 | 作者卡明确MIT；本仓库保留来源、版本、上游原始行、文件哈希和原始数据卡。以知识问答／补全为主，会混入模型知识缺口 |
| [Amazon MASSIVE](https://github.com/alexa/massive#accessing-and-processing-the-data) | 平行zh-CN、zh-TW、en-US；18种scenario可原生单次选择，60种intent超出Julia原生20选项 | 数据CC BY 4.0；官方S3提供JSONL。Julia已公开报告此基准成绩，不能视作全新未知测试；不覆盖完整拼音矩阵 |
| [CLUE TNEWS](https://github.com/CLUEbenchmark/CLUE) | 中文新闻15分类，可单次选择；有标准开发集 | 本轮未核实可支持再分发的数据许可证；不能把代码许可当数据许可。缺少英文与拼音配对 |
| [CBLUE KUAKE-QIC](https://github.com/CBLUEbenchmark/CBLUE#kuake-qic) | 中文医疗查询11类，可原生选择 | 数据通过天池下载，需核对具体数据授权；医学领域较窄，未导入 |
| [READIN](https://github.com/thunlp/READIN#data) | 中文真实键盘／语音输入噪声，包含多种任务 | 拼音输入法产生的中文噪声不等于整句拼音；本轮未核实明确数据再分发许可，未导入 |
| [CLINC150](https://github.com/clinc/oos-eval) | 英文150意图与域外案例 | [CC BY 3.0](https://github.com/clinc/oos-eval/blob/master/LICENSE)；超出原生20选项且缺少中文对照，未导入 |

TokSuite固定版本：`f3b76350c0bb4f94d008746f7ab1f3a04642958e`。字段、配置、作者说明与许可均见[固定数据卡](https://huggingface.co/datasets/toksuite/toksuite_chinese/blob/f3b76350c0bb4f94d008746f7ab1f3a04642958e/README.md)。其中colloquial39条、spelled_out13条、traditional33条，其余十组各40条。外部文件下载清单和哈希见[manifest](../../tests/evaluation/toksuite-manifest.json)。英文TokSuite也公开可用，但中英文包含本地化人名／地名／数字，不能只靠set_id就认定逐字同义。

## 本次工程判断

已有可直接使用的数据集，因此不声称“没有中文／拼音／混用数据”。采用外部TokSuite与独立编写的上下文业务决策集，各自统计。外部集可验证真实既有基准上的扰动表现；自建集控制答案、语义和提示语言，补上纯数字、否定、改口、条件、信息不足及三种输出类型。

自建504行由36个语义家族展开，不是504个独立问题；没有独立人工标注团队复核。发布前固定答案，保留每条理由；模型跑错后不修改答案。不能把公共知识问答分数直接当中文业务判断准确率，不能把转写变体数量当独立证据。尚未覆盖声调拼音、拼音缩写、方言、长文本、多轮历史和真实流量分布。

## 可复现入口

参见[评测说明](../../tests/evaluation/README.md)：重建自建数据、按固定SHA导入外部数据、运行HTTP评测、逐行保存预测并按来源／输入形式／提示语言／任务／领域拆解。结果必须保留请求错误分母，并按概率argmax评判score，不对期望分数取整。
