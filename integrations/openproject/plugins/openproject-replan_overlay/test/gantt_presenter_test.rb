# Gantt layout checks that need no OpenProject: plain Ruby and Minitest.
#   ruby -Ilib test/gantt_presenter_test.rb
require "minitest/autorun"
require "open_project/replan_overlay/gantt_presenter"

class GanttPresenterTest < Minitest::Test
  # The span starts on Monday 2026-08-03 and runs 70 days; the status date
  # is day 42, Monday 2026-09-14.
  def overlay(work_packages)
    { "span" => { "start_date" => "2026-08-03", "day_count" => 70, "status_offset_days" => 42 },
      "work_packages" => work_packages }
  end

  def row(id, start, duration, status: "not_started", demands: [])
    { "work_package_id" => id, "replan_task_id" => "task.#{id}", "title" => "Task #{id}", "tracking_status" => status,
      "planned_start" => "2026-08-03", "planned_finish" => "2026-08-03",
      "start_offset_days" => start, "duration_days" => duration, "resource_demands" => demands }
  end

  def demand(start, duration, per_day:, capacity:)
    { "resource_id" => "resource.person", "resource_name" => "Person", "unit" => "hours", "planned_amount" => per_day * duration,
      "start_date" => "2026-08-03", "start_offset_days" => start, "duration_days" => duration,
      "amount_per_day" => per_day, "daily_capacity" => capacity }
  end

  def gantt(rows, **range)
    OpenProject::ReplanOverlay::GanttPresenter.new(overlay: overlay(rows), **range)
  end

  def test_default_range_starts_four_weeks_before_the_status_date
    g = gantt([])
    assert_equal 14, g.from_offset
    assert_equal 70, g.to_offset
    assert_equal 56 * 14, g.width
    assert_equal({ "left" => 28 * 14, "label" => "Status date 14 Sep 2026" }, g.status_line)
  end

  def test_rows_outside_the_range_are_not_rendered_and_edges_are_clipped
    g = gantt([row("old", 0, 5), row("edge", 10, 10), row("late", 60, 20)])
    assert_equal %w[edge late], g.tasks.map { |task| task["work_package_id"] }
    edge, late = g.tasks
    assert_equal({ "left" => 0, "width" => 6 * 14 }, edge["bar"])
    assert_equal({ "left" => 46 * 14, "width" => 10 * 14 }, late["bar"])
  end

  def test_an_explicit_range_is_honoured_and_kept_inside_the_span
    g = gantt([], from: "2026-08-10", to: "2026-08-23")
    assert_equal [7, 21], [g.from_offset, g.to_offset]
    assert_equal "10 Aug 2026 – 23 Aug 2026", g.range_label
    wide = gantt([], from: "2026-01-01", to: "2027-01-01")
    assert_equal [0, 70], [wide.from_offset, wide.to_offset]
  end

  def test_lane_height_puts_100_percent_at_60_percent_and_caps_overload
    lanes = gantt([row("a", 42, 5, demands: [
      demand(42, 5, per_day: 1, capacity: 2),
      demand(42, 5, per_day: 3, capacity: 2),
      demand(42, 5, per_day: 10, capacity: 2),
      demand(42, 5, per_day: 1, capacity: 0)
    ])]).tasks.first["lanes"]
    assert_equal [30.0, 90.0, 100.0, 0], lanes.map { |lane| lane["fill_percent"] }
    assert_equal [false, true, true, false], lanes.map { |lane| lane["over"] }
    assert_equal 60.0, lanes.first["line_percent"]
    refute lanes.last["capacity_known"]
    assert_equal "Person: 15 h over 5 days, 150% of daily capacity", lanes[1]["title"]
  end

  def test_closed_tasks_are_grouped_as_done
    g = gantt([row("open", 40, 3), row("closed", 40, 3, status: "accepted")])
    assert_equal ["open"], g.open_tasks.map { |task| task["work_package_id"] }
    assert_equal ["closed"], g.done_tasks.map { |task| task["work_package_id"] }
  end

  # A range that starts before the 25th names its month at the start.
  def test_axis_labels_months_and_mondays
    ticks = gantt([], from: "2026-08-24", to: "2026-09-13").ticks
    assert_equal [["month", "Aug 2026"], ["week", "31"], ["month", "Sep 2026"], ["week", "7"]], ticks.map { |tick| [tick["kind"], tick["label"]] }
  end

  def test_scales_change_pixels_per_day_ticks_and_weekends
    days = gantt([], scale: "days", from: "2026-08-24", to: "2026-08-30")
    assert_equal 36, days.pixels_per_day
    assert_equal 7 * 36, days.width
    assert_equal 7, days.ticks.size
    assert_equal 2, days.weekends.size
    months = gantt([], scale: "months")
    assert_equal 4, months.pixels_per_day
    assert_equal [%w[month Aug], %w[month Sep], %w[month Oct]], months.ticks.map { |tick| [tick["kind"], tick["label"]] }
    assert_empty months.weekends
    assert_equal "weeks", gantt([], scale: "fortnights").scale
  end

  def test_presets_bound_the_range_around_the_status_date_and_mark_the_current_one
    presets = gantt([]).presets.to_h { |preset| [preset["label"], preset] }
    assert_equal %w[2026-09-11 2026-09-27], presets["2 weeks"].values_at("from", "to")
    assert_equal %w[2026-08-03 2026-10-11], presets["Whole project"].values_at("from", "to")
    refute presets["Whole project"]["current"]
    whole = gantt([], from: "2026-08-03", to: "2026-10-11").presets.find { |preset| preset["label"] == "Whole project" }
    assert whole["current"]
  end

  def week(label, start, finish, planned:, capacity:)
    { "label" => label, "start_date" => start, "finish_date" => finish, "planned" => planned, "capacity" => capacity }
  end

  # Weeks run Monday to Sunday; the status date, Monday 14 Sep, opens 2026-W38.
  def test_load_profile_draws_weeks_against_capacity_and_marks_only_remaining_overload
    person = { "id" => "resource.person", "name" => "Person", "class" => "human", "unit" => "hours", "weeks" => [
      week("2026-W33", "2026-08-10", "2026-08-16", planned: 4, capacity: 10),
      week("2026-W34", "2026-08-17", "2026-08-23", planned: 5, capacity: 10),
      week("2026-W37", "2026-09-07", "2026-09-13", planned: 15, capacity: 10),
      week("2026-W38", "2026-09-14", "2026-09-20", planned: 30, capacity: 10),
      week("2026-W39", "2026-09-21", "2026-09-27", planned: 3, capacity: 10),
      week("2026-W40", "2026-09-28", "2026-10-04", planned: 2, capacity: 0),
      week("2026-W41", "2026-10-05", "2026-10-11", planned: 0, capacity: 10)
    ] }
    slot = { "id" => "resource.slot", "name" => "", "class" => "environment", "unit" => "concurrent_slots", "concurrent" => true, "weeks" => [] }
    agent = { "id" => "resource.agent", "name" => "Agent", "class" => "ai_agent", "unit" => "agent_hours", "weeks" => [] }
    data = overlay([]).merge(
      "resources" => [slot, agent, person],
      "overloaded_weeks" => [{ "resource_id" => "resource.person", "week" => "2026-W38", "load" => 12, "overload" => 2 }]
    )
    rows = OpenProject::ReplanOverlay::GanttPresenter.new(overlay: data).load_rows
    assert_equal ["Person", "Agent", "resource.slot"], rows.map { |row| row["name"] }
    assert_equal ["People · h", "AI agents · agent h", "Environments · slots · busiest day"], rows.map { |row| row["sub"] }
    weeks = rows.first["weeks"]
    assert_equal %w[2026-W34 2026-W37 2026-W38 2026-W39 2026-W40], weeks.map { |w| w["label"] }
    assert_equal [true, true, false, false, false], weeks.map { |w| w["past"] }
    assert_equal [27.5, 55.0, 55.0, 16.5, 0], weeks.map { |w| w["fill_percent"] }
    assert_equal [0, 27.5, 44.0, 0, 0], weeks.map { |w| w["hump_percent"] }
    assert_equal [false, false, true, false, false], weeks.map { |w| w["over"] }
    refute weeks.last["capacity_known"]
    assert_equal({ "left" => 0, "width" => 7 * 14 }, weeks.first["box"])
    assert_equal "Person, 2026-W38 (14 Sep 2026 – 20 Sep 2026): 30 of 10 h planned, 300%; still overloaded: 12 h to do, 2 over",
                 weeks[2]["title"]
    assert_match(/no overload remains/, weeks[1]["title"])
    assert_match(/no capacity set/, weeks[4]["title"])
  end

  def test_a_version_1_overlay_has_no_gantt
    refute OpenProject::ReplanOverlay::GanttPresenter.new(overlay: { "work_packages" => [] }).available?
  end
end
