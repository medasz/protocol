package main

import (
	"fmt"
	"io"
	"os"
	"time"

	"context"

	"protocol/icmp/internal/app"
	"protocol/icmp/internal/shell"
	"protocol/icmp/internal/transport"
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

var buildSlaveRuntime = func(cfg slaveConfig) (serviceRunner, io.Closer, error) {
	executor, err := shell.NewCmdShell(cfg.maxDataSize)
	if err != nil {
		return nil, nil, fmt.Errorf("create shell error: %w", err)
	}

	service := app.SlaveService{
		Config: app.SlaveConfig{
			Delay:    time.Duration(cfg.delay) * time.Millisecond,
			Timeout:  time.Duration(cfg.timeout) * time.Millisecond,
			TestMode: cfg.isTest,
			Logger:   os.Stdout,
		},
		Client: transport.PcapPollClient{
			TargetIP: cfg.target,
			Timeout:  time.Duration(cfg.timeout) * time.Millisecond,
			Resolver: transport.OSResolver{},
			ID:       1,
			Seq:      1,
		},
		Executor: executor,
	}
	return service, executor, nil
}

func runSlave(cfg slaveConfig) error {
	fmt.Printf("启动配置 -> Target: %s, Delay: %d, TestMode: %v\n", cfg.target, cfg.delay, cfg.isTest)
	service, closer, err := buildSlaveRuntime(cfg)
	if err != nil {
		return err
	}
	if closer != nil {
		defer closer.Close()
	}
	return service.Run(context.Background())
}
