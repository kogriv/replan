# Presenter checks that need no OpenProject: plain Ruby and Minitest.
#   ruby -Ilib test/overlay_presenter_test.rb
require "minitest/autorun"
require "open_project/replan_overlay/overlay_presenter"

class OverlayPresenterTest < Minitest::Test
  def overlay(version: 2)
    work_packages = [
      { "work_package_id" => "11", "replan_task_id" => "task.late", "title" => "Late", "tracking_status" => "not_started",
        "planned_start" => "2026-10-05", "planned_finish" => "2026-10-09",
        "resource_demands" => [{ "demand_id" => "d.late", "resource_id" => "resource.person", "resource_name" => "Person", "unit" => "hours" }],
        "dependencies" => [{ "direction" => "predecessor", "status" => "open", "task_status" => "in_progress", "task_title" => "Soon" }],
        "warnings" => [{ "code" => "w", "message" => "Something to look at" }], "open_proposals" => [{ "id" => "p" }] },
      { "work_package_id" => "12", "replan_task_id" => "task.soon", "title" => "Soon", "tracking_status" => "in_progress",
        "planned_start" => "2026-09-28", "planned_finish" => "2026-09-29",
        "resource_demands" => [{ "demand_id" => "d.soon", "resource_id" => "resource.slot", "unit" => "concurrent_slots" }],
        "dependencies" => [{ "direction" => "predecessor", "status" => "open", "task_status" => "accepted", "task_title" => "Done" }],
        "warnings" => [], "open_proposals" => [] },
      { "work_package_id" => "9", "replan_task_id" => "task.done", "title" => "Done", "tracking_status" => "accepted",
        "planned_start" => "2026-09-21", "planned_finish" => "2026-09-21", "resource_demands" => [] }
    ]
    data = { "schema_version" => version, "status_date" => "2026-09-28", "week" => "2026-W40",
             "generated_at" => "2026-09-28T09:15:00+07:00", "work_packages" => work_packages }
    if version >= 2
      data.merge!(
        "warning_count" => 1, "open_proposal_count" => 1,
        "person_actions" => [{ "label" => "Accept or return: Soon", "task_id" => "task.soon", "work_package_id" => "12" }],
        "resources" => [{ "id" => "resource.slot", "name" => "Slot" }],
        "overloaded_weeks" => [{ "resource_id" => "resource.person", "resource_name" => "Person", "unit" => "hours",
                                 "week" => "2026-W46", "load" => 5.45, "capacity" => 5, "overload" => 0.45 }]
      )
    end
    data
  end

  def presenter(version: 2)
    data = overlay(version:)
    OpenProject::ReplanOverlay::OverlayPresenter.new(overlay: data, work_packages: data["work_packages"])
  end

  def counters(version: 2)
    presenter(version:).counters.to_h { |counter| [counter["key"], counter] }
  end

  def test_counters_answer_questions
    got = counters
    assert_equal %w[1 1 2 1 1], got.values_at("waiting", "overloaded", "open", "warnings", "proposals").map { |c| c["value"] }
    assert got["overloaded"]["alert"]
    assert_equal ["Person in 2026-W46: 5.45 of 5 hours"], got["overloaded"]["details"]
    refute got["waiting"]["alert"]
  end

  def test_version_1_overlay_shows_dashes_for_what_it_cannot_answer
    got = counters(version: 1)
    assert_equal "–", got["waiting"]["value"]
    assert_equal "–", got["overloaded"]["value"]
    assert_equal "2", got["open"]["value"]
    refute presenter(version: 1).version2?
  end

  def test_attention_lists_open_tasks_soonest_finish_first_with_signals_in_words
    rows = presenter.attention_rows
    assert_equal %w[task.soon task.late], rows.map { |row| row["replan_task_id"] }
    soon, late = rows
    # A predecessor that is done does not count as waiting.
    assert_equal ["waits on you"], soon["signals"].map { |signal| signal["text"] }
    assert_equal ["waits for 1 task", "1 warning", "1 open proposal"], late["signals"].map { |signal| signal["text"] }
    assert_equal "Something to look at", late["signals"][1]["title"]
    assert_equal "28 Sep 2026 – 29 Sep 2026", soon["window_label"]
  end

  def test_names_come_from_resources_and_lanes_and_fall_back_to_ids
    got = presenter
    assert_equal "Slot", got.resource_name("resource.slot")
    assert_equal "Person", got.resource_name("resource.person")
    assert_equal "resource.unknown", got.resource_name("resource.unknown")
    assert_equal ["Person"], got.attention_rows.last["resource_labels"]
  end

  # Closed tasks are not in the attention table; the Gantt page keeps them
  # under "Done".
  def test_done_tasks_need_no_attention
    refute_includes presenter.attention_rows.map { |row| row["replan_task_id"] }, "task.done"
  end

  def test_header_facts_are_readable
    assert_equal ["Status date 28 Sep 2026", "week 2026-W40", "generated 28 Sep 2026 09:15"], presenter.header_facts
  end
end
