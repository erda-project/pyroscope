package appmetadata

import (
	"time"

	"github.com/pyroscope-io/pyroscope/pkg/storage/metadata"
)

type ApplicationMetadata struct {
	ID uint `gorm:"primarykey"`
	// Fully Qualified Name. Eg app.cpu ({__name__}.{profile_type})
	FQName string `gorm:"column:name;index,unique;not null;default:null" json:"name"`

	SpyName         string                   `gorm:"column:spy_name" json:"spyName,omitempty"`
	SampleRate      uint32                   `gorm:"column:sample_rate" json:"sampleRate,omitempty"`
	Units           metadata.Units           `gorm:"column:units" json:"units,omitempty"`
	AggregationType metadata.AggregationType `gorm:"column:aggregation_type" json:"-"`
	SampleType      string                   `gorm:"column:sample_type" json:"sampleType,omitempty"`
	IsDeleted       bool                     `gorm:"column:is_deleted" json:"isDeleted,omitempty"`
	CreatedAt       time.Time                `gorm:"column:created_at" json:"createdAt"`
	UpdatedAt       time.Time                `gorm:"column:updated_at" json:"updatedAt"`
	DeletedAt       uint64                   `gorm:"column:deleted_at" json:"deletedAt,omitempty"`
	OrgID           string                   `gorm:"column:org_id" json:"orgID,omitempty"`
	OrgName         string                   `gorm:"column:org_name" json:"orgName,omitempty"`
	ProjectID       string                   `gorm:"column:project_id" json:"projectID,omitempty"`
	ProjectName     string                   `gorm:"column:project_name" json:"projectName,omitempty"`
	AppID           string                   `gorm:"column:app_id" json:"appID,omitempty"`
	AppName         string                   `gorm:"column:app_name" json:"appName,omitempty"`
	Workspace       string                   `gorm:"column:workspace" json:"workspace,omitempty"`
	ClusterName     string                   `gorm:"column:cluster_name" json:"clusterName,omitempty"`
	ServiceName     string                   `gorm:"column:service_name" json:"serviceName,omitempty"`
}

func (ApplicationMetadata) TableName() string {
	return "erda_profile_app"
}
