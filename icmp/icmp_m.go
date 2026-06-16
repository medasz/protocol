package main

import (
	"context"
	"os"

	"protocol/icmp/internal/app"
	"protocol/icmp/internal/stdio"
	"protocol/icmp/internal/transport"
)

type masterConfig struct {
	src string
	dst string
}

type serviceRunner interface {
	Run(context.Context) error
}

var buildMasterRunner = func(cfg masterConfig) serviceRunner {
	return app.MasterService{
		Responder: transport.PcapMasterResponder{
			SrcIP:    cfg.src,
			DstIP:    cfg.dst,
			Resolver: transport.OSResolver{},
		},
		Commands: stdio.NewNonBlockingCommandSource(os.Stdin),
		Results:  stdio.NewWriterResultSink(os.Stdout),
	}
}

func runMaster(cfg masterConfig) error {
	return buildMasterRunner(cfg).Run(context.Background())
}
