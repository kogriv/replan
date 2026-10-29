require "date"

module OpenProject
  module ReplanOverlay
    # Lays out the plan as a Gantt chart in pixels, from the overlay alone.
    # Every amount and capacity comes from RePlan; this class only places
    # them on an axis (docs/dev/openproject-plugin-redesign-design.md).
    class GanttPresenter
      # Pixels per day for each scale; weeks is the default.
      SCALES = { "days" => 36, "weeks" => 14, "months" => 4 }.freeze
      DEFAULT_SCALE = "weeks"
      DEFAULT_DAYS_BACK = 28
      # Range presets as days before and after the status date; nil means the
      # plan's own edge.
      PRESETS = [
        ["2 weeks", 3, 14], ["Month", 7, 30], ["Quarter", 14, 90], ["Whole project", nil, nil]
      ].freeze
      # A lane is drawn up to about 165% of daily capacity: 100% sits at 60%
      # of the lane height.
      LINE_PERCENT = 60.0
      # The weekly load profile: 100% of capacity sits at 55% of the row, and
      # an overload is drawn as a hump above it, up to 80% more.
      LOAD_LINE_PERCENT = 55.0
      LOAD_HUMP_CAP = 0.8
      # Resource groups of the profile, in this order.
      CLASS_LABELS = [
        %w[human People], %w[role Roles], ["ai_agent", "AI agents"], %w[environment Environments],
        ["service_quota", "Services and quotas"], ["release_window", "Release windows"]
      ].freeze
      CLOSED_STATUSES = %w[accepted done cancelled].freeze
      UNIT_LABELS = {
        "hours" => "h", "agent_hours" => "agent h", "review_hours" => "review h", "ci_minutes" => "CI min",
        "concurrent_slots" => "slots", "release_slots" => "release slots", "tokens" => "tokens"
      }.freeze

      attr_reader :overlay, :from_offset, :to_offset, :scale

      # from and to are optional ISO dates bounding the visible range; scale
      # is days, weeks or months.
      def initialize(overlay:, from: nil, to: nil, scale: nil)
        @overlay = overlay || {}
        @scale = SCALES.key?(scale.to_s) ? scale.to_s : DEFAULT_SCALE
        span = @overlay["span"] || {}
        @start = parse_date(span["start_date"])
        @day_count = span["day_count"].to_i
        @status_offset = span["status_offset_days"].to_i
        @from_offset = clamp_offset(offset_of(from) || [@status_offset - DEFAULT_DAYS_BACK, 0].max)
        @to_offset = clamp_offset(offset_of(to)&.+(1) || @day_count)
        @to_offset = [@from_offset + 7, @day_count].min if @to_offset <= @from_offset
      end

      def available?
        !@start.nil? && @day_count.positive?
      end

      def pixels_per_day
        SCALES.fetch(scale)
      end

      def width
        (to_offset - from_offset) * pixels_per_day
      end

      def status_date
        date_at(@status_offset).iso8601
      end

      def status_left
        x(@status_offset)
      end

      def span_start
        date_at(0).iso8601
      end

      def span_finish
        date_at([@day_count - 1, 0].max).iso8601
      end

      # Preset ranges as from and to dates, with the one in use marked.
      def presets
        PRESETS.map do |label, back, ahead|
          from = back ? [@status_offset - back, 0].max : 0
          to = ahead ? [@status_offset + ahead, @day_count].min : @day_count
          { "label" => label, "from" => date_at(from).iso8601, "to" => date_at(to - 1).iso8601,
            "current" => from == from_offset && to == to_offset }
        end
      end

      def range_label
        "#{date_label(date_at(from_offset))} – #{date_label(date_at(to_offset - 1))}"
      end

      def from_date
        date_at(from_offset).iso8601
      end

      def to_date
        date_at(to_offset - 1).iso8601
      end

      # Axis ticks: the first of every month, labelled with month and year;
      # every day on the days scale, every Monday on the weeks scale, both
      # labelled with the day; nothing more on the months scale.
      def ticks
        (from_offset...to_offset).filter_map do |offset|
          day = date_at(offset)
          if day.day == 1 || (offset == from_offset && day.day < 25)
            { "kind" => "month", "left" => x(offset), "label" => day.strftime(scale == "months" ? "%b" : "%b %Y") }
          elsif scale == "days" || (scale == "weeks" && day.monday?)
            { "kind" => "week", "left" => x(offset), "label" => day.day.to_s }
          end
        end
      end

      # Weekends are shaded where a day is wide enough to see.
      def weekends
        return [] if scale == "months"

        (from_offset...to_offset).filter_map do |offset|
          { "left" => x(offset), "width" => pixels_per_day } if date_at(offset).saturday? || date_at(offset).sunday?
        end
      end

      def mondays
        (from_offset...to_offset).filter_map { |offset| x(offset) if date_at(offset).monday? }
      end

      def status_line
        return nil unless @status_offset.between?(from_offset, to_offset)

        { "left" => x(@status_offset), "label" => "Status date #{date_label(date_at(@status_offset))}" }
      end

      def open_tasks
        tasks.reject { |task| task["closed"] }
      end

      def done_tasks
        tasks.select { |task| task["closed"] }
      end

      def tasks
        @tasks ||= Array(overlay["work_packages"])
                   .select { |row| visible?(row["start_offset_days"], row["duration_days"]) }
                   .sort_by { |row| [row["start_offset_days"].to_i, row["planned_finish"].to_s, row["work_package_id"].to_s] }
                   .map { |row| task(row) }
      end

      # One row per resource the plan uses, grouped by class, with a bar for
      # every visible week that has planned load. The bar's height is the
      # week's planned load over its capacity. Above 100% the hump is marked
      # "over" only where RePlan's workload still finds the week overloaded,
      # the same resource-weeks the overview counts; elsewhere the plan went
      # over but that work is done, and the hump is drawn muted.
      def load_rows
        order = CLASS_LABELS.map(&:first)
        @load_rows ||= Array(overlay["resources"])
                       .each_with_index
                       .sort_by { |resource, index| [order.index(resource["class"].to_s) || order.size, index] }
                       .map { |resource, _index| load_row(resource) }
      end

      def load_line_percent
        LOAD_LINE_PERCENT
      end

      private

      def load_row(resource)
        name = resource["name"].to_s.empty? ? resource["id"].to_s : resource["name"].to_s
        unit = unit_label(resource["unit"])
        sub = [class_label(resource["class"]), unit, resource["concurrent"] ? "busiest day" : nil]
        {
          "resource_id" => resource["id"].to_s,
          "name" => name,
          "sub" => sub.reject { |part| part.to_s.empty? }.join(" · "),
          "weeks" => Array(resource["weeks"]).filter_map { |week| load_week(resource["id"].to_s, name, unit, week) }
        }
      end

      def load_week(resource_id, name, unit, week)
        start = offset_of(week["start_date"])
        finish = offset_of(week["finish_date"])
        planned = number(week["planned"])
        return nil if start.nil? || finish.nil? || !planned.positive?
        return nil unless visible?(start, finish - start + 1)

        capacity = number(week["capacity"])
        ratio = capacity.positive? ? planned / capacity : nil
        still_over = overloads[[resource_id, week["label"].to_s]]
        hump = ratio && ratio > 1.0 ? [ratio - 1.0, LOAD_HUMP_CAP].min * LOAD_LINE_PERCENT : 0
        {
          "label" => week["label"].to_s,
          "box" => box(start, finish - start + 1),
          "past" => finish < @status_offset,
          "capacity_known" => !ratio.nil?,
          "fill_percent" => ratio ? ([ratio, 1.0].min * LOAD_LINE_PERCENT).round(1) : 0,
          "hump_percent" => hump.round(1),
          "over" => hump.positive? && !still_over.nil?,
          "title" => load_title(name, unit, week, start, finish, planned, capacity, ratio, still_over)
        }
      end

      # resource id and ISO week of every resource-week still overloaded.
      def overloads
        @overloads ||= Array(overlay["overloaded_weeks"]).to_h do |row|
          [[row["resource_id"].to_s, row["week"].to_s], row]
        end
      end

      def load_title(name, unit, week, start, finish, planned, capacity, ratio, still_over)
        tip = "#{name}, #{week['label']} (#{date_label(date_at(start))} – #{date_label(date_at(finish))}): "
        return "#{tip}#{amount_label(planned)} #{unit} planned, no capacity set for this week" unless ratio

        tip += "#{amount_label(planned)} of #{amount_label(capacity)} #{unit} planned, #{(ratio * 100).round}%"
        if still_over
          tip + "; still overloaded: #{amount_label(number(still_over['load']))} #{unit} to do, " \
                "#{amount_label(number(still_over['overload']))} over"
        elsif ratio > 1.0
          tip + "; the plan went over capacity, but no overload remains: that work is done or moved"
        else
          tip
        end
      end

      def class_label(value)
        CLASS_LABELS.to_h.fetch(value.to_s, value.to_s.tr("_", " "))
      end

      def task(row)
        start = row["start_offset_days"].to_i
        duration = [row["duration_days"].to_i, 1].max
        {
          "work_package_id" => row["work_package_id"].to_s,
          "work_package_url" => row["work_package_url"].to_s,
          "task_id" => row["replan_task_id"].to_s,
          "title" => row["title"].to_s,
          "status" => row["tracking_status"].to_s.tr("_", " "),
          "closed" => CLOSED_STATUSES.include?(row["tracking_status"].to_s),
          "window" => window_label(row["planned_start"], row["planned_finish"]),
          "bar" => box(start, duration),
          "lanes" => Array(row["resource_demands"]).filter_map { |demand| lane(demand) }
        }
      end

      def lane(demand)
        return nil if demand["start_date"].to_s == ""

        start = demand["start_offset_days"].to_i
        duration = [demand["duration_days"].to_i, 1].max
        return nil unless visible?(start, duration)

        per_day = number(demand["amount_per_day"])
        capacity = number(demand["daily_capacity"])
        ratio = capacity.positive? ? per_day / capacity : nil
        name = demand["resource_name"].to_s.empty? ? demand["resource_id"].to_s : demand["resource_name"].to_s
        amount = "#{amount_label(number(demand['planned_amount']))} #{unit_label(demand['unit'])}"
        tip = "#{name}: #{amount} over #{duration} #{duration == 1 ? 'day' : 'days'}"
        tip += ratio ? ", #{(ratio * 100).round}% of daily capacity" : ", capacity unknown"
        {
          "name" => name,
          "resource_id" => demand["resource_id"].to_s,
          "amount" => amount,
          "box" => box(start, duration),
          "capacity_known" => !ratio.nil?,
          "fill_percent" => ratio ? ([ratio, 100.0 / LINE_PERCENT].min * LINE_PERCENT).round(1) : 0,
          "over" => !ratio.nil? && ratio > 1.0,
          "line_percent" => LINE_PERCENT,
          "title" => tip
        }
      end

      def visible?(start, duration)
        start = start.to_i
        start + [duration.to_i, 1].max > from_offset && start < to_offset
      end

      # A box clipped to the visible range, at least three pixels wide.
      def box(start, duration)
        left = [start, from_offset].max
        right = [start + duration, to_offset].min
        { "left" => x(left), "width" => [x(right) - x(left), 3].max }
      end

      def x(offset)
        (offset - from_offset) * pixels_per_day
      end

      def date_at(offset)
        @start + offset
      end

      def offset_of(value)
        date = parse_date(value)
        date && @start ? (date - @start).to_i : nil
      end

      def clamp_offset(offset)
        offset.clamp(0, [@day_count, 0].max)
      end

      def parse_date(value)
        Date.iso8601(value.to_s)
      rescue ArgumentError, TypeError
        nil
      end

      def window_label(start, finish)
        start_date = parse_date(start)
        finish_date = parse_date(finish)
        return "" unless start_date

        return date_label(start_date) if finish_date.nil? || finish_date == start_date

        "#{date_label(start_date)} – #{date_label(finish_date)}"
      end

      def date_label(date)
        date.strftime("%-d %b %Y")
      end

      def unit_label(unit)
        UNIT_LABELS.fetch(unit.to_s, unit.to_s)
      end

      def amount_label(value)
        rounded = value.round(2)
        rounded == rounded.to_i ? rounded.to_i.to_s : rounded.to_s
      end

      def number(value)
        Float(value)
      rescue ArgumentError, TypeError
        0.0
      end
    end
  end
end
