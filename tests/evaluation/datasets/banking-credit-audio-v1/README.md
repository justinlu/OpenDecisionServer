# 显式信用卡业务音频补充集

原banking-audio-v1没有信用卡专题，不能把一般银行卡挂失当成信用卡标签。本补充集新增30条由助手编写并经独立AI审查的语义用例：还款、账单查询、可用额度查询、账单分期申请、信用卡申请，各3组办理／咨询对照。没有按模型输出修改文本或标签，也未经银行业务人员复核。

`cases.jsonl`保存语句和规则标签；`task.json`保存共用问题、两声线及生成参数；`manifest.json`记录实际音频SHA-256和生成源码快照；`audio/`包含60条干净语音、20条15dB合成噪声语音、4条非语言控制，共84文件。Tingting／190和Eddy／175均为本机合成条件，不能当作真人或方言覆盖。

本轮业务域实验使用30条文本及60条干净音频；它们与旧集分别报告。噪声和控制素材保留供后续使用，不混入本轮主分母。`index.html`可离线试听全部素材。

```sh
python3 tests/evaluation/banking_audio.py validate --dataset tests/evaluation/datasets/banking-credit-audio-v1
python3 tests/evaluation/banking_audio.py prepare --dataset tests/evaluation/datasets/banking-credit-audio-v1
```

已有manifest时prepare仅核验，不覆盖录音；首次生成需要macOS say／afconvert。语料中的业务是测试假设，不代表实际机具提供信用卡申请或分期办理；navigate仅指进入对应页面，不执行真实操作。

本集都显式说出信用卡及具体目标，没有覆盖含糊卡种、隐含产品、多轮产品确认或信用卡通用操作与专属操作的全部交叉。它是补充诊断集，不能用较强的产品关键词表现掩盖旧集低分，也不代表生产人群准确率。
