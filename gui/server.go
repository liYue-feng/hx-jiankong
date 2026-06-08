package gui

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

type Server struct {
	Port       int
	ConfigDir  string
	LogManager *LogManager
	AppHandler *AppHandler

	mu     sync.RWMutex
	wsClients map[*websocket.Conn]bool
}

type LogManager struct {
	mu    sync.RWMutex
	lines []string
}

type AppHandler struct {
	ConfigDir    string
	CurrentConfig string
	LogManager   *LogManager
	OnStart      func(configPath, mode, patient, dept, doctor string) error
	OnStop       func() error
	OnGetStatus  func() map[string]interface{}
	OnListConfigs func() []map[string]string
}

func NewServer(port int, configDir string) *Server {
	lm := &LogManager{lines: make([]string, 0, 1000)}
	return &Server{
		Port:       port,
		ConfigDir:  configDir,
		LogManager: lm,
		wsClients:  make(map[*websocket.Conn]bool),
	}
}

func (s *Server) Start() error {
	mux := http.NewServeMux()

	// API 路由
	mux.HandleFunc("/api/health", s.handleHealth)
	mux.HandleFunc("/api/status", s.handleStatus)
	mux.HandleFunc("/api/start", s.handleStart)
	mux.HandleFunc("/api/stop", s.handleStop)
	mux.HandleFunc("/api/configs", s.handleConfigs)
	mux.HandleFunc("/api/config/load", s.handleLoadConfig)
	mux.HandleFunc("/api/config/save", s.handleSaveConfig)
	mux.HandleFunc("/api/logs", s.handleLogs)
	mux.HandleFunc("/ws", s.handleWebSocket)

	// 静态文件
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			http.ServeFile(w, r, filepath.Join(s.ConfigDir, "gui", "index.html"))
			return
		}
		http.ServeFile(w, r, filepath.Join(s.ConfigDir, "gui", r.URL.Path))
	})

	addr := fmt.Sprintf("127.0.0.1:%d", s.Port)
	log.Printf("GUI 服务器启动在 http://%s", addr)
	return http.ListenAndServe(addr, mux)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"status": "ok"})
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	status := map[string]interface{}{
		"state":   "idle",
		"config":  "",
		"uptime":  "0s",
		"slot":    "-",
		"window":  "-",
		"step":    "-",
	}
	if s.AppHandler != nil && s.AppHandler.OnGetStatus != nil {
		for k, v := range s.AppHandler.OnGetStatus() {
			status[k] = v
		}
	}
	// always include logs
	status["logs"] = s.LogManager.GetRecent(20)
	json.NewEncoder(w).Encode(status)
}

func (s *Server) handleStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", 405)
		return
	}
	var req struct {
		ConfigPath string `json:"config_path"`
		Mode       string `json:"mode"`
		Patient    string `json:"patient"`
		Department string `json:"department"`
		Doctor     string `json:"doctor"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
		return
	}

	if s.AppHandler == nil || s.AppHandler.OnStart == nil {
		json.NewEncoder(w).Encode(map[string]interface{}{"error": "处理器未初始化"})
		return
	}

	err := s.AppHandler.OnStart(req.ConfigPath, req.Mode, req.Patient, req.Department, req.Doctor)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
}

func (s *Server) handleStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", 405)
		return
	}
	if s.AppHandler != nil && s.AppHandler.OnStop != nil {
		s.AppHandler.OnStop()
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
}

func (s *Server) handleConfigs(w http.ResponseWriter, r *http.Request) {
	configs := []map[string]string{}
	if s.AppHandler != nil && s.AppHandler.OnListConfigs != nil {
		configs = s.AppHandler.OnListConfigs()
	} else {
		// 默认扫描 configs 目录
		files, _ := filepath.Glob(filepath.Join(s.ConfigDir, "*.yaml"))
		for _, f := range files {
			name := filepath.Base(f)
			configs = append(configs, map[string]string{
				"path": f,
				"name": strings.TrimSuffix(name, ".yaml"),
			})
		}
	}
	json.NewEncoder(w).Encode(configs)
}

func (s *Server) handleLoadConfig(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		json.NewEncoder(w).Encode(map[string]interface{}{"error": "缺少路径参数"})
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"path":    path,
		"content": string(data),
	})
}

func (s *Server) handleSaveConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", 405)
		return
	}
	var req struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
		return
	}
	if err := os.WriteFile(req.Path, []byte(req.Content), 0644); err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
}

func (s *Server) handleLogs(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(map[string]interface{}{
		"logs": s.LogManager.GetRecent(100),
	})
}

func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	s.mu.Lock()
	s.wsClients[conn] = true
	s.mu.Unlock()

	// 发送历史日志
	for _, line := range s.LogManager.GetRecent(50) {
		conn.WriteMessage(websocket.TextMessage, []byte(line))
	}

	// 保持连接
	go func() {
		defer func() {
			s.mu.Lock()
			delete(s.wsClients, conn)
			s.mu.Unlock()
			conn.Close()
		}()
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				break
			}
		}
	}()
}

// Broadcast 广播日志到所有 WebSocket 客户端
func (s *Server) Broadcast(msg string) {
	// AddLog 已调用 LogManager.Add，这里不再重复添加
	s.mu.RLock()
	defer s.mu.RUnlock()
	for conn := range s.wsClients {
		conn.WriteMessage(websocket.TextMessage, []byte(msg))
	}
}

func (lm *LogManager) Add(msg string) {
	lm.mu.Lock()
	defer lm.mu.Unlock()
	// 加时间戳
	line := fmt.Sprintf("[%s] %s", time.Now().Format("2006/01/02 15:04:05"), msg)
	lm.lines = append(lm.lines, line)
	if len(lm.lines) > 1000 {
		lm.lines = lm.lines[len(lm.lines)-500:]
	}
}

func (lm *LogManager) GetRecent(n int) []string {
	lm.mu.RLock()
	defer lm.mu.RUnlock()
	if len(lm.lines) <= n {
		result := make([]string, len(lm.lines))
		copy(result, lm.lines)
		return result
	}
	start := len(lm.lines) - n
	result := make([]string, n)
	copy(result, lm.lines[start:])
	return result
}

// 跨包函数 (engine -> gui)
func (s *Server) SetAppHandler(h *AppHandler) {
	s.AppHandler = h
}

func (s *Server) AddLog(msg string) {
	s.LogManager.Add(msg)
	s.Broadcast(msg)
}
