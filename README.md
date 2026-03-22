# Clash for Claw

<div align="center">
  <img src="Assets/ClashForClaw-icon-preview.png" alt="Clash for Claw icon" width="88" />

  <p><strong>面向 OpenClaw / Claw 链路的本地代理控制端</strong></p>
  <p>将订阅入口、后台驻留和链路状态收拢到一个可直接交付的 Windows 安装程序中。</p>

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

## 发布形式

- 正式发布物为安装版 `Clash-for-Claw-x.y.z-setup.exe`
- 安装包默认包含桌面端、后台组件与 `mihomo.exe`
- `portable.zip` 仅保留给手动部署或调试场景

当前公开版本：[`v0.1.2`](https://github.com/KeyanHu-git/Clash-for-Claw/releases/tag/v0.1.2)

## 为什么做这个事情

OpenClaw / Claw 在 Windows 环境中运行时，链路稳定性往往不取决于模型本身，而取决于宿主机是否具备一个长期可用、可观察、可维护的本地代理入口。

如果这一层缺失，常见问题会混在一起出现：

- 订阅入口与本地端口切换缺少统一控制
- 后台驻留、开机启动和静默运行缺少稳定承载
- 网关可达性、互联网连通性和流量状态缺少直观反馈

`Clash for Claw` 的定位不是修改 OpenClaw 仓库本身，而是在其外部提供一层独立的本地控制面：

- 前台负责配置、状态确认和可视化反馈
- 后台负责静默运行、驻留和服务化
- 代理入口与 OpenClaw 仓库保持解耦，便于后续独立更新

## 首次配置

首次启动通常只需要完成 3 组参数：

1. `网关地址`
   OpenClaw / Claw 网关地址。  
   如果网关就在本机，通常可以先试 `http://127.0.0.1:18789`。

2. `访问令牌`
   网关鉴权令牌。  
   如果 OpenClaw 侧启用了鉴权，将对应 token 粘贴到此处即可。

3. `入口模式`
   二选一即可：
   - `订阅模式`：在“订阅”页粘贴订阅 URL，这是默认推荐方式
   - `本地端口`：如果本机已经存在现成代理入口，可在“设置”页填写本地端口，默认是 `7890`

## 快速接入

1. 下载并安装 `setup.exe`
2. 启动 `Clash for Claw`
3. 在首页填写 `网关地址` 和 `访问令牌`
4. 打开“订阅”页，粘贴订阅 URL
5. 点击“下载订阅”，再切回订阅模式
6. 首页出现“本地可用 / 网关可达 / 互联网可用”后，即表示链路已经连通

## 订阅来源

本项目不提供订阅服务。接入订阅模式时，需要准备一条支持 `Clash` / `Mihomo` 的订阅 URL。

获取订阅时，建议至少确认以下事项：

- 可提供直接导入的 `Clash` / `Mihomo` 订阅链接
- 节点地区、流量额度、有效期与实际场景匹配
- 订阅更新策略清晰且稳定
- 支付方式、售后规则和风险说明透明

拿到订阅链接后，直接粘贴到“订阅”页即可，一行一个。

## 运行边界

- 默认工作流围绕本地入口构建，不要求改写整机网络出口
- 默认监听 `127.0.0.1`
- 支持后台驻留、开机自启、静默启动和服务模式
- 配置、日志与状态信息均保存在本地

补充说明：

- 在 Docker Desktop 环境中，`host.docker.internal` 仍可能访问宿主机 loopback 端口，因此这不是物理隔离
- 如果当前网络无法访问 GitHub，也可以手动将 `mihomo.exe` 放到应用目录下的 `bin` 中

## 构建

```powershell
git clone https://github.com/KeyanHu-git/Clash-for-Claw.git
cd Clash-for-Claw
dotnet build ClashForClaw.csproj -p:Platform=x64
```

生成默认发布包：

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\build-release-matrix.ps1 -Version 0.1.2 -MihomoPath C:\path\to\mihomo.exe
```

单独生成安装版 `.exe`：

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\build-installer.ps1 -Version 0.1.2 -MihomoPath C:\path\to\mihomo.exe
```

## 协议

本项目采用 [MIT License](LICENSE)。

## 致谢

- 数字生命背景动效参考：Shelter / MrIShelter，《一起赛博摸鱼吧——数学公式下的生命构型与动态》  
  来源：[和鲸社区](https://www.heywhale.com/mw/project/687e3f38c678037e34ebf61e)

## 支持

- https://github.com/KeyanHu-git/Clash-for-Claw
