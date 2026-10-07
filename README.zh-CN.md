# Codex 额度统计

[English](README.md) | [简体中文](README.zh-CN.md)

用于 CPA 的 Codex 实际用量统计插件，不包含预测功能。
待发布版本：**0.21.4**。仓库暂时保持**私有**，不提交公共插件市场。

## 统计内容

- 总账号使用额度，各账号可单独展开、收起。
- 累计请求、失败数、Tokens 和 USD/Credits 参考值。
- 按手动录入的真实付款日期划分订阅区间。
- 分开的月账本、周账本、五小时账本，最新在前，显示进行中状态。
- 最近 1、7、30 天的实际模型和服务层级用量。
- 上游返回的主额度、周额度观测，不预测容量、耗尽时间或未来用量。

K/M/B 分别表示千、百万、十亿，均保留单位，显示取整不改变账本数值。
USD/Credits 是按 token 单价换算的**参考值，不是实际扣费或订阅额度上限**。
推理、缓存 token 与输出、输入 token 有重叠，不重复相加；不同额度池可能
覆盖相同请求，不跨池相加用量或百分比。月账本按实际付款区间而非自然月划分。
首次观测可能是不完整的起点，安装前或写入失败的数据不能凭空恢复。

## 接入 CPA

插件 ID 保持为 `cpa-quota-estimator`，以兼容原有配置与数据库。
不要同时加载本插件与原插件。已有接入基线为 **CPA 8.0.16 / Linux amd64 /
原生 ABI v1**；其他版本和平台需要另行验证。

将 [docs/config.example.yaml](docs/config.example.yaml) 的相关字段合并到
现有 CPA 配置，升级时保留原 `data_path`。示例不含真实主机或凭据，
也不是可直接替换现有配置的完整文件。

页面路径：`/v0/resource/plugins/cpa-quota-estimator/dashboard`。
该路径仅提供静态页面，真实数据通过 CPA 已认证的管理 API 获取。
继续使用原有 HTTPS 管理入口，支持同源管理面板授权桥接。

管理 API 前缀：`/v0/management/cpa-quota-estimator`。

| 方法 | 路径 | 内容 |
| --- | --- | --- |
| GET | `/overview` | 账号、版本、丢失事件计数 |
| GET | `/accounting?account=<AuthID>&limit=50&offset=0` | 累计、订阅、重置账本 |
| GET | `/usage?account=<AuthID>&days=7` | 实际模型用量 |
| POST | `/subscriptions` | 新增或编辑 `{id,account,paid_at,amount_usd}` |
| DELETE | `/subscriptions` | 删除 `{account,id}` |

付款时间使用 Unix 秒，页面日期使用 Asia/Shanghai。
未知单价显示缺失，不根据额度消耗推算价格。

## 私有安装与发布

`plugin.json` 提供插件清单，`registry.json` 是 CPA 插件源条目，
并声明 `auth_required: true`。私有仓库的索引、仓库信息和 Release 下载
都需要 GitHub 认证；这个标记本身不提供凭据。按当前 CPA 版本支持的方式
配置认证，或先通过 GitHub 认证下载产物，再本地安装。
不要将令牌写入清单、索引或公开 URL。

在 Linux amd64、Go 1.22.12、具备 C 编译器的环境中构建：

```sh
go test ./...
go vet ./...
make build VERSION=0.21.4
make package VERSION=0.21.4
```

符合 CPA 安装约定的产物：

- `cpa-quota-estimator_0.21.4_linux_amd64.zip`
- `checksums.txt`：ZIP 的 SHA-256
- `plugin.json`：版本 `0.21.4`，Release 标签 `v0.21.4`

ZIP 根目录包含 `cpa-quota-estimator.so`。安装前核对校验值。
SQLite 开启 WAL 时应使用在线备份接口，不能只复制主数据库文件。
通过正常 CPA 维护流程更新插件，保留数据和回滚副本。
本仓库不包含远程部署脚本、服务器地址或部署密钥。

GitHub 的 **Private Release** 工作流仅手动触发，执行测试、打包 Linux amd64
并创建私有仓库内的**草稿 Release**，不部署、不上架。普通 CI 只做上传内容
检查和语法检查。手动工作流通过前，0.21.4 的运行时改动尚未完成功能验证。
CPA 的常规插件市场发现要求已发布的 Release；草稿不会被自动发现，
需由仓库所有者明确发布后才能通过该方式安装。

## 隐私与来源

数据库含账号标识、用量及付款记录，应连同 WAL/SHM、导出和备份一起保密。
响应头采集默认关闭，开启后仅保存额度字段白名单。
默认价格目录请求不会上传账号或用量，但会向 `models.dev` 暴露正常连接元数据。
详见 [SECURITY.md](SECURITY.md) 和 [代码审查记录](docs/CODE_REVIEW.md)。

基于 Autsunset 的 MIT 项目 `cpa-quota-estimator`，审查起点提交为
`ab33be577bec7f8b652b9228cfd7c953d4859826`。保留
[LICENSE](LICENSE) 中的原作者版权及嵌入式 Lucide 图标的 ISC 声明。
第三方说明见 [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md)。
