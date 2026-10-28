# 银行自助机具音频意图集 v1

本集用于测试 d1-Omni-2B 的直接音频理解和决策，重点区分“打开功能”与“咨询问题”。不执行任何真实银行操作。文本及标签由助手在首次推理前编写并经独立AI检查，尚未经银行业务人员复核，不根据测试输出调整。

## 数据结构

- `cases.jsonl`：128条语义用例及历史、标签、对照组和理由。10类功能各5组办理／咨询对照，共100条；另28条上下文或边界用例。
- `task.json`：所有样本共同的两道选择题、标签定义及生成设置。
- `audio/`：实际冻结的16kHz、单声道、PCM16 WAV，可直接试听、提交给支持WAV的推理入口。
- `manifest.json`：每个音频的来源用例、声线、条件、时长、SHA-256，以及文本／问题和生成源码身份。
- `generation-source.py.txt`：生成音频时的实际源码快照，防止后续评测工具修复混淆生成来源。
- `index.html`：评测完成后生成的离线试听和错例浏览页。

共128条语义用例，两种声线生成256条干净音频；固定16组困难对照、32条语义用例，各声线加15dB合成噪声，另64条；静音、音调、噪声、点击4条无语言控制，总计324个音频文件。它们不是324个独立用户样本，配对和多声线需要按语义／family聚类分析。

## 标注约定

mode分布：navigate 55、inquire 55、clarify 6、cancel 4、human 4、other 4。功能：withdraw、deposit、transfer、balance、statement、pin、loss、replace、phone、mobile、unknown。

“帮我查余额”“能帮我打开转账吗”属于navigate；“查余额收费吗”“转账要多久到账”属于inquire。只有业务名、多个目标尚未决定、缺少指代时clarify。明确取消／转人工优先按当前要求；功能标签仍可保留明确的业务对象。历史仅用于消解最新诉求，不能覆盖当前否定和改口。音频中没有可辨识话语时先澄清。

这些是本测试定义的引导约定，机具支持的10类功能也是测试假设，不代表某家银行已确认的生产办理范围。navigate只表示引导进入功能页面，不代表已授权交易。

## 运行与复现

在仓库根目录：

```sh
python3 tests/evaluation/banking_audio.py validate
python3 tests/evaluation/banking_audio.py prepare
python3 tests/evaluation/banking_audio.py run \
 --url http://127.0.0.1:18084/v1/systemone \
 --output dist/banking-audio-fresh-results
python3 tests/evaluation/banking_audio.py audit --output dist/banking-audio-fresh-results
```

已有manifest时prepare只核验，不重新合成。首次生成使用macOS `say`／`afconvert`，声线Tingting与Eddy，速率190／175；在其他平台可直接使用已提交的WAV和标准库评测器。系统或声线版本变化可能改变合成音频，需另建数据集版本，而非覆盖本版。

主评测音频请求只包含WAV本身、统一问题和必要历史，不携带当前转写、gold、理由或文件名。文本对照使用同一模型和题目。主结果区分text、clean、noise15和control；24条候选顺序反转与12条交错重复只作诊断。错误请求计入分母，没有重试择优。

## 解读边界

这是原创文字场景＋本机合成语音，不含真实说话人录音、真实ATM环境音或方言。两声线使用不同语速，声线切片同时包含语速影响；15dB噪声是可重现的合成压力条件。该集适合发现意图规则和音频路径的问题，不能据此认证银行生产正确率。

评测报告位于 `tests/evaluation/results/2026-10-28-banking-audio-v1/`，保留原始响应、逐条失败、分层统计及模型身份。生产接入前还需按实际机具功能表和用户用语补充真实录音，并单独确认歧义处理及误导航容忍度。

主评测结束后补充了二分类和四选一辨句诊断，明确单列为post-hoc。辨句候选有原句线索，不代表ASR准确率，不替换主结果。详见结果报告。
