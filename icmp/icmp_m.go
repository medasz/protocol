package main

import (
	"context"
	"fmt"
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
		hub := web.NewHub()
		srv := web.NewServer(hub)
		go func() {
			if err := srv.Start(":" + cfg.port); err != nil {
				os.Exit(1)
			}
		}()
		cmds = hub
		results = hub
		return app.MasterService{
			Responder: transport.PcapMasterResponder{
				SrcIP:         cfg.src,
				AllowedDstIPs: parseMasterDstAllowlist(cfg.dst),
				Resolver:      transport.OSResolver{},
			},
			Commands: cmds,
			Results:  results,
			Agents:   hub,
		}
	} else {
		cmds = stdio.NewNonBlockingCommandSource(os.Stdin)
		results = stdio.NewWriterResultSink(stdio.WrapConsoleWriter(os.Stdout))
	}

	return app.MasterService{
		Responder: transport.PcapMasterResponder{
			SrcIP:         cfg.src,
			AllowedDstIPs: parseMasterDstAllowlist(cfg.dst),
			Resolver:      transport.OSResolver{},
		},
		Commands: cmds,
		Results:  results,
	}
}

func runMaster(cfg masterConfig) error {
	if !cfg.web && len(parseMasterDstAllowlist(cfg.dst)) != 1 {
		return fmt.Errorf("master without -web requires exactly one -dst target")
	}
	return buildMasterRunner(cfg).Run(context.Background())
}
