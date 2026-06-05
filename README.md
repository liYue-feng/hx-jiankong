# 华医通自动挂号助手 (HX Jiankong)

> 基于 Windows UI 自动化的华医通小程序自动监控、抢号、补号工具。
> Go 后台服务 + Python 桌面 GUI + Web GUI 双前端，支持云码验证码识别、Server酱微信推送。

## ✨ 功能特性

- 🎯 **定点抢号**：到点自动点击预约按钮，云码识别验证码，锁号后微信推送
- 🔄 **监控补号**：循环检查目标医生排期，发现号源立即抢号
- 🪟 **后台运行**：使用 `PrintWindow` API 截图，`PostMessage` 投递点击，**不抢前台焦点**，可正常使用电脑
- 🤖 **OCR 识别**：Tesseract 识别页面文字（"全部号源"/"医生公告"/"历史记录"），自动判定当前页面
- 🎨 **动森风格 GUI**：暖羊皮纸配色 + 薄荷青强调色，Python tkinter 桌面窗口
- 🌐 **Web GUI**：动森风格 H5 界面（pill 圆角 + 3D 阴影），浏览器访问
- 📱 **微信推送**：Server酱集成，验证码出现/锁号成功/无号源都推送
- 🌙 **夜间免打扰**：01:00 - 06:00 自动休眠
- ♻️ **热更新**：修改 YAML 配置自动生效，无需重启

## 📋 技术栈

| 层 | 技术 |
|----|------|
| 后台服务 | Go 1.21 + net/http + WebSocket |
| 工作流引擎 | Go (engine/workflow.go) |
| 窗口控制 | Win32 API (PrintWindow / PostMessage / EnumWindows) |
| OCR | Tesseract OCR (中文+英文) |
| 验证码识别 | 云码 OCR API (codetype 1004) |
| 桌面 GUI | Python 3.10+ tkinter |
| Web GUI | HTML + CSS (Nunito + Noto Sans SC + Zen Maru Gothic) |
| 配置 | YAML (工作流) + JSON (运行时) |
| 推送 | Server酱 (sctapi.ftqq.com) |

## 🚀 快速开始

### 前置依赖

1. **Go 1.21+**（用于编译后台服务）
2. **Python 3.10+**（用于桌面 GUI，依赖：`requests`, `pillow`, `pyautogui`, `pywin32`）
3. **Tesseract OCR**（中文识别模型）—— `winget install tesseract-ocr.tesseract`
4. **PC 微信**（华医通小程序窗口需要打开）

### 编译

```bash
cd hx_jiankong
go mod tidy
go build -o hx_jiankong.exe .
pip install requests pillow pyautogui pywin32
```

### 启动

**双击 `启动.bat`** 即可，会同时启动：
- Go 服务器（http://127.0.0.1:8088）
- Python 桌面窗口

或手动启动：

```bash
hx_jiankong.exe       # 启动后台服务（后续会自动打开浏览器）
python hx_gui.py      # 启动桌面 GUI
```

## 📂 项目结构

```
hx_jiankong/
├── 启动.bat              ← 一键启动脚本
├── main.go               ← Go 服务器入口
├── hx_gui.py             ← Python tkinter 桌面窗口
├── engine/
│   ├── types.go          ← 数据结构定义
│   ├── workflow.go       ← 工作流引擎（YAML 步骤解释器）
│   ├── window.go         ← Win32 窗口截图 / 点击
│   └── ocr.go            ← Tesseract OCR 封装
├── gui/
│   ├── server.go         ← HTTP API + WebSocket 服务器
│   └── index.html        ← 动森风格 Web GUI
├── notify/
│   └── notify.go         ← Server酱推送 + 云码识别
├── configs/
│   ├── 定点抢号.yaml     ← 定点抢号工作流
│   └── 监控补号.yaml     ← 监控补号工作流
├── png/                  ← 页面参考截图（坐标校准用）
├── screenshots/          ← 运行时截图
└── logs/                 ← 运行日志
```

## 🎮 使用流程

### 模式一：定点抢号

适用场景：医院 8:00 整放号，提前进入医生详情页等待。

1. 提前 5 分钟：手动进入华医通 → 医生详情页（看到带倒计时的灰色"预约"按钮）
2. 双击 `启动.bat`，选择"定点抢号"
3. 填入：就诊人姓名、医生姓名
4. 点击启动
5. 程序自动等到按钮变绿 → 点击 → 截图验证码 → 云码识别 → 自动输入 → 点确认
6. 锁号成功 → Server酱推送，你手动支付即可

### 模式二：监控补号

适用场景：晚上 20:00 后医院放退号，需要循环检测。

1. 双击 `启动.bat`，选择"监控补号"
2. 填入：就诊人姓名、科室、医生姓名
3. 点击启动
4. 程序自动从小程序首页 → 预约挂号 → 选就诊人 → 搜索医生 → 进详情页
5. 检测排期 → 有号立即抢 → 无号回退等 30-180 秒重试
6. 小程序超时退出会自动重新进入

## ⚙️ 配置说明

### `configs/*.yaml`（工作流）

```yaml
window_keyword: "华医通"          # 窗口关键字
night_start: "01:00"               # 夜间休眠开始
night_end: "06:00"                 # 夜间休眠结束
sleep_min: 30                      # 轮询间隔下限（秒）
sleep_max: 180                     # 轮询间隔上限（秒）
sct_key: "SCT...."                 # Server酱 Key
yunma_token: "...."                # 云码 Token

steps:
  - name: "点击医生卡片"
    action: click
    x: 340
    y: 335
    wait_after: 2
```

支持的 action：`click` / `wait` / `random_wait` / `screenshot` / `ocr` / `template_match` / `keypress`

### 坐标校准

需要根据实际页面调整的坐标见 [`docs/坐标配置.md`](#待补充)，主要包括：

| 阶段 | 坐标项 |
|------|--------|
| 入口 | 诊疗服务按钮、预约挂号、就诊卡、挂号须知、搜索栏、搜索输入框 |
| 监控 | 医生卡片、返回按钮、搜索按钮、排期 OCR 区域、标题 OCR 区域 |
| 抢号 | 预约按钮、验证码输入框、确认按钮 |

**校准方法**：GUI 点击"截图调试" → 用画图打开 `screenshots/*.png` → 鼠标量坐标 → 填入 YAML

## 🔐 关于安全与合规

- 本工具仅用于个人挂号便利，**严禁倒卖号源**
- 验证码识别使用商业 API，遵守平台规则
- 请勿高频请求，本工具默认轮询间隔 30-180 秒
- 仓库中 Token 已脱敏，使用前请替换为你自己的

## 📜 License

MIT
