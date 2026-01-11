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
