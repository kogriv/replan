require "date"
require "open_project/replan_overlay/gantt_presenter"

module OpenProject
  module ReplanOverlay
    # One task's page from the overlay alone: what was done, time booked,
    # what it waits on and what waits on it, and the task with its resource
    # lanes on a Gantt axis of its own (docs/dev/openproject-plugin-redesign-design.md).
    class TaskPresenter
      # Days of context before and after the task on its own axis.
      MARGIN_DAYS = 7
      # Up to this many days the axis shows days; beyond it, weeks.
      DAYS_SCALE_LIMIT = 45

      attr_reader :row

      def initialize(overlay:, row:)
        @overlay = overlay || {}
        @row = row || {}
      end

      # Evidence, actuals and dependencies arrive with overlay version 2.
      def detailed?
        row.key?("evidence")
      end

      def title
        row["title"].to_s
      end

      # Status, planned window and when time was booked, in words.
      def facts
        facts = [row["tracking_status"].to_s.tr("_", " ")]
        window = window_label(row["planned_start"], row["planned_finish"])
        facts << "planned #{window}" unless window.empty?
        booked = window_label(row["first_actual_date"], row["last_actual_date"])
        facts << "time booked #{booked}" unless booked.empty?
        facts.reject(&:empty?)
      end

      # The task and its lanes on an axis around the task window.
      def gantt
        @gantt ||= begin
          start = parse_date(row["planned_start"])
          finish = parse_date(row["planned_finish"]) || start
          days = start && finish ? (finish - start).to_i + 1 + (2 * MARGIN_DAYS) : 0
          GanttPresenter.new(
            overlay: @overlay.merge("work_packages" => [row]),
            from: start && (start - MARGIN_DAYS).iso8601,
            to: finish && (finish + MARGIN_DAYS).iso8601,
            scale: days <= DAYS_SCALE_LIMIT ? "days" : "weeks"
          )
        end
      end

      # Warnings and open proposals in words, and overloaded resources.
      def signals
        signals = Array(row["warnings"]).map { |warning| { "tone" => "attention", "text" => warning["message"].to_s } }
        signals += Array(row["open_proposals"]).map do |proposal|
          text = proposal["rationale"].to_s.empty? ? proposal["id"].to_s : proposal["rationale"].to_s
          { "tone" => "neutral", "text" => "Open proposal: #{text}", "title" => proposal["id"].to_s }
        end
        overloaded = Array(row["overloaded_resource_ids"]).map { |id| resource_name(id) }
        signals << { "tone" => "danger", "text" => "Overloaded: #{overloaded.join(', ')}" } if overloaded.any?
        signals
      end

      # What was done, newest first.
      def evidence
        Array(row["evidence"]).each_with_index
                              .sort_by { |item, index| [item["date"].to_s, index] }
                              .reverse
                              .map do |item, _index|
          { "date" => date_label(item["date"]), "kind" => item["kind"].to_s.tr("_", " "),
            "summary" => item["summary"].to_s, "id" => item["id"].to_s }
        end
      end

      # Time booked, newest first.
      def actuals
        Array(row["actuals"]).each_with_index
                             .sort_by { |item, index| [item["date"].to_s, index] }
                             .reverse
                             .map do |item, _index|
          { "date" => date_label(item["date"]),
            "resource" => item["resource_name"].to_s.empty? ? resource_name(item["resource_id"]) : item["resource_name"].to_s,
            "amount" => amount(item["amount"], item["unit"]), "demand_id" => item["demand_id"].to_s }
        end
      end

      # Planned, booked and remaining by resource demand.
      def demands
        Array(row["resource_demands"]).map do |demand|
          unit = demand["unit"]
          {
            "resource" => demand["resource_name"].to_s.empty? ? resource_name(demand["resource_id"]) : demand["resource_name"].to_s,
            "resource_id" => demand["resource_id"].to_s,
            "demand_id" => demand["demand_id"].to_s,
            "planned" => amount(demand["planned_amount"], unit),
            "actual" => amount(demand["actual_amount"], unit),
            "remaining" => amount(demand["remaining_amount"], unit),
            "window" => window_label(demand["start_date"], demand["finish_date"])
          }
        end
      end

      # What the task waits on.
      def predecessors
        dependencies("predecessor")
      end

      # What waits on the task.
      def successors
        dependencies("successor")
      end

      private

      def dependencies(direction)
        Array(row["dependencies"]).select { |dependency| dependency["direction"] == direction }.map do |dependency|
          {
            "work_package_id" => dependency["work_package_id"].to_s,
            "title" => dependency["task_title"].to_s.empty? ? dependency["task_id"].to_s : dependency["task_title"].to_s,
            "task_id" => dependency["task_id"].to_s,
            "task_status" => dependency["task_status"].to_s.tr("_", " "),
            "status" => dependency["status"].to_s,
            "criticality" => dependency["criticality"].to_s
          }
        end
      end

      def resource_name(resource_id)
        resource = Array(@overlay["resources"]).find { |item| item["id"].to_s == resource_id.to_s }
        resource && !resource["name"].to_s.empty? ? resource["name"].to_s : resource_id.to_s
      end

      def amount(value, unit)
        number = Float(value || 0)
        rounded = number.round(2)
        rounded = rounded.to_i if rounded == rounded.to_i
        "#{rounded} #{GanttPresenter::UNIT_LABELS.fetch(unit.to_s, unit.to_s)}".strip
      rescue ArgumentError, TypeError
        "0 #{GanttPresenter::UNIT_LABELS.fetch(unit.to_s, unit.to_s)}".strip
      end

      def window_label(start, finish)
        start_date = parse_date(start)
        finish_date = parse_date(finish)
        return "" unless start_date
        return date_label(start) if finish_date.nil? || finish_date == start_date

        "#{date_label(start)} – #{date_label(finish)}"
      end

      def date_label(value)
        date = parse_date(value)
        date ? date.strftime("%-d %b %Y") : value.to_s
      end

      def parse_date(value)
        Date.iso8601(value.to_s)
      rescue ArgumentError, TypeError
        nil
      end
    end
  end
end
