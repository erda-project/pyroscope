package appmetadata

import (
	"time"

	"github.com/pyroscope-io/pyroscope/pkg/storage/metadata"
	"github.com/pyroscope-io/pyroscope/pkg/storage/segment"
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
	PodIP           string                   `gorm:"column:pod_ip" json:"podIP,omitempty"`
}

func (ApplicationMetadata) TableName() string {
	return "erda_profile_app"
}

func (a ApplicationMetadata) ToSegmentKey() *segment.Key {
	keys := make(map[string]string)
	keys["__name__"] = a.FQName
	if a.OrgID != "" {
		keys["DICE_ORG_ID"] = a.OrgID
	}
	if a.OrgName != "" {
		keys["DICE_ORG_NAME"] = a.OrgName
	}
	if a.Workspace != "" {
		keys["DICE_WORKSPACE"] = a.Workspace
	}
	if a.ProjectID != "" {
		keys["DICE_PROJECT_ID"] = a.ProjectID
	}
	if a.ProjectName != "" {
		keys["DICE_PROJECT_NAME"] = a.ProjectName
	}
	if a.AppID != "" {
		keys["DICE_APPLICATION_ID"] = a.AppID
	}
	if a.AppName != "" {
		keys["DICE_APPLICATION_NAME"] = a.AppName
	}
	if a.ClusterName != "" {
		keys["DICE_CLUSTER_NAME"] = a.ClusterName
	}
	if a.ServiceName != "" {
		keys["DICE_SERVICE"] = a.ServiceName
	}
	if a.PodIP != "" {
		keys["POD_IP"] = a.PodIP
	}
	return segment.NewKey(keys)
}
