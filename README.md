# Clash for Claw

<div align="center">
  <img src="Assets/ClashForClaw-icon-preview.png" alt="Clash for Claw icon" width="88" />

  <p><strong>面向 OpenClaw / Claw 链路的本地代理控制端</strong></p>
  <p>把订阅接入、后台驻留和链路状态整合进一个开箱即用的 Windows 程序。</p>

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

## 安装包

- 正式发布物为安装版 `Clash-for-Claw-x.y.z-setup.exe`
- 安装包默认包含桌面端、后台组件与 `mihomo.exe`
- 正常使用不需要额外下载运行组件

当前公开版本：[`v0.1.1`](https://github.com/KeyanHu-git/Clash-for-Claw/releases/tag/v0.1.1)

## 项目定位

`Clash for Claw` 不是 OpenClaw 的替代品，也不改动 OpenClaw 仓库本身。它负责的是 Windows 侧这层本地控制面，把代理入口、后台驻留和链路状态集中到一个独立的桌面程序里。

它主要解决三件事：

- 在订阅模式和本地端口之间做统一切换
- 为后台驻留、开机启动和服务模式提供稳定入口
- 把本地服务、网关和外网状态集中展示出来，便于排查

## 首次使用前要准备什么

第一次接入通常只需要准备 3 项内容：

1. `OpenClaw 网关地址`
   如果网关就在本机，直接先填 `http://127.0.0.1:18789`。

2. `OpenClaw 访问令牌`
   对应 OpenClaw 的 `gateway.auth.token`。

3. `代理入口`
   二选一即可：
   - `订阅模式`：准备一条支持 `Clash` / `Mihomo` 的订阅 URL
   - `本地端口`：如果本机已经有现成代理入口，准备好端口号，默认是 `7890`

## 这些内容从哪里获取

### 1. 网关地址

- 本机默认先试 `http://127.0.0.1:18789`
- 如果需要从 OpenClaw 侧确认，可执行 `openclaw dashboard --no-open`

### 2. 访问令牌

- 可执行 `openclaw config get gateway.auth.token`
- 也可以直接查看 OpenClaw 配置中的 `gateway.auth.token`

如果拿到的是 `http://127.0.0.1:18789/#token=...` 这类完整链接，不要整串粘贴到一个输入框里。  
在本软件中应当分开填写：

- `网关地址`：`http://127.0.0.1:18789`
- `访问令牌`：`#token=` 后面的那段 token

### 3. 订阅 URL

订阅一般来自代理服务提供方后台的 `Clash` / `Mihomo` 导入入口。本项目不提供订阅，也不内置节点。

接入前建议至少确认：

- 能直接提供 `Clash` / `Mihomo` 订阅链接
- 流量额度、有效期和节点地区与实际场景匹配
- 订阅更新策略明确
- 服务规则、售后方式和风险说明清楚

## 怎么填写

### 首页

- `网关地址`：填写 OpenClaw 网关地址
- `访问令牌`：填写 OpenClaw 的 token

### 订阅页

- 把订阅 URL 粘贴到输入框
- 支持一行一条
- 点击 `导入 URL`
- 选中要启用的订阅

### 设置页

- `本地端口`：只在本地端口模式下需要调整
- 默认值是 `7890`

## 快速开始

1. 下载并安装 `Clash-for-Claw-x.y.z-setup.exe`
2. 启动 OpenClaw 网关
3. 打开 `Clash for Claw`
4. 在首页填写 `网关地址` 和 `访问令牌`
5. 在“订阅”页导入订阅 URL，或在“设置”页填写本地端口
6. 回到首页，确认 `本地服务 / 网关 / 外网` 三项状态正常

## 后台与服务模式

- 默认可以作为桌面程序常驻运行
- 可选开启 `Windows 服务模式`
- 服务模式通常需要管理员权限
- 如果服务注册失败，程序会提示是否回退为计划任务

数据目录默认分为两处：

- 桌面模式：`%AppData%\ClashForClaw`
- 服务模式：`%ProgramData%\ClashForClaw`

服务模式使用公共数据目录，是为了避免把运行时数据绑死在安装目录里。这样更新、覆盖安装或移动程序目录时，后台配置和日志不会一起丢失。

## 运行说明

- 默认围绕本地代理入口工作，不改写整机网络出口
- 默认监听 `127.0.0.1`
- 配置、日志和状态信息都保存在本地
- OpenClaw 仓库可以独立更新，不依赖本项目的发布节奏

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
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\build-release-matrix.ps1 -Version 0.1.1 -MihomoPath C:\path\to\mihomo.exe
```

单独生成安装版 `.exe`：

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\build-installer.ps1 -Version 0.1.1 -MihomoPath C:\path\to\mihomo.exe
```

## 协议

本项目采用 [MIT License](LICENSE)。

## 致谢

- 数字生命背景动效参考：Shelter / MrIShelter，《一起赛博摸鱼吧——数学公式下的生命构型与动态》  
  来源：[和鲸社区](https://www.heywhale.com/mw/project/687e3f38c678037e34ebf61e)
