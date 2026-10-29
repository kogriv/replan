package app

type Diagnostic struct {
	Code     string `json:"code"`
	Message  string `json:"message"`
	Severity string `json:"severity"`
	Target   Target `json:"target,omitempty"`
}

type Target struct {
	Kind string `json:"kind,omitempty"`
	ID   string `json:"id,omitempty"`
}
