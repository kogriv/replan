require_relative "lib/open_project/wiki_api/version"

Gem::Specification.new do |spec|
  spec.name = "openproject-wiki_api"
  spec.version = OpenProject::WikiAPI::VERSION
  spec.authors = ["RePlan contributors"]
  spec.email = ["maintainers@example.invalid"]

  spec.summary = "API v3 collection, text read, create and update for OpenProject wiki pages"
  spec.description = "Extends OpenProject API v3 so an API-token participant can list, read, create and update wiki pages through the upstream WikiPages services and standard wiki permissions."
  spec.homepage = "https://github.com/kogriv/replan"
  spec.license = "Apache-2.0"

  spec.files = Dir["lib/**/*", "README.md"]
  spec.require_paths = ["lib"]
end
