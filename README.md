# manax

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

**Clash Meta 内核控制工具.**

## 功能

> macOS 的支持尚未完整测试.

- [x] 守护 meta 内核
- [x] 管理订阅配置
- [x] 系统代理, 无感切换 TUN
- [x] 拉取 meta 内核和 GeoData

## 快速安装

获取 manax 二进制, 然后

```bash
./manax install
```

提权说明: 需要写入系统服务来控制 TUN 模式.

### 关于资源包

当用户无法访问 GitHub 时, 可能需要手动准备 meta 内核和 GeoData (下称 "资源包"). 资源包的结构应该如下:

```text
resource.zip
  - meta kernel
  - geosite.dat
  - geoip.dat
  - country.mmdb
  - GeoLite2-ASN.mmdb
```

### 卸载

```bash
manax uninstall
```

## 常用命令

```
状态
   status                         显示 Manax 状态
   reload                         重载当前配置
   stop                           停止内核与代理

控制
   mode <direct|rule|global>      设置运行模式
   system <on|off>                开关系统代理
   tun <on|off>                   开关 TUN

配置
   conf add <name> <url-or-file>  导入订阅或配置
   conf list                      列出配置
   conf use <name>                加载配置
   conf update [name]             更新配置
   conf rm <name>                 删除配置

节点
   node list                      列出节点
   node test                      对所有节点测速
   node use <node>                按序号选择节点
```

## 构建与发布

在 `CGO_ENABLED=0` 下编译, 所得版本和 goreleaser 一致.

```bash
mkdir -p bin
CGO_ENABLED=0 go build -o bin/manax ./cmd/manax
```

如果希望带有版本号:

```bash
CGO_ENABLED=0 go build -o bin/manax -ldflags "-X 'github.com/zhhc99/manax/internal/cli.Version=v1.0.0'" ./cmd/manax
```

提交代码后, 推送 tag 触发 goreleaser:

```bash
git tag vX.Y.Z
git push origin vX.Y.Z
```

GitHub Actions 会自动编译打包二进制然后发布. 可以手动填写该发布的 Release Notes.
