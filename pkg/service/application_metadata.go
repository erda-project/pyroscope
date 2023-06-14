package service

import (
	"context"
	"errors"
	"net/url"

	"github.com/pyroscope-io/pyroscope/pkg/model"
	"github.com/pyroscope-io/pyroscope/pkg/model/appmetadata"
	"gorm.io/gorm"
)

type ApplicationMetadataService struct {
	db *gorm.DB
}

func NewApplicationMetadataService(db *gorm.DB) ApplicationMetadataService {
	return ApplicationMetadataService{db: db}
}

func (svc ApplicationMetadataService) List(ctx context.Context) (apps []appmetadata.ApplicationMetadata, err error) {
	tx := svc.db.WithContext(ctx)
	query, ok := ctx.Value("query").(url.Values)
	if ok {
		if query.Get("projectID") != "" {
			tx = tx.Where("project_id = ?", query.Get("projectID"))
		}
		if query.Get("workspace") != "" {
			tx = tx.Where("workspace = ?", query.Get("workspace"))
		}
		if query.Get("orgID") != "" {
			tx = tx.Where("org_id = ?", query.Get("orgID"))
		}
		if query.Get("appID") != "" {
			tx = tx.Where("app_id = ?", query.Get("appID"))
		}
		if query.Get("podIP") != "" {
			tx = tx.Where("pod_ip = ?", query.Get("podIP"))
		}
		if query.Get("name") != "" {
			tx = tx.Where("name = ?", query.Get("name"))
		}
	}
	result := tx.Find(&apps)
	return apps, result.Error
}

func (svc ApplicationMetadataService) Get(ctx context.Context, name string) (appmetadata.ApplicationMetadata, error) {
	app := appmetadata.ApplicationMetadata{}
	if err := model.ValidateAppName(name); err != nil {
		return app, err
	}

	tx := svc.db.WithContext(ctx)
	res := tx.Where("fq_name = ?", name).First(&app)

	switch {
	case errors.Is(res.Error, gorm.ErrRecordNotFound):
		return app, model.ErrApplicationNotFound
	default:
		return app, res.Error
	}
}

func (svc ApplicationMetadataService) CreateOrUpdate(ctx context.Context, application appmetadata.ApplicationMetadata) error {
	if err := model.ValidateAppName(application.FQName); err != nil {
		return err
	}

	tx := svc.db.WithContext(ctx)

	// Only update the field if it's populated
	return tx.Where(appmetadata.ApplicationMetadata{
		FQName:      application.FQName,
		ProjectID:   application.ProjectID,
		ProjectName: application.ProjectName,
		OrgID:       application.OrgID,
		OrgName:     application.OrgName,
		Workspace:   application.Workspace,
		AppID:       application.AppID,
		SpyName:     application.SpyName,
		ServiceName: application.ServiceName,
		PodIP:       application.PodIP,
	}).Assign(application).FirstOrCreate(&appmetadata.ApplicationMetadata{}).Error
}

func (svc ApplicationMetadataService) Delete(ctx context.Context, name string) error {
	if err := model.ValidateAppName(name); err != nil {
		return err
	}

	tx := svc.db.WithContext(ctx)
	return tx.Where("fq_name = ?", name).Delete(appmetadata.ApplicationMetadata{}).Error
}
