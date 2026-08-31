package metadata

import (
	"context"
	"strings"

	metadatasdk "github.com/domainry/domainry-metadata-sdk"
	metadatadomain "github.com/domainry/domainry-metadata/internal/domain/metadata/service"
)

type localizationRepository interface {
	ListLocalizedTexts(context.Context, metadatasdk.LocalizedTextQuery) ([]metadatasdk.LocalizedText, error)
	ProjectionName(context.Context) (string, error)
}

type LocalizationApplicationService struct {
	repository  localizationRepository
	definitions metadatasdk.Definitions
}

func NewLocalizationApplicationService(repository localizationRepository, definitions metadatasdk.Definitions) *LocalizationApplicationService {
	return &LocalizationApplicationService{repository: repository, definitions: definitions}
}

func (s *LocalizationApplicationService) List(ctx context.Context, query metadatasdk.LocalizedTextQuery) ([]metadatasdk.LocalizedText, error) {
	return s.repository.ListLocalizedTexts(ctx, query)
}

func (s *LocalizationApplicationService) Coverage(ctx context.Context, query metadatasdk.LocalizedTextCoverageQuery) (metadatasdk.LocalizedTextCoverage, error) {
	query.WorkspaceID = strings.TrimSpace(query.WorkspaceID)
	query.Locale = strings.TrimSpace(query.Locale)
	query.FallbackLocale = strings.TrimSpace(query.FallbackLocale)
	if query.WorkspaceID == "" {
		return metadatasdk.LocalizedTextCoverage{}, &metadatasdk.Error{StatusCode: 400, Code: "metadata.workspace_required"}
	}
	if query.Locale == "" {
		return metadatasdk.LocalizedTextCoverage{}, &metadatasdk.Error{StatusCode: 400, Code: "metadata.localized_texts.locale_required"}
	}
	if query.FallbackLocale == query.Locale {
		query.FallbackLocale = ""
	}
	values, err := s.repository.ListLocalizedTexts(ctx, metadatasdk.LocalizedTextQuery{WorkspaceID: query.WorkspaceID})
	if err != nil {
		return metadatasdk.LocalizedTextCoverage{}, err
	}
	snapshot, err := s.definitions.Snapshot(ctx)
	if err != nil {
		return metadatasdk.LocalizedTextCoverage{}, err
	}
	name, err := s.repository.ProjectionName(ctx)
	if err != nil {
		return metadatasdk.LocalizedTextCoverage{}, err
	}
	return metadatadomain.LocalizedTextCoverage(query, name, snapshot.Definitions, values), nil
}

var _ metadatasdk.Localization = (*LocalizationApplicationService)(nil)
