package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
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

	}

}
