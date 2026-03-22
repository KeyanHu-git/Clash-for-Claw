# Clash for Claw

<div align="center">
  <img src="Assets/ClashForClaw-icon-preview.png" alt="Clash for Claw icon" width="88" />

  <p><strong>给 OpenClaw / Claw 准备的本地代理控制端</strong></p>
  <p>下载一个安装包，填几项必要参数，就能把订阅入口、后台驻留和状态可视化收进一个 Windows 应用里。</p>

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

## 怎么下载

- 普通用户直接去 [Releases](https://github.com/KeyanHu-git/Clash-for-Claw/releases) 下载 `Clash-for-Claw-x.y.z-setup.exe`
- 正式交付物就是安装版 `.exe`，不是让用户自己进文件夹找一堆可执行文件
- 安装包默认已经带好桌面端、后台组件和 `mihomo.exe`
- 安装完成后，启动 `Clash for Claw`，按下面几项配置即可开始用

当前公开版本：[`v0.1.3`](https://github.com/KeyanHu-git/Clash-for-Claw/releases/tag/v0.1.3)

## 第一次启动要配什么

第一次使用，通常只需要看 3 组参数：

1. `网关地址`
   OpenClaw / Claw 网关地址。  
   如果网关就在本机，通常可先试 `http://127.0.0.1:18789`。

2. `访问令牌`
   用于访问网关的令牌。  
   如果你的 OpenClaw 侧启用了鉴权，把对应 token 粘贴进来即可。

3. `入口模式`
   二选一即可：
   - `订阅模式`：在“订阅”页粘贴订阅 URL，这是默认推荐方式
   - `本地端口`：如果你本机已经有现成代理入口，就在“设置”里填本地端口，默认是 `7890`

## 最短上手流程

1. 安装并启动 `Clash for Claw`
2. 在首页填好 `网关地址` 和 `访问令牌`
3. 打开“订阅”页，粘贴订阅 URL
4. 点击“下载订阅”，再切回订阅模式
5. 首页看到“本地可用 / 网关可达 / 互联网可用”后，就说明链路已经通了

## 订阅一般怎么买

一般不是在这个项目里买，而是去购买一个明确支持 `Clash` / `Mihomo` 订阅链接的代理服务。

购买前建议至少确认这几件事：

- 对方能提供可直接粘贴的 `Clash` / `Mihomo` 订阅 URL
- 节点地区、流量额度、有效期是不是符合你的使用场景
- 订阅更新是否稳定，售后和失效处理是否清楚
- 支付方式、退款规则和风险说明是否透明

拿到订阅链接后，直接粘贴到“订阅”页即可，一行一个。

## 这个软件默认帮你做了什么

- 默认优先走订阅模式，失败时可回退到本地端口
- 默认绑定 `127.0.0.1`，不会强行改写你整台机器的系统级全局代理
- 支持后台驻留、开机自启、静默启动和服务模式
- 配置、日志和状态都保存在本地，敏感值不回显

补充说明：

- 在 Docker Desktop 环境里，`host.docker.internal` 仍可能访问宿主机 loopback 端口，所以这不是“容器完全不可见”的物理隔离
- 如果当前网络无法访问 GitHub，也可以手动把 `mihomo.exe` 放到应用目录下的 `bin` 中

## 给开发者

如果你是自己构建：

```powershell
git clone https://github.com/KeyanHu-git/Clash-for-Claw.git
cd Clash-for-Claw
dotnet build ClashForClaw.csproj -p:Platform=x64
```

生成默认发布包：

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\build-release-matrix.ps1 -Version 0.1.3 -MihomoPath C:\path\to\mihomo.exe
```

单独生成安装版 `.exe`：

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\build-installer.ps1 -Version 0.1.3 -MihomoPath C:\path\to\mihomo.exe
```

## 协议

本项目采用 [MIT License](LICENSE)。

## 致谢

- 数字生命背景动效参考：Shelter / MrIShelter，《一起赛博摸鱼吧——数学公式下的生命构型与动态》  
  来源：[和鲸社区](https://www.heywhale.com/mw/project/687e3f38c678037e34ebf61e)

## 支持一下

如果这个项目对你有帮助，欢迎点个 Star：

- https://github.com/KeyanHu-git/Clash-for-Claw
