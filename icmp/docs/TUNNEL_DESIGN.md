# ICMP Tunnel 核心架构与协议设计

## 1. 概述与背景
当前项目中已经实现了基于 ICMP 的 Shell 命令执行（一问一答模式），且底层的收发包逻辑（`transport` 层的 `PollClient` 和 `MasterResponder`）已与业务层解耦。
为了支持更复杂的应用（如 Socks5 代理或端口转发），需要在现有的不可靠 ICMP 传输之上，构建一个可靠的数据流传输层（Tunnel Layer）。

**本设计的核心决策**：
- **模式**：应用层代理模式（非 VPN 虚拟网卡模式），ICMP Payload 仅包含应用层数据。
- **可靠性**：纯手写实现 ARQ（自动重传请求）机制，提供类似 TCP 的 Seq/Ack 滑动窗口。
- **多路复用**：支持动态 SessionID，允许多个并发 TCP 连接复用同一个 ICMP 隧道。
- **隐蔽性**：初期以纯明文实现，便于抓包调试底层状态机。

## 2. 协议头设计 (TunnelHeader)
为了在无连接的 ICMP 协议中实现 TCP 级别的功能，我们必须在每次发送的 ICMP Payload 开头插入一段 16 字节的自定义协议头（其中包含 1 字节 Padding 用于对齐）。

```go
package protocol

// 隧道控制标志位
const (
	TunnelTypeSYN  uint8 = 1 // 建立连接
	TunnelTypeDATA uint8 = 2 // 传输数据
	TunnelTypeACK  uint8 = 3 // 纯确认包 (无数据负载)
	TunnelTypeFIN  uint8 = 4 // 断开连接
)

// TunnelHeader 是附加在原生 ICMP 内部数据区最前面的自定义头部
type TunnelHeader struct {
	SessionID uint32 // 会话 ID，用于区分不同的并发连接
	Type      uint8  // 数据包类型 (SYN, DATA, ACK, FIN)
	Seq       uint32 // 序列号 (表示本包发送的字节序号)
	Ack       uint32 // 确认号 (表示已确认收到的包序号)
	Length    uint16 // 实际数据的长度 (不包含本头部的 16 字节)
}
```

## 3. 架构分层

整个系统自下而上分为三层：

### 3.1 传输层 (Transport Layer)
保持现有的 `pcap_client` 和 `pcap_master` 原封不动。它们就像一根单纯的“电线”，只负责把封装好的 `[]byte` 变成真实的 ICMP 报文发出，或从中提取 `[]byte` 抛给上层。

### 3.2 隧道连接层 (Tunnel Layer) - 本次核心新增
主要包含两个对象：
1. **`TunnelManager`**：全局唯一。内部维护一个 `map[uint32]*ICMPConn`。负责把传输层收到的包，根据包头里的 `SessionID` 分发给具体的连接实例。
2. **`ICMPConn`**：为上层业务提供标准的 `net.Conn` 接口（实现了 `Read`, `Write`, `Close`）。每个 `ICMPConn` 代表一个独立的并发连接，内部拥有独立的生命周期和重传机制。

### 3.3 应用层 (App Layer)
拿到 `ICMPConn` 后，完全把它当成一个普通的 TCP 连接来用。可以像 `slave.go` 里那样执行 Shell，也可以在上面启动一个 Socks5 Server，或者写一个本地端口映射（比如 `1080 -> 远端 22`）。

## 4. 核心工作流 (以单次 HTTP 请求为例)

1. **建立连接 (Handshake)**：
   - 浏览器连接到本地 1080 端口。
   - `TunnelManager` 生成一个随机的 `SessionID`，创建一个新的 `ICMPConn`，并向 Master 发送一个 `Type=TunnelTypeSYN` 的包。
   - Master 收到 SYN，在自己的 map 里注册这个 `SessionID`，并向真正的目标服务器发起真正的 TCP 拨号。同时向 Slave 回复一个 `Type=TunnelTypeACK`。

2. **数据传输与重传 (Data Transfer & ARQ)**：
   - 上层调用 `ICMPConn.Write()` 发送 HTTP 请求。
   - 数据被切割并加上 `TunnelHeader`（带有当前的 Seq 号），发往对端，同时将该包存入本地的“未确认队列” (Unacked Queue)。
   - 对端收到后，回复带有对应 Ack 号的包，并将收到的数据存入“接收缓冲区”，供上层 `Read()` 读取。
   - 若发送方迟迟未收到 Ack（触发内置的超时 Goroutine），则从“未确认队列”中重发该包。

3. **断开连接 (Teardown)**：
   - 本地连接断开时，发送 `Type=TunnelTypeFIN`。
   - 对端收到后，向对应的真实服务器发出关闭指令（`conn.Close()`），并清理 map 中的资源。

## 5. 待讨论的工程细节 (留作后续优化)
- **拥塞控制与滑动窗口的局限说明**：当前版本的滑动窗口仅在发送端硬编码限制为 `sendWindowSize = 64`，缺乏对接收端可用缓冲区的动态反馈（协议头未传递 Window 字段），不具备真正的流量控制 (Flow Control) 能力；同时，拥塞控制算法（如慢启动、拥塞避免、RTO 指数退避）完全未实现，仅提供基于固定 100ms 超时的简单重传。后续版本需引入拥塞控制核心算法。
- 长时间断网或恶意包攻击导致的 Session 内存泄漏清理（定期心跳探测与超时销毁）。
