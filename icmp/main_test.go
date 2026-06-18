package main

import (
	"context"
	"errors"
	"io"
	"testing"
)

func TestParseMasterArgs(t *testing.T) {
	cfg := parseMasterArgs([]string{"-src", "10.0.0.1", "-dst", "10.0.0.2"})
	if cfg.src != "10.0.0.1" || cfg.dst != "10.0.0.2" {
		t.Fatalf("unexpected master config: %+v", cfg)
	}
}

func TestParseMasterDstAllowlist(t *testing.T) {
	got := parseMasterDstAllowlist("10.0.0.2, 10.0.0.3")
	want := []string{"10.0.0.2", "10.0.0.3"}
	if len(got) != len(want) {
		t.Fatalf("allowlist length = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("allowlist[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestParseSlaveArgs(t *testing.T) {
	cfg := parseSlaveArgs([]string{"-t", "10.0.0.1", "-d", "500", "-o", "2000", "-s", "128", "-r"})
	if cfg.target != "10.0.0.1" || cfg.delay != 500 || cfg.timeout != 2000 || cfg.maxDataSize != 128 || !cfg.isTest {
		t.Fatalf("unexpected slave config: %+v", cfg)
	}
}

type fakeRunner struct {
	err    error
	called bool
}

func (f *fakeRunner) Run(context.Context) error {
	f.called = true
	return f.err
}

type fakeCloser struct {
	closed bool
}

func (f *fakeCloser) Close() error {
	f.closed = true
	return nil
}

func TestRunMasterUsesBuilder(t *testing.T) {
	restore := buildMasterRunner
	defer func() { buildMasterRunner = restore }()

	runner := &fakeRunner{}
	buildMasterRunner = func(cfg masterConfig) serviceRunner {
		if cfg.src != "10.0.0.1" || cfg.dst != "10.0.0.2" {
			t.Fatalf("unexpected cfg: %+v", cfg)
		}
		return runner
	}

	if err := runMaster(masterConfig{src: "10.0.0.1", dst: "10.0.0.2"}); err != nil {
		t.Fatalf("runMaster() error = %v", err)
	}
	if !runner.called {
		t.Fatal("expected runner to be called")
	}
}

func TestRunMasterRequiresDstWithoutWeb(t *testing.T) {
	if err := runMaster(masterConfig{src: "10.0.0.1"}); err == nil {
		t.Fatal("expected error")
	}
}

func TestRunSlaveUsesBuilderAndCloser(t *testing.T) {
	restore := buildSlaveRuntime
	defer func() { buildSlaveRuntime = restore }()

	runner := &fakeRunner{}
	closer := &fakeCloser{}
	buildSlaveRuntime = func(cfg slaveConfig) (serviceRunner, io.Closer, error) {
		if cfg.target != "10.0.0.1" {
			t.Fatalf("unexpected cfg: %+v", cfg)
		}
		return runner, closer, nil
	}

	if err := runSlave(slaveConfig{target: "10.0.0.1"}); err != nil {
		t.Fatalf("runSlave() error = %v", err)
	}
	if !runner.called || !closer.closed {
		t.Fatalf("runner called=%v closer closed=%v", runner.called, closer.closed)
	}
}

func TestRunSlaveBuilderError(t *testing.T) {
	restore := buildSlaveRuntime
	defer func() { buildSlaveRuntime = restore }()

	buildSlaveRuntime = func(cfg slaveConfig) (serviceRunner, io.Closer, error) {
		return nil, nil, errors.New("boom")
	}

	if err := runSlave(slaveConfig{}); err == nil {
		t.Fatal("expected error")
	}
}

func TestRunCLIUsageAndHelp(t *testing.T) {
	if err := runCLI(nil); !errors.Is(err, errUsage) {
		t.Fatalf("runCLI(nil) error = %v", err)
	}
	if err := runCLI([]string{"help"}); err != nil {
		t.Fatalf("runCLI(help) error = %v", err)
	}
	if err := runCLI([]string{"unknown"}); !errors.Is(err, errUsage) {
		t.Fatalf("runCLI(unknown) error = %v", err)
	}
}
