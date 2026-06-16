# ICMP 重构后架构设计

## 1. 目录结构

```text
icmp/
├─ main.go
├─ icmp_m.go
├─ icmp_s.go
├─ internal/
│  ├─ app/
│  │  ├─ master.go
│  │  └─ slave.go
│  ├─ protocol/
│  │  ├─ message.go
│  │  └─ packet.go
│  ├─ shell/
│  │  └─ cmd.go
│  ├─ stdio/
│  │  └─ stdio.go
│  └─ transport/
│     ├─ interfaces.go
│     ├─ pcap_adapter.go
│     ├─ pcap_master.go
│     ├─ pcap_slave.go
│     ├─ resolver.go
│     └─ resolver_windows.go
└─ docs/
   ├─ CURRENT_ARCHITECTURE.md
   ├─ TARGET_ARCHITECTURE.md
   └─ PERFORMANCE_REPORT.md
```

## 2. 分层说明

### CLI 装配层

- `main.go`：统一子命令入口和参数解析。
- `icmp_m.go`：装配 `master` 运行时依赖。
- `icmp_s.go`：装配 `slave` 运行时依赖。

职责：

- 解析配置
- 组装依赖
- 启动服务

### 应用层

- `internal/app/master.go`
- `internal/app/slave.go`

职责：

- 编排主从业务流程
- 不直接依赖 `pcap`、`exec.Command`、标准输入输出
- 仅面向抽象接口编程

### 协议层

- `internal/protocol/message.go`
- `internal/protocol/packet.go`

职责：

- 定义 ICMP 交换消息模型
- 统一处理 Echo Request / Reply 的解析与构造
- 承担跨层复用的数据复制与报文元信息封装

### 传输层

- `internal/transport/interfaces.go`
- `internal/transport/pcap_master.go`
- `internal/transport/pcap_slave.go`
- `internal/transport/pcap_adapter.go`
- `internal/transport/resolver*.go`

职责：

- 提供 `pcap` 收发实现
- 提供网卡、路由、ARP 解析实现
- 通过接口向上暴露收发能力，不把底层依赖泄漏给应用层

### 系统适配层

- `internal/shell/cmd.go`
- `internal/stdio/stdio.go`

职责：

- 标准输入命令采集
- 标准输出结果落地
- `cmd.exe` 执行器适配

## 3. 模块调用层级

```text
CLI(main / icmp_m / icmp_s)
        |
        v
Application(app)
        |
        +----> Protocol(protocol)
        |
        +----> Transport(transport)
        |
        +----> Adapters(shell / stdio)
```

规则：

- `protocol` 不依赖 `app`、`transport`、`shell`、`stdio`
- `app` 仅依赖 `protocol` 和 `transport` 暴露的接口
- `transport` 不反向依赖 `app`
- CLI 只做装配，不承载核心业务

## 4. 关键接口

### 传输接口

- `transport.MasterResponder`
- `transport.PollClient`
- `transport.AddressResolver`

### 应用交互接口

- `app.CommandSource`
- `app.ResultSink`
- `shell.Executor`

这些接口的意义：

- 统一主从业务交互规范
- 允许后续替换为原始 socket、文件管道、远程执行器等新实现
- 为测试提供稳定 mock 边界

## 5. 依赖注入策略

当前使用“构造时注入 + 包级可替换工厂”组合策略：

- 运行态通过构造函数或结构体字段注入
- 测试态通过可替换 builder / opener 注入 fake 依赖

这样既保留了运行时简洁性，也提升了入口层和系统适配层的测试能力。

## 6. 架构收益

- 高内聚：协议、传输、执行器、CLI 职责清晰
- 低耦合：上层不感知 `pcap` 和系统命令细节
- 可扩展：后续可增加 Linux shell、不同传输后端或不同消息编码方式
- 可测试：纯逻辑和大部分系统适配逻辑均可通过 fake 依赖验证
