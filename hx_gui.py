#!/usr/bin/env python
# -*- coding: utf-8 -*-
"""
华医通自动挂号 - 动森风格桌面窗口
Go 服务器后台 + Python tkinter 桌面窗口
"""

import os
import sys
import json
import time
import threading
import subprocess
import tkinter as tk
from tkinter import ttk, scrolledtext, messagebox

try:
    import requests
except ImportError:
    subprocess.check_call([sys.executable, "-m", "pip", "install", "requests"])
    import requests

# ========== 动森配色 ==========
C = {
    "bg": "#f8f8f0",
    "card": "#f7f3df",
    "text": "#794f27",
    "text2": "#9f927d",
    "success": "#6fba2c",
    "success_a": "#5a9e1e",
    "error": "#e05a5a",
    "error_a": "#c94444",
    "warning": "#f5c31c",
    "border": "#aaa69d",
    "primary": "#19c8b9",
    "header_start": "#7DC395",
    "header_end": "#19c8b9",
    "white": "#ffffff",
    "log_bg": "#2b2118",
    "log_fg": "#d4cfc3",
    "focus": "#ffcc00",
}

SERVER_URL = "http://127.0.0.1:8088"
BASE_DIR = os.path.dirname(os.path.abspath(__file__))


class App:
    def __init__(self, root):
        self.root = root
        self.root.title("华医通自动挂号")
        self.root.geometry("900x660")
        self.root.minsize(800, 560)
        self.root.configure(bg=C["bg"])
        self.root.protocol("WM_DELETE_WINDOW", self.on_close)

        self.running = False
        self.stop_poll = False
        self.config_visible = False

        self.setup_ui()
        self.start_polling()
        self.log("桌面窗口已就绪", "info")

    # ========== UI 构建 ==========
    def setup_ui(self):
        # --- Header ---
        header = tk.Frame(self.root, bg=C["header_start"], height=52)
        header.pack(fill=tk.X)
        header.pack_propagate(False)

        title = tk.Label(header, text="华医通自动挂号",
                        font=("Microsoft YaHei", 16, "bold"),
                        fg=C["white"], bg=C["header_start"])
        title.pack(side=tk.LEFT, padx=20, pady=10)

        # 状态指示器
        status_box = tk.Frame(header, bg=C["header_start"])
        status_box.pack(side=tk.RIGHT, padx=20, pady=10)

        self.status_frame = tk.Frame(status_box, bg="#9fd5b5", highlightbackground="#bce4cf",
                                      highlightthickness=1, padx=10, pady=2)
        self.status_frame.pack()

        self.status_dot = tk.Canvas(self.status_frame, width=14, height=14,
                                     bg="#9fd5b5", highlightthickness=0)
        self.status_dot.pack(side=tk.LEFT, padx=(0, 6))
        self.dot_id = self.status_dot.create_oval(1, 1, 13, 13, fill="#d9d9d9", outline="")

        self.status_text = tk.Label(self.status_frame, text="空闲",
                                     font=("Microsoft YaHei", 10, "bold"),
                                     fg=C["white"], bg="#9fd5b5")
        self.status_text.pack(side=tk.LEFT)

        # --- 主体 ---
        main = tk.Frame(self.root, bg=C["bg"])
        main.pack(fill=tk.BOTH, expand=True, padx=14, pady=10)

        # 左侧面板
        left = tk.Frame(main, bg=C["bg"], width=380)
        left.pack(side=tk.LEFT, fill=tk.BOTH, padx=(0, 7))
        left.pack_propagate(False)

        self.build_control_panel(left)
        self.build_status_panel(left)

        # 右侧面板
        right = tk.Frame(main, bg=C["bg"])
        right.pack(side=tk.LEFT, fill=tk.BOTH, expand=True, padx=(7, 0))

        self.build_right_panel(right)

    def build_control_panel(self, parent):
        card = tk.Frame(parent, bg=C["card"], highlightbackground="#e8dcc8",
                        highlightthickness=2, padx=16, pady=12)
        card.pack(fill=tk.X, pady=(0, 8))

        tk.Label(card, text="控制面板", font=("Microsoft YaHei", 12, "bold"),
                fg=C["text"], bg=C["card"]).pack(anchor=tk.W)

        # 分隔线
        sep = tk.Frame(card, bg="#e8dcc8", height=2)
        sep.pack(fill=tk.X, pady=(4, 10))

        # 运行模式
        row1 = tk.Frame(card, bg=C["card"])
        row1.pack(fill=tk.X, pady=3)
        tk.Label(row1, text="运行模式", font=("Microsoft YaHei", 10),
                fg=C["text2"], bg=C["card"], width=8, anchor=tk.W).pack(side=tk.LEFT)
        self.mode_var = tk.StringVar(value="定点抢号")
        mode_cb = ttk.Combobox(row1, textvariable=self.mode_var,
                                values=["定点抢号", "监控补号"],
                                state="readonly", font=("Microsoft YaHei", 10), width=18)
        mode_cb.pack(side=tk.LEFT, padx=(4, 0))
        mode_cb.bind("<<ComboboxSelected>>", self._on_mode_change)

        # 就诊人
        row2 = tk.Frame(card, bg=C["card"])
        row2.pack(fill=tk.X, pady=3)
        tk.Label(row2, text="就诊人", font=("Microsoft YaHei", 10),
                fg=C["text2"], bg=C["card"], width=8, anchor=tk.W).pack(side=tk.LEFT)
        self.in_patient = tk.Entry(row2, font=("Microsoft YaHei", 10), width=20,
                                    bg=C["white"], fg=C["text"], relief=tk.FLAT,
                                    insertbackground=C["text"])
        self.in_patient.pack(side=tk.LEFT, padx=(4, 0))

        # 科室
        row3 = tk.Frame(card, bg=C["card"])
        row3.pack(fill=tk.X, pady=3)
        tk.Label(row3, text="科室", font=("Microsoft YaHei", 10),
                fg=C["text2"], bg=C["card"], width=8, anchor=tk.W).pack(side=tk.LEFT)
        self.in_dept = tk.Entry(row3, font=("Microsoft YaHei", 10), width=20,
                                 bg=C["white"], fg=C["text"], relief=tk.FLAT)
        self.in_dept.pack(side=tk.LEFT, padx=(4, 0))

        # 医生
        row4 = tk.Frame(card, bg=C["card"])
        row4.pack(fill=tk.X, pady=3)
        tk.Label(row4, text="医生", font=("Microsoft YaHei", 10),
                fg=C["text2"], bg=C["card"], width=8, anchor=tk.W).pack(side=tk.LEFT)
        self.in_doctor = tk.Entry(row4, font=("Microsoft YaHei", 10), width=20,
                                   bg=C["white"], fg=C["text"], relief=tk.FLAT)
        self.in_doctor.pack(side=tk.LEFT, padx=(4, 0))

        # 配置文件
        row5 = tk.Frame(card, bg=C["card"])
        row5.pack(fill=tk.X, pady=3)
        tk.Label(row5, text="配置文件", font=("Microsoft YaHei", 10),
                fg=C["text2"], bg=C["card"], width=8, anchor=tk.W).pack(side=tk.LEFT)
        self.config_var = tk.StringVar(value="定点抢号.yaml")
        configs = self._list_configs()
        self.cfg_cb = ttk.Combobox(row5, textvariable=self.config_var,
                                    values=configs, state="readonly",
                                    font=("Microsoft YaHei", 10), width=18)
        self.cfg_cb.pack(side=tk.LEFT, padx=(4, 0))

        # 按钮行
        btns = tk.Frame(card, bg=C["card"])
        btns.pack(fill=tk.X, pady=(10, 0))

        self.btn_start = tk.Button(btns, text="启动", font=("Microsoft YaHei", 10, "bold"),
                                    bg=C["success"], fg=C["white"], relief=tk.FLAT,
                                    activebackground=C["success_a"], activeforeground=C["white"],
                                    padx=18, pady=4, bd=0, cursor="hand2",
                                    command=self.start)
        self.btn_start.pack(side=tk.LEFT, padx=(0, 6))

        self.btn_stop = tk.Button(btns, text="停止", font=("Microsoft YaHei", 10, "bold"),
                                   bg=C["error"], fg=C["white"], relief=tk.FLAT,
                                   activebackground=C["error_a"], activeforeground=C["white"],
                                   padx=18, pady=4, bd=0, cursor="hand2",
                                   command=self.stop, state=tk.DISABLED)
        self.btn_stop.pack(side=tk.LEFT, padx=(0, 6))

        tk.Button(btns, text="截图", font=("Microsoft YaHei", 9),
                 bg=C["bg"], fg=C["text"], relief=tk.FLAT,
                 padx=12, pady=4, bd=0, cursor="hand2",
                 command=self._debug).pack(side=tk.LEFT, padx=(0, 6))

        tk.Button(btns, text="配置", font=("Microsoft YaHei", 9),
                 bg=C["bg"], fg=C["text"], relief=tk.FLAT,
                 padx=12, pady=4, bd=0, cursor="hand2",
                 command=self._toggle_config).pack(side=tk.LEFT)

    def build_status_panel(self, parent):
        card = tk.Frame(parent, bg=C["card"], highlightbackground="#e8dcc8",
                        highlightthickness=2, padx=16, pady=12)
        card.pack(fill=tk.X, pady=(8, 0))

        tk.Label(card, text="运行状态", font=("Microsoft YaHei", 12, "bold"),
                fg=C["text"], bg=C["card"]).pack(anchor=tk.W)

        sep = tk.Frame(card, bg="#e8dcc8", height=2)
        sep.pack(fill=tk.X, pady=(4, 10))

        self.info_vars = {}
        items = [("状态", "state", "空闲"), ("步骤", "step", "-"),
                  ("运行时间", "uptime", "-"), ("号源", "slot", "-"),
                  ("窗口", "window", "-")]
        for label, key, default in items:
            row = tk.Frame(card, bg=C["card"])
            row.pack(fill=tk.X, pady=2)
            tk.Label(row, text=label, font=("Microsoft YaHei", 9),
                    fg=C["text2"], bg=C["card"], width=8, anchor=tk.W).pack(side=tk.LEFT)
            var = tk.StringVar(value=default)
            self.info_vars[key] = var
            lbl = tk.Label(row, textvariable=var, font=("Microsoft YaHei", 9, "bold"),
                          fg=C["text"], bg=C["card"], anchor=tk.W)
            lbl.pack(side=tk.LEFT, padx=(4, 0))
            self.info_vars[key + "_label"] = lbl

    def build_right_panel(self, parent):
        # 日志区域
        log_card = tk.Frame(parent, bg=C["card"], highlightbackground="#e8dcc8",
                           highlightthickness=2)
        log_card.pack(fill=tk.BOTH, expand=True)

        log_header = tk.Frame(log_card, bg=C["card"], padx=12, pady=6)
        log_header.pack(fill=tk.X)
        tk.Label(log_header, text="运行日志", font=("Microsoft YaHei", 12, "bold"),
                fg=C["text"], bg=C["card"]).pack(side=tk.LEFT)
        tk.Button(log_header, text="清空", font=("Microsoft YaHei", 8),
                 bg=C["bg"], fg=C["text"], relief=tk.FLAT, bd=0, padx=8,
                 command=self._clear_logs, cursor="hand2").pack(side=tk.RIGHT)

        log_body = tk.Frame(log_card, bg=C["log_bg"], padx=1, pady=1)
        log_body.pack(fill=tk.BOTH, expand=True, padx=8, pady=(0, 8))

        self.log_area = scrolledtext.ScrolledText(
            log_body, font=("Consolas", 9),
            bg=C["log_bg"], fg=C["log_fg"],
            insertbackground="white", relief=tk.FLAT,
            wrap=tk.WORD, borderwidth=0, highlightthickness=0,
            padx=6, pady=6
        )
        self.log_area.pack(fill=tk.BOTH, expand=True)
        self.log_area.config(state=tk.DISABLED)

        # 配置编辑器 (默认隐藏)
        self.config_frame = tk.Frame(parent, bg=C["card"],
                                      highlightbackground="#e8dcc8", highlightthickness=2)

        cfg_header = tk.Frame(self.config_frame, bg=C["card"], padx=12, pady=6)
        cfg_header.pack(fill=tk.X)
        tk.Label(cfg_header, text="YAML 配置编辑器 (保存后热更新)",
                font=("Microsoft YaHei", 12, "bold"),
                fg=C["text"], bg=C["card"]).pack(side=tk.LEFT)

        self.config_editor = scrolledtext.ScrolledText(
            self.config_frame, font=("Consolas", 9),
            bg=C["log_bg"], fg=C["log_fg"],
            insertbackground="white", relief=tk.FLAT,
            height=8, borderwidth=0, highlightthickness=0,
            padx=6, pady=6
        )
        self.config_editor.pack(fill=tk.BOTH, expand=True, padx=8, pady=(0, 4))

        cfg_btns = tk.Frame(self.config_frame, bg=C["card"], padx=8)
        cfg_btns.pack(fill=tk.X, pady=(0, 8))
        tk.Button(cfg_btns, text="加载", font=("Microsoft YaHei", 9),
                 bg=C["bg"], fg=C["text"], relief=tk.FLAT, bd=0, padx=12,
                 command=self._load_config_content, cursor="hand2").pack(side=tk.LEFT, padx=(0, 6))
        tk.Button(cfg_btns, text="保存 (热更新)", font=("Microsoft YaHei", 9, "bold"),
                 bg=C["success"], fg=C["white"], relief=tk.FLAT, bd=0, padx=12,
                 command=self._save_config_content, cursor="hand2").pack(side=tk.LEFT, padx=(0, 6))
        tk.Button(cfg_btns, text="关闭", font=("Microsoft YaHei", 9),
                 bg=C["bg"], fg=C["text"], relief=tk.FLAT, bd=0, padx=12,
                 command=self._toggle_config, cursor="hand2").pack(side=tk.LEFT)

    # ========== 日志 ==========
    def log(self, msg, level="info"):
        self.log_area.config(state=tk.NORMAL)
        ts = time.strftime("%H:%M:%S")
        colors = {"success": C["success"], "error": C["error"],
                   "warn": C["warning"], "info": C["primary"], "": C["log_fg"]}
        tag = level if level in colors else ""
        self.log_area.insert(tk.END, f"[{ts}] {msg}\n", tag)
        for t, c in colors.items():
            if t:
                self.log_area.tag_config(t, foreground=c)
        self.log_area.see(tk.END)
        self.log_area.config(state=tk.DISABLED)

    def _clear_logs(self):
        self.log_area.config(state=tk.NORMAL)
        self.log_area.delete(1.0, tk.END)
        self.log_area.config(state=tk.DISABLED)

    # ========== 配置管理 ==========
    def _list_configs(self):
        d = os.path.join(BASE_DIR, "configs")
        if not os.path.exists(d):
            return []
        return sorted([f for f in os.listdir(d) if f.endswith((".yaml", ".yml"))])

    def _on_mode_change(self, event=None):
        mode = self.mode_var.get()
        mapping = {"定点抢号": "定点抢号.yaml", "监控补号": "监控补号.yaml"}
        if mode in mapping and mapping[mode] in self._list_configs():
            self.config_var.set(mapping[mode])

    def _toggle_config(self):
        if self.config_visible:
            self.config_frame.pack_forget()
        else:
            self.config_frame.pack(fill=tk.BOTH, padx=0, pady=(8, 0))
            self._load_config_content()
        self.config_visible = not self.config_visible

    def _load_config_content(self):
        config_name = self.config_var.get()
        config_path = os.path.join(BASE_DIR, "configs", config_name)
        try:
            resp = requests.get(
                f"{SERVER_URL}/api/config/load",
                params={"path": config_path}, timeout=5
            )
            data = resp.json()
            if "content" in data:
                self.config_editor.delete(1.0, tk.END)
                self.config_editor.insert(1.0, data["content"])
                self.log("配置已加载", "info")
        except Exception as e:
            self.log(f"加载配置失败: {e}", "error")

    def _save_config_content(self):
        config_name = self.config_var.get()
        config_path = os.path.join(BASE_DIR, "configs", config_name)
        content = self.config_editor.get(1.0, tk.END)
        try:
            resp = requests.post(f"{SERVER_URL}/api/config/save", json={
                "path": config_path, "content": content
            }, timeout=5)
            data = resp.json()
            if data.get("ok"):
                self.log("配置已保存 (热更新生效)", "success")
            else:
                self.log(f"保存失败: {data}", "error")
        except Exception as e:
            self.log(f"保存失败: {e}", "error")

    # ========== 启动/停止 ==========
    def start(self):
        config_path = os.path.join(BASE_DIR, "configs", self.config_var.get())
        payload = {
            "config_path": config_path,
            "mode": self.mode_var.get(),
            "patient": self.in_patient.get(),
            "department": self.in_dept.get(),
            "doctor": self.in_doctor.get(),
        }
        try:
            resp = requests.post(f"{SERVER_URL}/api/start", json=payload, timeout=10)
            data = resp.json()
            if data.get("ok"):
                self.set_running(True)
                self.log(f"已启动: {payload['mode']} - {payload['doctor']}", "success")
            else:
                msg = data.get("error", str(data))
                self.log(f"启动失败: {msg}", "error")
                messagebox.showerror("启动失败", msg)
        except requests.ConnectionError:
            msg = f"无法连接服务器 {SERVER_URL}"
            self.log(msg, "error")
            messagebox.showerror("连接失败", f"{msg}\n请确保 Go 服务器已启动")
        except Exception as e:
            self.log(f"启动失败: {e}", "error")

    def stop(self):
        try:
            resp = requests.post(f"{SERVER_URL}/api/stop", timeout=10)
            data = resp.json()
            if data.get("ok"):
                self.set_running(False)
                self.log("已停止", "warn")
        except Exception as e:
            self.log(f"停止失败: {e}", "error")

    def _debug(self):
        self.log("正在截图调试...", "info")
        try:
            resp = requests.get(f"{SERVER_URL}/api/status", timeout=5)
            data = resp.json()
            self.log(f"状态: state={data.get('state')}, step={data.get('step')}, "
                     f"slot={data.get('slot')}, window={data.get('window')}", "info")
        except Exception as e:
            self.log(f"调试失败: {e}", "error")

    def set_running(self, running):
        self.running = running
        if running:
            self.status_dot.itemconfig(self.dot_id, fill=C["success"])
            self.status_text.config(text="运行中")
            self.btn_start.config(state=tk.DISABLED)
            self.btn_stop.config(state=tk.NORMAL)
        else:
            self.status_dot.itemconfig(self.dot_id, fill="#d9d9d9")
            self.status_text.config(text="空闲")
            self.btn_start.config(state=tk.NORMAL)
            self.btn_stop.config(state=tk.DISABLED)

    # ========== 轮询 ==========
    def start_polling(self):
        self.poll_thread = threading.Thread(target=self._poll_loop, daemon=True)
        self.poll_thread.start()

    def _poll_loop(self):
        while not self.stop_poll:
            try:
                resp = requests.get(f"{SERVER_URL}/api/status", timeout=3)
                if resp.ok:
                    data = resp.json()
                    self.root.after(0, self._update_status, data)
            except Exception:
                pass
            time.sleep(2)

    def _update_status(self, data):
        state = str(data.get("state", "-"))
        self.info_vars["state"].set(state)
        self.info_vars["step"].set(str(data.get("step", "-")))
        self.info_vars["uptime"].set(str(data.get("uptime", "-")))
        self.info_vars["slot"].set(str(data.get("slot", "-")))
        self.info_vars["window"].set(str(data.get("window", "-")))

        # 号源高亮
        slot_label = self.info_vars.get("slot_label")
        if slot_label:
            if data.get("slot") == "有号!":
                slot_label.config(fg=C["success"])
            else:
                slot_label.config(fg=C["text"])

        is_running = state in ("运行中", "running")
        if is_running != self.running:
            self.set_running(is_running)

    def on_close(self):
        if self.running:
            if not messagebox.askyesno("确认", "任务正在运行中，确定退出吗？\n(Go 服务器会继续运行)"):
                return
            self.stop()
        self.stop_poll = True
        self.root.destroy()


# ========== 启动 Go 服务器 ==========
def start_go_server():
    exe_path = os.path.join(BASE_DIR, "hx_jiankong.exe")
    if not os.path.exists(exe_path):
        print(f"[ERROR] 未找到 {exe_path}")
        return None
    try:
        proc = subprocess.Popen(
            [exe_path],
            cwd=BASE_DIR,
            creationflags=subprocess.CREATE_NO_WINDOW if sys.platform == "win32" else 0
        )
        return proc
    except Exception as e:
        print(f"[ERROR] 启动 Go 服务器失败: {e}")
        return None


def main():
    # 检查 Go 服务器是否已在运行
    server_running = False
    try:
        resp = requests.get(f"{SERVER_URL}/api/health", timeout=2)
        if resp.ok:
            print("[INFO] Go 服务器已在运行")
            server_running = True
    except Exception:
        pass

    if not server_running:
        print("[INFO] 正在启动 Go 服务器...")
        proc = start_go_server()
        if proc:
            # 等待服务启动
            for _ in range(15):
                time.sleep(0.5)
                try:
                    requests.get(f"{SERVER_URL}/api/health", timeout=1)
                    print("[INFO] Go 服务器已启动")
                    break
                except Exception:
                    pass
            else:
                print("[WARN] Go 服务器可能启动较慢，请稍候...")
        else:
            print("[WARN] Go 服务器启动失败，界面功能将不可用")

    root = tk.Tk()
    app = App(root)
    root.mainloop()


if __name__ == "__main__":
    main()
