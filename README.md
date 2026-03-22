# Clash for Claw

<div align="center">
  <img src="Assets/ClashForClaw-icon-preview.png" alt="Clash for Claw icon" width="88" />

  <p><strong>面向 OpenClaw / Claw 链路的本地桌面控制端</strong></p>
  <p>把代理入口、后台驻留和可视化配置收拢到一个更安静、更适合长期运行的 Windows 工具里。</p>

  <p>
    <a href="https://github.com/KeyanHu-git/Clash-for-Claw/releases">
      <img src="https://img.shields.io/github/v/release/KeyanHu-git/Clash-for-Claw?style=flat-square&label=Release" alt="GitHub release" />
    </a>
    <img src="https://img.shields.io/github/license/KeyanHu-git/Clash-for-Claw?style=flat-square&label=License" alt="License" />
    <a href="https://github.com/KeyanHu-git/Clash-for-Claw/stargazers">
      <img src="https://img.shields.io/github/stars/KeyanHu-git/Clash-for-Claw?style=flat-square&label=Stars" alt="GitHub stars" />
    </a>
    <img src="https://visitor-badge.laobi.icu/badge?page_id=KeyanHu-git.Clash-for-Claw&left_text=Visitors&left_color=64748b&right_color=14b8a6&style=flat-square" alt="Visitors" />
  </p>
</div>

<p align="center">
  <img src="docs/readme-hero.png" alt="Clash for Claw overview" width="920" />
</p>

## 发布说明

- 发布只保留一个用户向安装包。
- 默认产物为 self-contained Windows x64 版本，并优先打入仓库内置的 `mihomo.exe`。
- `scripts/build-release-matrix.ps1` 现在只输出一套正式发布物。
- 如果需要替换打包时使用的 `mihomo.exe`，仍然可以通过 `-MihomoPath` 显式注入。

## 为什么做这个项目

这个项目来源于 Windows 环境下通过 Docker 使用 OpenClaw / Claw 的实际链路需求。  
当宿主机缺少稳定、可控、可长期驻留的本地代理入口时，模型联网能力、订阅切换与日常使用体验都容易一起退化。

`Clash for Claw` 的目标不是去改变 OpenClaw 仓库本身，而是在它之外提供一个独立、稳定、可视化的本地代理控制层：

- 前台负责配置、状态确认和用户可见反馈
- 后台负责静默运行、开机启动与服务化
- 本地代理入口保持与 OpenClaw 仓库解耦，便于 OpenClaw 后续自由更新

## 核心特性

- 围绕 OpenClaw / Claw 的真实联网链路设计，聚焦本地代理入口与关键状态可视化
- 支持订阅模式与本地端口模式，界面内集中展示主要运行状态
- 支持后台驻留、开机自启、静默启动与服务模式
- 中文界面，敏感值不回显，配置与日志本地持久化
- 正式发布包默认内置后端组件与 `mihomo.exe`，安装后即可直接进入完整流程

## 快速开始

### 方式一：下载发布版本

1. 打开 [Releases](https://github.com/KeyanHu-git/Clash-for-Claw/releases) 下载最新版本。
2. 下载安装包后，程序会部署桌面端、后台组件以及默认使用的 `mihomo.exe`。
3. 启动 `Clash for Claw`，填写网关地址与令牌。
4. 在“订阅”页添加订阅，或切换到本地端口模式。
5. 验证联网正常后，再按需开启开机自启、静默启动或服务模式。

说明：

- 默认实现绑定 `127.0.0.1`，不会改写本机系统级全局代理。
- 在 Docker Desktop 环境中，`host.docker.internal` 仍可能访问宿主机 loopback 端口，所以这不是“对容器完全不可见”的物理隔离，而是“对本机网络设置不侵入”的软件隔离。
- 如果当前网络无法访问 GitHub，可手动将 `mihomo.exe` 放到应用目录下的 `bin` 中，或设置环境变量 `CLASH_FOR_CLAW_MIHOMO_DOWNLOAD_URL` / `CLASH_FOR_CLAW_MIHOMO_RELEASE_API_URL` 指向自定义源。

### 方式二：从源码构建

环境要求：

- Windows 10 2004+ 或 Windows 11
- .NET 10 SDK
- WinUI 3 / Windows App SDK 开发环境

```powershell
git clone https://github.com/KeyanHu-git/Clash-for-Claw.git
cd Clash-for-Claw
dotnet build ClashForClaw.csproj -p:Platform=x64
```

生成默认发布物：

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\build-release-matrix.ps1 -Version 0.1.3 -MihomoPath C:\path\to\mihomo.exe
```

单独构建安装包：

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\build-installer.ps1 -Version 0.1.3 -MihomoPath C:\path\to\mihomo.exe
```

补充说明：

- 订阅模式依赖独立的 `mihomo.exe` 运行时进程。
- 本地调试时可将 `mihomo.exe` 放在 `backend/ClashForClaw.Service/bin/`，或设置环境变量 `CLASH_FOR_CLAW_MIHOMO_PATH`。
- 如果需要自定义下载源，可设置 `CLASH_FOR_CLAW_MIHOMO_DOWNLOAD_URL` 或 `CLASH_FOR_CLAW_MIHOMO_RELEASE_API_URL`。

## 发布

- 当前公开版本：[v0.1.3](https://github.com/KeyanHu-git/Clash-for-Claw/releases/tag/v0.1.3)

## 协议

本项目采用 [MIT License](LICENSE)。

## 致谢

- 数字生命背景动效参考：Shelter / MrIShelter，《一起赛博摸鱼吧——数学公式下的生命构型与动态》
  来源：[和鲸社区](https://www.heywhale.com/mw/project/687e3f38c678037e34ebf61e)

## 支持一下

如果这个项目对你有帮助，欢迎给仓库点一个 Star：

- https://github.com/KeyanHu-git/Clash-for-Claw
