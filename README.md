# Clash for Claw

<div align="center">
  <img src="Assets/ClashForClaw-icon-preview.png" alt="Clash for Claw icon" width="88" />

  <p><strong>面向 OpenClaw / Claw 链路的本地桌面控制端。</strong></p>
  <p>把代理入口、后台驻留和可视化配置收拢到一个更安静、更容易长期使用的 Windows 工具里。</p>

  <p>
    <a href="https://github.com/KeyanHu-git/Clash-for-Claw/stargazers">
      <img src="https://img.shields.io/github/stars/KeyanHu-git/Clash-for-Claw?style=flat-square&label=Stars" alt="GitHub stars" />
    </a>
    <a href="https://github.com/KeyanHu-git/Clash-for-Claw/releases">
      <img src="https://img.shields.io/github/v/release/KeyanHu-git/Clash-for-Claw?style=flat-square&label=Release" alt="GitHub release" />
    </a>
    <img src="https://visitor-badge.laobi.icu/badge?page_id=KeyanHu-git.Clash-for-Claw&left_text=Visitors&left_color=0f172a&right_color=14b8a6&style=flat-square" alt="Visitors" />
    <img src="https://img.shields.io/github/license/KeyanHu-git/Clash-for-Claw?style=flat-square&label=License" alt="License" />
  </p>
</div>

<p align="center">
  <img src="docs/readme-hero.png" alt="Clash for Claw overview" width="920" />
</p>

## 为什么做这个项目

我是在 Windows 环境里通过 Docker 使用 Claw。实际跑下来发现，只要宿主机没有开启全局代理，Claw 的出网链路就不够稳定，很多能力部署好了也不容易真正用顺手。

`Clash for Claw` 就是为这个问题写的。它的核心目标不是做一个花哨的前端，而是给 Claw 提供一条可长期驻留的本地代理链路：前台可视化配置，后台静默运行，真正需要时还可以注册成 Windows 服务。桌面界面之所以存在，是为了让不熟悉命令行、服务管理和代理配置的朋友也能安心使用。

## 核心亮点

- 围绕 OpenClaw / Claw 的实际链路设计，不做无关的大而全面板
- 支持订阅模式与本地端口模式，关键状态集中可见
- 支持后台驻留、开机自启、静默启动和服务模式
- 中文界面、敏感值不回显、配置与日志本地持久化
- 配合 `OpenClaw-Adapter.exe` 使用，适合在个人 Windows 环境长期运行

## 快速开始

### 方式一：下载发布版本

1. 打开 [Releases](https://github.com/KeyanHu-git/Clash-for-Claw/releases) 下载最新版本。
2. 准备可运行的 `OpenClaw-Adapter.exe`。
3. 启动 `Clash for Claw`，填写网关地址与令牌。
4. 在“订阅”页添加订阅，或切换到本地端口模式。
5. 验证联网正常后，再按需开启开机自启、静默启动或服务模式。

### 方式二：从源码构建

环境要求：

- Windows 10 2004+ 或 Windows 11
- .NET 10 SDK
- WinUI 3 / Windows App SDK 开发环境

```powershell
git clone https://github.com/KeyanHu-git/Clash-for-Claw.git
cd Clash-for-Claw
dotnet build OpenClawAdapter.csproj -p:Platform=x64
```

## 发布

- 当前公开版本：[v0.1.0](https://github.com/KeyanHu-git/Clash-for-Claw/releases/tag/v0.1.0)

## 协议

本项目采用 [MIT License](LICENSE)。

## 致谢

- 数学生命背景动效参考：Shelter / MrIShelter，《一起赛博摸鱼呀——数学公式下的生命构型与动态》  
  来源：[和鲸社区](https://www.heywhale.com/mw/project/687e3f38c678037e34ebf61e)

## 支持一下

如果这个项目对你有帮助，欢迎给仓库点一个 Star：

- https://github.com/KeyanHu-git/Clash-for-Claw
