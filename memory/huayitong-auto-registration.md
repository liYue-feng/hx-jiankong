---
name: huayitong-auto-registration
description: 华医通自动挂号项目 - Go 后台 + Python tkinter GUI 桌面应用，位于 E:/hx_jiankong，已开源到 GitHub
type: project
---
# 华医通自动挂号项目

## 仓库
- GitHub: https://github.com/liYue-feng/hx-jiankong
- 本地: `E:/hx_jiankong/`

## 技术架构
- **后台服务**：Go 1.21 (`hx_jiankong.exe`)，端口 8088，HTTP API + WebSocket
- **桌面 GUI**：Python tkinter (`hx_gui.py`)，动森风格配色（暖羊皮纸 + 薄荷青）
- **Web GUI**：H5 动森风格（备用，`gui/index.html`）
- **窗口控制**：Win32 API，`PrintWindow`（不抢前台截图）+ `PostMessage`（异步点击）
- **OCR**：Tesseract（chi_sim），判断当前页面（"全部号源"=列表页 / "医生公告"=详情页 / "历史记录"=前页）
- **验证码**：云码 API (`api.ymocr.com`)，codetype=1004
- **推送**：Server酱 (`sctapi.ftqq.com`)
- **配置**：YAML 工作流引擎，热更新

## 两种模式
- **定点抢号**：到点（如 8:00）自动点预约按钮 → 截验证码 → 云码识别 → 输入 → 确认 → 锁号推送
- **监控补号**：循环进医生详情页 → 检测号源 → 有号自动抢 → 30-180s 随机间隔

## 关键文件
- `main.go` - 入口
- `engine/{types,workflow,window,ocr}.go` - 工作流引擎
- `gui/server.go` - HTTP/WebSocket 服务器
- `notify/notify.go` - Server酱推送
- `configs/{定点抢号,监控补号}.yaml` - 工作流配置
- `启动.bat` - 一键启动（同时拉起 Go 服务 + Python GUI）

## 凭证管理
- 公开仓库代码中已脱敏为 `YOUR_..._HERE` 占位符
- 实际凭证 (Server酱 Key / 云码 Token) 仅保存在本地，不进入版本控制
- 运行时通过环境变量 `SERVERCHAN_KEY` 或本地 YAML 字段配置

## 夜间免打扰
01:00 - 06:00 自动休眠（之前是 22:00-07:00，已改）

## 待办
- 坐标校准（医生卡片、返回按钮、搜索按钮、排期 OCR 区域、入口流程各按钮）
- Python GUI tkinter 不支持 8 位 hex 色值（如 `#ffffff30`），需用普通 6 位色
