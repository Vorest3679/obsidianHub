package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"obsidianhub/internal/archive"
	"obsidianhub/internal/config"
	"obsidianhub/internal/fetcher"
	"obsidianhub/internal/service"
	"obsidianhub/internal/store"
)

// main 是程序入口：读取启动参数和配置，组装各个组件，然后按单次或定时模式同步订阅。
func main() {
	// flag.String / flag.Bool 返回指针；Parse 解析命令行后再通过解引用取得用户输入。
	configPath := flag.String("config", "./config.json", "path to config JSON")
	once := flag.Bool("once", false, "sync once and exit")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatal(err)
	}

	st, err := store.Open(cfg.DatabasePath)
	if err != nil {
		log.Fatal(err)
	}
	// main 返回时关闭数据库连接池，确保程序退出前释放资源。
	defer st.Close()

	svc := &service.Service{
		Config:  cfg,
		Fetcher: fetcher.New(cfg.UserAgent),
		Store:   st,
		Writer:  archive.New(cfg.VaultPath),
		Logger:  log.New(os.Stdout, "obsidianhub ", log.LstdFlags),
	}

	// 将 Ctrl+C 和 SIGTERM 转换为 Context 取消信号；下游 HTTP 请求会收到同一个 ctx。
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// 同步失败只记日志，不让常驻进程退出；SyncAll 会继续尝试其他订阅。
	sync := func() {
		if err := svc.SyncAll(ctx); err != nil {
			svc.Logger.Printf("sync completed with errors: %v", err)
		}
	}
	sync()
	if *once {
		return
	}

	// Ticker.C 每隔配置的秒数发出一次事件；Stop 在退出时释放定时器资源。
	ticker := time.NewTicker(time.Duration(cfg.IntervalSecs) * time.Second)
	defer ticker.Stop()
	svc.Logger.Printf("running; interval=%s, vault=%s", tickerDuration(cfg.IntervalSecs), cfg.VaultPath)
	for {
		select {
		case <-ctx.Done():
			// 收到退出信号后，离开循环并让 defer 清理资源。
			svc.Logger.Println("stopped")
			return
		case <-ticker.C:
			sync()
		}
	}
}

// tickerDuration 把配置中的秒数转换为 time.Duration，供日志显示定时周期。
func tickerDuration(seconds int) time.Duration {
	return time.Duration(seconds) * time.Second
}
