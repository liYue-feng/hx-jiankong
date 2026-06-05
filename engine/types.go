package engine

import "time"

// WorkflowStep 表示一个工作流步骤
type WorkflowStep struct {
	Action      string            `yaml:"action" json:"action"`
	Desc        string            `yaml:"desc" json:"desc"`
	X           int               `yaml:"x,omitempty" json:"x,omitempty"`
	Y           int               `yaml:"y,omitempty" json:"y,omitempty"`
	Region      []int             `yaml:"region,omitempty" json:"region,omitempty"` // [left, top, right, bottom]
	Keywords    []string          `yaml:"keywords,omitempty" json:"keywords,omitempty"`
	Timeout     int               `yaml:"timeout,omitempty" json:"timeout,omitempty"`     // 秒
	Retry       int               `yaml:"retry,omitempty" json:"retry,omitempty"`         // 重试次数
	WaitBefore  int               `yaml:"wait_before,omitempty" json:"wait_before,omitempty"` // 步骤前等待秒数
	WaitAfter   int               `yaml:"wait_after,omitempty" json:"wait_after,omitempty"`   // 步骤后等待秒数
	UntilTime   string            `yaml:"until_time,omitempty" json:"until_time,omitempty"`   // "08:00:00" 等待到指定时间
	Interval    *IntervalConfig   `yaml:"interval,omitempty" json:"interval,omitempty"`
	OnFound     *ConditionalStep  `yaml:"on_found,omitempty" json:"on_found,omitempty"`
	OnNotFound  *ConditionalStep  `yaml:"on_not_found,omitempty" json:"on_not_found,omitempty"`
	SubSteps    []WorkflowStep    `yaml:"substeps,omitempty" json:"substeps,omitempty"`
	CheckColor  *ColorCheck       `yaml:"check_color,omitempty" json:"check_color,omitempty"`
	Params      map[string]string `yaml:"params,omitempty" json:"params,omitempty"`
}

type IntervalConfig struct {
	Min int `yaml:"min" json:"min"`
	Max int `yaml:"max" json:"max"`
}

type ConditionalStep struct {
	Message    string         `yaml:"message,omitempty" json:"message,omitempty"`
	NotifyUser bool           `yaml:"notify_user,omitempty" json:"notify_user,omitempty"`
	Steps      []WorkflowStep `yaml:"steps,omitempty" json:"steps,omitempty"`
	Goto       string         `yaml:"goto,omitempty" json:"goto,omitempty"` // 跳转到标签
	Loop       bool           `yaml:"loop,omitempty" json:"loop,omitempty"` // 循环回主步骤
}

type ColorCheck struct {
	Color   string `yaml:"color" json:"color"` // teal=青色(预约), gray=灰色(约满)
	Region  []int  `yaml:"region,omitempty" json:"region,omitempty"`
}

type WorkflowConfig struct {
	Name        string         `yaml:"name" json:"name"`
	Mode        string         `yaml:"mode" json:"mode"` // snipe / monitor
	Description string         `yaml:"description" json:"description"`
	Schedule    ScheduleConfig `yaml:"schedule" json:"schedule"`
	Patient     string         `yaml:"patient" json:"patient"`
	Department  string         `yaml:"department" json:"department"`
	Doctor      string         `yaml:"doctor" json:"doctor"`
	SCTKey      string         `yaml:"sct_key" json:"sct_key"`           // Server酱 Key
	YunmaToken  string         `yaml:"yunma_token" json:"yunma_token"`   // 云码验证码 Token
	Steps       []WorkflowStep `yaml:"steps" json:"steps"`
	Recovery    []WorkflowStep `yaml:"recovery,omitempty" json:"recovery,omitempty"` // 超时恢复步骤
	OnSuccess   []WorkflowStep `yaml:"on_success,omitempty" json:"on_success,omitempty"`
	OnNoSlots   []WorkflowStep `yaml:"on_no_slots,omitempty" json:"on_no_slots,omitempty"`
	AppConfig   AppConfig      `yaml:"app_config" json:"app_config"`
}

type ScheduleConfig struct {
	Enabled     bool   `yaml:"enabled" json:"enabled"`
	EnterTime   string `yaml:"enter_time" json:"enter_time"`   // 进入时间 如 "07:50"
	TargetTime  string `yaml:"target_time" json:"target_time"` // 目标时间 如 "08:00"
	NightStart  int    `yaml:"night_start" json:"night_start"`
	NightEnd    int    `yaml:"night_end" json:"night_end"`
	RefreshMin  int    `yaml:"refresh_min" json:"refresh_min"`
	RefreshMax  int    `yaml:"refresh_max" json:"refresh_max"`
	NoSlotMin   int    `yaml:"noslot_min" json:"noslot_min"`   // 无号推送间隔(分钟)
	NoSlotMax   int    `yaml:"noslot_min" json:"noslot_max"`   // 无号推送间隔(分钟)
	SessionTimeout int `yaml:"session_timeout" json:"session_timeout"` // 小程序超时(分钟)
}

type AppConfig struct {
	BackButton    ClickTarget `yaml:"back_button" json:"back_button"`
	SearchButton  ClickTarget `yaml:"search_button" json:"search_button"`
	ScheduleRegion []int      `yaml:"schedule_region" json:"schedule_region"` // 排期区域
	TitleRegion    []int      `yaml:"title_region" json:"title_region"`       // 标题区域(用于页面识别)
	MiniAppTitle  string     `yaml:"miniapp_title" json:"miniapp_title"`     // 小程序窗口标题
	OCR           OCRConfig  `yaml:"ocr" json:"ocr"`
}

type ClickTarget struct {
	X int `yaml:"x" json:"x"`
	Y int `yaml:"y" json:"y"`
}

type OCRConfig struct {
	Language string `yaml:"language" json:"language"` // chi_sim / eng
	PSM      int    `yaml:"psm" json:"psm"`           // Tesseract PageSegMode
}

// Runtime state
type WorkflowState int

const (
	StateIdle       WorkflowState = iota
	StateRunning
	StatePaused
	StateWaiting    // 等待中(夜间休眠/时间等待)
	StateSuccess    // 挂号成功
	StateError
	StateStopped
)

func (s WorkflowState) String() string {
	switch s {
	case StateIdle:    return "空闲"
	case StateRunning: return "运行中"
	case StatePaused:  return "已暂停"
	case StateWaiting: return "等待中"
	case StateSuccess: return "挂号成功"
	case StateError:   return "出错"
	case StateStopped: return "已停止"
	default:           return "未知"
	}
}

type RuntimeContext struct {
	State          WorkflowState
	CurrentStep    int
	StepName       string
	LogLines       []string
	StartTime      time.Time
	LastSlotCheck  time.Time
	LastNotify     time.Time
	WindowHandle   uintptr
	Config         *WorkflowConfig
	FoundSlot      bool
	SlotText       string
}
