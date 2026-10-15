# 安全策略

## 支持的版本

只有最新发布会收到安全修复：`latest` 镜像标签，或最新的 `v*` tag。

## 报告漏洞

**请不要开公开 Issue。** 使用 GitHub 的私密报告渠道：

仓库 → **Security** → **Advisories** → **Report a vulnerability**

若该入口不可见，说明维护者还没开启：仓库 → **Settings** → **Security** → 打开 **Private vulnerability reporting**。

报告里请包含：受影响的版本、复现步骤、影响面，以及是否已经有公开讨论。

## 范围界定

- 本项目包含 Exporter 源码、部署清单与文档。**上游 tinyproxy 自身的漏洞**请报给 [tinyproxy/tinyproxy](https://github.com/tinyproxy/tinyproxy)。
- Alpine 基础镜像与系统包的漏洞由上游跟进，同时通过 [`.github/dependabot.yml`](.github/dependabot.yml) 自动提 PR。
- 下面这些是**已知的配置风险，不算本项目漏洞**，但欢迎提改进建议：
  - `tinyproxy/tinyproxy.conf` 默认 `Allow 0.0.0.0/0`，配合端口暴露会成为一个开放代理
  - `tinyproxy/filter` 里是**示例白名单**，启用过滤后只有其中列出的域名可访问
  - 生产 compose 与 K8s 清单默认把端口绑在回环或集群内，但 `TINYPROXY_BIND=0.0.0.0` 会改变这一点

## 我们的响应

- 3 个工作日内确认收到报告
- 确认后给出修复计划与预计时间线
- 修复发布后通过 GitHub Security Advisory 公开，并在其中致谢（除非你希望匿名）
