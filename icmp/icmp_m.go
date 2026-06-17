package main

import (
	"context"
	"os"

	"protocol/icmp/internal/app"
	"protocol/icmp/internal/stdio"
	"protocol/icmp/internal/transport"
	"protocol/icmp/internal/web"
)

type masterConfig struct {
	src  string
	dst  string
	web  bool
	port string
}

type serviceRunner interface {
	Run(context.Context) error
}

var buildMasterRunner = func(cfg masterConfig) serviceRunner {
	var cmds app.CommandSource
	var results app.ResultSink

	if cfg.web {
		bridge := web.NewWsBridge()
		srv := web.NewServer(bridge, cfg.dst)
		go func() {
			if err := srv.Start(":" + cfg.port); err != nil {
				os.Exit(1)
			}
		}()
		cmds = bridge
		results = bridge
	} else {
		cmds = stdio.NewNonBlockingCommandSource(os.Stdin)
		results = stdio.NewWriterResultSink(stdio.WrapConsoleWriter(os.Stdout))
	}

	return app.MasterService{
		Responder: transport.PcapMasterResponder{
			SrcIP:    cfg.src,
			DstIP:    cfg.dst,
			Resolver: transport.OSResolver{},
		},
		Commands: cmds,
		Results:  results,
	}
}

func runMaster(cfg masterConfig) error {
	return buildMasterRunner(cfg).Run(context.Background())
}
