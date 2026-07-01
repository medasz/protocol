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
│  │  ├─ master_test.go
│  │  ├─ slave.go
│  │  └─ slave_test.go
│  ├─ protocol/
│  │  ├─ message.go
│  │  ├─ packet.go
│  │  ├─ packet_bench_test.go
│  │  ├─ packet_test.go
│  │  └─ tunnel.go
│  ├─ shell/
│  │  ├─ cmd.go
│  │  └─ cmd_test.go
│  ├─ socks/
│  │  └─ server.go
│  ├─ stdio/
│  │  ├─ console_other.go
│  │  ├─ console_windows.go
│  │  ├─ stdio.go
│  │  └─ stdio_test.go
│  ├─ transport/
│  │  ├─ interfaces.go
│  │  ├─ pcap_adapter.go
│  │  ├─ pcap_master.go
│  │  ├─ pcap_slave.go
│  │  ├─ pcap_transport_test.go
│  │  ├─ pcap_tunnel_listener.go
│  │  ├─ resolver.go
│  │  ├─ resolver_bench_test.go
│  │  ├─ resolver_other.go
│  │  ├─ resolver_other_test.go
│  │  ├─ resolver_test.go
│  │  ├─ resolver_windows.go
│  │  └─ resolver_windows_test.go
│  ├─ tunnel/
│  │  ├─ conn.go
│  │  ├─ conn_test.go
│  │  └─ manager.go
│  └─ web/
│     ├─ server.go
│     └─ server_test.go
└─ docs/
   ├─ 2026-06-22-icmp-tunnel-cli-design.md
   ├─ CURRENT_ARCHITECTURE.md
   ├─ PERFORMANCE_REPORT.md
   ├─ TARGET_ARCHITECTURE.md
   ├─ TUNNEL_DESIGN.md
   ├─ architecture.md
   ├─ data_flow.md
   └─ struct_relations.md
```

## 2. 分层说明

### CLI 装配层

- `main.go`：统一子命令入口和参数解析。
- `icmp_m.go`：装配 `master` 运行时依赖。
- `icmp_s.go`：装配 `slave` 运行时依赖。

职责：

- 解析配置
- 组装依赖（包括 TunnelManager、Socks5 服务和 Web 控制台）
- 启动服务

### 业务应用层 (App Layer)

- `internal/app/master.go`
- `internal/app/slave.go`

职责：

- 编排主从业务流程
- 不直接依赖 `pcap`、`exec.Command`、标准输入输出
- 仅面向抽象接口编程
- 调度“可靠隧道层（Tunnel Layer）”进行高级特性（如 Socks5、端口转发、PTY 等）的多路复用流分发与交换

### 可靠隧道层 (Tunnel Layer)

- `internal/tunnel/conn.go`：实现可靠的模拟连接 `ICMPConn`，对外暴露标准的 `net.Conn` 接口。
- `internal/tunnel/manager.go`：管理 `ICMPConn` 会话，进行 SYN/ACK/FIN 的分发与会话流控。
- `internal/socks/server.go`：在隧道上构建的标准 Socks5 代理服务器。
- `internal/web/server.go`：Web 控制台及 API 服务，支持并发 Agent 列表展示及通过 WebSocket 进行 PTY 交互。

职责：
- 将不可靠的 ICMP 报文流转化为可靠的双向字节流 (模拟 TCP 握手、重传及窗口机制)。
- 支持多会话 SessionID 级别的多路复用。

### 协议层 (Protocol Layer)

- `internal/protocol/message.go`
- `internal/protocol/packet.go`
- `internal/protocol/tunnel.go`

职责：

- 定义 ICMP 交换消息模型（`ProtocolShell` 与 `ProtocolTunnel`）
- 统一处理 Echo Request / Reply 的解析与构造
- 承担跨层复用的数据复制与报文元信息封装
- 定义 16 字节可靠隧道包头 `TunnelHeader` 并支持序列化/反序列化

### 传输层 (Transport Layer)

- `internal/transport/interfaces.go`
- `internal/transport/pcap_adapter.go`
- `internal/transport/pcap_master.go`
- `internal/transport/pcap_slave.go`
- `internal/transport/pcap_transport_test.go`
- `internal/transport/pcap_tunnel_listener.go`
- `internal/transport/resolver*.go`

职责：

- 提供基于 `pcap` 的网络包拦截、伪造、发送与接收实现
- 提供网卡、路由、ARP 解析实现
- 通过接口向上暴露收发能力，不把底层依赖泄漏给应用层

### 系统适配与交互层 (Adapters Layer)

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
        +----> Tunnel(tunnel / socks / web)
        |         |
        |         v
        +----> Protocol(protocol)
        |
        +----> Transport(transport)
        |
        +----> Adapters(shell / stdio)
```

规则：

- `protocol` 不依赖其他任何包
- `tunnel` 依赖 `protocol` 进行头部组装，不依赖具体传输层
- `app` 仅依赖 `protocol`、`tunnel` 和 `transport` 暴露的接口
- `transport` 不反向依赖 `app`
- CLI 只做装配，不承载核心业务

## 4. 关键接口与核心组件

### 传输接口

- `transport.MasterResponder`
- `transport.PollClient`
- `transport.AddressResolver`

### 隧道与连接组件

- `tunnel.SendFunc`：隧道发送回调函数接口
- `tunnel.ICMPConn`：实现了 `net.Conn`，提供 ARQ（超时重传）及固定发送窗口大小限制。
- `tunnel.TunnelManager`：基于 Map 进行多并发会话 Session 复用的核心中枢。

### 应用交互接口

- `app.CommandSource`
- `app.ResultSink`
- `app.AgentTracker`
- `shell.Executor`

这些接口与组件的意义：

- 统一主从业务交互规范
- 提供可靠的数据流管道，使 Socks5 代理与交互式 PTY 等高级功能成为可能
- 为测试提供稳定 mock 边界

## 5. 依赖注入策略

当前使用“构造时注入 + 包级可替换工厂”组合策略：

- 运行态通过构造函数或结构体字段注入
- 测试态通过可替换 builder / opener 注入 fake 依赖

这样既保留了运行时简洁性，也提升了入口层和系统适配层的测试能力。

## 6. 架构收益

- 高内聚：协议、传输、隧道、执行器、CLI 职责清晰
- 低耦合：上层不感知 `pcap` 和系统命令细节
- 可扩展：不仅可执行传统 Shell，还能在此架构之上运行任意 TCP 服务（如端口转发、SOCKS5）
- 可测试：纯逻辑和大部分系统适配逻辑均可通过 fake 依赖验证
