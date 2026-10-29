require "date"
require "time"

module OpenProject
  module ReplanOverlay
    class OverlayPresenter
      CLOSED_STATUSES = %w[accepted done cancelled].freeze

      attr_reader :overlay, :work_packages

      def initialize(overlay:, work_packages:)
        @overlay = overlay || {}
        @work_packages = Array(work_packages)
      end

      def schema_version
        Integer(overlay.fetch("schema_version", 1))
      rescue ArgumentError, TypeError
        1
      end

      # Version 2 carries names and the counters' inputs; a version 1
      # overlay still renders, with ids and without those counters.
      def version2?
        schema_version >= 2
      end

      def resource_name(resource_id)
        resource_names.fetch(resource_id.to_s, resource_id.to_s)
      end

      def resource_names
        @resource_names ||= begin
          names = {}
          work_packages.each do |row|
            demands_for(row).each do |demand|
              names[demand["resource_id"].to_s] ||= demand["resource_name"] if demand["resource_name"].to_s != ""
            end
          end
          Array(overlay["resources"]).each do |resource|
            names[resource["id"].to_s] = resource["name"] if resource["name"].to_s != ""
          end
          names
        end
      end

      # Header facts under the page title.
      def header_facts
        facts = []
        facts << "Status date #{date_label(overlay['status_date'])}" if overlay["status_date"].to_s != ""
        facts << "week #{overlay['week']}" if overlay["week"].to_s != ""
        facts << "generated #{generated_label(overlay['generated_at'])}" if overlay["generated_at"].to_s != ""
        facts
      end

      # The five counters of the project page, each answering a question.
      # A counter the overlay cannot answer shows a dash.
      def counters
        [
          counter("waiting", "Waiting on a person", version2? ? person_actions.size : nil,
                  person_actions.map { |action| action_label(action) }),
          counter("overloaded", "Overloaded weeks ahead", version2? ? overloaded_weeks.size : nil,
                  overloaded_weeks.map { |week| overload_label(week) }, alert: overloaded_weeks.any?),
          counter("open", "Open tasks", open_rows.size, []),
          counter("warnings", "Warnings", version2? ? numeric(overlay["warning_count"]).to_i : nil,
                  work_packages.flat_map { |row| Array(row["warnings"]).map { |warning| warning["message"].to_s } }),
          counter("proposals", "Open proposals",
                  version2? ? numeric(overlay["open_proposal_count"]).to_i : work_packages.count { |row| truthy?(row["has_open_proposal"]) },
                  [])
        ]
      end

      def person_actions
        Array(overlay["person_actions"])
      end

      def overloaded_weeks
        Array(overlay["overloaded_weeks"])
      end

      def action_label(action)
        action["label"].to_s
      end

      def overload_label(week)
        "#{week['resource_name'] || resource_name(week['resource_id'])} in #{week['week']}: " \
          "#{number_label(numeric(week['load']))} of #{amount_label(numeric(week['capacity']), week['unit'])}"
      end

      # Open tasks, the soonest finish first, as the attention table lists
      # them: names, a status, the window, resources by name and signals in
      # words.
      def attention_rows
        waiting_on_person = person_actions.map { |action| action["task_id"].to_s }
        open_rows.sort_by { |row| [row["planned_finish"].to_s, row["planned_start"].to_s, row["work_package_id"].to_s] }.map do |row|
          row.merge(
            "window_label" => window_label(row["planned_start"], row["planned_finish"]),
            "resource_labels" => demands_for(row).map { |demand| resource_name(demand["resource_id"]) }.uniq,
            "signals" => signals_for(row, waiting_on_person.include?(row["replan_task_id"].to_s))
          )
        end
      end

      def status_label(status)
        status.to_s.tr("_", " ")
      end

      def window_label(start, finish)
        return "" if start.to_s == "" && finish.to_s == ""
        return date_label(start || finish) if start.to_s == finish.to_s || finish.to_s == ""

        "#{date_label(start)} – #{date_label(finish)}"
      end

      def date_label(value)
        Date.iso8601(value.to_s).strftime("%-d %b %Y")
      rescue ArgumentError, TypeError
        value.to_s
      end

      def work_package_row(work_package_id)
        work_packages.find { |item| item["work_package_id"].to_s == work_package_id.to_s }
      end

      def amount_label(value, unit)
        return "0 #{unit}" if value.zero?

        formatted = value.round(2)
        formatted = formatted.to_i if formatted == formatted.to_i
        "#{formatted} #{unit}"
      end

      private

      def counter(key, label, value, details, alert: false)
        { "key" => key, "label" => label, "value" => value.nil? ? "–" : value.to_s, "alert" => alert, "details" => details }
      end

      def open_rows
        work_packages.reject { |row| closed_status?(row["tracking_status"]) }
      end

      def closed_status?(status)
        CLOSED_STATUSES.include?(status.to_s)
      end

      # Signals in words; version 1 overlays fall back to their counts.
      def signals_for(row, waits_on_person)
        signals = []
        signals << { "text" => "waits on you", "tone" => "attention" } if waits_on_person
        waiting_for = Array(row["dependencies"]).select do |dependency|
          dependency["direction"] == "predecessor" && dependency["status"] == "open" && !closed_status?(dependency["task_status"])
        end
        if row.key?("dependencies")
          if waiting_for.any?
            signals << { "text" => "waits for #{plural(waiting_for.size, 'task')}", "tone" => "neutral",
                         "title" => waiting_for.map { |dependency| dependency["task_title"] }.join("; ") }
          end
        elsif numeric(row["open_dependency_count"]).positive?
          signals << { "text" => "waits for #{plural(numeric(row['open_dependency_count']).to_i, 'dependency')}", "tone" => "neutral" }
        end
        warnings = row.key?("warnings") ? Array(row["warnings"]) : Array(row["warning_codes"]).map { |code| { "message" => code } }
        if warnings.any?
          signals << { "text" => plural(warnings.size, "warning"), "tone" => "attention",
                       "title" => warnings.map { |warning| warning["message"] }.join("; ") }
        end
        proposals = row.key?("open_proposals") ? Array(row["open_proposals"]).size : (truthy?(row["has_open_proposal"]) ? 1 : 0)
        signals << { "text" => plural(proposals, "open proposal"), "tone" => "neutral" } if proposals.positive?
        overloaded = Array(row["overloaded_resource_ids"])
        if overloaded.any?
          signals << { "text" => "overloaded: #{overloaded.map { |id| resource_name(id) }.join(', ')}", "tone" => "danger" }
        end
        signals
      end

      def plural(count, noun)
        "#{count} #{noun}#{count == 1 ? '' : 's'}"
      end

      def number_label(value)
        formatted = value.round(2)
        formatted == formatted.to_i ? formatted.to_i.to_s : formatted.to_s
      end

      def generated_label(value)
        Time.iso8601(value.to_s).strftime("%-d %b %Y %H:%M")
      rescue ArgumentError, TypeError
        value.to_s
      end

      def demands_for(row)
        Array(row["resource_demands"])
      end

      def numeric(value)
        return 0.0 if value.nil? || value == ""

        Float(value)
      rescue ArgumentError, TypeError
        0.0
      end

      def truthy?(value)
        value == true || value.to_s == "true"
      end
    end
  end
end
