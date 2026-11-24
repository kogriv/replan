package displayurl

import "testing"

func TestNormalize(t *testing.T) {
	for _, tc := range []struct{ raw, want string }{
		{"  HtTpS://Example.test:8443/pm/work_packages/42?x=a%20b&y=2#top  ", "https://Example.test:8443/pm/work_packages/42?x=a%20b&y=2#top"},
		{"http://localhost/work_packages/42", "http://localhost/work_packages/42"},
		{"https://[2001:db8::1]:443/", "https://[2001:db8::1]:443/"},
		{"http://192.0.2.3/", "http://192.0.2.3/"},
		{"javascript:", ""}, {"data:", ""}, {"//example.test/work_packages/42", ""},
		{"/api/v3/work_packages/42", ""}, {"https:example.test", ""}, {"https:///path", ""},
		{"https://user@example.test/", ""}, {"https://example.test/a b", ""},
		{"\thttps://example.test/", ""}, {"https://example.test/\n", ""},
		{"https://example.test/a\u00a0b", ""}, {"https://example.test/a\x7fb", ""},
		{"https://example.test:65536/", ""}, {"https://[not-ip]/", ""},
		{"https://[192.0.2.3]/", ""}, {"https://a..test/", ""},
		{"https://-example.test/", ""}, {"https://300.0.0.1/", ""},
		{"https://example.test/a%xx", ""}, {"https://example.test/a\\b", ""},
	} {
		if got := Normalize(tc.raw); got != tc.want {
			t.Errorf("Normalize(%q) = %q, want %q", tc.raw, got, tc.want)
		}
	}
}
