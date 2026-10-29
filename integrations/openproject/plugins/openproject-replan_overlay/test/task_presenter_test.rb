# Task page checks that need no OpenProject: plain Ruby and Minitest.
#   ruby -Ilib test/task_presenter_test.rb
require "minitest/autorun"
require "open_project/replan_overlay/task_presenter"

class TaskPresenterTest < Minitest::Test
  # The span starts on Monday 2026-08-03 and runs 70 days; the task runs
  # from day 42, Monday 2026-09-14, to Wednesday 2026-09-16.
  def overlay
    { "span" => { "start_date" => "2026-08-03", "day_count" => 70, "status_offset_days" => 42 },
      "resources" => [{ "id" => "resource.person", "name" => "Person" }] }
  end

  def row(extra = {})
    { "work_package_id" => "7", "replan_task_id" => "task.seven", "title" => "Seven", "tracking_status" => "review_ready",
      "planned_start" => "2026-09-14", "planned_finish" => "2026-09-16", "start_offset_days" => 42, "duration_days" => 3,
      "first_actual_date" => "2026-09-15", "last_actual_date" => "2026-09-16",
      "resource_demands" => [
        { "demand_id" => "demand.person", "resource_id" => "resource.person", "unit" => "hours", "planned_amount" => 6,
          "actual_amount" => 4.5, "remaining_amount" => 1.5, "start_date" => "2026-09-14", "finish_date" => "2026-09-16",
          "start_offset_days" => 42, "duration_days" => 3, "amount_per_day" => 2, "daily_capacity" => 4 }
      ],
      "evidence" => [
        { "id" => "evidence.a", "date" => "2026-09-15", "kind" => "implementation_completed", "summary" => "Built." },
        { "id" => "evidence.b", "date" => "2026-09-16", "kind" => "operator_acceptance", "summary" => "Accepted." }
      ],
      "actuals" => [
        { "date" => "2026-09-15", "resource_id" => "resource.person", "amount" => 3, "unit" => "hours" },
        { "date" => "2026-09-16", "resource_id" => "resource.person", "resource_name" => "Person", "amount" => 1.5, "unit" => "hours" }
      ],
      "warnings" => [{ "code" => "x", "message" => "Demand has no capacity." }],
      "open_proposals" => [{ "id" => "proposal.p", "rationale" => "Move it a week." }],
      "overloaded_resource_ids" => ["resource.person"],
      "dependencies" => [
        { "direction" => "predecessor", "task_id" => "task.six", "task_title" => "Six", "work_package_id" => "6",
          "task_status" => "accepted", "status" => "resolved", "criticality" => "required" },
        { "direction" => "successor", "task_id" => "task.eight", "task_title" => "", "work_package_id" => "",
          "task_status" => "not_started", "status" => "open", "criticality" => "preferred" }
      ] }.merge(extra)
  end

  def task(extra = {})
    OpenProject::ReplanOverlay::TaskPresenter.new(overlay: overlay, row: row(extra))
  end

  def test_facts_name_status_window_and_booked_dates
    assert_equal ["review ready", "planned 14 Sep 2026 – 16 Sep 2026", "time booked 15 Sep 2026 – 16 Sep 2026"], task.facts
    assert_equal ["not started"], task("tracking_status" => "not_started", "planned_start" => nil, "first_actual_date" => nil).facts
  end

  def test_evidence_and_actuals_are_newest_first_with_names_and_units
    assert_equal ["16 Sep 2026", "operator acceptance", "Accepted."], task.evidence.first.values_at("date", "kind", "summary")
    assert_equal [["16 Sep 2026", "Person", "1.5 h"], ["15 Sep 2026", "Person", "3 h"]],
                 task.actuals.map { |actual| actual.values_at("date", "resource", "amount") }
  end

  def test_demands_show_planned_booked_and_remaining
    assert_equal [["Person", "6 h", "4.5 h", "1.5 h", "14 Sep 2026 – 16 Sep 2026"]],
                 task.demands.map { |demand| demand.values_at("resource", "planned", "actual", "remaining", "window") }
  end

  def test_signals_are_words
    assert_equal ["Demand has no capacity.", "Open proposal: Move it a week.", "Overloaded: Person"], task.signals.map { |signal| signal["text"] }
  end

  def test_dependencies_by_direction_fall_back_to_the_task_id
    assert_equal [%w[6 Six resolved]], task.predecessors.map { |dependency| dependency.values_at("work_package_id", "title", "status") }
    assert_equal [["", "task.eight", "open"]], task.successors.map { |dependency| dependency.values_at("work_package_id", "title", "status") }
  end

  # A week of context on both sides, in days while that fits.
  def test_the_task_gets_its_own_axis_around_its_window
    gantt = task.gantt
    assert_equal "days", gantt.scale
    assert_equal ["2026-09-07", "2026-09-23"], [gantt.from_date, gantt.to_date]
    assert_equal ["7"], gantt.tasks.map { |item| item["work_package_id"] }
    assert_equal 1, gantt.tasks.first["lanes"].size
    long = task("planned_start" => "2026-08-03", "planned_finish" => "2026-10-11", "start_offset_days" => 0, "duration_days" => 70)
    assert_equal "weeks", long.gantt.scale
  end

  def test_a_version_1_row_is_not_detailed
    refute OpenProject::ReplanOverlay::TaskPresenter.new(overlay: overlay, row: { "title" => "Old" }).detailed?
    assert task.detailed?
  end
end
