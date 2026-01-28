package models

import "time"

type Task struct {
	ID         string     `gorm:"primaryKey;size:64" json:"taskId"`
	Name       string     `gorm:"size:255;not null" json:"taskName"`
	Status     string     `gorm:"size:32;not null" json:"status"`
	Config     string     `gorm:"type:json;default:null" json:"config,omitempty"`
	Creator    string     `gorm:"size:64;default:null" json:"creator,omitempty"`
	CreatedAt  time.Time  `gorm:"autoCreateTime" json:"createdAt"`
	UpdatedAt  time.Time  `gorm:"autoUpdateTime" json:"updatedAt"`
	StartedAt  *time.Time `json:"startedAt,omitempty"`
	FinishedAt *time.Time `json:"finishedAt,omitempty"`
}

type Target struct {
	ID        uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	TaskID    string    `gorm:"size:64;index" json:"taskId"`
	Target    string    `gorm:"size:512;not null" json:"target"`
	CreatedAt time.Time `gorm:"autoCreateTime" json:"createdAt"`
	UpdatedAt time.Time `gorm:"autoUpdateTime" json:"updatedAt"`
}

type Finding struct {
	ID         uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	TaskID     string    `gorm:"size:64;index" json:"taskId"`
	Target     string    `gorm:"size:512" json:"target"`
	TemplateID string    `gorm:"size:128" json:"templateId"`
	Severity   string    `gorm:"size:32" json:"severity"`
	Title      string    `gorm:"size:512" json:"title"`
	Details    string    `gorm:"type:json" json:"details,omitempty"`
	RawRef     string    `gorm:"size:1024" json:"rawRef,omitempty"`
	CreatedAt  time.Time `gorm:"autoCreateTime" json:"createdAt"`
}

type TaskLog struct {
	ID        uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	TaskID    string    `gorm:"size:64;index" json:"taskId"`
	Action    string    `gorm:"size:64" json:"action"`
	Actor     string    `gorm:"size:128" json:"actor,omitempty"`
	Message   string    `gorm:"type:text" json:"message,omitempty"`
	CreatedAt time.Time `gorm:"autoCreateTime" json:"createdAt"`
}

type SastTask struct {
	ID            string    `gorm:"primaryKey;size:64" json:"id"`
	Type          string    `gorm:"size:32;not null" json:"type"` // "Git" or "Upload"
	Target        string    `gorm:"size:512;not null" json:"target"`
	Branch        string    `gorm:"size:128" json:"branch,omitempty"`
	Status        string    `gorm:"size:32;not null" json:"status"` // pending, running, completed, failed
	Rules         string    `gorm:"type:json" json:"rules"`         // Selected CWEs
	Result        string    `gorm:"type:text" json:"result,omitempty"`
	EnableAIAudit bool      `gorm:"default:false" json:"enableAiAudit"`
	CreatedAt     time.Time `gorm:"autoCreateTime" json:"startTime"`
	UpdatedAt     time.Time `gorm:"autoUpdateTime" json:"updatedAt"`
}

type SastFinding struct {
	ID          uint64 `gorm:"primaryKey;autoIncrement" json:"id"`
	TaskID      string `gorm:"size:64;index" json:"taskId"`
	RuleID      string `gorm:"size:64" json:"ruleId"`
	Description string `gorm:"size:512" json:"description"`
	Severity    string `gorm:"size:32" json:"severity"`
	File        string `gorm:"size:512" json:"file"`
	Line        int    `gorm:"default:0" json:"line"`
	Message     string `gorm:"type:text" json:"message"`
	CodeFlow    string `gorm:"type:json" json:"codeFlow,omitempty"`
	AIAnalysis  string `gorm:"type:text" json:"aiAnalysis,omitempty"`
}

type ScaTask struct {
	ID              string    `gorm:"primaryKey;size:64" json:"id"`
	Type            string    `gorm:"size:32;not null" json:"type"` // "Git" or "Upload"
	Target          string    `gorm:"size:512;not null" json:"target"`
	Branch          string    `gorm:"size:128" json:"branch,omitempty"`
	Status          string    `gorm:"size:32;not null" json:"status"` // pending, running, completed, failed
	Result          string    `gorm:"type:text" json:"result,omitempty"`
	MaxSeverity     string    `gorm:"size:32" json:"maxSeverity"` // Critical, High, Medium, Low
	VulnCount       int       `gorm:"default:0" json:"vulnCount"`
	ProjectLanguage string    `gorm:"size:64" json:"projectLanguage"` // java, go, python, javascript, etc.
	CreatedAt       time.Time `gorm:"autoCreateTime" json:"startTime"`
	UpdatedAt       time.Time `gorm:"autoUpdateTime" json:"updatedAt"`
}

type ScaFinding struct {
	ID           uint64 `gorm:"primaryKey;autoIncrement" json:"id"`
	TaskID       string `gorm:"size:64;index" json:"taskId"`
	PackageName  string `gorm:"size:255" json:"packageName"`
	Version      string `gorm:"size:64" json:"version"`
	Language     string `gorm:"size:32" json:"language"`
	License      string `gorm:"size:128" json:"license"`
	VulnID       string `gorm:"size:64" json:"vulnId"` // CVE-XXX or OSV ID
	Severity     string `gorm:"size:32" json:"severity"`
	Description  string `gorm:"type:text" json:"description"`
	FixedVersion string `gorm:"size:64" json:"fixedVersion"`
	Reference    string `gorm:"size:1024" json:"reference"`
	FilePath     string `gorm:"size:512" json:"filePath"`
}
