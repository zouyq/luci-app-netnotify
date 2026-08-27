# luci-app-netnotify

[![CI](https://github.com/zouyq/luci-app-netnotify/actions/workflows/ci.yml/badge.svg)](https://github.com/zouyq/luci-app-netnotify/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

低资源友好的 OpenWrt 局域网设备上下线通知插件：Go 守护进程 + LuCI JS（无 Lua）。

| 名称 | 值 |
|------|-----|
| 包名 | `luci-app-netnotify` |
| 二进制 | `netnotifyd` |
| UCI | `netnotify` |
| License | MIT |

## 功能

- 事件驱动上下线：netlink 邻居为主，ARP 仅用于 `pending_up` / `suspect`
- 推送渠道：钉钉机器人、企业微信机器人、企业微信应用、Bark、通用 Webhook
- MAC 别名 / 黑白名单、内置 OUI 厂商名回退
- 上下线消息可附带对齐的在线设备列表与在线时长
- 定时状态汇报（可选）

## 目录

```
.
├── Makefile                 # OpenWrt 包
├── src/                     # Go module (github.com/zouyq/netnotify)
├── root/                    # UCI / init / menu / ACL / oui_base.txt
├── htdocs/                  # LuCI JS
├── scripts/gen-oui.sh       # 可选：从 IEEE 下载并精简 OUI 库
└── .github/workflows/       # CI / Release
```

## 本机开发

```bash
cd src
go test ./...
go build -o ../bin/netnotifyd ./cmd/netnotifyd
./../bin/netnotifyd -config ./example.json test
```

交叉编译示例：

```bash
cd src
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags="-s -w" \
  -o ../bin/netnotifyd ./cmd/netnotifyd
```

CI 会构建：`linux-amd64` / `linux-arm64` / `linux-armv7` / `linux-mipsle`。

## OpenWrt 编译

将本仓库放入 `package/luci-app-netnotify`（或 feeds），然后：

```bash
make package/luci-app-netnotify/compile V=s
```

也可从 [Releases](https://github.com/zouyq/luci-app-netnotify/releases) 下载对应架构的 `netnotifyd`，放到 `bin/netnotifyd` 后再打包。

## 配置与运行

```bash
uci set netnotify.main.enable=1
uci set netnotify.main.device_name=HomeRouter
uci set netnotify.main.channel=wecom_app   # dingtalk | wecom_bot | wecom_app | bark | webhook
uci commit netnotify
/etc/init.d/netnotify enable
/etc/init.d/netnotify start
netnotifyd test
```

LuCI：`服务` → `NetNotify`。

## 刷新 OUI 精简库（可选）

仓库已内置 `root/usr/share/netnotify/oui_base.txt`。需要更新时：

```bash
./scripts/gen-oui.sh
```

脚本从 IEEE 下载 OUI，按常见消费电子/IoT 厂商关键词过滤后写回。构建 ipk **不强制联网**。

## 网络检测（WAN 看门狗）

将原 `network_check.sh` 逻辑内置：

1. 依次请求配置的 `generate_204` 主机  
2. 全部失败则探测备用 IP（默认 223.5.5.5 / 119.29.29.29）  
3. 仍失败则 `/sbin/ifup <wan>`（有冷却时间）  
4. 恢复后可选推送：WAN IP、在线设备数、负载、启动时间、运行时长  

LuCI：`服务` → `网络通知` → `网络检测`。

## Release

打 tag 会触发发布多架构二进制：

```bash
git tag v0.3.0
git push origin v0.3.0
```

## 说明

- 与 `luci-app-pushbot` / 其它上下线推送插件请勿同时启用，避免重复通知
- Netlink + 真实 ARP 仅 Linux 构建；Windows 可 `go test` / stub 编译
- OUI 数据来自 IEEE，仅用于本地厂商名展示

## License

MIT © zouyq
