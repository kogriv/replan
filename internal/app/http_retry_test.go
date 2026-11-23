package app

import (
	"errors"
	"net/http"
	"net/url"
	"testing"
)

func TestSameOriginRedirectClientRefusesHTTPSDowngrade(t *testing.T) {
	client := sameOriginRedirectClient(&http.Client{})
	from, err := url.Parse("https://openproject.example/api/v3/work_packages")
	if err != nil {
		t.Fatal(err)
	}
	to, err := url.Parse("http://openproject.example/api/v3/work_packages")
	if err != nil {
		t.Fatal(err)
	}
	err = client.CheckRedirect(
		&http.Request{URL: to},
		[]*http.Request{{URL: from}},
	)
	var refused *redirectRefusedError
	if !errors.As(err, &refused) {
		t.Fatalf("error = %v, want redirectRefusedError", err)
	}
	if refused.fromOrigin != "https://openproject.example:443" || refused.toOrigin != "http://openproject.example:80" {
		t.Fatalf("origins = %q -> %q", refused.fromOrigin, refused.toOrigin)
	}
}

func TestSameOriginRedirectClientAllowsDefaultPortEquivalence(t *testing.T) {
	client := sameOriginRedirectClient(&http.Client{})
	from, _ := url.Parse("https://openproject.example/start")
	to, _ := url.Parse("https://openproject.example:443/end")
	if err := client.CheckRedirect(
		&http.Request{URL: to},
		[]*http.Request{{URL: from}},
	); err != nil {
		t.Fatalf("same-origin redirect refused: %v", err)
	}
}
