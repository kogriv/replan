package app

import (
	"regexp"
	"strings"

	"github.com/kogriv/replan/internal/displayurl"
	"github.com/kogriv/replan/internal/domain"
)

var workPackageID = regexp.MustCompile(`^[1-9][0-9]*$`)

// reportOpenProjectBase reads only configured bases. A binding layer can
// point at a different installation from the public workspace profile.
func reportOpenProjectBase(ws domain.Workspace) string {
	if ws.OpenProjectBindingBaseURL != "" {
		return ws.OpenProjectBindingBaseURL
	}
	for _, profile := range ws.Project.SourceProfiles {
		if profile.Source == "openproject" && profile.BaseURL != "" {
			return profile.BaseURL
		}
	}
	return ""
}

func reportSourceURL(ref domain.SourceRef, openProjectBase string) string {
	if ref.Source != "openproject" {
		return displayurl.Normalize(ref.URL)
	}
	if ref.ObjectType != "work_package" || !workPackageID.MatchString(ref.ObjectID) {
		return ""
	}
	base := displayurl.Normalize(openProjectBase)
	if base == "" || strings.ContainsAny(base, "?#") {
		return ""
	}
	return displayurl.NormalizeWorkPackage(strings.TrimRight(base, "/") + "/work_packages/" + ref.ObjectID)
}

func (v *workspaceValidator) sourceRefURLs(kind, id string, refs ...domain.SourceRef) {
	for _, ref := range refs {
		if ref.URL == "" || displayurl.Normalize(ref.URL) != "" || openProjectRelativeEvidence(ref) {
			continue
		}
		v.warn("invalid_source_ref_url", "source_ref URL must be an absolute HTTP(S) URL or a recognized OpenProject API reference", kind, id)
	}
}

func openProjectRelativeEvidence(ref domain.SourceRef) bool {
	if ref.Source != "openproject" || !workPackageID.MatchString(ref.ObjectID) {
		return false
	}
	var collection string
	switch ref.ObjectType {
	case "work_package", "relation", "user", "time_entry", "project":
		collection = ref.ObjectType + "s"
	default:
		return false
	}
	// HAL self links may include the installation's subpath. Validate their
	// syntax against a placeholder origin without resolving them for display.
	path := strings.Trim(ref.URL, " ")
	return strings.HasPrefix(path, "/") && !strings.HasPrefix(path, "//") &&
		displayurl.Normalize("https://example.invalid"+path) != "" &&
		!strings.ContainsAny(path, "?#") &&
		strings.HasSuffix(path, "/api/v3/"+collection+"/"+ref.ObjectID)
}
