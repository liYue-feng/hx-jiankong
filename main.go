package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sync"
	"time"

	"hx_jiankong/engine"
	"hx_jiankong/gui"
	"hx_jiankong/notify"
)

func main() {
	baseDir := filepath.Dir(os.Args[0])
	if abs, err := filepath.Abs(baseDir); err == nil {
		baseDir = abs
	}

	logFile, _ := os.OpenFile(filepath.Join(baseDir, "logs", "app.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if logFile != nil {
		log.SetOutput(logFile)
		defer logFile.Close()
	}

	// 创建 GUI 服务器
	server := gui.NewServer(8088, baseDir)

	// 创建日志桥接
	var currentEngine *engine.Engine
	var engineMu sync.Mutex

	server.AddLog("华医通助手已启动")
	server.AddLog(fmt.Sprintf("工作目录: %s", baseDir))

	// 设置应用处理器

	server.SetAppHandler(&gui.AppHandler{
		ConfigDir: baseDir,
		OnStart: func(configPath, mode, patient, dept, doctor string) error {
			engineMu.Lock()
			defer engineMu.Unlock()

			// 如果有正在运行的引擎，先停止
			if currentEngine != nil {
				currentEngine.Stop()
				currentEngine = nil
				time.Sleep(500 * time.Millisecond)
			}

			eng, err := engine.NewEngine(configPath)
			if err != nil {
				return err
			}

			// 设置运行时参数
			eng.Config.Name = fmt.Sprintf("%s-%s-%s", mode, patient, doctor)
			eng.Config.Patient = patient
			eng.Config.Department = dept
			eng.Config.Doctor = doctor
			eng.Config.Mode = mode

			// 设置通知器
			sctKey := eng.Config.SCTKey
			if sctKey == "" {
				sctKey = os.Getenv("SERVERCHAN_KEY")
			}
			notifier := notify.NewNotifier(sctKey)

			eng.NotifyFunc = func(title, body string, urgent bool) {
				server.AddLog(fmt.Sprintf("推送: %s - %s", title, body))
				if err := notifier.Send(title, body, urgent); err != nil {
					server.AddLog(fmt.Sprintf("推送失败: %v", err))
				}
			}

			// 桥接日志
			eng.LogChan = make(chan string, 100)
			go func() {
				for msg := range eng.LogChan {
					server.AddLog(msg)
				}
			}()

			currentEngine = eng

			// 后台运行工作流
			go func() {
				defer func() {
					if r := recover(); r != nil {
						server.AddLog(fmt.Sprintf("工作流异常: %v", r))
					}
					currentEngine = nil
				}()
				eng.Run()
			}()

			return nil
		},
		OnStop: func() error {
			if currentEngine != nil {
				currentEngine.Stop()
				currentEngine = nil
			}
			return nil
		},
		OnGetStatus: func() map[string]interface{} {
			result := map[string]interface{}{
				"state":  "idle",
				"config": "",
				"uptime": "0s",
				"slot":   "-",
				"window": "-",
				"step":   "-",
			}
			if currentEngine != nil {
				result["state"] = currentEngine.State.String()
				result["config"] = currentEngine.ConfigPath
				result["uptime"] = time.Since(currentEngine.StartTime).Round(time.Second).String()
				if currentEngine.FoundSlot {
					result["slot"] = "有号!"
				} else {
					result["slot"] = "监控中"
				}
				if currentEngine.WindowHWND != 0 {
					result["window"] = fmt.Sprintf("HWND=%x", currentEngine.WindowHWND)
				} else {
					result["window"] = "未找到"
				}
				result["step"] = fmt.Sprintf("步骤 %d/%d", currentEngine.CurrentStep+1, len(currentEngine.Config.Steps))
			}
			return result
		},
		OnListConfigs: func() []map[string]string {
			entries, err := os.ReadDir(filepath.Join(baseDir, "configs"))
			if err != nil {
				return nil
			}
			var configs []map[string]string
			for _, e := range entries {
				if !e.IsDir() && (filepath.Ext(e.Name()) == ".yaml" || filepath.Ext(e.Name()) == ".yml") {
					path := filepath.Join(baseDir, "configs", e.Name())
					configs = append(configs, map[string]string{
						"path": path,
						"name": e.Name()[:len(e.Name())-len(filepath.Ext(e.Name()))],
					})
				}
			}
			return configs
		},
	})

	// 优雅退出
	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, os.Interrupt)
		<-sig
		log.Println("收到退出信号")
		if currentEngine != nil {
			currentEngine.Stop()
		}
		os.Exit(0)
	}()

	// 启动服务器
	addr := "http://127.0.0.1:8088"
	fmt.Printf("\n华医通助手已启动\n")
	fmt.Printf("请打开浏览器访问: %s\n\n", addr)

	// 自动打开浏览器
	go func() {
		time.Sleep(500 * time.Millisecond)
		exec.Command("cmd", "/c", "start", addr).Start()
	}()

	if err := server.Start(); err != nil {
		log.Fatalf("服务器启动失败: %v", err)
	}
}
