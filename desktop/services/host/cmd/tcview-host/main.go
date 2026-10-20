package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/suchasplus/til-consensus/desktop/services/host/internal/rpc"
	"github.com/suchasplus/til-consensus/desktop/services/host/internal/runmanager"
)

func main() {
	base, err := os.UserConfigDir()
	if err != nil {
		base = os.TempDir()
	}
	dataDir := flag.String("data-dir", filepath.Join(base, "tcview"), "持久化目录")
	providerMode := flag.String("provider-mode", "cli", "调用模式：cli（默认）或显式 mock 测试")
	delay := flag.Int("mock-delay-ms", 500, "每次模拟调用的延迟（毫秒）")
	flag.Parse()
	log.SetOutput(os.Stderr)
	if *delay < 0 {
		log.Fatal("mock-delay-ms 不能小于 0")
	}
	server := rpc.New(os.Stdout)
	manager, err := runmanager.New(runmanager.Options{DataDir: *dataDir, ProviderMode: *providerMode, Delay: time.Duration(*delay) * time.Millisecond, Notify: server.Notify})
	if err != nil {
		log.Fatal(err)
	}
	server.Bind(manager)
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	go func() { <-signals; _ = os.Stdin.Close() }()
	err = server.Serve(os.Stdin)
	server.CancelPending()
	if closeErr := manager.Close(); closeErr != nil {
		log.Printf("保存后台状态: %v", closeErr)
	}
	server.Close()
	if err != nil {
		log.Printf("读取桌面协议: %v", err)
		os.Exit(1)
	}
}
