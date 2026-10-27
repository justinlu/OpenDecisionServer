# Chinese Decision 2B Q8_0：本机加速评测环境

用户要求准确率优先、尽量使用加速。本次使用官方2B Q8_0权重和作者GGUF推理代码，在Apple GPU上运行；不是CPU速度对照。模型与独立Python环境保存在仓库已忽略的 `dist/`，不提交2GB权重。

固定资源：

- 模型：`quincyqiang/chinese-decision-2b-gguf`，revision `2357aa9aad05cbd87bcfc11a1b74e5705e3b5bbc`。
- GGUF：`chinese-decision-2b-Q8_0.gguf`，2,012,012,800 bytes；SHA-256 `4f902f73781f87cc8388617bc7e2e99eb14fdeb8551f7ebb48ffbde69394cc02`，已实测核验。
- tokenizer.json：SHA-256 `a5cd9732badce41de57e6efce8302930ded1c1188c5f81feb2bd6c24c4a1941f`。
- 作者代码：[yanqiangmiffy/chinese-decision](https://github.com/yanqiangmiffy/chinese-decision)，commit `9bf28a803b7e3474c52f030bc72618656c054726`。
- 依赖：Python3.13.18、`llama-cpp-python==0.3.36`、`transformers>=5.3,<6`、fastapi、uvicorn；最终完整版本见 `requirements.txt`。不需要PyTorch。
- 0.3.36源码包以PyPI官方SHA-256核验：`832db0699007f1be95a7e41ef12e88926b02ba836461e36a36372db2760c1a2e`。构建启用Metal；原生API确认GPU offload支持，见 `runtime.json`。

配套分词器、模板、校准参数、训练元数据与许可也来自同一模型revision，文件清单和哈希见 `model-manifest.json`。

## GPU配置

作者GGUF后端默认硬编码CPU。`metal-backend.patch`仅把加载层数设为全部offload、启用KQV offload、把服务device标记改为metal，并打开底层加载日志以核验GPU使用。未修改权重、提示编译、分词、候选概率、温度或逐题状态清理。首次CPU环境仅做接口验证；完整正式测试以GPU为准。

应用补丁：

```sh
patch -d dist/chinese-decision-upstream -p1 < \
  tests/evaluation/results/2026-10-27-chinese-decision-2b/metal-backend.patch
```

## 独立服务

从OpenDecisionServer根目录执行（权重和依赖就绪后）：

```sh
PYTHONPATH="$PWD/dist/chinese-decision-upstream" \
TOKENIZERS_PARALLELISM=false HF_HUB_OFFLINE=1 \
VECLIB_MAXIMUM_THREADS=4 \
dist/chinese-decision-venv/bin/python \
  dist/chinese-decision-upstream/scripts/serve.py \
  --backend gguf --base dist/chinese-decision-2b-q8 \
  --gguf dist/chinese-decision-2b-q8/chinese-decision-2b-Q8_0.gguf \
  --threads 4 --context-size 16384 --host 127.0.0.1 --port 18082
```

`GET /health`应显示 `chinese-decision-2b`、metal、gguf、step4994和温度 `1.074872374534607`。服务逐题清理混合注意力／循环状态，并验证Transformers与GGUF的完整输入token一致。

## 固定评测

先验证三种题型和A/B/A状态隔离，再执行；输出目录必须不存在。

```sh
python3 tests/evaluation/evaluate.py \
  --dataset tests/evaluation/custom.jsonl tests/evaluation/toksuite.jsonl \
  --url http://127.0.0.1:18082/v1/systemone \
  --model chinese-decision-2b --timeout 120 \
  --output /tmp/ods-chinese-decision-2b-evaluation
```

保留原504+485行的顺序、描述和标准答案；不根据测试结果修改提示或温度。HTTP错误与超时留在分母；score按概率argmax判分。原始行内的 `request.model` 仍是固定数据集中的值，实际发送时由命令行 `--model` 覆盖，并记录在 `metadata.json`。

这是两套完整服务的准确率比较：Julia-1为CPU ONNX，本候选为GPU Q8_0，两者模型、提示、分词器和运行时不同，不能把差异归因于参数量或GPU单一因素，也不作同硬件CPU速度结论。
