# Run from the plugin directory: ruby -Ilib test/display_url_test.rb
require "cgi"
require "erb"
require "minitest/autorun"
require "tempfile"
require "open_project/replan_overlay/display_url"
require "open_project/replan_overlay/overlay_loader"

class DisplayURLTest < Minitest::Test
  DisplayURL = OpenProject::ReplanOverlay::DisplayURL
  Loader = OpenProject::ReplanOverlay::OverlayLoader
  LINK_TEMPLATE = File.expand_path("../app/views/open_project/replan_overlay/overlays/_source_link.html.erb", __dir__)

  def render_link(url, label)
    ERB.new(File.read(LINK_TEMPLATE)).result(binding).strip
  end

  def h(value)
    CGI.escapeHTML(value)
  end

  def test_normalizes_supported_urls
    {
      "  HtTpS://Example.test:8443/pm/work_packages/42?x=a%20b&y=2#top  " =>
        "https://Example.test:8443/pm/work_packages/42?x=a%20b&y=2#top",
      "http://localhost/work_packages/42" => "http://localhost/work_packages/42",
      "https://[2001:db8::1]:443/" => "https://[2001:db8::1]:443/",
      "http://192.0.2.3/" => "http://192.0.2.3/"
    }.each do |raw, expected|
      assert_equal expected, DisplayURL.normalize(raw)
    end
  end

  def test_rejects_unsupported_or_malformed_urls
    [
      "javascript:", "data:", "//example.test/work_packages/42", "/api/v3/work_packages/42",
      "https:example.test", "https:///path", "https://user@example.test/", "https://example.test/a b",
      "\thttps://example.test/", "https://example.test/\n", "https://example.test/a\u00a0b",
      "https://example.test/a\x7fb", "https://example.test:65536/", "https://[not-ip]/",
      "https://[192.0.2.3]/", "https://a..test/", "https://-example.test/", "https://300.0.0.1/",
      "https://example.test/a%xx", "https://example.test/a\\b"
    ].each do |raw|
      assert_nil DisplayURL.normalize(raw), raw.inspect
    end
  end

  def test_renders_safe_links_and_plain_labels_from_legacy_overlay_json
    Tempfile.create(["replan-display-url", ".json"]) do |file|
      urls = [
        "  HTTPS://example.test/pm/work_packages/42?a=1&b=2  ",
        "/api/v3/work_packages/42", "https://example.test/pm/api/v3/work_packages/42",
        "javascript:", "https://a..test/",
        {"url" => "https://example.test/"}, ["https://example.test/"], 123, nil
      ]
      rows = urls.map { |url| {"work_package_id" => "42", "work_package_url" => url} }
      rows << {"work_package_id" => "42"}
      file.write(JSON.generate("schema_version" => 1, "openproject_project_id" => "sample", "work_packages" => rows))
      file.flush

      state = Loader.new(project_identifier: "sample", path: file.path).load
      assert state["available"]
      first, *fallbacks = state.fetch("work_packages")
      assert_equal '<a href="https://example.test/pm/work_packages/42?a=1&amp;b=2">#42&lt;&amp;</a>',
                   render_link(first["work_package_url"], "#42<&")
      fallbacks.each do |row|
        assert_equal "#42&lt;&amp;", render_link(row["work_package_url"], "#42<&")
        assert_equal "Open #42&lt;&amp; in OpenProject", render_link(row["work_package_url"], "Open #42<& in OpenProject")
      end
    end
  end
end
