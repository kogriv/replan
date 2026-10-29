require_relative "lib/open_project/replan_overlay/version"

Gem::Specification.new do |spec|
  spec.name = "openproject-replan_overlay"
  spec.version = OpenProject::ReplanOverlay::VERSION
  spec.authors = ["RePlan contributors"]
  spec.email = ["maintainers@example.invalid"]

  spec.summary = "Read-only OpenProject renderer for RePlan overlay artifacts"
  spec.description = "Renders RePlan openproject_overlay.json data inside OpenProject without owning RePlan planning state."
  spec.homepage = "https://github.com/kogriv/replan"
  spec.license = "Apache-2.0"

  spec.files = Dir["app/**/*", "config/**/*", "lib/**/*", "README.md"]
  spec.require_paths = ["lib"]
end
