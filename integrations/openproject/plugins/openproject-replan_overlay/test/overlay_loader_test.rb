# Overlay lookup checks that need no OpenProject: plain Ruby and Minitest.
#   ruby -Ilib test/overlay_loader_test.rb
require "minitest/autorun"
require "tmpdir"
require "fileutils"
require "open_project/replan_overlay/overlay_loader"

class OverlayLoaderTest < Minitest::Test
  Loader = OpenProject::ReplanOverlay::OverlayLoader

  def setup
    @tmp = Dir.mktmpdir("replan-overlay-loader")
    @saved = %w[REPLAN_OPENPROJECT_OVERLAY_DIR REPLAN_OPENPROJECT_OVERLAY_PATH].to_h { |key| [key, ENV.delete(key)] }
  end

  def teardown
    FileUtils.remove_entry(@tmp)
    @saved.each { |key, value| ENV[key] = value if value }
  end

  def overlay(project, title = "Task of #{project}")
    { "schema_version" => 2, "openproject_project_id" => project, "work_packages" => [{ "title" => title }] }
  end

  def write(relative, data)
    path = File.join(@tmp, relative)
    FileUtils.mkdir_p(File.dirname(path))
    File.write(path, data.is_a?(String) ? data : JSON.generate(data))
    path
  end

  def titles(state)
    state["work_packages"].map { |row| row["title"] }
  end

  def test_the_single_file_is_shown_to_its_own_project_only
    path = write("one.json", overlay("alpha"))
    own = Loader.new(project_identifier: "alpha", path: path).load
    assert own["available"]
    assert_equal ["Task of alpha"], titles(own)

    other = Loader.new(project_identifier: "beta", path: path).load
    refute other["available"]
    assert_equal "overlay_other_project", other["error"]
    assert_empty other["work_packages"]
    assert_empty other["overlay"]
    # The refusal names neither the other project nor the file.
    refute_includes other["message"], "alpha"
    assert_nil other["path"]
  end

  def test_an_overlay_that_names_no_project_is_not_shown_to_a_project
    path = write("one.json", overlay("").merge("openproject_project_id" => nil))
    state = Loader.new(project_identifier: "alpha", path: path).load
    assert_equal "overlay_project_unknown", state["error"]
    # Without a page project, as in the runtime check, the file is read as it is.
    assert Loader.new(path: path).load["available"]
  end

  def test_a_project_gets_its_own_file_from_the_directory
    write("projects/alpha/openproject_overlay.json", overlay("alpha"))
    write("projects/beta/raw/openproject_overlay.json", overlay("beta"))
    single = write("one.json", overlay("alpha", "From the single file"))
    dir = File.join(@tmp, "projects")

    assert_equal ["Task of alpha"], titles(Loader.new(project_identifier: "alpha", path: single, dir: dir).load)
    assert_equal ["Task of beta"], titles(Loader.new(project_identifier: "beta", path: single, dir: dir).load)
    # A project without a directory falls back to the single file, which is another project's.
    assert_equal "overlay_other_project", Loader.new(project_identifier: "gamma", path: single, dir: dir).load["error"]
  end

  def test_a_file_in_the_wrong_projects_directory_is_refused
    write("projects/beta/openproject_overlay.json", overlay("alpha"))
    state = Loader.new(project_identifier: "beta", dir: File.join(@tmp, "projects")).load
    assert_equal "overlay_other_project", state["error"]
  end

  def test_an_identifier_cannot_leave_the_directory
    write("secret/openproject_overlay.json", overlay("../secret"))
    FileUtils.mkdir_p(File.join(@tmp, "projects"))
    state = Loader.new(project_identifier: "../secret", dir: File.join(@tmp, "projects"), path: File.join(@tmp, "missing.json")).load
    assert_equal "overlay_file_missing", state["error"]
  end

  def test_the_environment_names_the_directory_and_the_file
    write("projects/alpha/openproject_overlay.json", overlay("alpha"))
    ENV["REPLAN_OPENPROJECT_OVERLAY_DIR"] = File.join(@tmp, "projects")
    ENV["REPLAN_OPENPROJECT_OVERLAY_PATH"] = write("one.json", overlay("beta"))
    assert_equal ["Task of alpha"], titles(Loader.new(project_identifier: "alpha").load)
    assert_equal ["Task of beta"], titles(Loader.new(project_identifier: "beta").load)
  end

  def test_missing_invalid_and_unsupported_overlays_are_explained
    assert_equal "overlay_file_missing", Loader.new(project_identifier: "alpha", path: File.join(@tmp, "none.json")).load["error"]
    assert_equal "overlay_invalid_json", Loader.new(project_identifier: "alpha", path: write("bad.json", "{")).load["error"]
    assert_equal "overlay_invalid_shape", Loader.new(project_identifier: "alpha", path: write("shape.json", { "x" => 1 })).load["error"]
    newer = overlay("alpha").merge("schema_version" => 99)
    assert_equal "overlay_unsupported_version", Loader.new(project_identifier: "alpha", path: write("new.json", newer)).load["error"]
  end
end
