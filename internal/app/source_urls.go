package app

import (
	"net/url"
	"path"
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

func reportSourceURL(ref domain.SourceRef, ws domain.Workspace) string {
	if ref.Source != "openproject" {
		return displayurl.Normalize(ref.URL)
	}
	if ref.ObjectType != "work_package" || !workPackageID.MatchString(ref.ObjectID) {
		return ""
	}
	route := "/work_packages/" + ref.ObjectID
	// The binding layer replaces all canonical OpenProject task references.
	if ws.OpenProjectBindingsLoaded {
		if target := openProjectTaskDisplayURL(ref.URL, route); target != "" {
			return target
		}
	}
	base := displayurl.Normalize(reportOpenProjectBase(ws))
	if base == "" || strings.ContainsAny(base, "?#") {
		return ""
	}
	return openProjectTaskDisplayURL(strings.TrimRight(base, "/")+route, route)
}

func openProjectTaskDisplayURL(raw, route string) string {
	target := displayurl.NormalizeWorkPackage(raw)
	parsed, err := url.Parse(target)
	if err != nil || !strings.HasSuffix(strings.TrimSuffix(parsed.EscapedPath(), "/"), route) {
		return ""
	}
	// Check the decoded path as well so escaped segments cannot hide an API route.
	if strings.Contains(path.Clean(parsed.Path), "/api/v3/work_packages/") {
		return ""
	}
	return target
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
	case "work_package", "relation", "user", "project":
		collection = ref.ObjectType + "s"
	case "time_entry":
		collection = "time_entries"
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
