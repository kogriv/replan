require "json"

module OpenProject
  module ReplanOverlay
    # Finds and reads the RePlan overlay of one OpenProject project.
    #
    # A server may hold an overlay per project, in a directory:
    #
    #   <REPLAN_OPENPROJECT_OVERLAY_DIR>/<project identifier>/openproject_overlay.json
    #   <REPLAN_OPENPROJECT_OVERLAY_DIR>/<project identifier>/raw/openproject_overlay.json
    #
    # and, as before, one overlay file named by REPLAN_OPENPROJECT_OVERLAY_PATH.
    # Either way an overlay is shown only to the project it names in
    # `openproject_project_id`: a project never sees another project's plan.
    class OverlayLoader
      DEFAULT_RELATIVE_PATH = "public/replan/openproject_overlay.json"
      OVERLAY_FILE = "openproject_overlay.json"
      # Overlay versions this plugin reads. Version 2 keeps every version 1
      # field, so the current views read both.
      SUPPORTED_SCHEMA_VERSIONS = [1, 2].freeze
      # Identifiers go into a file path; anything else is not looked up.
      IDENTIFIER = /\A[a-z0-9][a-z0-9_-]*\z/i

      attr_reader :path, :project_identifier

      # project_identifier is the page's project; without it the single file
      # is read as it is, for checks that have no project.
      def initialize(project_identifier: nil, path: nil, dir: nil, root: nil)
        @project_identifier = project_identifier.to_s
        @dir = first_present(dir, ENV["REPLAN_OPENPROJECT_OVERLAY_DIR"])
        @path = project_file || first_present(path, ENV["REPLAN_OPENPROJECT_OVERLAY_PATH"]) || default_path(root)
      end

      def load
        return empty("overlay_path_missing", "RePlan overlay path is not configured.") if path.to_s.empty?
        return empty("overlay_file_missing", "RePlan overlay file was not found.", path: path) unless File.file?(path)

        parsed = JSON.parse(File.read(path))
        return empty("overlay_invalid_shape", "RePlan overlay JSON must contain a work_packages array.", path:) unless parsed.is_a?(Hash) && parsed["work_packages"].is_a?(Array)

        version = parsed.fetch("schema_version", 1)
        unless SUPPORTED_SCHEMA_VERSIONS.include?(version)
          return empty("overlay_unsupported_version",
                       "RePlan overlay schema version #{version} is not supported by this plugin, which reads versions " \
                       "#{SUPPORTED_SCHEMA_VERSIONS.join(' and ')}. Update the plugin or generate the overlay with a matching RePlan.",
                       path:)
        end

        mismatch = other_project(parsed)
        return mismatch if mismatch

        {
          "available" => true,
          "error" => nil,
          "message" => nil,
          "path" => path,
          "overlay" => parsed,
          "work_packages" => parsed["work_packages"]
        }
      rescue JSON::ParserError => e
        empty("overlay_invalid_json", "RePlan overlay JSON could not be parsed: #{e.message}", path: path)
      rescue StandardError => e
        empty("overlay_load_failed", "RePlan overlay could not be loaded: #{e.class}: #{e.message}", path: path)
      end

      private

      # The project's own file in the overlay directory, when there is one.
      def project_file
        return nil if @dir.nil? || !project_identifier.match?(IDENTIFIER)

        [File.join(@dir, project_identifier, OVERLAY_FILE), File.join(@dir, project_identifier, "raw", OVERLAY_FILE)]
          .find { |candidate| File.file?(candidate) }
      end

      # An overlay of another project, or of no named project, is not shown.
      # Neither message names the other project or the file: the reader may
      # have no right to know either.
      def other_project(parsed)
        return nil if project_identifier.empty?

        owner = parsed["openproject_project_id"].to_s
        return nil if owner == project_identifier

        if owner.empty?
          empty("overlay_project_unknown",
                "The RePlan overlay on this server does not say which OpenProject project it belongs to, so it is not shown. " \
                "Generate it with a RePlan that writes openproject_project_id.",
                path: nil)
        else
          empty("overlay_other_project",
                "This project has no RePlan overlay yet. The overlay on this server belongs to another project and is not shown here.",
                path: nil)
        end
      end

      def first_present(*values)
        values.map(&:to_s).find { |value| !value.empty? }
      end

      def default_path(root)
        root ||= Rails.root if defined?(Rails)
        root ? File.join(root.to_s, DEFAULT_RELATIVE_PATH) : nil
      end

      def empty(error, message, path: self.path)
        {
          "available" => false,
          "error" => error,
          "message" => message,
          "path" => path,
          "overlay" => {},
          "work_packages" => []
        }
      end
    end
  end
end
