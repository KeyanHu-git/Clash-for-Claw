# Clash for Claw

<div align="center">
  <img src="Assets/ClashForClaw-icon-preview.png" alt="Clash for Claw icon" width="88" />

  <p><strong>为 OpenClaw / OpenClaw Adapter 准备的一套更轻、更聚焦、更中文化的本地代理桌面前端。</strong></p>

  <p>
    <a href="https://github.com/KeyanHu-git/Clash-for-Claw/stargazers">
      <img src="https://img.shields.io/github/stars/KeyanHu-git/Clash-for-Claw?style=for-the-badge&logo=github&label=Stars" alt="GitHub stars" />
    </a>
    <img src="https://visitor-badge.laobi.icu/badge?page_id=KeyanHu-git.Clash-for-Claw&left_text=Visitors&left_color=0f172a&right_color=14b8a6&style=for-the-badge" alt="Visitors" />
    <img src="https://img.shields.io/github/last-commit/KeyanHu-git/Clash-for-Claw?style=for-the-badge&label=Updated" alt="Last commit" />
    <img src="https://img.shields.io/badge/WinUI%203-.NET%2010-0ea5a5?style=for-the-badge" alt="WinUI 3 .NET 10" />
  </p>
</div>

<p align="center">
  <img src="docs/readme-overview.png" alt="Clash for Claw overview" width="760" />
</p>

## 为什么要做这个

OpenClaw 这条链路需要的，其实不是一个“面向所有代理玩法的超级控制台”，而是一个：

- 只关心本机回环链路
- 对 OpenClaw / Adapter 足够友好
- 中文清晰
- 默认不打扰用户
- 能稳定常驻，也能在需要时完全静默

的桌面产品。

`Clash for Claw` 的目标不是替代所有 Clash GUI，而是把和 OpenClaw 最相关、最常用、最该顺手的那一小部分体验，做得更轻、更稳、更像产品。

## 它和 CFW 有什么不一样

> 不是要“打败 CFW”，而是走另一条更聚焦的产品路线。

| 维度 | Clash for Windows | Clash for Claw |
| --- | --- | --- |
| 定位 | 通用型 Clash 图形客户端 | 面向 OpenClaw / OpenClaw Adapter 的专用桌面前端 |
| 默认思路 | 功能全面、适合泛代理用户 | 聚焦主流程，尽量少按钮、少打扰 |
| 代理策略 | 常见场景偏系统代理工作流 | 默认只走本地回环链路，系统代理默认关闭，可手动开启 |
| 页面结构 | 功能多、配置深 | 概览 / 订阅 / 设置 / 日志与高级 / 说明 |
| 订阅体验 | 通用订阅管理 | 做 CFW 风格的订阅条，但围绕 Adapter 链路状态与切换来设计 |
| 后台驻留 | 更偏桌面客户端逻辑 | 桌面后台、托盘、服务模式、计划任务降级都考虑进来 |
| 产品目标 | 全能 | 小巧、精致、够用、顺手 |

一句话：

`Clash for Claw` 更像一个“为 OpenClaw 量身裁剪过的桌面控制面板”，而不是一个大而全的代理工具箱。

## 当前已经能做什么

### 已实现

- WinUI 3 桌面 UI，中文界面
- 侧栏式结构：概览 / 订阅 / 设置 / 日志与高级 / 说明
- 本地回环模式工作流
- 网关地址与令牌配置
- URL 中的 token 自动解析，但不回显
- 订阅模式与本地端口模式切换
- 订阅缺失时自动回退本地端口
- 本地服务、网关、互联网状态探测
- 订阅列表页、单条刷新、全量刷新、导入、本地删除等基础交互
- 订阅页支持 CFW 风格条形卡片基础样式
- 上下行速率与更新时间展示
- 设置持久化到本机配置文件
- 托盘常驻、开机自启、静默启动、关闭最小化到托盘
- 调起 `OpenClaw-Adapter.exe --daemon`
- 服务模式启停与计划任务降级逻辑
- 程序化动态背景与数学水母背景

### 正在继续打磨

- 服务模式长期稳定性
- 订阅自动切换与更多边界场景
- 发布与分发体验
- 更完整的图标、安装包与更新体验

## 适合谁

- 正在使用 OpenClaw，并且想把代理入口收拢到一个本地桌面工具里的人
- 不喜欢“设置很多、页面很多、看着很累”的代理类客户端的人
- 希望代理 UI 只负责配置和观察，后台自己安静跑的人

## 快速开始

### 运行前你需要知道

这个仓库当前是 **WinUI 前端**。

真正负责本地 API、订阅处理、Mihomo 子进程和后台模式的，是配套的：

- `OpenClaw-Adapter.exe`

也就是说：

- 这个仓库负责桌面界面
- `OpenClaw-Adapter.exe` 负责后台能力

默认情况下，前端会优先在应用目录旁边寻找 `OpenClaw-Adapter.exe`。

### 方式一：从源码运行

当前最稳妥的使用方式，是直接本地编译运行。

#### 1. 准备环境

- Windows 10 2004+ 或 Windows 11
- .NET 10 SDK
- WinUI 3 / Windows App SDK 开发环境
- 一份可运行的 `OpenClaw-Adapter.exe`

#### 2. 克隆仓库

```powershell
git clone https://github.com/KeyanHu-git/Clash-for-Claw.git
cd Clash-for-Claw
```

#### 3. 准备后端

把 `OpenClaw-Adapter.exe` 放到以下任一位置：

- 与本项目生成后的 `OpenClawAdapter.exe` 同目录
- 在“设置”页里手动指定 CLI 路径

#### 4. 构建

```powershell
dotnet build OpenClawAdapter.csproj -p:Platform=x64
```

#### 5. 启动

```powershell
.\bin\x64\Debug\net10.0-windows10.0.19041.0\win-x64\OpenClawAdapter.exe
```

### 首次使用建议

1. 在“概览”页确认后台已经拉起
2. 在“概览”页填写网关地址与令牌
3. 去“订阅”页添加你的订阅 URL
4. 先用订阅模式跑通，再决定是否启用本地端口模式
5. 如果你只想后台安静运行，再去“设置”页尝试服务模式

## 推荐使用方式

对大多数人来说，推荐顺序是：

1. 先用桌面后台模式跑通
2. 再打开开机自启 / 静默启动
3. 最后再尝试服务模式

这样排错最容易，体验也最稳。

## 本地文件与数据位置

- 界面设置：`%AppData%\OpenClawAdapter\settings.json`
- 服务模式数据目录：`%ProgramData%\OpenClawAdapter`
- 服务模式日志：`%ProgramData%\OpenClawAdapter\logs\adapter.log`

## 安全提醒

- 不要把真实订阅 URL、真实 token、真实网关令牌提交到仓库
- README、截图、示例配置只应使用占位符
- 即使界面支持从 URL 自动提取 token，也不意味着这些 URL 适合进入 Git 历史

## 为什么值得点一个 Star

如果你也在找这样一种东西：

- 不想被一大堆设置淹没
- 想让 OpenClaw 的代理入口更清楚
- 想要一个更轻、更紧凑、更像产品的本地桌面端

那这个项目就是为你做的。

如果它对你有帮助，欢迎给仓库点一个 Star：

- [Star Clash for Claw](https://github.com/KeyanHu-git/Clash-for-Claw)

这会让我更有动力把以下事情继续做下去：

- 更稳定的服务模式
- 更完整的订阅体验
- 更漂亮的安装与分发
- 更成熟的图标、品牌和视觉细节

## 致谢

### 数学水母动态背景

本项目背景灵感与可视化参考来源于和鲸社区转载内容，使用时应标注来源：

- 作者：Shelter / MrIShelter
- 标题：一起赛博摸鱼呀——数学公式下的生命构型与动态
- 时间：2025/08/04
- 来源：https://www.heywhale.com/mw/project/687e3f38c678037e34ebf61e

## 说明

- 本项目与 Clash for Windows 无官方从属关系
- 当前仍在快速迭代中，README 会随版本继续更新
