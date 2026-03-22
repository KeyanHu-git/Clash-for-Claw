# Clash for Claw

<div align="center">
  <img src="Assets/ClashForClaw-icon-preview.png" alt="Clash for Claw icon" width="88" />

  <p><strong>面向 OpenClaw / Claw 链路的本地桌面控制端。</strong></p>
  <p>把代理入口、后台驻留和可视化配置收拢到一个更安静、更容易长期使用的 Windows 工具里。</p>

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

## Package defaults

- `full`: default release profile. It is self-contained and now prefers the repo-bundled `mihomo.exe`, so subscription mode is available immediately after install in normal builds.
- `slim`: smallest profile. It keeps backend support but leaves `mihomo` on the runtime auto-download path.
- `scripts/build-release-matrix.ps1` now emits `full` and `slim`. The old `full-subscription` role is folded into `full`.
- `-MihomoPath` still works as an override when a custom `mihomo.exe` must be injected during packaging.

## 为什么做这个项目

本项目源于 Windows 环境下通过 Docker 使用 Claw 的实际场景。在这一部署方式下，宿主机如果缺少稳定的代理出口，Claw 的联网链路就容易受到影响，进而影响相关能力的持续使用体验。

`Clash for Claw` 正是在这一背景下构建的。项目聚焦于为 Claw 提供一条可长期驻留的本地代理链路：前台负责可视化配置与状态确认，后台负责静默运行，并可按需注册为 Windows 服务。桌面界面的加入，则让不同技术背景的用户都能更直观地完成配置、观察状态并安心使用。

## 核心亮点

- 围绕 OpenClaw / Claw 的实际链路设计，聚焦本地代理入口与关键状态管理
- 支持订阅模式与本地端口模式，关键状态集中可见
- 支持后台驻留、开机自启、静默启动和服务模式
- 中文界面、敏感值不回显、配置与日志本地持久化
- 安装版默认内置后台组件与 `mihomo.exe`，同时保留 `slim` 轻量路径，适合在个人 Windows 环境长期运行

## 快速开始

### 方式一：下载发布版本

1. 打开 [Releases](https://github.com/KeyanHu-git/Clash-for-Claw/releases) 下载最新版本。
2. 默认推荐 `full` 版：安装后即可直接使用订阅模式，正常构建会优先内置仓库自带的 `mihomo.exe`。
3. 如果你更看重体积，可选择 `slim` 版；它会保留后台组件，但让 `mihomo` 走按需自动下载路径。
4. 双击安装，程序会被安装到当前用户目录，并自动部署内置后台组件与开始菜单入口。
5. 启动 `Clash for Claw`，填写网关地址与令牌。
6. 在“订阅”页添加订阅，或切换到本地端口模式。
7. 验证联网正常后，再按需开启开机自启、静默启动或服务模式。

说明：

- `full` 版会优先打入仓库内置的 `mihomo.exe`，因此更适合直接交付给最终用户。
- `slim` 版默认不捆绑 `mihomo.exe`，首次进入订阅模式时会尝试自动下载最新兼容版。
- 如果当前网络无法访问 GitHub，可手动把 `mihomo.exe` 放到应用目录下的 `bin` 中，或设置环境变量 `CLASH_FOR_CLAW_MIHOMO_DOWNLOAD_URL` / `CLASH_FOR_CLAW_MIHOMO_RELEASE_API_URL` 指向自定义源。
- 当前实现默认绑定 `127.0.0.1`，不会改写本机的全局出站路径；但在 Docker Desktop 环境下，`host.docker.internal` 仍可能访问这些 loopback 端口，所以它还不等于“对容器完全隔离”。

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

如需一次性生成当前默认的 `full` / `slim` 两种发布物，可直接执行：

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\build-release-matrix.ps1 -Version 0.1.2 -MihomoPath C:\path\to\mihomo.exe
```

`build-release-matrix.ps1` currently produces two artifacts by default: `full` and `slim`. `full` is the user-facing default and replaces the old `full-subscription` packaging role.

如需单独构建带 `mihomo.exe` sidecar 的安装包，可在打包时显式提供路径：

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\build-installer.ps1 -Version 0.1.2 -MihomoPath C:\path\to\mihomo.exe
```

说明：

- 订阅模式依赖独立的 `mihomo.exe` 运行时，不再编译进 `ClashForClaw.Service.exe`
- 默认用户路径是 `full = self-contained + bundled mihomo`，`slim` 继续保留为 `framework-dependent + runtime auto-download`
- 本地调试可将 `mihomo.exe` 放在 `backend/ClashForClaw.Service/bin/`，或设置环境变量 `CLASH_FOR_CLAW_MIHOMO_PATH`
- 如果需要自定义下载源，可设置 `CLASH_FOR_CLAW_MIHOMO_DOWNLOAD_URL` 或 `CLASH_FOR_CLAW_MIHOMO_RELEASE_API_URL`
- 打包安装器时可通过 `scripts/build-installer.ps1 -MihomoPath <path-to-mihomo.exe>` 显式提供运行时二进制

## 发布

- 当前公开版本：[v0.1.2](https://github.com/KeyanHu-git/Clash-for-Claw/releases/tag/v0.1.2)

## 协议

本项目采用 [MIT License](LICENSE)。

## 致谢

- 数学生命背景动效参考：Shelter / MrIShelter，《一起赛博摸鱼呀——数学公式下的生命构型与动态》  
  来源：[和鲸社区](https://www.heywhale.com/mw/project/687e3f38c678037e34ebf61e)

## 支持一下

如果这个项目对你有帮助，欢迎给仓库点一个 Star：

- https://github.com/KeyanHu-git/Clash-for-Claw
