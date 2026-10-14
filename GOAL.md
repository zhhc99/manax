# manax - Goal

## 目标

交付一个 Go 编写的跨平台 CLI, 管理兼容内核 (如 mihomo) 的生命周期, 用户配置, 节点选择, 系统代理和 TUN. 支持 Windows, Linux 和 macOS.

核心原则:

- 每个平台分发一个可携带的 `manax` 二进制. 不捆绑兼容内核, 不依赖在线安装脚本.
- 同一个二进制提供 CLI, 安装与卸载功能, 以及不向普通帮助展示的 `daemon` 和 `reload` 内部入口.
- daemon 是 system-wise 服务, 随系统启动, 统一管理内核进程和特权网络操作.
- 每个用户的配置, 订阅和偏好保存在自己的用户目录. 不支持多个用户同时使用, 但不同用户分别登录时必须加载各自的配置, 互不覆盖.
- 每个用户有一个 user-wise login task. 用户登录时运行一次 `manax reload`, 读取该用户的配置并通知 daemon 应用. 不创建常驻的用户级 daemon.
- daemon 不识别当前用户, 不监听登录事件, 不轮询用户会话, 也不在开机时猜测或恢复某个用户的配置.
- 支持离线部署. 用户可指定本地兼容内核文件, 也可选择从兼容内核发布源或指定 URL 下载.
- 不编写 `go test` 测试套件. 使用临时脚本和原生系统手动验证输出及实际行为.

## 必须参考的项目

以 [zhhc99/gpuctl](https://github.com/zhhc99/gpuctl) 的仓库结构和实现技术为直接参考. 先阅读其源码, 沿用简洁的模块划分和轻量实现, 不引入重量级框架.

必须参考:

- `cmd/gpuctl` 的单一进程入口, 通过同一二进制的 `daemon` 入口启动后台服务.
- `internal/cli` 的轻量命令分发, `switch` 路由, 简洁帮助与输出.
- `internal/daemon`, `internal/ipc`, `internal/config`, `internal/locale` 的职责划分.
- 本地 IPC 请求与响应设计. Linux 和 macOS 使用 Unix Domain Socket, Windows 使用 Named Pipe. 不开放 TCP 管理端口.
- 标准库优先, 尽量支持 `CGO_ENABLED=0` 构建.
- `.goreleaser.yaml` 和 `.github/workflows` 的跨平台构建与 tag 发布流程.

Manax 使用 `manax install` 和 `manax uninstall` 管理安装. 不要求用户下载或运行独立安装脚本. 参考 gpuctl 的工程结构和轻量实现, 但按本文定义的 system-wise daemon 与 user-wise login task 模型实现.

## 交付结构

```text
cmd/manax/main.go
internal/cli/           命令分发, 帮助和输出
internal/daemon/        后台服务和内核生命周期
internal/ipc/           本地 IPC 协议与平台传输
internal/config/        当前用户配置, 订阅索引和活动配置
internal/core/          内核进程与 Clash 兼容 API 客户端
internal/systemproxy/   系统代理平台适配
internal/tun/           TUN 控制
internal/service/       系统服务与用户登录任务安装适配
internal/locale/        用户可见文案
.goreleaser.yaml
.github/workflows/
```

## 安装与运行模型

- `manax install` 将二进制安装到 system-wise 固定路径, 安装兼容内核到受保护的固定路径, 注册 system-wise 服务并设为开机启动.
- 安装时允许选择本地内核文件, 兼容内核发布源或指定 URL. 本地文件方式必须完全离线可用. 下载是可选项.
- 首次全局安装时创建 system-wise daemon. 后续用户运行 `manax install` 时复用已有 daemon 和兼容内核, 只为当前用户建立独立的 login task. 不覆盖现有内核, 其他用户的配置或登录任务.
- `manax install` 需要提权时由自身发起平台授权. Linux 优先使用 `pkexec`, 无法使用时提示通过 `sudo` 执行. Windows 使用 UAC. macOS 使用系统授权方式.
- 安装流程必须保留发起安装的用户身份, 以便创建属于该用户的 login task. 如果命令直接以 root 身份运行且无法确定原始用户, 显示警告并建议用户以自己的身份重新执行.
- 用户登录时, 对应的 login task 以该用户身份运行一次 `manax reload` 后退出. 该命令读取当前用户自己的配置, 然后通过 IPC 通知 daemon 应用配置和该用户的模式, 系统代理及 TUN 设置.
- daemon 只管理一个活动内核. 收到 `reload` 或用户 CLI 的控制请求后应用请求方提供的配置路径和设置. daemon 不自行寻找用户目录, 不存储唯一的全局用户配置, 也不依赖登录事件.
- `manax conf use <name>` 更新当前用户的活动配置记录, 并立即通知 daemon 加载该配置. 下次该用户登录时, login task 会再次应用该用户当前选中的配置.
- 用户登录自动运行 login task.
- 内核崩溃后只按间隔尝试重新启动内核. 不自动撤销系统代理, 不自动关闭 TUN, 不做其他恢复动作.
- `manax uninstall` 移除 system-wise 服务和程序, 并清理由 Manax 创建的登录任务. 默认保留用户配置.

## 命令界面

无参数时显示简洁帮助. 错误写入 stderr, 失败命令返回非零退出码. `manax help <command>` 显示命令用法. `daemon` 是内部入口, 不出现在常规帮助中.

```text
manax
   status                         显示服务, 内核和代理状态
   reload                         重新加载当前用户配置
   install                        (hidden) 安装内核, 系统服务和当前用户登录任务
   uninstall                      (hidden) 卸载 Manax
   mode <direct|rule|global>      设置运行模式
   system <on|off>                开关系统代理
   tun <on|off>                   开关 TUN

   conf add <name> <url-or-file>  导入订阅或本地配置
   conf list                      列出配置
   conf use <name>                加载指定配置
   conf update [name]             更新指定配置, 默认更新全部
   conf rm <name>                 删除配置

   node list [group]              列出节点并并行测速
   node use <group> <node>        选择策略组节点

   version                        显示版本
   help <command>                 显示命令帮助
```

上述为功能, 输出样式应该参考 gpuctl 项目.

## 功能要求

### 状态

`manax status` 一次显示服务, 内核, 当前用户配置, 模式, 系统代理, TUN 和当前节点最近一次测速延迟. 查询状态不主动测速.

```text
$ manax status
服务      运行中
内核      运行中 (版本信息)
配置      home
模式      rule
系统代理  开启 (127.0.0.1:7890)
TUN       关闭
节点      Proxy=JP-01 (82 ms)
```

未安装服务时, `status` 提示运行 `manax install`. 内核未运行时显示实际状态.

### 用户配置与登录任务

- 配置和订阅保存在当前用户的配置目录, 支持多份命名配置.
- 每个用户的活动配置, 模式, 系统代理开关和 TUN 开关分别保存. 不把用户偏好写入全局配置.
- Windows 使用该用户的登录计划任务. Linux 使用该用户的登录级一次性任务. macOS 使用该用户的 LaunchAgent 一次性执行 `reload`.
- login task 以当前登录用户身份执行, 不提权, 读取该用户自己的配置并通过 IPC 请求 daemon 应用.
- 用户 A 退出后, 用户 B 下次登录时应用 B 自己的配置. 不支持多个用户同时控制或同时运行独立内核.
- daemon 不负责发现用户和解析 home 目录. 登录任务和 CLI 明确提供当前用户配置路径.

### 模式

- `direct`: 所有请求直连.
- `rule`: 按配置规则决定直连或代理.
- `global`: 所有请求使用全局代理策略.
- 切换模式不自动更改系统代理或 TUN 开关.

### 系统代理

- `manax system on` 覆写系统代理设置, 指向兼容内核的本地代理地址.
- `manax system off` 清除系统代理设置. 不保存或恢复原配置, 不检查用户手动修改.
- 系统代理与 TUN 是两个独立开关. 每个用户的开关偏好保存在该用户配置中, 登录刷新时按该用户设置应用.
- Windows 和 macOS 实现平台对应的系统代理操作. Linux 仅支持 GNOME 和 KDE. 检测不到受支持的桌面环境时提示不支持.

### TUN

- `manax tun on/off` 独立控制兼容内核的 TUN 功能, 不隐式开关系统代理.
- TUN 开关偏好保存在当前用户配置中, 登录刷新时由 daemon 按该用户设置应用.
- 内核崩溃或重启时, 不自动撤销系统代理或关闭 TUN.

### 配置

- 支持从 URL 导入订阅, 或从本地文件导入配置.
- `conf list` 显示当前用户的配置和活动选择.
- `conf use` 让 daemon 加载所选配置, 并更新当前用户的活动配置记录.
- `conf update <name>` 更新指定订阅. `conf update` 更新所有可更新的订阅.
- `conf rm` 删除指定配置并更新当前用户的索引.

### 兼容内核

- 内核作为独立子进程运行, 由 daemon 启动, 监控和重启.
- 内核安装到统一的受保护路径, 不依赖用户原始文件名或所在目录.
- 通过 Clash 兼容 API 获取状态, 切换模式, 列出节点和选择节点.
- 用户可提供自己的兼容内核. Manax 不要求其运行期间连接特定上游.
- 配置引用的订阅, 规则数据库或其他资源可能需要网络. Manax 本身不得强制联网.

### 节点与测速

- `node list` 列出可用节点并并行测速. `node list <group>` 只测指定策略组.
- 测速期间显示动态加载符号. 延迟确定后替换为毫秒数, 失败时显示超时或错误状态.
- 设置并发上限和单节点超时. 单个节点失败不影响其他结果.
- 使用 Clash 兼容 API 的单节点延迟测试接口. 不使用可能清除自动策略组固定选择的组测速接口.
- `node use <group> <node>` 通过 API 选择节点.

输出示例:

```text
$ manax node list Proxy
JP-01  82 ms
JP-02  ⠋ 测速中
JP-03  超时
```

测速中的加载符号持续刷新. 测速完成后替换为延迟或失败状态.

## 平台要求

- Windows: system-wise Windows Service, Named Pipe IPC, user-wise 登录计划任务, 系统代理和 TUN.
- Linux: systemd system service, Unix Domain Socket IPC, user-wise 一次性登录任务. 系统代理支持 GNOME 和 KDE.
- macOS: launchd system daemon, Unix Domain Socket IPC, user-wise LaunchAgent 一次性任务, 系统代理和 TUN.
- 每个平台提供一个可携带的 `manax` 二进制. 使用本地兼容内核文件的安装过程必须离线完成.

## 验收标准

- [ ] 每个平台发布一个独立 `manax` 二进制, 不捆绑兼容内核或独立安装脚本.
- [ ] `manax install` 可在需要时提权, 安装全局二进制和兼容内核, 注册 system-wise daemon, 并创建当前用户的 login task.
- [ ] system-wise daemon 随系统启动, 但不自行选择或恢复某个用户的配置.
- [ ] 用户登录时 login task 以该用户身份执行一次 `manax reload`, 读取用户配置并通知 daemon 应用, 随后退出.
- [ ] 不存在常驻的用户级 daemon, 不监听或轮询用户会话, 不要求手动执行 `manax start`.
- [ ] 不同用户的配置和偏好相互独立. 用户切换时加载新登录用户自己的配置, 不支持同时会话.
- [ ] 普通用户可运行 CLI 并通过本地 IPC 控制 daemon.
- [ ] `status` 显示服务, 内核, 当前用户配置, 模式, 系统代理, TUN 和最近测速结果.
- [ ] `direct`, `rule`, `global` 与 `system`, `tun` 按定义独立工作.
- [ ] 内核崩溃后只间隔重试启动, 不自动更改系统代理或 TUN.
- [ ] 本地兼容内核可离线安装. 下载内核是可选项.
- [ ] `conf` 命令可新增, 列出, 切换, 更新和删除当前用户配置.
- [ ] `node list` 并行测速并动态刷新结果. `node use` 可以切换节点.
- [ ] Linux 非 GNOME/KDE 桌面环境下, `system on/off` 提示不支持.
- [ ] 使用临时脚本和原生系统手动验证 CLI 输出, 安装, 登录刷新, 启动, 卸载及平台功能. 不提交 `go test` 测试套件.
