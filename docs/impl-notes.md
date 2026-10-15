# 实现说明

## 无感知的 TUN 模式

用一个提权的守护进程来 supervise meta. manax 通过和该守护进程交互来影响 meta 行为, 包括开关 TUN 模式 (需要提权). 当然, 这也意味着其他代码可以在不提权时, 通过 ipc 和 daemon 交互来开关 TUN 模式.

可以控制权限来减少滥用, 但在信任用户可以掌控计算机的情况下, manax 选择避免这些 (主观上感觉) 过度防御的逻辑.

## meta 内核的生命周期

如果 daemon 优雅退出, 则不构成问题; 如果 daemon 被 sigkill 等强行终止, 则 daemon 无法通知 meta 内核退出. 对于此情况, `manax install` 部署服务时, 做如下处理:

- Linux: 依赖 systemd 的 control-group 杀掉 meta.
- macOS: 依赖 launchd 的 AbandonProcessGroup=false 行为, meta 被视作 daemon 的子进程而被杀死.
- Windows: 依赖 SCM 的 Job Object, daemon 终止时 job 句柄关闭, meta 作为 job 内的进程被杀死.
