package main

import (
	"bufio"
	"fmt"
	"io"
	"os/exec"
)

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

func main() {
	stdin, stdout, err := createShell()
	if err != nil {
		fmt.Println("createShell error:", err)
		return
	}
	defer stdin.Close()

	// 读取 cmd.exe 启动横幅（第一行，包含版本号）
	reader := bufio.NewReader(stdout)
	line, _, _ := reader.ReadLine()
	fmt.Println(string(line))

	// 发送 whoami 命令
	stdin.Write([]byte("whoami\r\n"))

	// 跳过回显的命令行和空行，读取实际输出
	reader.ReadLine() // 跳过 "D:\code\protocol\icmp>whoami"
	line, _, _ = reader.ReadLine()
	line, _, _ = reader.ReadLine()
	line, _, _ = reader.ReadLine()
	fmt.Println(string(line))

	stdin.Write([]byte("exit\r\n"))
}
