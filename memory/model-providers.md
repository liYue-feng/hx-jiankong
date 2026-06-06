---
name: dual-model-providers
description: 用户有两个备选模型提供商可在整个电脑范围内切换：火山引擎(ark-code-latest)和百度千帆(qianfan-code-latest)
metadata: 
  node_type: memory
  type: reference
---
# 备选模型提供商配置

用户有两个模型提供商可切换，配置作用于整个电脑（全局），非项目级别。

## 火山引擎 (Volcano Engine)
- 模型名: `ark-code-latest`
- ANTHROPIC_BASE_URL: https://ark.cn-beijing.volces.com/api/coding
- ANTHROPIC_AUTH_TOKEN: `<REDACTED>` (ark-xxxxxxxx)
- 环境变量配置:
  - `ANTHROPIC_AUTH_TOKEN`: `<REDACTED>`
  - `ANTHROPIC_BASE_URL`: https://ark.cn-beijing.volces.com/api/coding
  - `ANTHROPIC_MODEL`: ark-code-latest
  - `ANTHROPIC_DEFAULT_OPUS_MODEL`: ark-code-latest
  - `ANTHROPIC_DEFAULT_SONNET_MODEL`: ark-code-latest
  - `ANTHROPIC_DEFAULT_HAIKU_MODEL`: ark-code-latest
  - `ANTHROPIC_SMALL_FAST_MODEL`: ark-code-latest
  - `CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC`: 1
  - `API_TIMEOUT_MS`: 600000

## 百度千帆 (Baidu Qianfan)
- 模型名: `qianfan-code-latest`
- 环境变量配置:
  - `ANTHROPIC_AUTH_TOKEN`: `<REDACTED>` (bce-v3/ALTAK-xxxxxxxx)
  - `ANTHROPIC_BASE_URL`: https://qianfan.baidubce.com/anthropic/coding
  - `CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC`: 1
  - `API_TIMEOUT_MS`: 600000
  - `ANTHROPIC_MODEL`: qianfan-code-latest
  - `ANTHROPIC_SMALL_FAST_MODEL`: qianfan-code-latest
  - `ANTHROPIC_DEFAULT_HAIKU_MODEL`: qianfan-code-latest
  - `ANTHROPIC_DEFAULT_SONNET_MODEL`: qianfan-code-latest
  - `ANTHROPIC_DEFAULT_OPUS_MODEL`: qianfan-code-latest
- 通过 skill `切换百度千帆` 切换

## 注意事项
- 切换时需要更新整个电脑的环境变量，否则运行可能会卡死
- 两个 skill 分别对应: `切换火山引擎` 和 `切换百度千帆`
- 公开仓库中所有 token 已脱敏，原始 token 仅保留在本地内存
